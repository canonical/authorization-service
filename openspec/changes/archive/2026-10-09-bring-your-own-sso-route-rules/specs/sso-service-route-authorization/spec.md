## ADDED Requirements

### Requirement: Tenant-admin routes require edit permission on the tenant in the path
The Authorization Service SHALL allow a request to a tenant-admin route of the SSO service only when the caller has `can_edit` on `tenant:{tenant_id}`, where `{tenant_id}` is the tenant named in the request path. The tenant-admin routes are:

- `GET` and `POST /api/v0/sso/tenants/{tenant_id}/connections`
- `GET`, `PATCH` and `DELETE /api/v0/sso/tenants/{tenant_id}/connections/{connection_id}`
- `POST /api/v0/sso/tenants/{tenant_id}/connections/{connection_id}/test-logins`
- `GET` and `PUT /api/v0/sso/tenants/{tenant_id}/policy`

#### Scenario: An admin of the tenant manages its connections
- **WHEN** a caller with `can_edit` on `tenant:T` sends `POST /api/v0/sso/tenants/T/connections`
- **THEN** the request is allowed

#### Scenario: An admin of another tenant is denied
- **WHEN** a caller with `can_edit` on `tenant:U` only sends `GET /api/v0/sso/tenants/T/policy`
- **THEN** the request is denied

#### Scenario: A member without edit permission is denied
- **WHEN** a caller who can view `tenant:T` but has no `can_edit` on it sends `PUT /api/v0/sso/tenants/T/policy`
- **THEN** the request is denied

### Requirement: Platform-admin routes require the platform admin role
The Authorization Service SHALL allow a request to a platform-admin route of the SSO service only when the caller is an `assignee` of `role:admin`. The platform-admin routes are:

- `GET /api/v0/sso/connections`
- `GET` and `PUT /api/v0/sso/tenants/{tenant_id}/domains`
- `DELETE /api/v0/sso/connections/{connection_id}`

#### Scenario: A platform admin lists every connection
- **WHEN** an `assignee` of `role:admin` sends `GET /api/v0/sso/connections`
- **THEN** the request is allowed

#### Scenario: A platform admin reads a tenant's domains before replacing them
- **WHEN** an `assignee` of `role:admin` sends `GET /api/v0/sso/tenants/T/domains`
- **THEN** the request is allowed

#### Scenario: A tenant admin cannot set the tenant's own domains
- **WHEN** a caller with `can_edit` on `tenant:T`, who is not an `assignee` of `role:admin`, sends `PUT /api/v0/sso/tenants/T/domains`
- **THEN** the request is denied, although the route sits under the tenant's path

#### Scenario: A tenant admin cannot delete a connection through the platform route
- **WHEN** a caller with `can_edit` on `tenant:T` sends `DELETE /api/v0/sso/connections/{connection_id}` for a connection of tenant T
- **THEN** the request is denied; the tenant's own route for it is `DELETE /api/v0/sso/tenants/T/connections/{connection_id}`

### Requirement: Every route and method has its own rule
The rules for the SSO service SHALL match each route by its exact path template and method, and SHALL NOT match by a path prefix: the tenant-admin and the platform-admin routes share the `/api/v0/sso` prefix, and the two platform-admin routes for a tenant's domains sit under `/api/v0/sso/tenants/{tenant_id}/`.

#### Scenario: A route with no rule is denied
- **WHEN** any caller sends a request under `/api/v0/sso` whose method and path match none of the twelve rules
- **THEN** the request is denied, as for any route without a rule

#### Scenario: The domains route is not covered by the tenant rules
- **WHEN** the rules are evaluated for `PUT /api/v0/sso/tenants/T/domains`
- **THEN** only the platform-admin rule for that exact path matches, and no tenant-admin rule does

### Requirement: The rules use the existing authorization model
The rules for the SSO service SHALL refer only to object types and relations that the authorization model already defines (`tenant` with `can_edit`, `role` with `assignee`), so that adding them changes no model and writes no tuple.

#### Scenario: The rules validate against the model
- **WHEN** `go run . seed validate` is run with the SSO service's rules file present
- **THEN** validation succeeds for `sso-service` and for every other service's rules
