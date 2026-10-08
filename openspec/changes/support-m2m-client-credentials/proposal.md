# Proposal

## Why

Currently, `authorization-service` acts as an Envoy/Istio external authorization provider (`ext_authz`) assuming all incoming requests carry a user session cookie (`session_id=...`), which is exchanged for an internal STS JWT via `sts.ExchangeSession`. Requests without a valid session cookie are rejected with HTTP 401.

To support Machine-to-Machine (M2M) callers (CLI tools, automated daemons, microservices) authenticating via Ory Hydra OAuth2 Client Credentials (`grant_type=client_credentials`), the service must support dual authentication: user session cookies and Hydra Bearer access tokens traversing the same Istio `CUSTOM` AuthorizationPolicy.

## What Changes

- **Configuration Additions**: Extend `ExtAuthzServiceConfig` with Hydra issuer and JWKS URL configuration (`HydraIssuer`, `HydraJwkSetURL`), and initialize a Hydra OIDC verifier alongside the STS verifier.
- **Client Protocol Extension**: Update STS client protobuf definition (`sts.proto`) to include `ExchangeToken(ExchangeTokenRequest) returns (ExchangeResponse)` and regenerate client stubs.
- **Dual Authentication Dispatch**: In `ExternalAuthzService.check`:
  - Inspect request headers for credentials (`Authorization: Bearer <token>` or `Cookie: session_id=...`).
  - For bearer tokens: verify cryptographic signature against Hydra JWKS and check issuer against configured Hydra issuer; call `sts.ExchangeToken` to swap Hydra token for an internal STS JWT.
  - For session cookies: preserve existing flow calling `sts.ExchangeSession`.
  - Reject requests lacking valid credentials or with failed token verification with HTTP 401 Unauthorized.
- **Uniform Authorization Pipeline**: Pass STS JWT claims (`sub`, `org`) through `resourceMapper.Map` and OpenFGA `BatchCheck`. Ensure zero-bypass for machine clients (machine `client_id` must have matching tuples in OpenFGA).
- **Envoy Response Header Injection**: Return HTTP 200 OK with `Authorization: Bearer <sts_jwt>` in `OkHttpResponse` headers for both authentication flows so downstream microservices receive the uniform internal token.
- **Observability**: Track authentication type (`cookie` vs `client_credentials`) in check metrics and capture verification/exchange latency.

## Capabilities

### New Capabilities
- `external-authorization`: Istio external authorization (`ext_authz`) supporting dual authentication (session cookies and Hydra client credentials Bearer tokens), STS token exchange, and OpenFGA policy enforcement.

### Modified Capabilities
<!-- None: no pre-existing specs existed in openspec/specs -->

## Impact

- `config/specs.go`: `ExtAuthzServiceConfig` gains Hydra fields.
- `config/helper.go`: Initializes Hydra verifier if configured and wires it into `ExternalAuthzService`.
- `client/proto/v1/sts/sts.proto`: Adds `ExchangeToken` RPC and `ExchangeTokenRequest` message; client stubs regenerated.
- `internal/service/authz/external.go`: Dispatches between Bearer token and cookie, verifies Hydra tokens, exchanges with STS, and applies OpenFGA check.
- `internal/metrics/authz.go` and `internal/service/authz/metrics.go`: Observability extensions for auth type and exchange durations.
- Envoy / Istio integration: Downstream workloads receive standard STS internal bearer token.
