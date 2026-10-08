# Design

## Context

The `authorization-service` acts as an Envoy/Istio external authorization provider (`ext_authz`) under an Istio `CUSTOM` AuthorizationPolicy. Currently, it assumes all incoming requests carry a user session cookie (`session_id=...`), which it exchanges for an internal STS JWT via `sts.ExchangeSession`.

To support Machine-to-Machine (M2M) callers utilizing OAuth2 Client Credentials from an IdP (such as Ory Hydra), the service must accept `Authorization: Bearer <token>`, exchange it for an internal STS JWT via `sts.ExchangeToken`, and enforce uniform OpenFGA checks.

Rather than coupling `authorization-service` to external IdP configurations (such as Hydra JWKS endpoints, key caches, and issuer validation), the service treats Bearer access tokens opaquely at the edge, pushing all signature verification, issuer checking, and claim extraction complexity to `secure-token-service` (STS).

![Authorization Service Architecture (Dark Mode)](../../../.archify/architecture-authorization-service-20261008-154500/visual-check/authorization-service.visual-check.2048x1320.dark.png)

See [proposal.md](file:///home/shipperizer/shipperizer/authorization-service/openspec/changes/support-m2m-client-credentials/proposal.md) for background and motivation.

## Goals / Non-Goals

**Goals:**
- Provide dual authentication dispatch supporting both user session cookies and M2M Bearer tokens over the same Istio custom policy.
- Enforce strict mutual exclusivity between credentials to prevent credential confusion or session smuggling.
- Delegate upstream access token validation (signature, issuer, expiration, scopes) entirely to STS, maintaining zero external IdP coupling in `authorization-service`.
- Enforce strict zero-bypass OpenFGA authorization for machine identities formatted as `user:<client_id>`.
- Inject internal STS JWTs into Envoy `OkHttpResponse` headers (`Authorization: Bearer <sts_jwt>`) for upstream services.
- Provide comprehensive observability with `auth_type` dimensions and stage latency histograms.

**Non-Goals:**
- Edge signature or issuer validation of incoming third-party access tokens in `authorization-service` (this responsibility belongs to STS).
- Managing external IdP (Hydra) JWKS URLs, key sets, or cache warming in `authorization-service`.
- Implementing the server-side `ExchangeToken` RPC in STS (tracked in companion issue [`canonical/secure-token-service#44`](https://github.com/canonical/secure-token-service/issues/44)).
- Providing bypasses or special exemptions for machine clients in OpenFGA.

## Decisions

### 1. Strict Mutual Exclusivity for Credentials (Option B)
When an incoming request presents both an `Authorization: Bearer <token>` header and a `Cookie: session_id=...`:
- **Decision**: Reject the request immediately with HTTP 400 Bad Request, denying authorization with reason `conflicting_credentials`.
- **Rationale**: Prevents credential ambiguity and potential session smuggling/confusion attacks where a caller inadvertently or maliciously mixes a human session with a machine service identity. Falling back from an invalid Bearer token to a cookie (or vice versa) is an anti-pattern that masks misconfigurations.
- **Documentation**: Document this contract explicitly in the repository README.

### 2. Delegated Token Validation & Zero IdP Knowledge in Authorization Service
- **Decision**: `authorization-service` does not fetch or cache Hydra JWKS, parse upstream tokens, or validate signatures. It extracts the raw Bearer token and forwards it directly to `sts.ExchangeToken`.
- **Rationale**: 
  - **Single Responsibility**: `authorization-service` is an authorization policy enforcement engine, not an identity broker. `secure-token-service` is the dedicated token and identity exchange service.
  - **Decoupling**: Adding or changing identity providers (e.g. migrating from Hydra, supporting multiple issuers) requires zero changes or re-deployments in `authorization-service`.
  - **Eliminating Double-Verification**: Verifying a token at both the edge and STS wastes CPU cycles and adds unnecessary latency.
  - **Operational Simplicity**: Removes external network calls, JWKS warm-up logic, and remote failure modes from `authorization-service` startup.

### 3. Subject Identity Modeling in OpenFGA (`user:<client_id>`)
- **Decision**: Retain `user:<client_id>` as the OpenFGA subject identity for machine clients.
- **Rationale**: The `core.fga` authorization model defines `type user` across roles, relations, and the `tenant_match` condition. By extracting `claims.Sub` (the machine's `client_id`) from the internal STS JWT and prefixing it with the rule's configured user resource type (`user`), machine clients fit seamlessly into OpenFGA tuple checks without schema changes.

### 4. Observability and Metric Cardinality
- **Decision**:
  - Add `auth_type` label (`cookie`, `client_credentials`, `none`) to `authz_check_total` and `authz_check_duration_seconds`.
  - Track `authz_check_sts_exchange_duration_seconds` partitioned by `exchange_type` (`session` vs `token`).
- **Rationale**: Cardinality is strictly bounded: `auth_type` adds only 3 values, preventing metric explosion while providing direct visibility into traffic breakdown between human users and automation.

### 5. Client Protobuf Generation via Buf
- **Decision**: Extend `client/proto/v1/sts/sts.proto` with `ExchangeToken(ExchangeTokenRequest) returns (ExchangeResponse)` and generate client Go stubs using `buf.gen.client.yaml`.
- **Rationale**: Preserves the established client code-generation pattern in the repository and allows generating mock interfaces via `mockgen` for unit testing.

## Risks / Trade-offs

- **[Risk] Existing clients sending ambient cookies alongside Bearer tokens**  
  → **Mitigation**: Return clear HTTP 400 error response stating conflicting credentials provided, and document this requirement prominently in `README.md`.
- **[Trade-off] Invalid tokens reach STS before rejection**  
  → **Mitigation**: Rather than filtering malformed tokens at the edge, STS rejects invalid tokens with standard gRPC unauthenticated errors, which `authorization-service` translates to HTTP 401/403. Because STS is deployed adjacent within the same local cluster / service mesh, the network overhead is negligible compared to the architectural simplicity and decoupled configuration.
