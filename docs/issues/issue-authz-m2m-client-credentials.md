# feat(authz): support dual authentication (session cookies and client credentials) in external authorization

**GitHub Issue**: [#100](https://github.com/canonical/authorization-service/issues/100)  
**Label**: `enhancement`  
**Companion Issue**: [`canonical/secure-token-service#44`](https://github.com/canonical/secure-token-service/issues/44)  
**Architecture Diagram**: [.archify/architecture-authorization-service-20261008-154500/visual-check/authorization-service.visual-check.2048x1320.dark.png](../../.archify/architecture-authorization-service-20261008-154500/visual-check/authorization-service.visual-check.2048x1320.dark.png) ([`candidate.json`](../../.archify/architecture-authorization-service-20261008-154500/candidate.json))

---

## Context & Motivation
The `authorization-service` acts as an Envoy/Istio external authorization provider (`ext_authz`) under an Istio `CUSTOM` AuthorizationPolicy. Currently, `ExternalAuthzService.check` assumes all incoming requests contain a user session cookie (`Cookie: session=...`), which it exchanges for an internal STS JWT via `sts.ExchangeSession`. If no cookie is present, the request is immediately rejected with HTTP 401 Unauthorized.

We want to expand authentication to support **Machine-to-Machine (M2M) callers** backed by OAuth2 Client Credentials (e.g. from Ory Hydra). Machine requests will arrive at the ingress gateway with `Authorization: Bearer <access_token>`.

Both user cookie requests and machine bearer token requests will pass through the **same Istio custom policy**. To keep `authorization-service` decoupled from specific identity providers and avoid double-verification overhead, the service treats bearer tokens opaquely and delegates all token verification complexity to `secure-token-service` (STS). The `authorization-service` inspects credentials, enforces strict mutual exclusivity, calls STS `ExchangeToken` for machine clients, and executes uniform OpenFGA authorization checks without any service account bypass.

Companion issue in Secure Token Service: [canonical/secure-token-service#44](https://github.com/canonical/secure-token-service/issues/44).

## Proposed Changes

### 1. Configuration (`config/specs.go`)
No external IdP / Hydra configuration is required in `authorization-service`. `ExtAuthzServiceConfig` continues to configure only STS and the internal STS JWKS verifier, maintaining clean separation of concerns.

### 2. Dual Authentication Dispatch in `check()` (`internal/service/authz/external.go`)
Update `ExternalAuthzService.check`:
1. Check headers for authentication credentials:
   - **Strict Mutual Exclusivity**: If both `Authorization: Bearer <token>` and `Cookie: session_id=...` are present, immediately return HTTP 400 Bad Request (`deny`, reason: `conflicting_credentials`).
   - **Machine Credential Flow** (`Authorization: Bearer <token>`):
     - Forward the raw bearer token directly to STS `ExchangeToken` without local signature or issuer verification:
       ```go
       resp, err := s.sts.ExchangeToken(ctx, &stsv1.ExchangeTokenRequest{Token: bearerToken})
       ```
   - **User Session Flow** (`Cookie: session=...`):
     - Extract session cookie value.
     - Invoke STS `ExchangeSession`:
       ```go
       resp, err := s.sts.ExchangeSession(ctx, &stsv1.ExchangeRequest{SessionCookie: sessionValue})
       ```
   - If neither credential is provided, return HTTP 401 Unauthorized (`deny`, reason: `no_credentials`).
2. Common Claims & Authorization Pipeline:
   - Verify the returned internal STS JWT with the existing STS verifier.
   - Extract claims from the STS internal JWT (`claims.Sub`, `claims.Org`). For machine clients, `claims.Sub` corresponds to the client ID.
   - Execute `resourceMapper.Map(ctx, userIdentity, method, path)`.
   - Execute `s.fga.BatchCheck`.
   - **Zero Bypass**: Machine clients are subject to standard OpenFGA tuple checks using `user:<client_id>`. If no relationship tuple permits the machine `client_id` for the resource, access is denied (HTTP 403 Forbidden). Permissions will be assigned via the upcoming authorization APIs.
   - On allow: return `okResponse(exchangeResp.AccessToken)`, injecting `Authorization: Bearer <sts_jwt>` for upstream services.

### 3. Observability
- Track authentication method in check metrics (`auth_type="cookie"` vs `auth_type="client_credentials"` vs `auth_type="none"`).
- Record latency for STS token exchange partitioned by `exchange_type` (`session` vs `token`).

## Acceptance Criteria
- [ ] Requests with `Authorization: Bearer <token>` forward the raw token to STS `ExchangeToken` without edge signature verification.
- [ ] Requests with session cookies continue calling `ExchangeSession` without regression.
- [ ] Requests presenting both credentials are rejected with HTTP 400 Bad Request.
- [ ] Requests lacking credentials return HTTP 401 Unauthorized.
- [ ] OpenFGA authorization checks apply strictly to machine client IDs (no bypass).
- [ ] Envoy receives `Authorization: Bearer <sts_jwt>` in `OkHttpResponse` for both flows.
- [ ] Comprehensive unit and integration tests covering both auth branches.
