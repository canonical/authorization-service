## Context

The `authorization-service` consumes events from Apache Kafka using `github.com/segmentio/kafka-go` (`cmd/listen.go` and `internal/integration/kafka/client.go`), and boots required topics using Kafka admin APIs (`cmd/ensure_topics.go` and `internal/integration/kafka/admin.go`).

In Charmed environments (such as Charmed Apache Kafka integrated with Charmed Identity Platform and Ory Hydra), brokers require clients to connect over TLS and authenticate via SASL `OAUTHBEARER` using OAuth2 Client Credentials.

Currently, `internal/integration/kafka/client.go` initializes `kafka.ReaderConfig` without a custom `Dialer`, and `internal/integration/kafka/admin.go` dials brokers using unauthenticated `kafka.DialContext`. Neither supports TLS encryption or OAuth2 authentication. Furthermore, `segmentio/kafka-go` does not provide an out-of-the-box `OAUTHBEARER` mechanism.

This design details how `authorization-service` configures TLS transport encryption, implements an RFC 7628 compliant `OAUTHBEARER` SASL mechanism using `golang.org/x/oauth2/clientcredentials`, and integrates them into both reader and admin connections.

## Goals / Non-Goals

**Goals:**
- Implement an RFC 7628 compliant `OAUTHBEARER` SASL mechanism compatible with `segmentio/kafka-go/sasl.Mechanism` and `sasl.StateMachine`.
- Automate token acquisition, caching, and refresh using `golang.org/x/oauth2/clientcredentials`.
- Support custom TLS CA certificates (`KAFKA_TLS_CA` or `KAFKA_TLS_CA_FILE`) and insecure skip verify for Kafka broker connections and OAuth token endpoint requests.
- Create a unified `kafka.Dialer` constructor used by both the Kafka consumer reader and topic provisioning admin client.
- Preserve backward compatibility with unauthenticated plaintext Kafka brokers when security settings are omitted.

**Non-Goals:**
- Supporting SASL/SCRAM, SASL/PLAIN, Kerberos/GSSAPI, or any authentication mechanism other than `OAUTHBEARER`.
- Supporting mutual TLS (mTLS) with client certificates/keys (only broker TLS validation is supported).
- Dynamic SASL re-authentication within an open, active TCP socket (broker re-auth is handled via connection renewal when the reader reconnects).
- Supporting OAuth authorization code or device flows (only Client Credentials flow is applicable).
- Replacing `segmentio/kafka-go` with alternative Kafka client libraries.

## Decisions

### Decision 1: Custom RFC 7628 SASL `OAUTHBEARER` Mechanism
`segmentio/kafka-go/sasl` defines two interfaces:
```go
type Mechanism interface {
    Name() string
    Start(ctx context.Context) (sess StateMachine, ir []byte, err error)
}

type StateMachine interface {
    Next(ctx context.Context, challenge []byte) (done bool, response []byte, err error)
}
```
We implement `OAuthBearerMechanism` in `internal/integration/kafka/oauth.go`:
1. `Name()` returns `"OAUTHBEARER"`.
2. `Start(ctx)`:
   - Fetches an access token from the configured `oauth2.TokenSource`.
   - Constructs the initial client response according to RFC 7628 / Kafka SASL OAUTHBEARER:
     `n,,\x01auth=Bearer <access_token>\x01\x01`
   - Returns the initial response and an `oauthBearerSession` state machine.
3. `Next(ctx, challenge)`:
   - If `len(challenge) == 0`: Authentication succeeded. Returns `done = true, nil, nil`.
   - If `len(challenge) > 0`: The broker responded with an error challenge (e.g. JSON error `{"status":"invalid_token"}`). The state machine returns an empty error acknowledgment `[]byte{0x01}` and returns an error containing the broker's response text.

### Decision 2: Token Acquisition and Refresh via `clientcredentials.Config`
We configure token retrieval using `golang.org/x/oauth2/clientcredentials.Config`:
- `ClientID`: `cfg.Kafka.OAuth.ClientID`
- `ClientSecret`: `cfg.Kafka.OAuth.ClientSecret`
- `TokenURL`: `cfg.Kafka.OAuth.TokenEndpointURI`
- `Scopes`: split from `cfg.Kafka.OAuth.Scope` (default: `"profile"`)
- `EndpointParams`: `url.Values{"audience": []string{cfg.Kafka.OAuth.Audience}}` (default: `"kafka"`)

