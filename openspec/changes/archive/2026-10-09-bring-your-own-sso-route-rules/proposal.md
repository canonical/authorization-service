## Why

The SSO service is a new service of the platform: it holds the connections of tenants to their own OIDC identity providers ("bring your own SSO") and exposes a management API for them through the gateway. Like the tenant service, it authenticates the caller's token and authorizes nothing itself: which caller may use which route is this service's decision, at the gateway. Without rules for it every one of its routes is denied. The same feature adds three routes to the tenant service (removing a member, reading and setting a tenant's MFA policy), which are denied for the same reason until they have rules.

## What Changes

- Add `authz/model/services/sso-service/rules.yaml`: twelve route rules for the SSO service's HTTP API under `/api/v0/sso`.
  - Eight tenant-admin routes (a tenant's connections, starting a test sign-in, the tenant's SSO policy) require `can_edit` on the tenant named in the path.
  - Four platform-admin routes (every connection across tenants, reading and setting a tenant's email domains, deleting any connection) require being an `assignee` of `role:admin`.
- Add three rules to `authz/model/services/tenant-service/rules.yaml`, with a new revision: `DELETE /api/v0/tenants/{tenant_id}/users/{user_id}` and `PUT /api/v0/tenants/{tenant_id}/mfa-policy` require `can_edit` on the tenant named in the path, `GET /api/v0/tenants/{tenant_id}/mfa-policy` requires `can_view` on it.
- No change to the authorization model: the rules use the `tenant` and `role` types that exist, as the tenant service's own rules do.

## Capabilities

### New Capabilities
- `sso-service-route-authorization`: which permission each route of the SSO service's management API requires.
- `tenant-service-route-authorization`: which permission the tenant service's three new routes require.

### Modified Capabilities

## Non-goals

- The SSO service's browser pages (`/login`, `/callback/{connection_id}`, `/consent`, `/error`) are not behind external authorization: a user reaches them in the middle of a sign-in, before they have a session.
- Its gRPC services are not routed through the gateway; `SSOSignInService` has no HTTP route at all.
- No new OpenFGA type, relation or tuple writer for connections: a connection belongs to one tenant, and the permission on that tenant decides.

## Impact

- `authz/model/services/sso-service/rules.yaml`: new file, loaded with the other services' rules; `go run . seed validate` checks it.
- `authz/model/services/tenant-service/rules.yaml`: three rules added and the revision raised, so that the next seed loads them.
- A deployment that routes `/api/v0/sso` to the SSO service through the gateway gets these rules with the next seed of the rules; nothing changes for the other services' routes.
