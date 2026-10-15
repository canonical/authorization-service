# Tasks

## 1. Multi-Architecture Binary Builds (`scripts/build-app.sh`, `Makefile`)

- [x] 1.1 Update `scripts/build-app.sh` and `Makefile` to resolve `GOOS`, `GOARCH`, and `APP_BINARY_PATH` from environment variables or `go env` defaults, and verify with `GOARCH=amd64 make build`, `GOARCH=arm64 make build`, and `GOARCH=riscv64 make build`.
- [x] 1.2 Add a `build-all` target to `Makefile` that compiles `bin/app-linux-amd64`, `bin/app-linux-arm64`, and `bin/app-linux-riscv64`, and verify by running `make build-all`.

## 2. Supported-Architecture Container and Rock Packaging (`rockcraft.yaml`, `build.sh`, `skaffold.yaml`, `docker/app/Dockerfile`)

- [x] 2.1 Update `rockcraft.yaml` to declare `amd64: null` and `arm64: null` under `platforms` and export `GOOS=linux`, `GOARCH="${CRAFT_ARCH_BUILD_FOR}"`, and `CGO_ENABLED=0` in `parts.go-build.override-build`, and verify `.platforms | keys` with `yq`.
- [x] 2.2 Update `build.sh` to default `IMAGE="${IMAGE:-authorization-service:dev}"`, detect the host architecture (`ARCH`), validate it against the supported rock platforms (`amd64` and `arm64`), and verify syntax with `sh -n build.sh`.
- [x] 2.3 Update `skaffold.yaml` to set `platforms: ["linux/amd64", "linux/arm64"]` and update `docker/app/Dockerfile` to use `golang:1.26-alpine` and accept `TARGETOS` and `TARGETARCH` build arguments, and verify YAML syntax with `yq`.

## 3. Continuous Integration and Release Pipelines (`.github/workflows/`)

- [x] 3.1 Update `.github/workflows/unittest.yaml` to run `make build-all` and assert ELF target architectures (`x86-64`, `aarch64`, `RISC-V`) with `file`.
- [x] 3.2 Update `.github/workflows/ci.yaml` to add the `get-architectures` job (with `yq` non-empty platform validation), run `build` across `strategy.matrix.architecture` (`amd64` and `arm64`), remove the single-file `rock:` input from `gh-publish-branch` and `gh-publish-candidate`, and pin `canonical/identity-team` workflows to `v1.17.1`.
- [x] 3.3 Open and integrate upstream PR [`canonical/identity-team#156`](https://github.com/canonical/identity-team/pull/156) (`v1.17.1`) for `_rock-promote.yaml` to pass `--all` to `skopeo copy` during candidate promotion.

## 4. Verification Suite

- [x] 4.1 Run `go test -short -race ./...` and confirm zero test or race detector failures.
- [x] 4.2 Run `make build-all` and confirm `file bin/app-linux-amd64 bin/app-linux-arm64 bin/app-linux-riscv64` outputs `x86-64`, `ARM aarch64`, and `UCB RISC-V` ELF binaries.
- [x] 4.3 Run `openspec validate --all` and confirm all specifications and changes pass validation.

## 5. Documentation & Rollout

- [x] 5.1 Update `LOCAL_DEVELOPMENT.md` and `DEVELOPMENT.md` to document `make build-all` (`amd64`, `arm64`, `riscv64` binaries), `amd64`/`arm64` Rock builds, and `docker manifest inspect` promotion verification.
- [x] 5.2 Confirm backward compatibility for existing image tags (`latest`, `stable`, `<version>`) across `k8s/` and `terraform/` manifests before rollout.

## Implementation Notes

- `riscv64` is intentionally supported at the Go binary level only (`make build-all` and `unittest.yaml`), while Rock and container images are packaged and published strictly for `amd64` and `arm64`.
- Local workflows in `.github/workflows/ci.yaml` delegate Rock building (`_rock-build.yaml`), multi-architecture manifest publishing (`_rock-gh-publish.yaml`), vulnerability scanning (`_rock-scan.yaml`), and candidate promotion (`_rock-promote.yaml`) to `canonical/identity-team` (`v1.17.1`) without duplicating centralized steps locally.
- Removing `with.rock` from `gh-publish-branch` and `gh-publish-candidate` enables `_rock-gh-publish.yaml`'s built-in multi-architecture mode (`*.rock` artifact download, `<tag>-amd64` and `<tag>-arm64` push, and `docker manifest create --amend` + `docker manifest push`).
- Upstream PR [`canonical/identity-team#156`](https://github.com/canonical/identity-team/pull/156) (`v1.17.1`) updates `canonical/identity-team/.github/workflows/_rock-promote.yaml` to pass `--all` on `rockcraft.skopeo --insecure-policy copy --all` so multi-architecture manifest lists preserve all architectures (`amd64` and `arm64`) during promotion to `<release-tag>` and `stable`.
