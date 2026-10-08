# Proposal

## Why

Currently, `authorization-service` acts as an Envoy/Istio external authorization provider (`ext_authz`) assuming all incoming requests carry a user session cookie (`session_id=...`), which is exchanged for an internal STS JWT via `sts.ExchangeSession`. Requests without a valid session cookie are rejected with HTTP 401.

To support Machine-to-Machine (M2M) callers (CLI tools, automated daemons, microservices) authenticating via OAuth2 Client Credentials (e.g., Ory Hydra `grant_type=client_credentials`), the service must support dual authentication: user session cookies and Bearer access tokens traversing the same Istio `CUSTOM` AuthorizationPolicy.

Crucially, `authorization-service` acts strictly as an authorization and policy enforcement filter. It does not validate incoming access token signatures or communicate directly with upstream identity providers. Instead, all token validation, signature verification, and claims translation complexity is delegated to `secure-token-service` (STS), treating session cookies and bearer tokens symmetrically as opaque credentials to be exchanged.

## What Changes

- **Client Protocol Extension**: Update STS client protobuf definition (`sts.proto`) to include `ExchangeToken(ExchangeTokenRequest) returns (ExchangeResponse)` and regenerate client stubs.
- **Dual Authentication Dispatch**: In `ExternalAuthzService.check`:
  - Inspect request headers for credentials (`Authorization: Bearer <token>` or `Cookie: session_id=...`).
  - Enforce strict mutual exclusivity: if both a Bearer token and a session cookie are provided, reject immediately with HTTP 400 Bad Request (`conflicting_credentials`).
  - For bearer tokens: forward the raw token directly to `sts.ExchangeToken` to swap it for an internal STS JWT without performing local cryptographic signature or issuer validation.
  - For session cookies: preserve existing flow calling `sts.ExchangeSession`.
  - Reject requests lacking credentials with HTTP 401 Unauthorized (`no_credentials`).
- **Delegated Token Validation in STS**: `secure-token-service` takes on the responsibility of cryptographically verifying the upstream access token (e.g., against Hydra's JWKS and issuer) before minting the internal STS JWT.
- **Uniform Authorization Pipeline**: Verify the internal STS JWT returned by STS using the existing STS OIDC verifier. Pass STS JWT claims (`sub`, `org`) through `resourceMapper.Map` and OpenFGA `BatchCheck`. Ensure zero-bypass for machine clients (machine `client_id` must have matching tuples in OpenFGA).
- **Envoy Response Header Injection**: Return HTTP 200 OK with `Authorization: Bearer <sts_jwt>` in `OkHttpResponse` headers for both authentication flows so downstream microservices receive the uniform internal token.
- **Observability**: Track authentication type (`cookie` vs `client_credentials` vs `none`) in check metrics and capture STS exchange latency partitioned by exchange type (`session` vs `token`).

## Capabilities

### New Capabilities
- `external-authorization`: Istio external authorization (`ext_authz`) supporting dual authentication (session cookies and client credentials Bearer tokens), delegated STS token exchange without edge signature validation, and OpenFGA policy enforcement.

### Modified Capabilities
<!-- None: no pre-existing specs existed in openspec/specs -->

## Impact

- `config/specs.go`: No changes required for external IdP / Hydra settings; `ExtAuthzServiceConfig` remains focused on STS and internal verifiers.
- `client/proto/v1/sts/sts.proto`: Adds `ExchangeToken` RPC and `ExchangeTokenRequest` message; client stubs regenerated.
- `internal/service/authz/external.go`: Dispatches between Bearer token and cookie, forwards raw token to `sts.ExchangeToken`, verifies returned internal STS JWT, and applies OpenFGA check.
- `internal/metrics/authz.go` and `internal/service/authz/metrics.go`: Observability extensions for auth type and STS exchange durations.
- Envoy / Istio integration: Downstream workloads receive standard STS internal bearer token.
