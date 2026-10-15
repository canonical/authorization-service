# multi-arch-artifacts Specification

## Purpose
This specification defines multi-architecture Go binary compilation (`amd64`, `arm64`, and `riscv64`) and multi-architecture Rock container image packaging and publishing (`amd64` and `arm64`) for `authorization-service`.

## Requirements
### Requirement: Multi-architecture Go binary compilation
The build scripts and CI pipeline SHALL compile the `authorization-service` binary for `linux/amd64`, `linux/arm64`, and `linux/riscv64`. When the caller runs `make build` without specifying `GOOS` or `GOARCH`, `scripts/build-app.sh` SHALL default to the host OS and architecture from `go env GOOS` and `go env GOARCH`.

#### Scenario: Explicit target architecture compilation
- **WHEN** a user or CI job runs `GOARCH=amd64 make build`, `GOARCH=arm64 make build`, or `GOARCH=riscv64 make build`
- **THEN** `scripts/build-app.sh` compiles a static ELF binary (`CGO_ENABLED=0`) for the requested `GOARCH`
- **THEN** the command exits with code `0`

#### Scenario: Building binaries for all supported binary architectures
- **WHEN** a user or CI job runs `make build-all` (or the CI binary build step)
- **THEN** the build system compiles static Linux binaries for all three architectures (`amd64`, `arm64`, and `riscv64`)
- **THEN** the command fails if any of the three architectures fails to compile

#### Scenario: Default host architecture compilation
- **WHEN** a user runs `make build` without setting `GOOS` or `GOARCH`
- **THEN** `scripts/build-app.sh` resolves `GOOS` and `GOARCH` from `go env` and builds a binary for the host OS and architecture

### Requirement: Supported-architecture Rockcraft packaging
`rockcraft.yaml` SHALL declare only the supported container image architectures (`amd64` and `arm64`) under `platforms` and SHALL explicitly set `GOOS=linux`, `GOARCH=${CRAFT_ARCH_BUILD_FOR}`, and `CGO_ENABLED=0` during the `go-build` step. Each packed Rock archive SHALL contain an `/usr/bin/app` binary that matches the target platform architecture (`amd64` or `arm64`).

#### Scenario: Packing amd64 and arm64 Rock images
- **WHEN** `rockcraft pack` builds the `amd64` or `arm64` platform target
- **THEN** the `go-build` part exports `GOOS=linux`, `GOARCH=${CRAFT_ARCH_BUILD_FOR}`, and `CGO_ENABLED=0` before running `make build`
- **THEN** the resulting `authorization-service_<version>_<arch>.rock` contains a matching `amd64` or `arm64` `/usr/bin/app` binary and passes `structure-tests.yaml`

#### Scenario: Local Rock loading detects and validates host architecture dynamically
- **WHEN** a developer runs `./build.sh` on an `amd64` or `arm64` machine
- **THEN** `./build.sh` defaults `IMAGE` to `authorization-service:dev` if unset, resolves the host architecture (`ARCH`), validates that `ARCH` is `amd64` or `arm64` (rejecting empty or unsupported rock architectures such as `riscv64` with a non-zero exit code), and copies `authorization-service_<version>_<ARCH>.rock` into the local container store

### Requirement: Dynamic CI image matrix and OCI manifest list publishing
The CI workflow (`.github/workflows/ci.yaml`) SHALL parse the supported image architectures (`amd64` and `arm64`) from `.platforms` in `rockcraft.yaml` and execute one parallel Rock build job per image architecture. On `main` branch pushes and release tag pushes, the publish job SHALL push per-architecture image tags (`<tag>-amd64` and `<tag>-arm64`) and create a multi-architecture OCI manifest list under `<tag>`.

#### Scenario: Matrix Rock build for amd64 and arm64
- **WHEN** `.github/workflows/ci.yaml` runs after `unit-test` passes
- **THEN** the `get-architectures` job extracts `["amd64", "arm64"]` from `rockcraft.yaml`
- **THEN** the `build` job runs `_rock-build.yaml` in parallel for `amd64` and `arm64` only

#### Scenario: Invalid platform configuration fails architecture discovery
- **WHEN** `rockcraft.yaml` omits the `platforms` map or produces an empty platform list
- **THEN** the `get-architectures` job fails with a non-zero exit code before the `build` job starts

#### Scenario: Multi-architecture OCI manifest publishing on main branch
- **WHEN** the `amd64` and `arm64` matrix `build` jobs succeed on `refs/heads/main`
- **THEN** `gh-publish-branch` uploads each `<version>-<arch>` image and publishes multi-architecture OCI manifest lists for `latest` and `<commit-sha>`

### Requirement: Backward compatibility for single-architecture consumers
Published OCI manifest lists SHALL preserve existing tag names (`latest`, `<commit-sha>`, and `<version>-candidate`) and promote `<version>-candidate` to `<version>` and `stable` via the centralized `canonical/identity-team` workflow ([`canonical/identity-team#156`](https://github.com/canonical/identity-team/pull/156)), so existing Kubernetes, Helm, and Terraform configurations pull the matching architecture without configuration changes.

#### Scenario: Existing deployment pulls published multi-architecture tag on amd64 or arm64 node
- **WHEN** a Kubernetes node on `linux/amd64` or `linux/arm64` pulls a published multi-architecture image tag (such as `ghcr.io/canonical/authorization-service:latest` or `ghcr.io/canonical/authorization-service:<version>-candidate`, and `:stable` / `:<version>` via the centralized promotion workflow tracked in [`canonical/identity-team#156`](https://github.com/canonical/identity-team/pull/156) and verified via `docker manifest inspect <tag>`)
- **THEN** the container runtime resolves the matching `amd64` or `arm64` platform digest from the OCI manifest list without changing the image reference
