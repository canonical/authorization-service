package integration

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"strconv"
	"testing"
	"time"

	dockercontainer "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	openfgasdk "github.com/openfga/go-sdk"
	"github.com/openfga/go-sdk/client"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"google.golang.org/protobuf/proto"

	kafka "github.com/segmentio/kafka-go"

	messagesv1 "github.com/canonical/authorization-service/api/v1"
	kafkaintegration "github.com/canonical/authorization-service/internal/integration/kafka"
	"github.com/canonical/authorization-service/internal/testutil"
)

const (
	ingestTopic = "test.authz.tuples"
	errorTopic  = "test.authz.tuples.errors"
)

var (
	kafkaBroker     string
	openfgaHTTPAddr string
	openfgaStoreID  string
	openfgaModelID  string
	testLogger      = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	ctx := context.Background()

	kafkaCtr, broker, err := startKafka(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start Kafka: %v\n", err)
		return 1
	}
	defer testutil.StopContainer(ctx, kafkaCtr)
	kafkaBroker = broker

	fgaCtr, addr, err := startOpenFGA(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start OpenFGA: %v\n", err)
		return 1
	}
	defer testutil.StopContainer(ctx, fgaCtr)
	openfgaHTTPAddr = addr

	storeID, modelID, err := setupOpenFGA(ctx, addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to setup OpenFGA: %v\n", err)
		return 1
	}
	openfgaStoreID = storeID
	openfgaModelID = modelID

	if err := createKafkaTopics(ctx, kafkaBroker); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create Kafka topics: %v\n", err)
		return 1
	}

	return m.Run()
}

// startKafka starts a KRaft-mode Kafka container and returns the broker address.
// It picks a free host port and pins it so KAFKA_ADVERTISED_LISTENERS matches
// the port that testcontainers exposes.
func startKafka(ctx context.Context) (testcontainers.Container, string, error) {
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

// startOpenFGA starts an OpenFGA container with an in-memory datastore.
func startOpenFGA(ctx context.Context) (testcontainers.Container, string, error) {
	req := testcontainers.ContainerRequest{
		Image:        "openfga/openfga:v1.14.1",
		Cmd:          []string{"run"},
		ExposedPorts: []string{"8080/tcp"},
		Env: map[string]string{
			"OPENFGA_DATASTORE_ENGINE": "memory",
			"OPENFGA_HTTP_ADDR":        "0.0.0.0:8080",
		},
		WaitingFor: wait.ForHTTP("/healthz").WithPort("8080/tcp").WithStartupTimeout(30 * time.Second),
	}

	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, "", fmt.Errorf("starting OpenFGA: %w", err)
	}

	host, err := ctr.Host(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("getting OpenFGA host: %w", err)
	}
	mappedPort, err := ctr.MappedPort(ctx, "8080/tcp")
	if err != nil {
		return nil, "", fmt.Errorf("getting OpenFGA port: %w", err)
	}

	addr := fmt.Sprintf("http://%s:%s", host, mappedPort.Port())
	return ctr, addr, nil
}

// setupOpenFGA creates an OpenFGA store and writes the minimal authorization model
// needed by the listener tests (user type + group type with member relation).
func setupOpenFGA(ctx context.Context, apiURL string) (storeID, modelID string, err error) {
	fgaClient, err := client.NewSdkClient(&client.ClientConfiguration{ApiUrl: apiURL})
	if err != nil {
		return "", "", fmt.Errorf("creating OpenFGA client: %w", err)
	}

	storeResp, err := fgaClient.CreateStoreExecute(
		fgaClient.CreateStore(ctx).Body(client.ClientCreateStoreRequest{Name: "integration-tests"}),
	)
	if err != nil {
		return "", "", fmt.Errorf("creating OpenFGA store: %w", err)
	}
	storeID = storeResp.GetId()
	if err := fgaClient.SetStoreId(storeID); err != nil {
		return "", "", fmt.Errorf("setting store ID: %w", err)
	}

	memberRelation := map[string]openfgasdk.Userset{
		"member": {This: &map[string]interface{}{}},
	}
	memberMeta := map[string]openfgasdk.RelationMetadata{
		"member": {
			DirectlyRelatedUserTypes: &[]openfgasdk.RelationReference{
				{Type: "user"},
			},
		},
	}
	authModel := openfgasdk.WriteAuthorizationModelRequest{
		SchemaVersion: "1.1",
		TypeDefinitions: []openfgasdk.TypeDefinition{
			{Type: "user"},
			{
				Type:      "group",
				Relations: &memberRelation,
				Metadata:  &openfgasdk.Metadata{Relations: &memberMeta},
			},
		},
	}

	modelResp, err := fgaClient.WriteAuthorizationModelExecute(
		fgaClient.WriteAuthorizationModel(ctx).Body(authModel),
	)
	if err != nil {
		return "", "", fmt.Errorf("writing authorization model: %w", err)
	}

	return storeID, modelResp.GetAuthorizationModelId(), nil
}

// createKafkaTopics pre-creates both the ingestion and error topics via the
// kafka-go admin API so tests never hit the auto-create race.
func createKafkaTopics(ctx context.Context, broker string) error {
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

	specs := make([]kafka.TopicConfig, 0, 2)
	for _, topic := range []string{ingestTopic, errorTopic} {
		specs = append(specs, kafka.TopicConfig{
			Topic:             topic,
			NumPartitions:     1,
			ReplicationFactor: 1,
		})
	}
	if err := controllerConn.CreateTopics(specs...); err != nil {
		return fmt.Errorf("creating topics: %w", err)
	}
	return nil
}

