## Why

In production and Charmed Kubernetes/Machine deployments (such as those integrating Charmed Apache Kafka with Charmed Identity Platform and Ory Hydra), Kafka brokers enforce transport encryption (TLS) and SASL authentication via OAuth2 Client Credentials (`OAUTHBEARER`). Currently, `authorization-service` only supports unauthenticated, plaintext communication via `segmentio/kafka-go` in both its message consumer (`cmd/listen.go`, `internal/integration/kafka/client.go`) and its topic provisioning admin client (`cmd/ensure_topics.go`, `internal/integration/kafka/admin.go`).

To securely connect `authorization-service` to Charmed Kafka clusters in these environments, the service needs support for TLS encryption (with custom CA certificate validation) and SASL/OAUTHBEARER authentication using OAuth2 Client Credentials. Other authentication mechanisms (such as SASL/SCRAM or SASL/PLAIN) and mutual TLS (mTLS client certificates) are explicitly excluded from scope.

## What Changes

- Extend `KafkaConfig` in `config/specs.go` and environment bindings in `config/viper.go` to support:
  - TLS configuration: `KAFKA_TLS_ENABLED`, `KAFKA_TLS_CA`, `KAFKA_TLS_CA_FILE`, and `KAFKA_TLS_INSECURE_SKIP_VERIFY`.
  - OAuth2 Client Credentials configuration: `KAFKA_OAUTH_CLIENT_ID`, `KAFKA_OAUTH_CLIENT_SECRET`, `KAFKA_OAUTH_TOKEN_ENDPOINT_URI`, `KAFKA_OAUTH_AUDIENCE`, and `KAFKA_OAUTH_SCOPE`.
- Implement an RFC 7628 compliant `OAUTHBEARER` SASL mechanism for `segmentio/kafka-go` using `golang.org/x/oauth2/clientcredentials` to retrieve, cache, and automatically refresh bearer tokens before authentication or broker reconnections.
- Build a centralized Kafka dialer factory (`internal/integration/kafka/dialer.go`) that configures TLS (`crypto/tls`) and the `OAUTHBEARER` SASL mechanism (`segmentio/kafka-go/sasl`) based on configuration.
- Wire the authenticated dialer into the Kafka reader (`internal/integration/kafka/client.go`) used by `cmd/listen.go`.
- Wire the authenticated dialer into the Kafka admin client (`internal/integration/kafka/admin.go`) used by `cmd/ensure_topics.go`.
- Preserve full backward compatibility: if TLS and OAuth configurations are omitted or disabled, the client defaults to unauthenticated plaintext connections.

## Capabilities

### New Capabilities
- `kafka-client-authentication`: Support for TLS encryption and SASL/OAUTHBEARER (OAuth2 Client Credentials) authentication for Kafka consumer and administrative topic operations.

### Modified Capabilities

## Impact

- `config/specs.go` & `config/viper.go`: New configuration fields, validation tags, and environment variable bindings for Kafka TLS and OAuth2 client credentials.
- `internal/integration/kafka/`:
  - New `dialer.go` providing TLS and SASL/OAUTHBEARER `kafka.Dialer` configuration.
  - New `oauth.go` implementing `sasl.Mechanism` and `sasl.StateMachine` for RFC 7628 `OAUTHBEARER` with `clientcredentials.Config`.
  - Updates to `client.go` to assign the dialer to `kafka.ReaderConfig`.
  - Updates to `admin.go` to use the dialer when discovering partitions and creating topics.
- Dependencies: Direct usage of `golang.org/x/oauth2/clientcredentials` and `golang.org/x/oauth2` (already present in `go.mod` as indirect dependencies).
- External Systems: Direct interoperability with Charmed Apache Kafka secured by TLS and Ory Hydra OAuth2 endpoints.
