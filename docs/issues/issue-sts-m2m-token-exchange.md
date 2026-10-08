# feat(grpc): add ExchangeToken RPC for IdP (Hydra) client credentials token exchange

**GitHub Issue**: [#44](https://github.com/canonical/secure-token-service/issues/44)  
**Label**: `enhancement`  
**Companion Issue**: [`canonical/authorization-service#100`](https://github.com/canonical/authorization-service/issues/100)  
**Architecture Diagram**: [.archify/architecture-secure-token-service-20261008-154500/secure-token-service.html](file:///home/shipperizer/shipperizer/secure-token-service/.archify/architecture-secure-token-service-20261008-154500/secure-token-service.html)

---

## Context & Motivation
Currently, STS provides the `ExchangeSession` gRPC RPC to exchange opaque user session cookies (originating from human OIDC or Ubuntu One OpenID login flows) for internal ES256 JWTs adhering to the ecosystem contract (`sub`, `email`, `org`).

In order to support Machine-to-Machine (M2M) communication (e.g., automated services, background daemons, CLI tools), machine clients will authenticate directly with our configured identity provider (Ory Hydra) using the OAuth2 Client Credentials flow (`grant_type=client_credentials`). 

Internal microservices across the Canonical ecosystem only accept and verify internal STS JWTs signed by STS's active ES256 key. To maintain token architecture consistency without requiring microservices to directly trust or verify third-party Hydra tokens, STS must expose a token exchange endpoint that swaps upstream IdP access tokens for standard internal STS JWTs.

Companion issue in Authorization Service: [canonical/authorization-service#100](https://github.com/canonical/authorization-service/issues/100).

## Proposed Changes

### 1. Protobuf API Definition (`api/proto/v1/sts.proto`)
Add `ExchangeToken` RPC and `ExchangeTokenRequest` message:
```protobuf
service SecurityTokenService {
  // ExchangeSession swaps an opaque session_id for a customized internal JWT.
  rpc ExchangeSession(ExchangeRequest) returns (ExchangeResponse);

  // ExchangeToken swaps an upstream IdP access token (e.g., Hydra client credentials)
  // for an internal STS JWT adhering to the ecosystem contract.
  rpc ExchangeToken(ExchangeTokenRequest) returns (ExchangeResponse);

  // RevokeUserSessions forces a logout for a specific user, invalidating all sessions.
  rpc RevokeUserSessions(RevokeUserRequest) returns (RevokeUserResponse);
}

message ExchangeTokenRequest {
  string token = 1; // Raw upstream IdP access token (JWT)
}
```

### 2. gRPC Server Implementation (`internal/grpc/server.go`)
Implement the `ExchangeToken` RPC:
- Validate `req.Token`: return `codes.InvalidArgument` if empty.
- Parse/verify claims from the incoming token:
  - Extract machine subject (`sub` / `client_id`).
  - Extract relevant custom claims (e.g., `scope`, tenant/org).
- Mint internal JWT via `KeyManager.MintToken`:
  - Set `sub` to the machine client ID.
  - Sign with active PostgreSQL ES256 key.
  - Return `stsv1.ExchangeResponse{ AccessToken: token, ExpiresIn: expiry }`.

### 3. Observability & Metrics
- Add Prometheus metric counter: `grpc_server_handled_total{grpc_method="ExchangeToken",grpc_code="..."}`.
- Emit structured Zap logs with correlation IDs.
- Trace RPC executions via OpenTelemetry.

## Acceptance Criteria
- [ ] `api/proto/v1/sts.proto` updated and protobuf bindings regenerated.
- [ ] `ExchangeToken` implemented in `internal/grpc/server.go`.
- [ ] Valid tokens return an internal STS JWT signed with ES256 with `sub` matching the machine client identity.
- [ ] Invalid/empty tokens return appropriate gRPC error codes (`InvalidArgument`, `Unauthenticated`).
- [ ] Unit and benchmark tests added to `internal/grpc/server_test.go`.
- [ ] Documentation updated in `docs/api/grpc.md` and `README.md`.
