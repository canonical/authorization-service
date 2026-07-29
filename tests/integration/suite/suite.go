//go:build integration

package suite

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"strconv"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jackc/pgx/v5/pgxpool"
	dockercontainer "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"google.golang.org/protobuf/proto"
	"go.opentelemetry.io/otel/trace/noop"
	kafka "github.com/segmentio/kafka-go"

	messagesv1 "github.com/canonical/authorization-service/api/v1"
	"github.com/canonical/authorization-service/internal/integration/postgres"
	"github.com/canonical/authorization-service/internal/service/listen"
	"github.com/canonical/authorization-service/migrations"
)

var FederatedServices = []string{"payments", "invoicing"}

func TopicFor(slug string) string { return slug + listen.TopicSuffix }

var TestLogger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

func StartKafka(ctx context.Context) (testcontainers.Container, string, error) {
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, "", fmt.Errorf("finding free port: %w", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	portStr := strconv.Itoa(port)

	req := testcontainers.ContainerRequest{
		Image:        "apache/kafka:3.8.0",
		ExposedPorts: []string{"9092/tcp"},
		Env: map[string]string{
			"KAFKA_NODE_ID":                          "1",
			"KAFKA_PROCESS_ROLES":                    "broker,controller",
			"KAFKA_CONTROLLER_QUORUM_VOTERS":         "1@localhost:9093",
			"KAFKA_LISTENERS":                        "PLAINTEXT://0.0.0.0:9092,CONTROLLER://0.0.0.0:9093",
			"KAFKA_ADVERTISED_LISTENERS":             "PLAINTEXT://localhost:" + portStr,
			"KAFKA_INTER_BROKER_LISTENER_NAME":       "PLAINTEXT",
			"KAFKA_CONTROLLER_LISTENER_NAMES":        "CONTROLLER",
			"KAFKA_LISTENER_SECURITY_PROTOCOL_MAP":   "PLAINTEXT:PLAINTEXT,CONTROLLER:PLAINTEXT",
			"KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR": "1",
			"KAFKA_AUTO_CREATE_TOPICS_ENABLE":        "true",
		},
		HostConfigModifier: func(hc *dockercontainer.HostConfig) {
			p, _ := network.ParsePort("9092/tcp")
			hc.PortBindings = network.PortMap{
				p: []network.PortBinding{{HostIP: netip.MustParseAddr("0.0.0.0"), HostPort: portStr}},
			}
		},
		WaitingFor: wait.ForLog("Kafka Server started").WithStartupTimeout(60 * time.Second),
	}

	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, "", fmt.Errorf("starting Kafka: %w", err)
	}

	return ctr, "localhost:" + portStr, nil
}

func StartPostgres(ctx context.Context) (testcontainers.Container, string, postgres.Config, error) {
	const (
		user = "cerberus"
		pass = "cerberus"
		db   = "cerberus"
	)

	req := testcontainers.ContainerRequest{
		Image:        "postgres:14-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     user,
			"POSTGRES_PASSWORD": pass,
			"POSTGRES_DB":       db,
		},
		WaitingFor: wait.ForListeningPort("5432/tcp").WithStartupTimeout(60 * time.Second),
	}

	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	var emptyConfig postgres.Config
	if err != nil {
		return nil, "", emptyConfig, fmt.Errorf("starting Postgres: %w", err)
	}

	host, err := ctr.Host(ctx)
	if err != nil {
		return nil, "", emptyConfig, fmt.Errorf("getting Postgres host: %w", err)
	}
	mappedPort, err := ctr.MappedPort(ctx, "5432/tcp")
	if err != nil {
		return nil, "", emptyConfig, fmt.Errorf("getting Postgres port: %w", err)
	}

	portNum, err := strconv.Atoi(mappedPort.Port())
	if err != nil {
		return nil, "", emptyConfig, fmt.Errorf("parsing Postgres port: %w", err)
	}

	pgConfig := postgres.Config{
		Host:           host,
		Port:           portNum,
		User:           user,
		Password:       pass,
		DBName:         db,
		SSLMode:        "disable",
		ConnectTimeout: 10 * time.Second,
	}

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, mappedPort.Port(), user, pass, db)
	return ctr, dsn, pgConfig, nil
}

