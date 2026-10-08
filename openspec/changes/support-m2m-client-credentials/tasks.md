# Tasks

## 1. Protobuf and Client Generation

- [x] 1.1 Update `client/proto/v1/sts/sts.proto` to define `ExchangeToken(ExchangeTokenRequest) returns (ExchangeResponse)` and regenerate Go stubs using `buf generate --template buf.gen.client.yaml`
- [x] 1.2 Regenerate STS client mocks via `mockgen` and verify `internal/service/authz/mocks` builds cleanly

## 2. Observability and Metrics

- [x] 2.1 Extend `internal/service/authz/metrics.go` interface and `internal/metrics/authz.go` with `auth_type` dimension (`cookie`, `client_credentials`, `none`) and STS exchange latency partitioned by exchange type (`session` vs `token`)
- [x] 2.2 Add unit tests in `internal/metrics/authz_test.go` asserting metric registration and observation behavior

## 3. External Authorization Dual Authentication Pipeline

- [x] 3.1 Update `ExternalAuthzService.check` in `internal/service/authz/external.go` to inspect headers and enforce strict mutual exclusivity (Option B: return HTTP 400 on conflicting credentials)
- [x] 3.2 Forward raw Bearer token directly to `sts.ExchangeToken` without edge signature or issuer verification
- [x] 3.3 Verify returned internal STS JWT using existing STS OIDC verifier
- [x] 3.4 Verify subject mapping to `user:<client_id>` and OpenFGA zero-bypass evaluation for machine callers
- [x] 3.5 Ensure Envoy `OkHttpResponse` injects `Authorization: Bearer <sts_jwt>` for both authentication flows
- [x] 3.6 Add comprehensive unit tests in `internal/service/authz/external_test.go` covering all branches: machine token success, STS exchange failure, conflicting credentials (HTTP 400), and OpenFGA denial

## 4. Documentation

- [x] 4.1 Document dual authentication flows, STS token exchange delegation, and strict mutual exclusivity behavior in `README.md`
