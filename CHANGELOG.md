# Changelog

## [2.0.2](https://github.com/canonical/authorization-service/compare/v2.0.1...v2.0.2) (2026-09-14)


### Bug Fixes

* make `openfga_authorization_model_id` optional ([#80](https://github.com/canonical/authorization-service/issues/80)) ([d734df6](https://github.com/canonical/authorization-service/commit/d734df681e54366c92a480da6ac47e102c908c21))
* make openfga_authorization_model_id optional ([60cb304](https://github.com/canonical/authorization-service/commit/60cb304e8b270de2b6e78bf27752140241e75f2a))

## [2.0.1](https://github.com/canonical/authorization-service/compare/v2.0.0...v2.0.1) (2026-09-02)


### Bug Fixes

* **deps:** update module github.com/getkin/kin-openapi to v0.144.0 [security] ([f6b628e](https://github.com/canonical/authorization-service/commit/f6b628e29c31d60846cceee2c29e801594bbd70d))
* **deps:** update module github.com/getkin/kin-openapi to v0.144.0 [security] ([#48](https://github.com/canonical/authorization-service/issues/48)) ([e8f28ec](https://github.com/canonical/authorization-service/commit/e8f28ec90784148355f2e43455540165cfcff651))
* passing of OpenFGA credentials to HTTPClient for the SDK ([46a9300](https://github.com/canonical/authorization-service/commit/46a930061b30d0b6ac7351120e82222ac3d491f1))

## [2.0.0](https://github.com/canonical/authorization-service/compare/v1.0.0...v2.0.0) (2026-09-01)


### ⚠ BREAKING CHANGES

* conform openfga env var names

### Features

* **authz:** log a single structured decision line per Check() call ([bfc8a4d](https://github.com/canonical/authorization-service/commit/bfc8a4decd665ac8a707f6ccb40d6e89314e36e0))
* **cli:** add persistent --fga-api-key flag to root command ([032e17f](https://github.com/canonical/authorization-service/commit/032e17f25f9ccf9e624f62e42c84f39d09b916ed))
* **config:** correlate SetupLogger output with service identity and traces ([de4f085](https://github.com/canonical/authorization-service/commit/de4f0859eb70674fed5ef470138b52127f4f7c85))
* **logging:** add trace-correlated log handler and capturing test helper ([ea44afe](https://github.com/canonical/authorization-service/commit/ea44afe411713f970fc60944c45c65f088a6231d))
* **logging:** propagate correlation ID through the Kafka ingest and worker pipeline ([07e29e6](https://github.com/canonical/authorization-service/commit/07e29e60ffdcaf1593e229a1a4bfb0a58a2d8bf9))
* **metrics:** add core Prometheus metrics package ([7d46cdf](https://github.com/canonical/authorization-service/commit/7d46cdfb2cbdc36b338ba894758f713cadf9810f))
* **metrics:** expose dedicated /metrics endpoint on each long-running binary ([5a16602](https://github.com/canonical/authorization-service/commit/5a166029bb572782fd8716b8a04d830d9c426111))
* **metrics:** instrument gRPC and REST transport layers ([2bc2b5c](https://github.com/canonical/authorization-service/commit/2bc2b5c4cff38c91e4450de442d3a572bb4fc495))
* **metrics:** instrument repository, authz check, and permissions services ([5736577](https://github.com/canonical/authorization-service/commit/57365777470b4c3bc6949a8436d89c717daaf3d5))
* **metrics:** instrument worker, reaper, and Kafka ingestion pipeline ([8ba8ae5](https://github.com/canonical/authorization-service/commit/8ba8ae566d6ee82709b7d5f639c561c1a299fdc4))
* **server:** correlate gRPC and REST requests with a request ID ([c5b12ce](https://github.com/canonical/authorization-service/commit/c5b12ce3b45c87ad64a5df454424e969766eb1c8))
* **terraform:** integrate Kafka pipeline, split deployments, and sequence bootstrapping ([29ac97e](https://github.com/canonical/authorization-service/commit/29ac97e219d215d0250ea27ddcb0bb96ace161d3))


### Bug Fixes

* add viper default for logging.add_source ([65ec149](https://github.com/canonical/authorization-service/commit/65ec1498ddbc0150941493fcf2338e45a245b24d))
* **cmd:** emit valid JSON fallback log when config fails to load ([317e5c6](https://github.com/canonical/authorization-service/commit/317e5c636fc34a227462f6700357622c71bb4f0d))
* **config:** bind environment variables and CLI flag for OpenFGA API key ([d8309ce](https://github.com/canonical/authorization-service/commit/d8309cee31d5939904a7c5e671a94926757417aa))
* conform openfga env var names ([ac7ca06](https://github.com/canonical/authorization-service/commit/ac7ca06dc72eadbee69e328bb747e17875e959cb))
* **postgres:** drop raw SQL text from query and exec logs ([92f560d](https://github.com/canonical/authorization-service/commit/92f560d5fea48743b47c25d911b67ae0679f3964))
* properly quote db init script ([d2abbf8](https://github.com/canonical/authorization-service/commit/d2abbf8449fe774ae96eb66a1a1deff92dd27450))
* update the default ports ([066f19f](https://github.com/canonical/authorization-service/commit/066f19f1e0383888042fa2f87413035194826a2c))

## [1.0.0](https://github.com/canonical/authorization-service/compare/v0.2.0...v1.0.0) (2026-08-27)


### ⚠ BREAKING CHANGES

* conform openfga env var names

### Features

* add dry-run and embedded ModelFS scanning to seed command ([e00e744](https://github.com/canonical/authorization-service/commit/e00e744764ec21b27305b199b1eed2d5162d7e8b))
* add Username config for valkey/redis connection ([8bd57eb](https://github.com/canonical/authorization-service/commit/8bd57eb6050dc13b98d39bd68592e65fb9b21776))
* **authz:** add dummy federated service and relocate fga.mod ([9362f5d](https://github.com/canonical/authorization-service/commit/9362f5d2e35b97fc95a9a58ff7c48882a466ac01))
* **authz:** enforce tenant validation in external check when multitenancy is enabled ([6521411](https://github.com/canonical/authorization-service/commit/65214117c46e261f2a6f4b32de6e8acb445d2553))
* **authz:** introduce modular OpenFGA core schema and folder structure ([6c76d2d](https://github.com/canonical/authorization-service/commit/6c76d2d065f2366c5082f0f603fef8fd51685513))
* **authz:** log a single structured decision line per Check() call ([bfc8a4d](https://github.com/canonical/authorization-service/commit/bfc8a4decd665ac8a707f6ccb40d6e89314e36e0))
* **authz:** refactor write-model command to compile modular schemas programmatically ([cdac6eb](https://github.com/canonical/authorization-service/commit/cdac6eb1e1391a279212a9eb4bad7929ff6132f5))
* **authz:** support toggling multitenancy with tenant_enabled flag in tenant_match condition ([3fac769](https://github.com/canonical/authorization-service/commit/3fac7692925d0e2804570ba2fe475c025759ecc3))
* **cli:** add seed validate subcommand and decouple from config ([b7df075](https://github.com/canonical/authorization-service/commit/b7df075706f2091d4c55a5870e7dc09d8c769f4d))
* **cli:** introduce top-level reaper command and --no-reaper flag ([8c83840](https://github.com/canonical/authorization-service/commit/8c83840a2822026205b4509249b7fa78d7dd04af))
* **config:** correlate SetupLogger output with service identity and traces ([de4f085](https://github.com/canonical/authorization-service/commit/de4f0859eb70674fed5ef470138b52127f4f7c85))
* **config:** integrate viper and mapstructure for unified configuration loading ([8269949](https://github.com/canonical/authorization-service/commit/82699498e79e98a10a04b35e73d217036bcbf0d0))
* **config:** wire multi-service listener and ensure-topics command ([2c7ddef](https://github.com/canonical/authorization-service/commit/2c7ddefc0dbf19ddad6ed3d5e11ce4cd7d1ad17f))
* **core:** add userset subject parsing, validation, and storage mirroring ([ea26815](https://github.com/canonical/authorization-service/commit/ea268158d73b4bb82b8fe276a69954b7461fcd4b))
* **db:** add user_set_subject_relation to authorization_tuples in migration 0003 ([778717e](https://github.com/canonical/authorization-service/commit/778717e39cc72f5fb19cc1972c933624a766cc77))
* **db:** introduce federated_service table and add index on service_id ([0d205e2](https://github.com/canonical/authorization-service/commit/0d205e261ac249ac6cdb88b8a8c63b463ae3a4ef))
* **deploy:** add Terraform infrastructure configurations for Canonical Kubernetes ([86fd2cd](https://github.com/canonical/authorization-service/commit/86fd2cdd3e42080be9b0080552198a60c84dee2f))
* **docker:** add Kafka broker to development dependencies ([7b06438](https://github.com/canonical/authorization-service/commit/7b064386818d5c6435a2ff671b677ff429eb13b9))
* **fga:** add multitenancy support to worker ingestion and external authz runtime ([987f558](https://github.com/canonical/authorization-service/commit/987f55896f79000e81276f86d1b5c11221b772e8))
* implement CLI command for seeding versioned route rules ([25acd95](https://github.com/canonical/authorization-service/commit/25acd954427bb103e12f4a9850dbf1b883feeac2))
* introduce real message structures for Permission updates and errors ([0657beb](https://github.com/canonical/authorization-service/commit/0657beb98e4d7c8c0301ca51a01ea5f6b635471f))
* introduce real message structures for Permission updates and errors ([#36](https://github.com/canonical/authorization-service/issues/36)) ([31e9f97](https://github.com/canonical/authorization-service/commit/31e9f971ea3a06807f9f3bb63292f327a1854c94))
* **kafka:** add Kafka consumer and publisher integration ([40b2b76](https://github.com/canonical/authorization-service/commit/40b2b765502363d1d2e5598eee573ec141ffe2be))
* **kafka:** consumer-group multi-topic consumption with in-order commits ([c985957](https://github.com/canonical/authorization-service/commit/c985957ada75a585cad103d1425f4a441175c469))
* **listen:** add listener service, config, and listen CLI command ([d457265](https://github.com/canonical/authorization-service/commit/d4572659b3f84650f3bcfd21ad77e23975bd3a40))
* **listen:** durable ingestion service, decoupled from OpenFGA ([babcbac](https://github.com/canonical/authorization-service/commit/babcbacecfb870e9ece83670c8e872df409d33ac))
* **logging:** add trace-correlated log handler and capturing test helper ([ea44afe](https://github.com/canonical/authorization-service/commit/ea44afe411713f970fc60944c45c65f088a6231d))
* **logging:** propagate correlation ID through the Kafka ingest and worker pipeline ([07e29e6](https://github.com/canonical/authorization-service/commit/07e29e60ffdcaf1593e229a1a4bfb0a58a2d8bf9))
* **metrics:** add core Prometheus metrics package ([7d46cdf](https://github.com/canonical/authorization-service/commit/7d46cdfb2cbdc36b338ba894758f713cadf9810f))
* **metrics:** expose dedicated /metrics endpoint on each long-running binary ([5a16602](https://github.com/canonical/authorization-service/commit/5a166029bb572782fd8716b8a04d830d9c426111))
* **metrics:** instrument gRPC and REST transport layers ([2bc2b5c](https://github.com/canonical/authorization-service/commit/2bc2b5c4cff38c91e4450de442d3a572bb4fc495))
* **metrics:** instrument repository, authz check, and permissions services ([5736577](https://github.com/canonical/authorization-service/commit/57365777470b4c3bc6949a8436d89c717daaf3d5))
* **metrics:** instrument worker, reaper, and Kafka ingestion pipeline ([8ba8ae5](https://github.com/canonical/authorization-service/commit/8ba8ae566d6ee82709b7d5f639c561c1a299fdc4))
* **model:** add ServiceSlug and Tenant to RuleWithTuples ([2f2f1fd](https://github.com/canonical/authorization-service/commit/2f2f1fd55cfee039cdabc4d72062afbf042fc999))
* **proto:** add WriteRequest and WriteRequestError messages ([d8da16e](https://github.com/canonical/authorization-service/commit/d8da16e93301cd742e73bc3428cf3e959cb10bfd))
* **proto:** align permission-update envelope with ID057 spec ([181cd62](https://github.com/canonical/authorization-service/commit/181cd62bc418aa7e0c0485bfe3fed5e25daba779))
* **repo:** join federated_service table on candidate rule query and scan service slug and tenant ([d73138f](https://github.com/canonical/authorization-service/commit/d73138f2f8c55d448465d7436336784f72ca6cc9))
* **repository:** add permission_update_work table and repository ([fff1415](https://github.com/canonical/authorization-service/commit/fff141599a5d7a5e92a830ab1951a2401e11bd70))
* **repo:** use transactional postgres advisory lock in ReclaimStale ([c4124aa](https://github.com/canonical/authorization-service/commit/c4124aa1f39aedc7e1c64cbaa8bbccebcecf36c6))
* **rules:** add required revision column to authorization_rule table ([2d475d4](https://github.com/canonical/authorization-service/commit/2d475d47ee5aef461707a96b5703c68788521199))
* **rules:** return matched rule on ResourceMapper Map ([6a101e1](https://github.com/canonical/authorization-service/commit/6a101e15b4a7928aa99aa2d8f4632c8744262d50))
* **server:** correlate gRPC and REST requests with a request ID ([c5b12ce](https://github.com/canonical/authorization-service/commit/c5b12ce3b45c87ad64a5df454424e969766eb1c8))
* **terraform:** integrate Kafka pipeline, split deployments, and sequence bootstrapping ([29ac97e](https://github.com/canonical/authorization-service/commit/29ac97e219d215d0250ea27ddcb0bb96ace161d3))
* **worker:** implement async permission-update worker processing stage ([9e493a8](https://github.com/canonical/authorization-service/commit/9e493a8ea45fb4ad7aca2190b58be966b655fd29))
* **worker:** implement periodic background reaper service ([3bb1ebf](https://github.com/canonical/authorization-service/commit/3bb1ebf2b08b49d497ba291eccf10639d78e7469))
* **worker:** implement retry and failure classification ([8af06bc](https://github.com/canonical/authorization-service/commit/8af06bcba2ea42e9090a8a6d7b37e55e765c6280))
* **worker:** sort claimed batch rows by EventTime for best-effort local ordering ([f8c1079](https://github.com/canonical/authorization-service/commit/f8c1079690436b480e2e2ce62da3c13f2b4e0136))


### Bug Fixes

* add contents:read pemission ([77adc43](https://github.com/canonical/authorization-service/commit/77adc4393a7c6ce8458fe461365b39b87511bc51))
* add missing actions permission for scan job ([9371828](https://github.com/canonical/authorization-service/commit/9371828431b522e82890a2cc618087f41e3c2193))
* add missing actions permission for scan job ([#27](https://github.com/canonical/authorization-service/issues/27)) ([748deab](https://github.com/canonical/authorization-service/commit/748deab6cb3da49cf87e05741ae74b2b2b6bb9d9))
* add viper default for logging.add_source ([65ec149](https://github.com/canonical/authorization-service/commit/65ec1498ddbc0150941493fcf2338e45a245b24d))
* ci publish action ([79f14fd](https://github.com/canonical/authorization-service/commit/79f14fd10adfce37dd09faec282c51f398007f46))
* ci publish action ([#16](https://github.com/canonical/authorization-service/issues/16)) ([b6fc818](https://github.com/canonical/authorization-service/commit/b6fc818a4c68a28c9504dbaf28ce5484456dc2ad))
* **ci:** add packages write permission to ci workflow ([5e568dc](https://github.com/canonical/authorization-service/commit/5e568dc9545ca30930636f8eb7df17c61bfde830))
* **ci:** add packages write permission to ci workflow ([9777350](https://github.com/canonical/authorization-service/commit/9777350f34342ce2d85efb83174cc3e750b0721d))
* **cmd:** emit valid JSON fallback log when config fails to load ([317e5c6](https://github.com/canonical/authorization-service/commit/317e5c636fc34a227462f6700357622c71bb4f0d))
* conform openfga env var names ([ac7ca06](https://github.com/canonical/authorization-service/commit/ac7ca06dc72eadbee69e328bb747e17875e959cb))
* **postgres:** drop raw SQL text from query and exec logs ([92f560d](https://github.com/canonical/authorization-service/commit/92f560d5fea48743b47c25d911b67ae0679f3964))
* properly quote db init script ([d2abbf8](https://github.com/canonical/authorization-service/commit/d2abbf8449fe774ae96eb66a1a1deff92dd27450))
* reorder codeql go setup and generate mocks before autobuild ([2528593](https://github.com/canonical/authorization-service/commit/25285930599c61569e09deea23634af820f8c13b))
* replace Chisel slices with full apt packages in rockcraft.yaml ([33447bf](https://github.com/canonical/authorization-service/commit/33447bf9f413e66278dee714421ed569b89aafef))
* replace Chisel slices with full apt packages in rockcraft.yaml ([#21](https://github.com/canonical/authorization-service/issues/21)) ([c6997c3](https://github.com/canonical/authorization-service/commit/c6997c37228c5a1d355a5158cf9d59c203b387c8))
* scope down additional permission for gh-publish job ([0d2fbed](https://github.com/canonical/authorization-service/commit/0d2fbed14baedbf01396594a56d36f3fcc6aab79))
* update the default ports ([066f19f](https://github.com/canonical/authorization-service/commit/066f19f1e0383888042fa2f87413035194826a2c))

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
