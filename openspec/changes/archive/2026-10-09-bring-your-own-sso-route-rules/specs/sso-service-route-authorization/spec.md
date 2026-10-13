## ADDED Requirements

### Requirement: Tenant-admin routes require edit permission on the tenant in the path
The Authorization Service SHALL allow a request to a tenant-admin route of the SSO service only when the caller has `can_edit` on `tenant:{tenant_id}`. Here `{tenant_id}` is the tenant that the request path names. The tenant-admin routes are:

- `GET` and `POST /api/v0/sso/tenants/{tenant_id}/connections`
- `GET`, `PATCH` and `DELETE /api/v0/sso/tenants/{tenant_id}/connections/{connection_id}`
- `POST /api/v0/sso/tenants/{tenant_id}/connections/{connection_id}/test-logins`
- `GET` and `PUT /api/v0/sso/tenants/{tenant_id}/policy`

#### Scenario: An admin of the tenant manages its connections
- **WHEN** a caller with `can_edit` on `tenant:T` sends `POST /api/v0/sso/tenants/T/connections`
- **THEN** the Authorization Service allows the request

#### Scenario: An admin of another tenant is denied
- **WHEN** a caller with `can_edit` on `tenant:U` only sends `GET /api/v0/sso/tenants/T/policy`
- **THEN** the Authorization Service denies the request

#### Scenario: A member without edit permission is denied
- **WHEN** a caller who can view `tenant:T` but has no `can_edit` on it sends `PUT /api/v0/sso/tenants/T/policy`
- **THEN** the Authorization Service denies the request

### Requirement: Platform-admin routes require the platform admin role
The Authorization Service SHALL allow a request to a platform-admin route of the SSO service only when the caller is an `assignee` of `role:admin`. The platform-admin routes are:

- `GET /api/v0/sso/connections`
- `GET` and `PUT /api/v0/sso/tenants/{tenant_id}/domains`
- `DELETE /api/v0/sso/connections/{connection_id}`

#### Scenario: A platform admin lists every connection
- **WHEN** an `assignee` of `role:admin` sends `GET /api/v0/sso/connections`
- **THEN** the Authorization Service allows the request

#### Scenario: A platform admin reads a tenant's domains before replacing them
- **WHEN** an `assignee` of `role:admin` sends `GET /api/v0/sso/tenants/T/domains`
- **THEN** the Authorization Service allows the request

#### Scenario: A tenant admin cannot set the tenant's own domains
- **WHEN** a caller with `can_edit` on `tenant:T`, who is not an `assignee` of `role:admin`, sends `PUT /api/v0/sso/tenants/T/domains`
- **THEN** the Authorization Service denies the request
- **AND** it denies the request although the route is under the path of the tenant

#### Scenario: A tenant admin cannot delete a connection through the platform route
- **WHEN** a caller with `can_edit` on `tenant:T` sends `DELETE /api/v0/sso/connections/{connection_id}` for a connection of tenant T
- **THEN** the Authorization Service denies the request
- **AND** the tenant's own route for this action is `DELETE /api/v0/sso/tenants/T/connections/{connection_id}`

### Requirement: Every route and method has its own rule
The rules for the SSO service SHALL match each route by its exact path template and method. They SHALL NOT match by a path prefix, for 2 reasons. The tenant-admin routes and the platform-admin routes share the prefix `/api/v0/sso`. Also, the 2 platform-admin routes for the domains of a tenant are under `/api/v0/sso/tenants/{tenant_id}/`.

#### Scenario: A route with no rule is denied
- **WHEN** any caller sends a request under `/api/v0/sso` whose method and path match none of the twelve rules
- **THEN** the Authorization Service denies the request, as for any route without a rule

#### Scenario: The domains route is not covered by the tenant rules
- **WHEN** the Authorization Service evaluates the rules for `PUT /api/v0/sso/tenants/T/domains`
- **THEN** only the platform-admin rule for that exact path matches
- **AND** no tenant-admin rule matches

### Requirement: The rules use the existing authorization model
The rules for the SSO service SHALL refer only to object types and relations that the authorization model already defines (`tenant` with `can_edit`, `role` with `assignee`). So the rules change no model and write no tuple.

#### Scenario: The rules validate against the model
- **WHEN** `go run . seed validate` runs with the SSO service's rules file present
- **THEN** validation succeeds for `sso-service` and for the rules of every other service
