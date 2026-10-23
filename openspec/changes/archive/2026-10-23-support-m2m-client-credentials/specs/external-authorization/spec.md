# Spec Delta

## Purpose

Provides Envoy external authorization (`ext_authz`) supporting dual authentication for human session cookies and machine client credentials tokens, exchanging them for internal STS JWTs and enforcing OpenFGA fine-grained authorization.

![Authorization Service Architecture (Dark Mode)](../../../../.archify/architecture-authorization-service-20261008-154500/visual-check/authorization-service.visual-check.2048x1320.dark.png)

## ADDED Requirements

### Requirement: Dual Credential Extraction and Authentication Dispatch
The external authorization service SHALL inspect incoming Envoy `CheckRequest` headers and dispatch authentication strictly based on whether a single valid credential format is provided.

#### Scenario: User session cookie credential present
- **WHEN** an incoming request contains a `Cookie` header containing a `session_id` cookie and no `Authorization: Bearer` header
- **THEN** the service dispatches to the user session flow and exchanges the session cookie with STS

#### Scenario: Machine Bearer token credential present
- **WHEN** an incoming request contains an `Authorization: Bearer <token>` header and no `session_id` cookie
- **THEN** the service dispatches to the machine credential flow and forwards the raw token to STS `ExchangeToken` without edge signature verification

#### Scenario: Conflicting credentials present
- **WHEN** an incoming request contains both an `Authorization: Bearer <token>` header and a `session_id` cookie
- **THEN** the service denies the request with HTTP 400 Bad Request and metric reason `conflicting_credentials`

#### Scenario: Missing authentication credentials
- **WHEN** an incoming request contains neither an `Authorization: Bearer` header nor a `session_id` cookie
- **THEN** the service denies the request with HTTP 401 Unauthorized and metric reason `no_credentials`

### Requirement: Delegated Bearer Token Exchange Without Edge Verification
The external authorization service SHALL NOT perform cryptographic signature or issuer verification on incoming Bearer access tokens, but SHALL forward the raw token directly to the Secure Token Service via `ExchangeToken`.

#### Scenario: Forwarding raw Bearer token to STS
- **WHEN** an incoming request carries an `Authorization: Bearer <token>` credential
- **THEN** the service forwards the raw token directly in `stsv1.ExchangeTokenRequest` to STS without inspecting or verifying its cryptographic signature or issuer

#### Scenario: Upstream token rejected by STS
- **WHEN** STS returns an unauthenticated or invalid token error during `ExchangeToken`
- **THEN** the service denies the request with HTTP 403 Forbidden and metric reason `sts_exchange_failed`

### Requirement: Secure Token Service Token Exchange
The external authorization service SHALL exchange credentials with the Secure Token Service (STS) for a short-lived internal STS JWT signed with ES256.

#### Scenario: Exchanging machine Bearer token
- **WHEN** an incoming Bearer token is provided
- **THEN** the service calls STS `ExchangeToken` RPC with the raw upstream Bearer token and receives an internal STS JWT

#### Scenario: Exchanging user session cookie
- **WHEN** a user session cookie is provided
- **THEN** the service calls STS `ExchangeSession` RPC with the session cookie value and receives an internal STS JWT

#### Scenario: STS exchange failure
- **WHEN** STS returns an error during session or token exchange
- **THEN** the service denies the request with HTTP 403 Forbidden and metric reason `sts_exchange_failed`

### Requirement: Uniform OpenFGA Authorization Policy Enforcement
The external authorization service SHALL extract the subject (`claims.Sub`) and organization (`claims.Org`) from the internal STS JWT and enforce OpenFGA relationship checks using the `user:<client_id>` subject format without privilege bypass.

#### Scenario: Authorized machine client
- **WHEN** STS mints an internal JWT with `sub` matching the machine client ID and OpenFGA evaluates that `user:<client_id>` possesses the required relation on the target resource
- **THEN** the service permits the request and returns an Envoy `OkHttpResponse`

#### Scenario: Unauthorized machine client (Zero-Bypass)
- **WHEN** STS mints an internal JWT for a machine client ID but no OpenFGA relation permits `user:<client_id>` for the target resource
- **THEN** the service denies the request with HTTP 403 Forbidden and metric reason `openfga_denied`

### Requirement: Internal JWT Injection in Envoy OkResponse
The external authorization service SHALL inject the internal STS JWT into the `Authorization: Bearer <sts_jwt>` header of the Envoy `OkHttpResponse` for both user and machine flows.

#### Scenario: Downstream header injection
- **WHEN** an authorization decision evaluates to allowed for either user session or machine credential flow
- **THEN** the service returns an Envoy `OkHttpResponse` containing the `Authorization: Bearer <sts_jwt>` header to be forwarded to upstream services

### Requirement: Authentication Observability and Metrics
The external authorization service SHALL observe and record authentication type (`cookie`, `client_credentials`, `none`) and stage latencies across all authorization decisions.

#### Scenario: Recording authentication type and outcome
- **WHEN** an external authorization check completes
- **THEN** the service records metric counters indicating the authentication type (`cookie`, `client_credentials`, or `none`), the outcome (`allow`, `deny`, `error`), and the terminal reason

#### Scenario: Recording stage latencies
- **WHEN** authorization processing takes place
- **THEN** the service observes durations for STS token exchange partitioned by exchange type (`session` vs `token`), resource mapping, and OpenFGA check
