# Tasks

- [x] Update `sessionCookieName` constant in `internal/service/authz/external.go` to `__Host-session_id`
- [x] Update unit tests (`internal/service/authz/external_test.go`) to the renamed cookie
- [x] Update integration tests (`tests/integration/check/check_integration_test.go`) to the renamed cookie
- [x] Update fake webserver nginx config (`k8s/fake-server/webserver.yaml`) cookie variable

## Verification

- [x] `go build ./...` and `go vet ./...` clean
- [x] `go test -short ./...` passes (17/17 packages)