// ── Per-test helpers ─────────────────────────────────────────────────────────

func newOpenFGAClient(t *testing.T) *client.OpenFgaClient {
	t.Helper()
	fgaClient, err := client.NewSdkClient(&client.ClientConfiguration{
		ApiUrl:               openfgaHTTPAddr,
		AuthorizationModelId: openfgaModelID,
	})
	if err != nil {
		t.Fatalf("newOpenFGAClient: %v", err)
	}
	if err := fgaClient.SetStoreId(openfgaStoreID); err != nil {
		t.Fatalf("newOpenFGAClient SetStoreId: %v", err)
	}
	return fgaClient
}

// newKafkaClient creates a Kafka client pointing at the test broker.
// The consumer group is unique per call so tests don't share committed offsets.
func newKafkaClient(t *testing.T, group string) *kafkaintegration.Client {
	t.Helper()
	c, err := kafkaintegration.NewClient(kafkaintegration.Config{
		Brokers:       []string{kafkaBroker},
		ConsumerGroup: group,
		Topic:         ingestTopic,
		Workers:       2,
	}, testLogger)
	if err != nil {
		t.Fatalf("newKafkaClient: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

// newErrorReader returns a kafka.Reader positioned at the start of the error topic.
// Uses a unique group per call so it always reads from offset 0.
func newErrorReader(t *testing.T, group string) *kafka.Reader {
	t.Helper()
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     []string{kafkaBroker},
		GroupID:     group,
		Topic:       errorTopic,
		MinBytes:    1,
		MaxBytes:    10e6,
		StartOffset: kafka.FirstOffset,
	})
	t.Cleanup(func() { r.Close() })
	return r
}

// publishWriteRequest serializes and publishes one WriteRequest to the ingest topic.
func publishWriteRequest(t *testing.T, w *kafka.Writer, req *messagesv1.WriteRequest, service string) {
	t.Helper()
	data, err := proto.Marshal(req)
	if err != nil {
		t.Fatalf("publishWriteRequest marshal: %v", err)
	}
	if err := w.WriteMessages(context.Background(), kafka.Message{
		Key:   []byte(fmt.Sprintf("key-%d", req.GetSequenceId())),
		Value: data,
		Headers: []kafka.Header{
			{Key: "service", Value: []byte(service)},
		},
	}); err != nil {
		t.Fatalf("publishWriteRequest write: %v", err)
	}
}

// newTestWriter returns a kafka.Writer for the ingest topic.
func newTestWriter(t *testing.T) *kafka.Writer {
	t.Helper()
	w := &kafka.Writer{
		Addr:                   kafka.TCP(kafkaBroker),
		Topic:                  ingestTopic,
		Balancer:               &kafka.LeastBytes{},
		AllowAutoTopicCreation: true,
	}
	t.Cleanup(func() { w.Close() })
	return w
}

// uniqueSuffix returns a short string unique within the test run.
func uniqueSuffix() string {
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}

// waitForTuple polls OpenFGA until the tuple appears or the deadline passes.
func waitForTuple(t *testing.T, fga *client.OpenFgaClient, user, relation, object string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := fga.ReadExecute(fga.Read(context.Background()).Body(client.ClientReadRequest{
			User:     openfgasdk.PtrString(user),
			Relation: openfgasdk.PtrString(relation),
			Object:   openfgasdk.PtrString(object),
		}))
		if err == nil && len(resp.GetTuples()) > 0 {
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("tuple %s#%s@%s not found in OpenFGA within 20s", user, relation, object)
}

// waitForAbsenceTuple asserts the tuple is NOT present after the listener has had
// enough time to process (waits waitFor, then checks once).
func waitForAbsenceTuple(t *testing.T, fga *client.OpenFgaClient, user, relation, object string, waitFor time.Duration) {
	t.Helper()
	time.Sleep(waitFor)
	resp, err := fga.ReadExecute(fga.Read(context.Background()).Body(client.ClientReadRequest{
		User:     openfgasdk.PtrString(user),
		Relation: openfgasdk.PtrString(relation),
		Object:   openfgasdk.PtrString(object),
	}))
	if err != nil {
		return // read error = tuple not there, which is what we want
	}
	if len(resp.GetTuples()) > 0 {
		t.Fatalf("tuple %s#%s@%s should NOT be in OpenFGA but was found", user, relation, object)
	}
}

// waitForErrorMessage polls the error reader until a matching message arrives or timeout.
// match is called for each decodeable message; the first one that returns true is returned.
// Pass nil to accept the first decodeable message.
func waitForErrorMessage(t *testing.T, r *kafka.Reader, timeout time.Duration, match func(*messagesv1.WriteRequestError) bool) *messagesv1.WriteRequestError {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for {
		msg, err := r.FetchMessage(ctx)
		if err != nil {
			t.Fatalf("waitForErrorMessage: no matching error message within %s: %v", timeout, err)
			return nil
		}
		if err := r.CommitMessages(ctx, msg); err != nil {
			t.Logf("commit warning: %v", err)
		}
		var errMsg messagesv1.WriteRequestError
		if err := proto.Unmarshal(msg.Value, &errMsg); err != nil {
			continue
		}
		if match == nil || match(&errMsg) {
			return &errMsg
		}
	}
}
