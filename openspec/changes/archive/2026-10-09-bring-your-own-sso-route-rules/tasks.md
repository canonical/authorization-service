## 1. Route rules

- [x] 1.1 Add `authz/model/services/sso-service/rules.yaml` with the service name, a revision and a description. The description says what the rules protect and what is not behind external authorization (the browser pages, the gRPC services)
- [x] 1.2 Add the eight tenant-admin rules (`can_edit` on `tenant:{tenant_id}`), one for each method and path template
- [x] 1.3 Add the four platform-admin rules (`assignee` on `role:admin`), one for each method and path template. Include `GET` and `PUT /api/v0/sso/tenants/{tenant_id}/domains`, which are under the tenant's path
- [x] 1.4 Add the Tenant Service's three rules and raise the revision of its rules. `can_edit` is for removing a member and for setting the MFA policy. `can_view` is for reading the MFA policy

## 2. Validation

- [x] 2.1 Run `go run . seed validate` and make sure that it succeeds for `sso-service` and for the rules of the other services
