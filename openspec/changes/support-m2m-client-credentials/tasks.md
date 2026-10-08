# Tasks

## 1. Protobuf and Client Generation

- [x] 1.1 Update `client/proto/v1/sts/sts.proto` to define `ExchangeToken(ExchangeTokenRequest) returns (ExchangeResponse)` and regenerate Go stubs using `buf generate --template buf.gen.client.yaml`
- [x] 1.2 Regenerate STS client mocks via `mockgen` and verify `internal/service/authz/mocks` builds cleanly

## 2. Configuration and Preemptive JWKS Warm-Up

- [x] 2.1 Extend `ExtAuthzServiceConfig` in `config/specs.go` with required `HydraJwkSetURL` and co-dependent `HydraIssuer`
- [x] 2.2 Add configuration validation unit tests in `config/specs_test.go` verifying required fields and co-dependency rules
- [x] 2.3 Implement preemptive JWKS pre-fetching and verifier initialization in `config/helper.go` so the key cache is warmed during startup

## 3. Observability and Metrics

- [x] 3.1 Extend `internal/service/authz/metrics.go` interface and `internal/metrics/authz.go` with `auth_type` dimension (`cookie`, `client_credentials`, `none`), Hydra verification latency histogram, and STS exchange latency partitioned by exchange type
- [x] 3.2 Add unit tests in `internal/metrics/authz_test.go` asserting metric registration and observation behavior

## 4. External Authorization Dual Authentication Pipeline

- [ ] 4.1 Update `ExternalAuthzService.check` in `internal/service/authz/external.go` to inspect headers and enforce strict mutual exclusivity (Option B: return HTTP 400 on conflicting credentials)
- [ ] 4.2 Implement cryptographic signature and issuer verification for Hydra Bearer tokens using the initialized Hydra verifier
- [ ] 4.3 Call `sts.ExchangeToken` for verified machine Bearer tokens and verify the returned internal STS JWT
- [ ] 4.4 Verify subject mapping to `user:<client_id>` and OpenFGA zero-bypass evaluation for machine callers
- [ ] 4.5 Ensure Envoy `OkHttpResponse` injects `Authorization: Bearer <sts_jwt>` for both authentication flows
- [ ] 4.6 Add comprehensive unit tests in `internal/service/authz/external_test.go` covering all branches: machine token success, invalid signature, issuer mismatch, conflicting credentials (HTTP 400), and OpenFGA denial

## 5. Documentation

- [ ] 5.1 Document dual authentication flows, Hydra configuration, and strict mutual exclusivity behavior in `README.md`
