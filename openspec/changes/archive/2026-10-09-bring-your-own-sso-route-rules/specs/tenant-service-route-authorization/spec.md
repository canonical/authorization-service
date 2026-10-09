## ADDED Requirements

### Requirement: Removing a member and setting the MFA policy require edit permission on the tenant
The Authorization Service SHALL allow `DELETE /api/v0/tenants/{tenant_id}/users/{user_id}` and `PUT /api/v0/tenants/{tenant_id}/mfa-policy` only when the caller has `can_edit` on `tenant:{tenant_id}`, where `{tenant_id}` is the tenant named in the request path.

#### Scenario: An admin of the tenant removes a member
- **WHEN** a caller with `can_edit` on `tenant:T` sends `DELETE /api/v0/tenants/T/users/U`
- **THEN** the request is allowed

#### Scenario: An admin of another tenant is denied
- **WHEN** a caller with `can_edit` on `tenant:V` only sends `PUT /api/v0/tenants/T/mfa-policy`
- **THEN** the request is denied

#### Scenario: A member without edit permission cannot change the MFA policy
- **WHEN** a caller who can view `tenant:T` but has no `can_edit` on it sends `PUT /api/v0/tenants/T/mfa-policy`
- **THEN** the request is denied

### Requirement: Reading the MFA policy requires view permission on the tenant
The Authorization Service SHALL allow `GET /api/v0/tenants/{tenant_id}/mfa-policy` only when the caller has `can_view` on `tenant:{tenant_id}`.

#### Scenario: A member reads the tenant's MFA policy
- **WHEN** a caller with `can_view` on `tenant:T` sends `GET /api/v0/tenants/T/mfa-policy`
- **THEN** the request is allowed

#### Scenario: A caller with no permission on the tenant is denied
- **WHEN** a caller with no relation to `tenant:T` sends `GET /api/v0/tenants/T/mfa-policy`
- **THEN** the request is denied
