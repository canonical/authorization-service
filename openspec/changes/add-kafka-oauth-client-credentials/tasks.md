## 1. Configuration & Environment Bindings

- [ ] 1.1 Extend `KafkaConfig` in `config/specs.go` with `TLS` and `OAuth` configuration structures and validation rules.
- [ ] 1.2 Bind environment variables (`KAFKA_TLS_*`, `KAFKA_OAUTH_*`) and set defaults (`audience: kafka`, `scope: profile`) in `config/viper.go`.
- [ ] 1.3 Add unit tests in `config/specs_test.go` and `config/viper_test.go` covering OAuth, TLS, and fallback unauthenticated configurations.

## 2. SASL OAUTHBEARER Mechanism Implementation

- [ ] 2.1 Implement `OAuthBearerMechanism` and `oauthBearerSession` satisfying `segmentio/kafka-go/sasl.Mechanism` and `sasl.StateMachine` in `internal/integration/kafka/oauth.go`.
- [ ] 2.2 Configure token acquisition via `golang.org/x/oauth2/clientcredentials` with token caching/refresh and custom TLS root CA support for OAuth endpoints.
- [ ] 2.3 Add unit tests in `internal/integration/kafka/oauth_test.go` verifying RFC 7628 initial response formatting, broker challenge completion, error challenges, and token fetch errors.

## 3. Centralized Kafka Dialer & TLS Construction

- [ ] 3.1 Implement `NewDialer` in `internal/integration/kafka/dialer.go` to construct `*kafka.Dialer` with TLS (custom CA certificates, InsecureSkipVerify) and SASL `OAUTHBEARER` mechanism.
- [ ] 3.2 Add unit tests in `internal/integration/kafka/dialer_test.go` validating dialer construction for plaintext, TLS, and OAuth configurations.

## 4. Consumer Reader & Admin Integration Wiring

- [ ] 4.1 Update `internal/integration/kafka/client.go` to inject the configured `*kafka.Dialer` into `kafka.ReaderConfig`.
- [ ] 4.2 Update `internal/integration/kafka/admin.go` (`EnsureTopics`) to use the configured dialer for broker communication and topic provisioning.
- [ ] 4.3 Update `cmd/listen.go` and `cmd/ensure_topics.go` call sites if necessary to pass dialer or updated configuration.

## 5. Verification & Testing

- [ ] 5.1 Run full unit test suite (`go test ./...`) to ensure all unit tests pass without regressions.
- [ ] 5.2 Verify backward compatibility with existing Kafka integration test suite in `tests/integration/`.
