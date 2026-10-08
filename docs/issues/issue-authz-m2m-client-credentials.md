# feat(authz): support dual authentication (session cookies and Hydra client credentials) in external authorization

**GitHub Issue**: [#100](https://github.com/canonical/authorization-service/issues/100)  
**Label**: `enhancement`  
**Companion Issue**: [`canonical/secure-token-service#44`](https://github.com/canonical/secure-token-service/issues/44)  
**Architecture Diagram**: [.archify/architecture-authorization-service-20261008-154500/authorization-service.html](file:///home/shipperizer/shipperizer/authorization-service/.archify/architecture-authorization-service-20261008-154500/authorization-service.html)

---

## Context & Motivation
The `authorization-service` acts as an Envoy/Istio external authorization provider (`ext_authz`) under an Istio `CUSTOM` AuthorizationPolicy. Currently, `ExternalAuthzService.check` assumes all incoming requests contain a user session cookie (`Cookie: session=...`), which it exchanges for an internal STS JWT via `sts.ExchangeSession`. If no cookie is present, the request is immediately rejected with HTTP 401 Unauthorized.

We want to expand authentication to support **Machine-to-Machine (M2M) callers** backed by the OAuth2 Client Credentials flow from Ory Hydra. Machine requests will arrive at the ingress gateway with `Authorization: Bearer <hydra_access_token>`.

Both user cookie requests and machine bearer token requests will pass through the **same Istio custom policy**. The `authorization-service` must inspect credentials, validate Hydra signatures at the edge, call STS `ExchangeToken` for machine clients, and execute uniform OpenFGA authorization checks without any service account bypass.

Companion issue in Secure Token Service: [canonical/secure-token-service#44](https://github.com/canonical/secure-token-service/issues/44).

## Proposed Changes

### 1. Configuration Additions (`config/specs.go`)
Extend `ExtAuthzServiceConfig` to include Hydra verification configuration:
```go
type ExtAuthzServiceConfig struct {
    JwkSetURL      string `validate:"required,http_url" envconfig:"EXTAUTHZ_JWK_SET_URL" mapstructure:"jwk_set_url"`
    HydraIssuer    string `validate:"omitempty,http_url" envconfig:"EXTAUTHZ_HYDRA_ISSUER" mapstructure:"hydra_issuer"`
    HydraJwkSetURL string `validate:"omitempty,http_url" envconfig:"EXTAUTHZ_HYDRA_JWK_SET_URL" mapstructure:"hydra_jwk_set_url"`
}
```

### 2. Dual Authentication Dispatch in `check()` (`internal/service/authz/external.go`)
Update `ExternalAuthzService.check`:
1. Check headers for authentication credentials:
   - **Machine Credential Flow** (`Authorization: Bearer <token>`):
     - Validate token signature against Hydra JWKS (`HydraJwkSetURL`).
     - Verify token issuer matches configured Hydra issuer (`HydraIssuer`).
     - Invoke STS `ExchangeToken`:
       ```go
       resp, err := s.sts.ExchangeToken(ctx, &stsv1.ExchangeTokenRequest{Token: bearerToken})
       ```
   - **User Session Flow** (`Cookie: session=...`):
     - Extract session cookie value.
     - Invoke STS `ExchangeSession`:
       ```go
       resp, err := s.sts.ExchangeSession(ctx, &stsv1.ExchangeRequest{SessionCookie: sessionValue})
       ```
   - If neither credential is provided or valid, return HTTP 401 Unauthorized (`deny`, reason: `no_credentials`).
2. Common Claims & Authorization Pipeline:
   - Extract claims from the returned STS internal JWT (`claims.Sub`, `claims.Org`). For machine clients, `claims.Sub` corresponds to the client ID.
   - Execute `resourceMapper.Map(ctx, userIdentity, method, path)`.
   - Execute `s.fga.BatchCheck`.
   - **Zero Bypass**: Machine clients are subject to standard OpenFGA tuple checks. If no relationship tuple permits the machine `client_id` for the resource, access is denied (HTTP 403 Forbidden). Permissions will be assigned via the upcoming authorization APIs.
   - On allow: return `okResponse(exchangeResp.AccessToken)`, injecting `Authorization: Bearer <sts_jwt>` for upstream services.

### 3. Observability
- Track authentication method in check metrics (`auth_type="cookie"` vs `auth_type="client_credentials"`).
- Record latency for Hydra signature verification and STS token exchange.

## Acceptance Criteria
- [ ] `ExtAuthzServiceConfig` supports Hydra JWKS and Issuer configuration.
- [ ] Requests with `Authorization: Bearer <hydra_token>` are cryptographically verified against Hydra before calling STS `ExchangeToken`.
- [ ] Requests with session cookies continue calling `ExchangeSession` without regression.
- [ ] Unauthenticated requests (no cookie, no bearer token, or invalid signature) return HTTP 401.
- [ ] OpenFGA authorization checks apply strictly to machine client IDs (no bypass).
- [ ] Envoy receives `Authorization: Bearer <sts_jwt>` in `OkHttpResponse` for both flows.
- [ ] Comprehensive unit and integration tests covering both auth branches.
