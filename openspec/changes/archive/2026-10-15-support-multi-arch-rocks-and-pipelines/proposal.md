# Proposal

## Why

`authorization-service` currently builds Go binaries and publishes Rock images only for `amd64`. Operators and developers need Go binaries built for `amd64`, `arm64`, and `riscv64`, and native Rock container images built and published for the supported image architectures (`amd64` and `arm64`).

## What Changes

- Update `scripts/build-app.sh` and `Makefile` to compile the `authorization-service` Go binary for `linux/amd64`, `linux/arm64`, and `linux/riscv64`.
- Update `.github/workflows/unittest.yaml` to build the Go binary across all three target architectures (`amd64`, `arm64`, `riscv64`) in CI.
- Update `rockcraft.yaml`, `build.sh`, `skaffold.yaml`, and `docker/app/Dockerfile` to build container and Rock images for `amd64` and `arm64`.
- Update `.github/workflows/ci.yaml` to read the supported image architectures (`amd64` and `arm64`) from `rockcraft.yaml`, build Rock images in a matrix, and publish a multi-architecture OCI manifest list.
- Delegate candidate image promotion to `canonical/identity-team/.github/workflows/_rock-promote.yaml` and open upstream PR [`canonical/identity-team#156`](https://github.com/canonical/identity-team/pull/156) (`skopeo copy --all`) so promoted release tags preserve both `amd64` and `arm64` manifests.

## Capabilities

### New Capabilities

- `multi-arch-artifacts`: Multi-architecture Go binary builds (`amd64`, `arm64`, `riscv64`) and multi-architecture Rock image packaging and publishing (`amd64` and `arm64`).

### Modified Capabilities

- `artifact-security-gate`: Multi-architecture binary build verification (`amd64`, `arm64`, `riscv64`) and candidate promotion delegation (`<version>-candidate` to `stable` and `<version>`) via `canonical/identity-team/.github/workflows/_rock-promote.yaml`.

## Non-goals

- Building or publishing Rock or Docker container images for `riscv64`.
- Modifying the runtime behavior, gRPC APIs, REST APIs, or CLI subcommands of `authorization-service`.
- Changing the base Ubuntu release (`ubuntu@24.04`) or Chisel slice selections in `rockcraft.yaml`.

## Impact

- Build and packaging files: `Makefile`, `scripts/build-app.sh`, `rockcraft.yaml`, `build.sh`, `skaffold.yaml`, `docker/app/Dockerfile`.
- CI workflows: `.github/workflows/unittest.yaml`, `.github/workflows/ci.yaml`, and upstream `canonical/identity-team/.github/workflows/_rock-promote.yaml` ([`canonical/identity-team#156`](https://github.com/canonical/identity-team/pull/156)).

## Success Metrics

- `make build` and CI build the `authorization-service` binary for `linux/amd64`, `linux/arm64`, and `linux/riscv64` with zero errors.
- `.github/workflows/ci.yaml` builds `amd64` and `arm64` Rock images in parallel and publishes a multi-architecture OCI manifest list to GHCR.
- `.github/workflows/ci.yaml` delegates post-scan candidate promotion to `canonical/identity-team/.github/workflows/_rock-promote.yaml` (with upstream multi-architecture preservation in [`canonical/identity-team#156`](https://github.com/canonical/identity-team/pull/156)).
