## 1. Route rules

- [x] 1.1 Add `authz/model/services/sso-service/rules.yaml` with the service name, a revision and a description of what the rules protect and what is deliberately not behind external authorization (the browser pages, the gRPC services)
- [x] 1.2 Add the eight tenant-admin rules (`can_edit` on `tenant:{tenant_id}`), one per method and path template
- [x] 1.3 Add the four platform-admin rules (`assignee` on `role:admin`), one per method and path template, including `GET` and `PUT /api/v0/sso/tenants/{tenant_id}/domains`, which sit under the tenant's path
- [x] 1.4 Add the tenant service's three rules (`can_edit` for removing a member and for setting the MFA policy, `can_view` for reading it) and raise its rules' revision

## 2. Validation

- [x] 2.1 Run `go run . seed validate` and confirm it succeeds for `sso-service` and for the other services' rules
