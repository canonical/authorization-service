# tenant-service-route-authorization Specification

## Purpose
This capability states which permission certain routes of the Tenant Service need at the gateway. These are the route that removes a member and the routes for a tenant's MFA policy. The Tenant Service does not authorize anything itself. So the gateway denies a route that has no rule.

A change of who belongs to a tenant needs `can_edit` on the tenant in the path. A change of what the tenant asks of a sign-in needs the same permission. To read the MFA policy, the caller needs `can_view`, as for reading the users of the tenant. The rules for the other routes of the Tenant Service are in `authz/model/services/tenant-service/rules.yaml`. This capability does not repeat them.
## Requirements
### Requirement: Removing a member and setting the MFA policy require edit permission on the tenant
The Authorization Service SHALL allow `DELETE /api/v0/tenants/{tenant_id}/users/{user_id}` and `PUT /api/v0/tenants/{tenant_id}/mfa-policy` only when the caller has `can_edit` on `tenant:{tenant_id}`. Here `{tenant_id}` is the tenant that the request path names.

#### Scenario: An admin of the tenant removes a member
- **WHEN** a caller with `can_edit` on `tenant:T` sends `DELETE /api/v0/tenants/T/users/U`
- **THEN** the Authorization Service allows the request

#### Scenario: An admin of another tenant is denied
- **WHEN** a caller with `can_edit` on `tenant:V` only sends `PUT /api/v0/tenants/T/mfa-policy`
- **THEN** the Authorization Service denies the request

#### Scenario: A member without edit permission cannot change the MFA policy
- **WHEN** a caller who can view `tenant:T` but has no `can_edit` on it sends `PUT /api/v0/tenants/T/mfa-policy`
- **THEN** the Authorization Service denies the request

### Requirement: Reading the MFA policy requires view permission on the tenant
The Authorization Service SHALL allow `GET /api/v0/tenants/{tenant_id}/mfa-policy` only when the caller has `can_view` on `tenant:{tenant_id}`.

#### Scenario: A member reads the tenant's MFA policy
- **WHEN** a caller with `can_view` on `tenant:T` sends `GET /api/v0/tenants/T/mfa-policy`
- **THEN** the Authorization Service allows the request

#### Scenario: A caller with no permission on the tenant is denied
- **WHEN** a caller with no relation to `tenant:T` sends `GET /api/v0/tenants/T/mfa-policy`
- **THEN** the Authorization Service denies the request

