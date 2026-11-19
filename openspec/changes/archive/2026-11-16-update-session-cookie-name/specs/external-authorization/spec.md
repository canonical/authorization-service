# Spec Delta

## MODIFIED Requirements

### Requirement: Dual Credential Extraction and Authentication Dispatch
The external authorization service SHALL inspect incoming Envoy `CheckRequest` headers and dispatch authentication strictly based on whether a single valid credential format is provided. The user session cookie name SHALL be `__Host-session_id`, as issued by the Secure Token Service under the `__Host-` cookie-name contract.

#### Scenario: User session cookie credential present
- **WHEN** an incoming request contains a `Cookie` header containing a `__Host-session_id` cookie and no `Authorization: Bearer` header
- **THEN** the service dispatches to the user session flow and exchanges the session cookie with STS

#### Scenario: Machine Bearer token credential present
- **WHEN** an incoming request contains an `Authorization: Bearer <token>` header and no `__Host-session_id` cookie
- **THEN** the service dispatches to the machine credential flow and forwards the raw token to STS `ExchangeToken` without edge signature verification

#### Scenario: Conflicting credentials present
- **WHEN** an incoming request contains both an `Authorization: Bearer <token>` header and a `__Host-session_id` cookie
- **THEN** the service denies the request with HTTP 400 Bad Request and metric reason `conflicting_credentials`

#### Scenario: Missing authentication credentials
- **WHEN** an incoming request contains neither an `Authorization: Bearer` header nor a `__Host-session_id` cookie
- **THEN** the service denies the request with HTTP 401 Unauthorized and metric reason `no_credentials`
