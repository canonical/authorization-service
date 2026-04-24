# Changelog

## [0.2.0](https://github.com/canonical/authorization-service/compare/v0.1.0...v0.2.0) (2026-04-24)


### Features

* add `authz` command and `write-model` subcommand + dependecies added ([58152a8](https://github.com/canonical/authorization-service/commit/58152a81ebeb589223e2b656c03890de0c11af15))
* add ADRs ([e3aa511](https://github.com/canonical/authorization-service/commit/e3aa511792106c2857a0a31afbf15f3ea35b6415))
* add dev-setup for local development and istio mesh configuration for external authz ([b5bc4d9](https://github.com/canonical/authorization-service/commit/b5bc4d9aba372b69c8368d9d471e6de6a90ab9ce))
* add empty goose migrations ([f38c922](https://github.com/canonical/authorization-service/commit/f38c922ed91661d09f9a31832d3c9cf0d5fd7265))
* add envoy v3 interface for Istio external authorization ([56cfdad](https://github.com/canonical/authorization-service/commit/56cfdadffdada2ab3ec3b600494f4ba832e934f4))
* add exchangeSession invocation + tests ([e3b2346](https://github.com/canonical/authorization-service/commit/e3b234615828180eeeb936d4b6dc6a1da7fd67d6))
* add infrastructure for local deployment ([7115e0e](https://github.com/canonical/authorization-service/commit/7115e0e5bdc4e4264b462d86e3800cc09a4573b6))
* add initial migration for external authz implementation ([2da3046](https://github.com/canonical/authorization-service/commit/2da30469780b9a4931d56f3bc19e2af8bf256bc4))
* add JWK set URL config for External Authz Service ([a78a7f1](https://github.com/canonical/authorization-service/commit/a78a7f10c79cc2c81f3e719a8540ec637154ce3f))
* add migrate CLI + test ([474faeb](https://github.com/canonical/authorization-service/commit/474faebe5a623c5406997c0e4cd8c973e039189d))
* add new targets for sts client generation ([a1dc9d8](https://github.com/canonical/authorization-service/commit/a1dc9d8f83ba3da73e43f4660c7e2b65f27b5940))
* add OpenFGA client + new Resolver related interfaces ([ee134e5](https://github.com/canonical/authorization-service/commit/ee134e5d0a87a2fe9ec85ffd9d68693add81437a))
* add possibility of eager check for gRPC STS connection ([66a2e15](https://github.com/canonical/authorization-service/commit/66a2e15ed6a4442fec6ba637918a051fa9c2244d))
* add postgres DB client + noop + test ([55e1649](https://github.com/canonical/authorization-service/commit/55e164938bf5db2c5ba9a3698f8a08423030b67e))
* add Postgres integration in config and helper ([51f1601](https://github.com/canonical/authorization-service/commit/51f16010a58d033a684523e9e8ec8dab2c41ac51))
* add profiles to skaffold development + add local image tagging and pushing + add reference to dev-setup docs in DEVELOPMENT.md ([d66c877](https://github.com/canonical/authorization-service/commit/d66c87795d523cb90f41e2fedc8d19eb811f4551))
* add resolver related objects and rule repository ([1795ae5](https://github.com/canonical/authorization-service/commit/1795ae5429d4ed01cd3da82a1382496ed6ce0c67))
* add sts wrapper client ([440f92a](https://github.com/canonical/authorization-service/commit/440f92ae28a95949911d2aedb7284bb4dd750d00))
* add support for multiple capture group identifiers ([fa766b0](https://github.com/canonical/authorization-service/commit/fa766b0d13c4dab73fbba0c12829a7f979e81861))
* choose underlying implementation based on enabled status ([041ce74](https://github.com/canonical/authorization-service/commit/041ce747e294792b38fd85b1f7cca8fb25a3a7dc))
* implement external authz Check with OIDC verification of STS access token + tests ([babbcf8](https://github.com/canonical/authorization-service/commit/babbcf82997a5e984bbc3f7ead08095afd491d73))
* initial project structure and configuration ([d3196cf](https://github.com/canonical/authorization-service/commit/d3196cfc2c96fb6a7edab335114889dad6da28aa))
* specify algos used by STS for it's jwt verifier ([b9ec2a5](https://github.com/canonical/authorization-service/commit/b9ec2a5a55f5607a48f26289af396c4f0347da1f))


### Bug Fixes

* adapt tests ([5727ea6](https://github.com/canonical/authorization-service/commit/5727ea64fa88090dc1545d761d8be2b202b2b9de))
* add check cmd to test grpc behaviour in place of istio ([a46ac63](https://github.com/canonical/authorization-service/commit/a46ac6399f793c826aeec90ef1f8cc090f167c88))
* adjust version cmd ([ed0b746](https://github.com/canonical/authorization-service/commit/ed0b74688df0053a80986efecaa9c1671db06596))
* cve action when no issues are found while scheduled task runs ([48a6f98](https://github.com/canonical/authorization-service/commit/48a6f988537f2dce298ea9614459c03f62cf45ee))
* force header in check response ([ff92dda](https://github.com/canonical/authorization-service/commit/ff92dda4913d77a01abad911aab2270197f9f75a))
* mock generation ([2228498](https://github.com/canonical/authorization-service/commit/22284985a4edfa3571159dd3cdc0b44a4c9b4045))
* rockcraft build for CI ([de94794](https://github.com/canonical/authorization-service/commit/de947941746f8d31113b1e513e701b9772efe5dc))
* test data ([74af697](https://github.com/canonical/authorization-service/commit/74af6971a77294237f808273464507f229dbd8da))
* tests ([b245b19](https://github.com/canonical/authorization-service/commit/b245b19d1bb9b23548fc1ec28eaa9a72029855e5))
* typo ([b1b846f](https://github.com/canonical/authorization-service/commit/b1b846fab93e6e4a014c21d15625fb2927e3ede5))
* use proper context for graceful shutdown ([f187fe4](https://github.com/canonical/authorization-service/commit/f187fe4e53871a82e29bc4134fa0c6733cad04d4))
* wording + replace tabs with spaces ([75c3757](https://github.com/canonical/authorization-service/commit/75c37577f93f5e1ffc462f89d09c1b7f743b0d7c))
* workflows and temporarily prevent TIOBE checks from running until project is registered ([92f79ac](https://github.com/canonical/authorization-service/commit/92f79ace6583b6f53b7547e958c2b386849fcc2c))