func RunMigrations(ctx context.Context, dsn string) error {
	db, err := sql.Open("pgx/v5", dsn)
	if err != nil {
		return fmt.Errorf("opening db: %w", err)
	}
	defer db.Close()

	deadline := time.Now().Add(30 * time.Second)
	for {
		if err = db.PingContext(ctx); err == nil {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("db not ready: %w", err)
		}
		time.Sleep(500 * time.Millisecond)
	}

	goose.SetBaseFS(migrations.EmbedMigrations)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("setting dialect: %w", err)
	}
	if err := goose.UpContext(ctx, db, ".", goose.WithNoColor(true)); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}
	return nil
}

func CreateKafkaTopics(ctx context.Context, broker string) error {
	conn, err := kafka.DialContext(ctx, "tcp", broker)
	if err != nil {
		return fmt.Errorf("dialing kafka: %w", err)
	}
	defer conn.Close()

	controller, err := conn.Controller()
	if err != nil {
		return fmt.Errorf("getting controller: %w", err)
	}
	controllerConn, err := kafka.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", controller.Host, controller.Port))
	if err != nil {
		return fmt.Errorf("dialing controller: %w", err)
	}
	defer controllerConn.Close()

	specs := make([]kafka.TopicConfig, 0, len(FederatedServices))
	for _, slug := range FederatedServices {
		specs = append(specs, kafka.TopicConfig{
			Topic:             TopicFor(slug),
			NumPartitions:     1,
			ReplicationFactor: 1,
		})
	}
	if err := controllerConn.CreateTopics(specs...); err != nil {
		return fmt.Errorf("creating topics: %w", err)
	}
	return nil
}

func NewTestPostgres(t *testing.T, dsn string, cfg postgres.Config) (*postgres.Client, *pgxpool.Pool) {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pgxpool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	client, err := postgres.NewClient(cfg, TestLogger, noop.NewTracerProvider().Tracer("test"))
	if err != nil {
		t.Fatalf("postgres client: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	return client, pool
}

func PublishEnvelope(t *testing.T, broker, slug string, env *messagesv1.PermissionUpdateEnvelope) {
	t.Helper()
	data, err := proto.Marshal(env)
	if err != nil {
		t.Fatalf("publishEnvelope marshal: %v", err)
	}
	PublishRaw(t, broker, slug, []byte(env.GetIdempotencyKey()), data)
}

func PublishRaw(t *testing.T, broker, slug string, key, value []byte) {
	t.Helper()
	w := &kafka.Writer{
		Addr:                   kafka.TCP(broker),
		Topic:                  TopicFor(slug),
		Balancer:               &kafka.LeastBytes{},
		AllowAutoTopicCreation: true,
	}
	defer w.Close()
	if err := w.WriteMessages(context.Background(), kafka.Message{Key: key, Value: value}); err != nil {
		t.Fatalf("publishRaw write: %v", err)
	}
}

func UniqueSuffix() string {
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}

func SampleEnvelope(slug, idempotencyKey string) *messagesv1.PermissionUpdateEnvelope {
	return &messagesv1.PermissionUpdateEnvelope{
		Version:        "1",
		Service:        slug,
		MessageId:      "msg-" + idempotencyKey,
		IdempotencyKey: idempotencyKey,
		Operations: []*messagesv1.PermissionOperation{
			{
				Op:       messagesv1.PermissionOp_PERMISSION_OP_WRITE,
				Subject:  "user:u1",
				Relation: "member",
				Object:   "group:g1",
			},
		},
	}
}
