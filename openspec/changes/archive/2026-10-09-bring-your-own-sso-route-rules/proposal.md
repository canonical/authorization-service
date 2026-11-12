## Why

The SSO service is a new service of the platform. It holds the connections of tenants to their own OIDC identity providers ("bring your own SSO"). It exposes a management API for them through the gateway.

Like the Tenant Service, the SSO service authenticates the caller's token and does not authorize anything itself. The gateway decides which caller can use which route. Without rules for the SSO service, the gateway denies every route of the service.

The same feature adds three routes to the Tenant Service. These routes remove a member, read the MFA policy of a tenant and set it. The gateway denies them for the same reason until they have rules.

## What Changes

- Add `authz/model/services/sso-service/rules.yaml`. It holds twelve route rules for the SSO service's HTTP API under `/api/v0/sso`.
  - Eight tenant-admin routes need `can_edit` on the tenant that the path names. They cover the connections of a tenant, the start of a test sign-in and the SSO policy of the tenant.
  - Four platform-admin routes need the caller to be an `assignee` of `role:admin`. They cover every connection across tenants, reading and setting the email domains of a tenant, and deleting any connection.
- Add three rules to `authz/model/services/tenant-service/rules.yaml`, with a new revision.
  - `DELETE /api/v0/tenants/{tenant_id}/users/{user_id}` and `PUT /api/v0/tenants/{tenant_id}/mfa-policy` need `can_edit` on the tenant that the path names.
  - `GET /api/v0/tenants/{tenant_id}/mfa-policy` needs `can_view` on that tenant.
- Do not change the authorization model. The rules use the `tenant` and `role` types that exist, as the rules of the Tenant Service do.

## Capabilities

### New Capabilities
- `sso-service-route-authorization`: which permission each route of the SSO service's management API needs.
- `tenant-service-route-authorization`: which permission the Tenant Service's three new routes need.

### Modified Capabilities

## Non-goals

- The browser pages of the SSO service (`/login`, `/callback/{connection_id}`, `/consent`, `/error`) are not behind external authorization. A user reaches them in the middle of a sign-in, before the user has a session.
- The gRPC services of the SSO service do not go through the gateway. `SSOSignInService` has no HTTP route at all.
- There is no new OpenFGA type, no new relation and no new tuple writer for connections. A connection belongs to one tenant, and the permission on that tenant decides.

## Impact

- `authz/model/services/sso-service/rules.yaml` is a new file. The Authorization Service loads it with the rules of the other services. `go run . seed validate` checks it.
- `authz/model/services/tenant-service/rules.yaml` gets three rules and a higher revision, so that the next seed loads them.
- A deployment that routes `/api/v0/sso` to the SSO service through the gateway gets these rules with the next seed of the rules. Nothing changes for the routes of the other services.
