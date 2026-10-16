# Spec Delta

## MODIFIED Requirements

### Requirement: Pull request vulnerability pre-check
The test verification workflow SHALL run static vulnerability analysis on Go dependencies and build and verify the Go binary for `linux/amd64`, `linux/arm64`, and `linux/riscv64` before container image builds. If any high or critical reachable vulnerability is detected, or if any binary fails to build or match its target ELF architecture, the workflow SHALL fail and skip container image builds.

#### Scenario: Clean dependencies in pull request
- **WHEN** a pull request has no known vulnerable Go dependencies and builds and verifies ELF binaries for `linux/amd64` (`x86-64`), `linux/arm64` (`aarch64`), and `linux/riscv64` (`RISC-V`)
- **THEN** vulnerability and binary build checks exit with code `0`
- **THEN** the workflow proceeds to build the container rock artifacts for the supported image architectures (`amd64` and `arm64`)

#### Scenario: Vulnerability detected in pull request
- **WHEN** a pull request introduces a dependency with a known reachable vulnerability
- **THEN** vulnerability checks exit with a non-zero exit code
- **THEN** the container rock build matrix jobs (`amd64` and `arm64`) do not run

### Requirement: Candidate artifact scanning and promotion gate
The release pipeline SHALL publish multi-architecture container artifacts (`amd64` and `arm64`) under temporary candidate tags (`<version>-candidate`, `<version>-candidate-amd64`, and `<version>-candidate-arm64`) and scan the candidate image before promotion. Promotion of `<version>-candidate` to `stable` and `<version>` via `canonical/identity-team` (`_rock-promote.yaml`) and release graduation SHALL occur ONLY when the scan finds zero blocking vulnerabilities.

#### Scenario: Clean candidate promoted to stable
- **WHEN** the candidate container image scan completes with zero blocking vulnerabilities
- **THEN** the promotion job invokes the centralized `canonical/identity-team` promotion workflow (`_rock-promote.yaml`, [`canonical/identity-team#156`](https://github.com/canonical/identity-team/pull/156)) with the `<version>-candidate` image reference to promote `stable` and `<version>` in the container registry (verified via `docker manifest inspect`)
- **THEN** the GitHub Release updates to `prerelease: false` and marks the release as latest

#### Scenario: Vulnerable candidate blocked from promotion
- **WHEN** the candidate container image scan detects an un-triaged blocking vulnerability
- **THEN** the security scan job fails with a non-zero exit code
- **THEN** the workflow does NOT tag the image as `stable` in the container registry
- **THEN** the GitHub Release remains quarantined as a pre-release
