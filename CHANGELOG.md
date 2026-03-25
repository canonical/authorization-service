# Changelog

## [0.2.0](https://github.com/canonical/authorization-service/compare/v0.1.0...v0.2.0) (2026-03-25)


### Features

* add ADRs ([e3aa511](https://github.com/canonical/authorization-service/commit/e3aa511792106c2857a0a31afbf15f3ea35b6415))
* add dev-setup for local development and istio mesh configuration for external authz ([b5bc4d9](https://github.com/canonical/authorization-service/commit/b5bc4d9aba372b69c8368d9d471e6de6a90ab9ce))
* add empty goose migrations ([f38c922](https://github.com/canonical/authorization-service/commit/f38c922ed91661d09f9a31832d3c9cf0d5fd7265))
* add envoy v3 interface for Istio external authorization ([56cfdad](https://github.com/canonical/authorization-service/commit/56cfdadffdada2ab3ec3b600494f4ba832e934f4))
* add exchangeSession invocation + tests ([e3b2346](https://github.com/canonical/authorization-service/commit/e3b234615828180eeeb936d4b6dc6a1da7fd67d6))
* add infrastructure for local deployment ([7115e0e](https://github.com/canonical/authorization-service/commit/7115e0e5bdc4e4264b462d86e3800cc09a4573b6))
* add migrate CLI + test ([474faeb](https://github.com/canonical/authorization-service/commit/474faebe5a623c5406997c0e4cd8c973e039189d))
* add new targets for sts client generation ([a1dc9d8](https://github.com/canonical/authorization-service/commit/a1dc9d8f83ba3da73e43f4660c7e2b65f27b5940))
* add possibility of eager check for gRPC STS connection ([66a2e15](https://github.com/canonical/authorization-service/commit/66a2e15ed6a4442fec6ba637918a051fa9c2244d))
* add postgres DB client + noop + test ([55e1649](https://github.com/canonical/authorization-service/commit/55e164938bf5db2c5ba9a3698f8a08423030b67e))
* add Postgres integration in config and helper ([51f1601](https://github.com/canonical/authorization-service/commit/51f16010a58d033a684523e9e8ec8dab2c41ac51))
* add profiles to skaffold development + add local image tagging and pushing + add reference to dev-setup docs in DEVELOPMENT.md ([d66c877](https://github.com/canonical/authorization-service/commit/d66c87795d523cb90f41e2fedc8d19eb811f4551))
* add sts wrapper client ([440f92a](https://github.com/canonical/authorization-service/commit/440f92ae28a95949911d2aedb7284bb4dd750d00))
* choose underlying implementation based on enabled status ([041ce74](https://github.com/canonical/authorization-service/commit/041ce747e294792b38fd85b1f7cca8fb25a3a7dc))
* initial project structure and configuration ([d3196cf](https://github.com/canonical/authorization-service/commit/d3196cfc2c96fb6a7edab335114889dad6da28aa))


### Bug Fixes

* adapt tests ([5727ea6](https://github.com/canonical/authorization-service/commit/5727ea64fa88090dc1545d761d8be2b202b2b9de))
* add check cmd to test grpc behaviour in place of istio ([a46ac63](https://github.com/canonical/authorization-service/commit/a46ac6399f793c826aeec90ef1f8cc090f167c88))
* adjust version cmd ([ed0b746](https://github.com/canonical/authorization-service/commit/ed0b74688df0053a80986efecaa9c1671db06596))
* cve action when no issues are found while scheduled task runs ([48a6f98](https://github.com/canonical/authorization-service/commit/48a6f988537f2dce298ea9614459c03f62cf45ee))
* force header in check response ([ff92dda](https://github.com/canonical/authorization-service/commit/ff92dda4913d77a01abad911aab2270197f9f75a))
* mock generation ([2228498](https://github.com/canonical/authorization-service/commit/22284985a4edfa3571159dd3cdc0b44a4c9b4045))
* rockcraft build for CI ([de94794](https://github.com/canonical/authorization-service/commit/de947941746f8d31113b1e513e701b9772efe5dc))
* typo ([b1b846f](https://github.com/canonical/authorization-service/commit/b1b846fab93e6e4a014c21d15625fb2927e3ede5))
* use proper context for graceful shutdown ([f187fe4](https://github.com/canonical/authorization-service/commit/f187fe4e53871a82e29bc4134fa0c6733cad04d4))
* wording + replace tabs with spaces ([75c3757](https://github.com/canonical/authorization-service/commit/75c37577f93f5e1ffc462f89d09c1b7f743b0d7c))
* workflows and temporarily prevent TIOBE checks from running until project is registered ([92f79ac](https://github.com/canonical/authorization-service/commit/92f79ace6583b6f53b7547e958c2b386849fcc2c))
