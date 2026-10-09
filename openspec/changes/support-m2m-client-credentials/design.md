# Design

## Context

The `authorization-service` acts as an Envoy/Istio external authorization provider (`ext_authz`) under an Istio `CUSTOM` AuthorizationPolicy. Currently, it assumes all incoming requests carry a user session cookie (`session_id=...`), which it exchanges for an internal STS JWT via `sts.ExchangeSession`.

To support Machine-to-Machine (M2M) callers utilizing OAuth2 Client Credentials from Ory Hydra, the service must accept `Authorization: Bearer <hydra_token>`, cryptographically verify it against Hydra, exchange it for an internal STS JWT via `sts.ExchangeToken`, and enforce uniform OpenFGA checks.

![Authorization Service Architecture (Dark Mode)](./architecture-authorization-service.dark.png)

See [proposal.md](file:///home/shipperizer/shipperizer/authorization-service/openspec/changes/support-m2m-client-credentials/proposal.md) for background and motivation.

## Goals / Non-Goals

**Goals:**
- Provide dual authentication dispatch supporting both user session cookies and Hydra M2M Bearer tokens over the same Istio custom policy.
- Enforce strict mutual exclusivity between credentials to prevent credential confusion or session smuggling.
- Preemptively fetch and warm the Hydra JWKS key cache during service startup to eliminate cold-start request latency.
- Enforce strict zero-bypass OpenFGA authorization for machine identities formatted as `user:<client_id>`.
- Inject internal STS JWTs into Envoy `OkHttpResponse` headers (`Authorization: Bearer <sts_jwt>`) for upstream services.
- Provide comprehensive observability with `auth_type` dimensions and dedicated stage latency histograms.

**Non-Goals:**
- Implementing the server-side `ExchangeToken` RPC in STS (tracked in companion issue [`canonical/secure-token-service#44`](https://github.com/canonical/secure-token-service/issues/44)).
- Providing bypasses or special exemptions for machine clients in OpenFGA.
- Supporting arbitrary third-party OIDC providers outside of configured Hydra and STS instances.

## Decisions

### 1. Strict Mutual Exclusivity for Credentials (Option B)
When an incoming request presents both an `Authorization: Bearer <token>` header and a `Cookie: session_id=...`:
- **Decision**: Reject the request immediately with HTTP 400 Bad Request, denying authorization with reason `conflicting_credentials`.
- **Rationale**: Prevents credential ambiguity and potential session smuggling/confusion attacks where a caller inadvertently or maliciously mixes a human session with a machine service identity. Falling back from an invalid Bearer token to a cookie (or vice versa) is an anti-pattern that masks misconfigurations.
- **Documentation**: Document this contract explicitly in the repository README.

### 2. Preemptive JWKS Warm-Up & Mandatory Hydra JWKS
- **Decision**: Configure `HydraJwkSetURL` as required on `ExtAuthzServiceConfig`, with co-dependency validation ensuring `HydraIssuer` is provided. On service initialization, preemptively fetch and cache Hydra's JWKS.
- **Rationale**: `oidc.NewRemoteKeySet` lazily fetches keys on first token verification or key miss. Pre-fetching warms the key cache during container startup, guaranteeing the first incoming M2M request does not suffer an external HTTP fetch delay.

### 3. Subject Identity Modeling in OpenFGA (`user:<client_id>`)
- **Decision**: Retain `user:<client_id>` as the OpenFGA subject identity for machine clients.
- **Rationale**: The `core.fga` authorization model defines `type user` across roles, relations, and the `tenant_match` condition. By extracting `claims.Sub` (the machine's `client_id`) from the STS JWT and prefixing it with the rule's configured user resource type (`user`), machine clients fit seamlessly into OpenFGA tuple checks without schema changes.

### 4. Observability and Metric Cardinality
- **Decision**:
  - Add `auth_type` label (`cookie`, `client_credentials`, `none`) to `authz_check_total` and `authz_check_duration_seconds`.
  - Add histogram `authz_check_hydra_verify_duration_seconds` for edge signature verification latency.
  - Track `authz_check_sts_exchange_duration_seconds` partitioned by `exchange_type` (`session` vs `token`).
- **Rationale**: Cardinality is strictly bounded: `auth_type` adds only 3 values, preventing metric explosion while providing direct visibility into traffic breakdown between human users and automation.

### 5. Client Protobuf Generation via Buf
- **Decision**: Extend `client/proto/v1/sts/sts.proto` with `ExchangeToken(ExchangeTokenRequest) returns (ExchangeResponse)` and generate client Go stubs using `buf.gen.client.yaml`.
- **Rationale**: Preserves the established client code-generation pattern in the repository and allows generating mock interfaces via `mockgen` for unit testing.

## Risks / Trade-offs

- **[Risk] Remote Hydra JWKS endpoint latency or failure at startup**  
  → **Mitigation**: Perform eager fetch with a bounded timeout during service initialization; log clear diagnostics if the remote JWKS cannot be reached.
- **[Risk] Existing clients sending ambient cookies alongside Bearer tokens**  
  → **Mitigation**: Return clear HTTP 400 error response stating conflicting credentials provided, and document this requirement prominently in `README.md`.
- **[Risk] Latency overhead from two sequential JWT verifications**  
  → **Mitigation**: In-memory JWKS key caches ensure local cryptographic validation is executed in microseconds.