Calling `config.TokenSource(ctx)` yields a thread-safe token source wrapped with automatic token caching and proactive renewal (`ReuseTokenSource`).
When a custom TLS CA is configured (`KAFKA_TLS_CA` or `KAFKA_TLS_CA_FILE`), a dedicated `*http.Client` sharing the custom root CA pool is injected into the context via `oauth2.HTTPClient` so token requests to Hydra succeed even with private CA certificates.

### Decision 3: Centralized Kafka Dialer Factory (`dialer.go`)
We create `NewDialer(cfg config.KafkaConfig) (*kafka.Dialer, error)` in `internal/integration/kafka/dialer.go`:
1. **TLS Setup**:
   - If `cfg.TLS.Enabled`, or if CA files/strings are provided:
     - Load root CAs from `cfg.TLS.CA` (inline PEM) or `cfg.TLS.CAFile`.
     - Set `InsecureSkipVerify: cfg.TLS.InsecureSkipVerify`.
     - If neither is enabled, `tlsConfig` is `nil`.
2. **SASL Setup**:
   - If OAuth credentials are set (`cfg.OAuth.ClientID != ""` or `cfg.OAuth.TokenEndpointURI != ""`):
     - Initialize `OAuthBearerMechanism`.
   - Otherwise, `Dialer.SASLMechanism` remains `nil`.
3. Returns `&kafka.Dialer{Timeout: 10 * time.Second, DualStack: true, TLS: tlsConfig, SASLMechanism: saslMech}`.

### Decision 4: Wire Dialer into Consumer Reader and Admin Client
- **Kafka Reader** (`internal/integration/kafka/client.go`):
  Assign the created `*kafka.Dialer` to `kafka.ReaderConfig.Dialer`.
- **Kafka Admin Client** (`internal/integration/kafka/admin.go`):
  Update `EnsureTopics` to accept `*kafka.Dialer`. Replace `kafka.DialContext(ctx, "tcp", broker)` with `dialer.DialContext(ctx, "tcp", broker)`.

### Decision 5: Configuration Bindings and Inferred Defaults
In `config/specs.go` and `config/viper.go`:
- Extend `KafkaConfig`:
  - `TLS`: `Enabled bool`, `CA string`, `CAFile string`, `InsecureSkipVerify bool`.
  - `OAuth`: `ClientID string`, `ClientSecret string`, `TokenEndpointURI string`, `Audience string`, `Scope string`.
- Bind environment variables:
  - `KAFKA_TLS_ENABLED`, `KAFKA_TLS_CA`, `KAFKA_TLS_CA_FILE`, `KAFKA_TLS_INSECURE_SKIP_VERIFY`
  - `KAFKA_OAUTH_CLIENT_ID`, `KAFKA_OAUTH_CLIENT_SECRET`, `KAFKA_OAUTH_TOKEN_ENDPOINT_URI`, `KAFKA_OAUTH_AUDIENCE`, `KAFKA_OAUTH_SCOPE`
- Defaults:
  - `KAFKA_OAUTH_AUDIENCE`: `"kafka"`.
  - `KAFKA_OAUTH_SCOPE`: `"profile"`.
- Validation:
  - If any OAuth field is set, `ClientID`, `ClientSecret`, and `TokenEndpointURI` are required.
  - If no TLS or OAuth fields are configured, unauthenticated plaintext communication is preserved.

## Risks / Trade-offs

- **[Risk]** Long-running consumer connections expire if broker enforces re-authentication timeout (`connections.max.reauth.ms`).
  → **Mitigation**: `kafka.Reader` detects closed/failed broker connections and automatically reconnects using `Dialer.DialContext`. Because `oauth2.TokenSource` automatically caches and renews tokens, each new dial acquires an active, unexpired token.
- **[Risk]** Invalid OAuth credentials or unreachable token endpoint causes consumer startup loop or obscure connection errors.
  → **Mitigation**: `OAuthBearerMechanism.Start` validates token acquisition before returning the SASL initial response, producing clear error messages stating token retrieval failed.
- **[Risk]** Existing unauthenticated dev setups and integration tests breaking.
  → **Mitigation**: When `TLS.Enabled` is false and OAuth configurations are empty, `Dialer` leaves `TLS` and `SASLMechanism` `nil`, preserving plaintext behavior.
