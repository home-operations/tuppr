# Changelog

## [0.1.29](https://github.com/home-operations/tuppr/compare/0.1.28...0.1.29) (2026-05-13)


### Bug Fixes

* **upgrade:** pin talosctl endpoints to IPs to survive CoreDNS drain ([eecc2e1](https://github.com/home-operations/tuppr/commit/eecc2e1f3f85a568f6e9fd4f90ab7fd4930acf79))


### Miscellaneous Chores

* fix metrics and cleanup dashboards ([3623587](https://github.com/home-operations/tuppr/commit/3623587aefc5825027b01afe23fc3e0889a428ad))

## [0.1.28](https://github.com/home-operations/tuppr/compare/0.1.27...0.1.28) (2026-05-13)


### Features

* **mise:** update tool aqua:operator-framework/operator-registry (1.67.0 → 1.68.0) ([756a584](https://github.com/home-operations/tuppr/commit/756a584ee3cd59a97dcac202f8f69e103cb62552))


### Bug Fixes

* **deps:** update kubernetes monorepo (v0.36.0 → v0.36.1) ([#271](https://github.com/home-operations/tuppr/issues/271)) ([329ccd4](https://github.com/home-operations/tuppr/commit/329ccd4da3b9fa95bd212291007f7db33a572390))
* **deps:** update module github.com/siderolabs/talos/pkg/machinery (v1.13.0 → v1.13.2) ([#266](https://github.com/home-operations/tuppr/issues/266)) ([6691a96](https://github.com/home-operations/tuppr/commit/6691a96abb0a2b341ca7129fe787e26966689499))
* **deps:** update module sigs.k8s.io/controller-runtime (v0.24.0 → v0.24.1) ([#268](https://github.com/home-operations/tuppr/issues/268)) ([dfc5a96](https://github.com/home-operations/tuppr/commit/dfc5a96e042e3a54f572e90b798f3c5d85cd39dd))
* **kubernetesupgrade:** upgrade lagging worker that joins after completion ([841a9cf](https://github.com/home-operations/tuppr/commit/841a9cf1d60e9b72e869e5ec74cc07f842b028b8))
* make Failed terminal and cap completion-cycle thrash ([de8062e](https://github.com/home-operations/tuppr/commit/de8062e0964388ba98f0ad09bf9c7f296101ec27))
* **mise:** update tool setup-envtest (0.24.0 → 0.24.1) ([4dab561](https://github.com/home-operations/tuppr/commit/4dab5612df9b773739ec63d44cec4252e85f33fe))

## [0.1.27](https://github.com/home-operations/tuppr/compare/0.1.26...0.1.27) (2026-05-12)


### Features

* **deps:** update module github.com/cosi-project/runtime (v1.14.1 → v1.15.0) ([#261](https://github.com/home-operations/tuppr/issues/261)) ([519b1a6](https://github.com/home-operations/tuppr/commit/519b1a69c9e2eec02a3f7c5a9d9e41d9d5078fef))
* report upgrade progress via Conditions and stop phase flicker ([e40fbe1](https://github.com/home-operations/tuppr/commit/e40fbe16097af690d9fa8b42fcc965cd55176b70))


### Bug Fixes

* **deps:** update module github.com/cosi-project/runtime (v1.15.0 → v1.15.1) ([#265](https://github.com/home-operations/tuppr/issues/265)) ([ee084de](https://github.com/home-operations/tuppr/commit/ee084dec315fa7932bea02e200487172507d5264))

## [0.1.26](https://github.com/home-operations/tuppr/compare/0.1.25...0.1.26) (2026-05-08)


### Bug Fixes

* fix kubernetes endpoint selection ([418c244](https://github.com/home-operations/tuppr/commit/418c24423a10818f76549cea74e8e91de8addbe4))

## [0.1.25](https://github.com/home-operations/tuppr/compare/0.1.24...0.1.25) (2026-05-08)


### Bug Fixes

* **ci:** release helm job needs controller-gen from mise ([#260](https://github.com/home-operations/tuppr/issues/260)) ([678907a](https://github.com/home-operations/tuppr/commit/678907ac2cfbdd1e0d29b8173b65d771a89cb390))
* **mise:** update tool go (1.26.2 → 1.26.3) ([edf5088](https://github.com/home-operations/tuppr/commit/edf50882b282218c689097f81a1a48eab89a8541))
* remove capabilities gate for argocd ([f46bda4](https://github.com/home-operations/tuppr/commit/f46bda47b84c1fbac0de40b4b90c697234c980e5))


### Miscellaneous Chores

* **deps:** update k8s.io/utils digest (28399d8 → ff6756f) ([#259](https://github.com/home-operations/tuppr/issues/259)) ([7e6ff72](https://github.com/home-operations/tuppr/commit/7e6ff72c779f1e411e0e6af37d5608b5055367b0))

## [0.1.24](https://github.com/home-operations/tuppr/compare/0.1.23...0.1.24) (2026-05-07)


### ⚠ BREAKING CHANGES

* **github-action:** Update action googleapis/release-please-action (v4.4.1 → v5) ([#208](https://github.com/home-operations/tuppr/issues/208))
* **github-action:** Update action azure/setup-helm (v4.3.1 → v5.0.0) ([#191](https://github.com/home-operations/tuppr/issues/191))
* **github-action:** Update action actions/create-github-app-token (v2.2.2 → v3.0.0) ([#184](https://github.com/home-operations/tuppr/issues/184))
* **github-action:** Update action jdx/mise-action (v3.6.3 → v4.0.0) ([#183](https://github.com/home-operations/tuppr/issues/183))
* **github-action:** Update action docker/build-push-action (v6.19.2 → v7.0.0) ([#170](https://github.com/home-operations/tuppr/issues/170))
* **github-action:** Update action docker/metadata-action (v5.10.0 → v6.0.0) ([#169](https://github.com/home-operations/tuppr/issues/169))
* **github-action:** Update action docker/setup-buildx-action (v3.12.0 → v4.0.0) ([#166](https://github.com/home-operations/tuppr/issues/166))
* **github-action:** Update action docker/login-action (v3.7.0 → v4.0.0) ([#161](https://github.com/home-operations/tuppr/issues/161))
* **github-action:** Update GitHub Artifact Actions (major) ([#136](https://github.com/home-operations/tuppr/issues/136))
* **github-action:** Update GitHub Artifact Actions (major) ([#49](https://github.com/home-operations/tuppr/issues/49))
* **github-action:** Update action codex-/return-dispatch (v2.1.0 → v3.0.0) ([#44](https://github.com/home-operations/tuppr/issues/44))
* **github-action:** Update action actions/checkout (v5.0.1 → v6.0.0) ([#42](https://github.com/home-operations/tuppr/issues/42))
* **github-action:** Update action golangci/golangci-lint-action (v8.0.0 → v9.0.0) ([#38](https://github.com/home-operations/tuppr/issues/38))
* **github-action:** Update GitHub Artifact Actions (major) ([#30](https://github.com/home-operations/tuppr/issues/30))

### Features

* add annotation to force reset failed upgrade ([1f74d81](https://github.com/home-operations/tuppr/commit/1f74d81104e5626a452126f68e837e449cd29430))
* add basic contributing/issues/PR template ([954866e](https://github.com/home-operations/tuppr/commit/954866e6d2eff7c4ab5ca0803798526860fc21e0))
* add configurable talos upgrade timeout ([#61](https://github.com/home-operations/tuppr/issues/61)) ([21d98a2](https://github.com/home-operations/tuppr/commit/21d98a2eb455c8256a58ad104b29f54a5cd38d88))
* add grafanadashboard ([6a7aa77](https://github.com/home-operations/tuppr/commit/6a7aa77e56cf019412c7c6e765ab84a86def88ac))
* add maintenance window support ([#91](https://github.com/home-operations/tuppr/issues/91)) ([85484e9](https://github.com/home-operations/tuppr/commit/85484e9ff6d8aa2a5944593a98df7b9b78c0e537))
* add node labels during upgrades ([#110](https://github.com/home-operations/tuppr/issues/110)) ([f33dad8](https://github.com/home-operations/tuppr/commit/f33dad8b38a79e642427bcc80198a039cde77a3a))
* Add node selector for talosupgrade ([#103](https://github.com/home-operations/tuppr/issues/103)) ([d2b9a10](https://github.com/home-operations/tuppr/commit/d2b9a102f569b8b387479cfe10c1e11187c33dfd))
* add node watching for kubernetesupgrade ([b094963](https://github.com/home-operations/tuppr/commit/b094963fbc8be388be31822ab025c25560155891))
* add placementPreset to upgradePolicy ([707d90c](https://github.com/home-operations/tuppr/commit/707d90c6f92869602326db028608d90d189384b4))
* add stage support ([#48](https://github.com/home-operations/tuppr/issues/48)) ([e27e3f8](https://github.com/home-operations/tuppr/commit/e27e3f8ed5e2f563aa63ee0a536a680f48211c4d))
* add suspend annotation support ([70f1cdc](https://github.com/home-operations/tuppr/commit/70f1cdc826f690e93a9c5c7ab03aa6d4fe89ea46))
* add the build version ([5af75c8](https://github.com/home-operations/tuppr/commit/5af75c854c8e7c18164fde2c104773e70f53995e))
* Allow overriding talos schematic and version through node annotations ([#102](https://github.com/home-operations/tuppr/issues/102)) ([3c97b55](https://github.com/home-operations/tuppr/commit/3c97b55d0db2712f226273163a022a0d02ebbba9))
* block upgrades when talos version or schematic happen outside cr ([a202989](https://github.com/home-operations/tuppr/commit/a2029898d5fea9e234e42fe7b5ae63b2b5aa571d))
* check for drift in talosUpgrade or kubernetesUpgrade ([a74b856](https://github.com/home-operations/tuppr/commit/a74b856e36740df21f1e0480b5b984795499c950))
* Check image availability before creating a new job ([#100](https://github.com/home-operations/tuppr/issues/100)) ([fe7d85b](https://github.com/home-operations/tuppr/commit/fe7d85b4ef22092b3efe47a08dc839c5d363f6e4))
* clean up completed jobs after successful node upgrade ([689afa3](https://github.com/home-operations/tuppr/commit/689afa3d84100a92a2cecd7cb9e4cd3ae19719a9))
* configure drain behavior ([#112](https://github.com/home-operations/tuppr/issues/112)) ([d742f2c](https://github.com/home-operations/tuppr/commit/d742f2ceecabeae2b308d102c7763d30ac49073a))
* **container:** update image golang (1.24 → 1.25) ([#3](https://github.com/home-operations/tuppr/issues/3)) ([bd77005](https://github.com/home-operations/tuppr/commit/bd77005fc58350bdcff1b99cc551fc53fd011f03))
* **container:** update image golang (1.25 → 1.26) ([#84](https://github.com/home-operations/tuppr/issues/84)) ([9e740cf](https://github.com/home-operations/tuppr/commit/9e740cf74ae0c7f64864d2404db5decc85adf100))
* continue to evaluate healthchecks until all pass ([956e066](https://github.com/home-operations/tuppr/commit/956e066401e5b6a1c4f7d52fa005382e63b95976))
* **dashboards:** add hooks to dashboards ([53d3df2](https://github.com/home-operations/tuppr/commit/53d3df270a5a6407037b3560e8a2f3d230088bd0))
* **deps:** update kubernetes packages (v0.34.3 → v0.35.0) ([#53](https://github.com/home-operations/tuppr/issues/53)) ([0f99b0e](https://github.com/home-operations/tuppr/commit/0f99b0e71ef4c6394b1b7926986c91c67715d0aa))
* **deps:** update module github.com/cosi-project/runtime (v1.10.7 → v1.11.0) ([#15](https://github.com/home-operations/tuppr/issues/15)) ([9715b87](https://github.com/home-operations/tuppr/commit/9715b87d12548e2a07605f93b2ce3db66d2ffb60))
* **deps:** update module github.com/cosi-project/runtime (v1.11.0 → v1.12.0) ([#34](https://github.com/home-operations/tuppr/issues/34)) ([fc96fec](https://github.com/home-operations/tuppr/commit/fc96fecffa7b0e7686950651dc00842a8b68a9da))
* **deps:** update module github.com/cosi-project/runtime (v1.12.0 → v1.13.0) ([#43](https://github.com/home-operations/tuppr/issues/43)) ([1b05ecb](https://github.com/home-operations/tuppr/commit/1b05ecbf321fe584b1ad13e25874a150125113e7))
* **deps:** update module github.com/cosi-project/runtime (v1.13.0 → v1.14.0) ([#86](https://github.com/home-operations/tuppr/issues/86)) ([fa944c2](https://github.com/home-operations/tuppr/commit/fa944c2beb2bb2947bdebf1b10f8fcd8a7fad725))
* **deps:** update module github.com/google/cel-go (v0.26.1 → v0.27.0) ([#81](https://github.com/home-operations/tuppr/issues/81)) ([60ce746](https://github.com/home-operations/tuppr/commit/60ce746e074707e1f8299b8593ec94231b8b0d1b))
* **deps:** update module github.com/google/cel-go (v0.27.0 → v0.28.0) ([#199](https://github.com/home-operations/tuppr/issues/199)) ([1a6e8ad](https://github.com/home-operations/tuppr/commit/1a6e8adb38e21e68eef6225c7405e18c57aff6c1))
* **deps:** update module github.com/google/go-containerregistry (v0.20.7 → v0.21.0) ([#127](https://github.com/home-operations/tuppr/issues/127)) ([277c9df](https://github.com/home-operations/tuppr/commit/277c9dfa8555c180514227cc52815a242c02c02f))
* **deps:** update module github.com/netresearch/go-cron (v0.11.0 → v0.12.0) ([#123](https://github.com/home-operations/tuppr/issues/123)) ([6e972a3](https://github.com/home-operations/tuppr/commit/6e972a31b1bd4ec86fabb0a9c8cbf71427c7706a))
* **deps:** update module github.com/netresearch/go-cron (v0.12.0 → v0.13.0) ([#132](https://github.com/home-operations/tuppr/issues/132)) ([4ef4b53](https://github.com/home-operations/tuppr/commit/4ef4b53c30a80455bc327634d67465f43768abb2))
* **deps:** update module github.com/netresearch/go-cron (v0.13.4 → v0.14.0) ([#205](https://github.com/home-operations/tuppr/issues/205)) ([2076109](https://github.com/home-operations/tuppr/commit/2076109f39aa74a6bbabba13c195db7dfe9c01a6))
* **deps:** update module github.com/onsi/ginkgo/v2 (v2.25.3 → v2.26.0) ([#20](https://github.com/home-operations/tuppr/issues/20)) ([e33a4e1](https://github.com/home-operations/tuppr/commit/e33a4e107f3e214bf3f5fe86773c8cd8de1884ae))
* **deps:** update module github.com/onsi/ginkgo/v2 (v2.26.0 → v2.27.1) ([#29](https://github.com/home-operations/tuppr/issues/29)) ([723bbe8](https://github.com/home-operations/tuppr/commit/723bbe8e4606b426a68bcf2f8b0e809ea8917ded))
* **deps:** update module github.com/onsi/gomega (v1.36.1 → v1.38.2) ([#6](https://github.com/home-operations/tuppr/issues/6)) ([7382abf](https://github.com/home-operations/tuppr/commit/7382abfaf2d75e52568f9f63ce229e3e1e93c89d))
* **deps:** update module github.com/onsi/gomega (v1.38.3 → v1.39.0) ([#70](https://github.com/home-operations/tuppr/issues/70)) ([17ccf2f](https://github.com/home-operations/tuppr/commit/17ccf2f8a246e73938168bdd54a66ca87c712467))
* **deps:** update module github.com/open-policy-agent/cert-controller (v0.15.0 → v0.16.0) ([#181](https://github.com/home-operations/tuppr/issues/181)) ([2ddddd7](https://github.com/home-operations/tuppr/commit/2ddddd7141bae1cd2459be1710a8f01e788838a3))
* **deps:** update module github.com/prometheus/client_golang (v1.22.0 → v1.23.2) ([#24](https://github.com/home-operations/tuppr/issues/24)) ([3f92913](https://github.com/home-operations/tuppr/commit/3f92913845b7b7f512b491220dfa6fc19923fd7c))
* **deps:** update module github.com/siderolabs/talos/pkg/machinery (v1.11.6 → v1.12.0) ([#58](https://github.com/home-operations/tuppr/issues/58)) ([aedf0f1](https://github.com/home-operations/tuppr/commit/aedf0f15ba6c9ab8e7cb99a5388ca73c2b8307de))
* **deps:** update module github.com/siderolabs/talos/pkg/machinery (v1.12.7 → v1.13.0) ([#221](https://github.com/home-operations/tuppr/issues/221)) ([c5ed040](https://github.com/home-operations/tuppr/commit/c5ed040744fcb7bbf257852a11124b4ca090a096))
* **deps:** update module google.golang.org/grpc (v1.78.0 → v1.79.1) ([#124](https://github.com/home-operations/tuppr/issues/124)) ([d763f93](https://github.com/home-operations/tuppr/commit/d763f933aa7fe3a8a4eea1f1954fa098b9aef4ce))
* **deps:** update module google.golang.org/grpc (v1.79.3 → v1.80.0) ([#193](https://github.com/home-operations/tuppr/issues/193)) ([53c6c11](https://github.com/home-operations/tuppr/commit/53c6c118c7ccc7c8d7f7399769da39c7e7111926))
* **deps:** update module google.golang.org/grpc (v1.80.0 → v1.81.0) ([#236](https://github.com/home-operations/tuppr/issues/236)) ([fe5a77a](https://github.com/home-operations/tuppr/commit/fe5a77a2ea9f8294f375070a3778fcca140d88ad))
* **deps:** update module sigs.k8s.io/controller-runtime (v0.21.0 → v0.22.1) ([c0dfb7a](https://github.com/home-operations/tuppr/commit/c0dfb7a408d9352345003b85b28bd921aa1a72e9))
* **deps:** update module sigs.k8s.io/controller-runtime (v0.22.4 → v0.23.1) ([#76](https://github.com/home-operations/tuppr/issues/76)) ([98100d9](https://github.com/home-operations/tuppr/commit/98100d91ac7154b068f1d0f69ec18b36b83652d9))
* **deps:** update module sigs.k8s.io/controller-runtime (v0.23.3 → v0.24.0) ([#231](https://github.com/home-operations/tuppr/issues/231)) ([e20eed8](https://github.com/home-operations/tuppr/commit/e20eed85f53126a4c2f818f0dc3dddec209ecba6))
* e2e tests on hetzner cloud ([#125](https://github.com/home-operations/tuppr/issues/125)) ([93ba48f](https://github.com/home-operations/tuppr/commit/93ba48fcbd2377981bbf731ebfdaaffa3c4646e8))
* first pass at support prometheus metrics ([2088dad](https://github.com/home-operations/tuppr/commit/2088dad470694d872a6d29274490a0fd31a5c27e))
* gha workflow for Hetzner e2e tests ([#126](https://github.com/home-operations/tuppr/issues/126)) ([47d25e5](https://github.com/home-operations/tuppr/commit/47d25e5e2d19029ff4c536e25d6d7c14842733b4))
* **helm:** always create talos sa, pass talosconfig as volume to the controller, update rbac ([db30dc6](https://github.com/home-operations/tuppr/commit/db30dc6512dcaaf6bdcebedff12a894fbf6133bc))
* implement log level and update existing log messages ([#121](https://github.com/home-operations/tuppr/issues/121)) ([db2ed49](https://github.com/home-operations/tuppr/commit/db2ed49042c8bcd59a3390b3e871b535feebccd3))
* improve metrics ([9c729e3](https://github.com/home-operations/tuppr/commit/9c729e3087eb4dc23f9591de7b27e9de1e666967))
* improve metrics and grafanadashboard ([8138d9d](https://github.com/home-operations/tuppr/commit/8138d9d9aa0bf68c22895d9398fc6f2ae725f10d))
* improve talos upgrade alerting rules ([f9ff3a0](https://github.com/home-operations/tuppr/commit/f9ff3a014b9cc64f2ed0dcece757d62b194cbdb9))
* initial support for KubernetesUpgrade ([a8718ce](https://github.com/home-operations/tuppr/commit/a8718ce14b48251bcc0efa4da8abcaa85bf6488f))
* **internal,controller,kubernetesupgrade,talosupgradae:** set job la… ([#62](https://github.com/home-operations/tuppr/issues/62)) ([121ef08](https://github.com/home-operations/tuppr/commit/121ef082e8b7ce90f956e256b7d4a35ada24bf16))
* **kubernetesupgrade:** allow private registry for component images via spec.kubernetes.imageRepository ([824b5f8](https://github.com/home-operations/tuppr/commit/824b5f8b586a3cb70845c5f74d23076fcfb3cfa6))
* **kubernetesupgrade:** removes superflous version ([#250](https://github.com/home-operations/tuppr/issues/250)) ([15a3cae](https://github.com/home-operations/tuppr/commit/15a3cae41849ef5bef27e58cff22a77ce1671e7d))
* **mise:** update tool aqua:operator-framework/operator-registry (1.55.0 → 1.67.0) ([4a4d0f7](https://github.com/home-operations/tuppr/commit/4a4d0f79cd239cf6d356259a6e5ab02fdff3f0e0))
* **mise:** update tool go (1.25.7 → 1.26.0) ([5c137e6](https://github.com/home-operations/tuppr/commit/5c137e6d2687d4fb0c8240f3ea2e5519d9436cad))
* **mise:** update tool kube-controller-tools (v0.18.0 → v0.21.0) ([840804e](https://github.com/home-operations/tuppr/commit/840804ee2934d5eb6b948d34b366fa7509b79512))
* **mise:** update tool kustomize (5.6.0 → 5.8.1) ([bde43a4](https://github.com/home-operations/tuppr/commit/bde43a4ea752507c37a721a275265e81b2b3c259))
* **mise:** update tool operator-sdk (1.41.1 → 1.42.2) ([e9ac13c](https://github.com/home-operations/tuppr/commit/e9ac13c3b44562034e0b8ce1f53c1ffad8645968))
* **monitoring:** add prometheus rule to helm charts to alert failed upgrade ([#163](https://github.com/home-operations/tuppr/issues/163)) ([c9d0a69](https://github.com/home-operations/tuppr/commit/c9d0a69da151de2457657bf7ca6ec8c5b53d8683))
* record update history ([16d064b](https://github.com/home-operations/tuppr/commit/16d064b24da780b9c05a8d46009bc772c623f87f))
* **release:** use release-please ([#143](https://github.com/home-operations/tuppr/issues/143)) ([cf902cb](https://github.com/home-operations/tuppr/commit/cf902cbecadb7efc5ed820b8fb8cef5e7db40d9a))
* run healthchecks concurrently ([27c889b](https://github.com/home-operations/tuppr/commit/27c889b8896664268a00070fcf1f107a510f6764))
* **talos:** sync machine.install.image in stored config after upgrade ([b3eef0a](https://github.com/home-operations/tuppr/commit/b3eef0a9d9441276c5fa82f31bbff49c327e8d7d))
* **talosupgrade:** add parallelism support for concurrent node upgrades ([#201](https://github.com/home-operations/tuppr/issues/201)) ([c1c87ff](https://github.com/home-operations/tuppr/commit/c1c87ffa60f548dd2b2f514a6fb3a979749b5da1))
* **talosupgrade:** allow per-node factory URL override via tuppr.home-operations.com/factory-url annotation ([a150806](https://github.com/home-operations/tuppr/commit/a1508063e9454a6aadc1d93c44a89e3e3ae9f3c0))
* **talosupgrade:** auto-detect schematic from ([be13b9a](https://github.com/home-operations/tuppr/commit/be13b9af1b9fe4e1b5b87efbde44bb3bfa61e749))
* **talosupgrade:** pre/post-upgrade hooks via spec.hooks ([13a4364](https://github.com/home-operations/tuppr/commit/13a4364a919225275f3c99967c8364118fd9a372))
* **talosupgrade:** preserve any registry's install image across upgrades ([05b5fd7](https://github.com/home-operations/tuppr/commit/05b5fd70f0a995c9bb413f9210c16e869afed461))
* try to prevent talos and kube upgrades running at the same time ([6c9bb34](https://github.com/home-operations/tuppr/commit/6c9bb3411ea5abea5d6079dd99ec353a6c3e3044))
* use self-signed cert to remove cert-manager deps ([#92](https://github.com/home-operations/tuppr/issues/92)) ([386b86a](https://github.com/home-operations/tuppr/commit/386b86a4e553f9f454f06c1071939394b19fa4c0))
* watch nodes instead ([003e8d8](https://github.com/home-operations/tuppr/commit/003e8d818e8fab2975d61bd6442b95f004e625af))


### Bug Fixes

* (hopefully) stop e2e test dns flakes ([#144](https://github.com/home-operations/tuppr/issues/144)) ([5c69afa](https://github.com/home-operations/tuppr/commit/5c69afa7694237c26a50db5eac690f7d76906ada))
* add healthchecking phase ([#117](https://github.com/home-operations/tuppr/issues/117)) ([05b89d1](https://github.com/home-operations/tuppr/commit/05b89d165e5b764d9d19913a182306ca069548fa))
* add missing controller-gen ([11f982d](https://github.com/home-operations/tuppr/commit/11f982daa1065bac086e1a4fcf82ec9df86003ba))
* add missing reporter ([ca1a302](https://github.com/home-operations/tuppr/commit/ca1a3024d7838ff53b71a8578780895da45519c1))
* add retry logic to handle certificate regeneration during upgrades ([1d28067](https://github.com/home-operations/tuppr/commit/1d28067283508872dacbce132f4ba964264462cf))
* allow repo without tags but not the other way around ([#97](https://github.com/home-operations/tuppr/issues/97)) ([32199f8](https://github.com/home-operations/tuppr/commit/32199f839db4c112a1c3d2326444e1672333c959))
* assign args with equals for consistency ([1c1cbef](https://github.com/home-operations/tuppr/commit/1c1cbef9f43e3410b2705a229882e21b17ddead0))
* cel not find the status field ([f857d49](https://github.com/home-operations/tuppr/commit/f857d49ba423c82425815fe2a7d0228f06dfd4e4))
* **ci:** fix helm lint and pin version ([6257a83](https://github.com/home-operations/tuppr/commit/6257a83a5f9d5a3298524e2990e99cc37200e263))
* consistency in rbac ([69cb766](https://github.com/home-operations/tuppr/commit/69cb76642eade36ce4b891adb42db4e67adf428a))
* **controller:** reset observedGeneration on job failure to allow retry ([#178](https://github.com/home-operations/tuppr/issues/178)) ([182bb70](https://github.com/home-operations/tuppr/commit/182bb7002cf20fc9ac54ae0488b58efbb96edcaf))
* correct helm template for dashboard ([5e186de](https://github.com/home-operations/tuppr/commit/5e186de8821c91d9c190a8ddbed08791b7b52068))
* correct indent of templates ([e6e1dd0](https://github.com/home-operations/tuppr/commit/e6e1dd014fe33679aef7b1c994847f3bfcb578b9))
* correctly handle cert creation ([#95](https://github.com/home-operations/tuppr/issues/95)) ([38ce45f](https://github.com/home-operations/tuppr/commit/38ce45f68926798b28a2b9a644d11fac04723b2e))
* **crds:** remove duplicated enum for kubernetes upgrade job phase ([#141](https://github.com/home-operations/tuppr/issues/141)) ([43de0e0](https://github.com/home-operations/tuppr/commit/43de0e00c826c8ba299441f90d39b7015fe82db8))
* **crds:** remove duplicated validation policy for job phase ([#140](https://github.com/home-operations/tuppr/issues/140)) ([9549fce](https://github.com/home-operations/tuppr/commit/9549fce6cd3cd6d6251c436a573884d8b8e2cdc7))
* crufty old name ([df9c635](https://github.com/home-operations/tuppr/commit/df9c635d64cb0520e8111f456fe9348ed638d505))
* delete failed jobs and record out-of-band upgraded nodes ([b181950](https://github.com/home-operations/tuppr/commit/b181950083ab4514b3e1e8735280a72aaacb222c))
* **deps:** update kubernetes monorepo (v0.35.3 → v0.35.4) ([#203](https://github.com/home-operations/tuppr/issues/203)) ([e7b6e76](https://github.com/home-operations/tuppr/commit/e7b6e765fef6746a19fbf1f4f2c07dce442b69cb))
* **deps:** update kubernetes packages (v0.34.0 → v0.34.1) ([#4](https://github.com/home-operations/tuppr/issues/4)) ([65c29ab](https://github.com/home-operations/tuppr/commit/65c29ab4ae0bacf61139097fcec5a111f666cfa2))
* **deps:** update kubernetes packages (v0.34.1 → v0.34.2) ([#41](https://github.com/home-operations/tuppr/issues/41)) ([0aacc92](https://github.com/home-operations/tuppr/commit/0aacc922ff03182659126d8c3118b3aa160edcc1))
* **deps:** update kubernetes packages (v0.34.2 → v0.34.3) ([#47](https://github.com/home-operations/tuppr/issues/47)) ([4ee5eb8](https://github.com/home-operations/tuppr/commit/4ee5eb841c15c908ee05c7febd13cf2209eb86b9))
* **deps:** update kubernetes packages (v0.35.0 → v0.35.1) ([#85](https://github.com/home-operations/tuppr/issues/85)) ([dba666f](https://github.com/home-operations/tuppr/commit/dba666f15cb76bf4a255630a7e0333dc9f684329))
* **deps:** update kubernetes packages (v0.35.1 → v0.35.2) ([#137](https://github.com/home-operations/tuppr/issues/137)) ([d7f6d5d](https://github.com/home-operations/tuppr/commit/d7f6d5ddd18fbc41a349529eab788f928609dc48))
* **deps:** update kubernetes packages (v0.35.2 → v0.35.3) ([#187](https://github.com/home-operations/tuppr/issues/187)) ([e8cee02](https://github.com/home-operations/tuppr/commit/e8cee027c3480be2d7196067b4969bb32f539d60))
* **deps:** update module github.com/cosi-project/runtime (v1.14.0 → v1.14.1) ([#192](https://github.com/home-operations/tuppr/issues/192)) ([8932a43](https://github.com/home-operations/tuppr/commit/8932a43dd90e328f4cec353d2b7adcd520b3c3c5))
* **deps:** update module github.com/google/cel-go (v0.26.0 → v0.26.1) ([#11](https://github.com/home-operations/tuppr/issues/11)) ([18c63f7](https://github.com/home-operations/tuppr/commit/18c63f7f4e8138f70732167e9294c54964429972))
* **deps:** update module github.com/google/go-containerregistry (v0.21.0 → v0.21.1) ([#134](https://github.com/home-operations/tuppr/issues/134)) ([4727816](https://github.com/home-operations/tuppr/commit/4727816477c5145c29305d5028fe99dac1be7dd6))
* **deps:** update module github.com/google/go-containerregistry (v0.21.1 → v0.21.2) ([#153](https://github.com/home-operations/tuppr/issues/153)) ([5389555](https://github.com/home-operations/tuppr/commit/5389555d75da479ff08ed8314f8b8b1e72bfd05b))
* **deps:** update module github.com/google/go-containerregistry (v0.21.2 → v0.21.3) ([#185](https://github.com/home-operations/tuppr/issues/185)) ([a89f927](https://github.com/home-operations/tuppr/commit/a89f927dbf372f86f660639f1e2cc95835f05d97))
* **deps:** update module github.com/google/go-containerregistry (v0.21.3 → v0.21.4) ([#198](https://github.com/home-operations/tuppr/issues/198)) ([58d55f7](https://github.com/home-operations/tuppr/commit/58d55f7ac19d07379f0b5899459edefbcfafa050))
* **deps:** update module github.com/google/go-containerregistry (v0.21.4 → v0.21.5) ([#202](https://github.com/home-operations/tuppr/issues/202)) ([411bd35](https://github.com/home-operations/tuppr/commit/411bd35f45f9fc445c246c845c8d1f275595d7b8))
* **deps:** update module github.com/netresearch/go-cron (v0.13.0 → v0.13.1) ([#173](https://github.com/home-operations/tuppr/issues/173)) ([0f4a9ab](https://github.com/home-operations/tuppr/commit/0f4a9ab956a5ba62a6c9d3ca305f8e6ca00ab28d))
* **deps:** update module github.com/netresearch/go-cron (v0.13.1 → v0.13.4) ([#196](https://github.com/home-operations/tuppr/issues/196)) ([f3cc15b](https://github.com/home-operations/tuppr/commit/f3cc15b243af9aa31b7cd317815e6e14c746defb))
* **deps:** update module github.com/onsi/ginkgo/v2 (v2.25.1 → v2.25.3) ([#5](https://github.com/home-operations/tuppr/issues/5)) ([4a20420](https://github.com/home-operations/tuppr/commit/4a20420d937b45008af950ecc6b997d0ec02464d))
* **deps:** update module github.com/onsi/ginkgo/v2 (v2.27.1 → v2.27.2) ([#31](https://github.com/home-operations/tuppr/issues/31)) ([69064bd](https://github.com/home-operations/tuppr/commit/69064bd9641b50f6559833c5c7559e01a42da1ef))
* **deps:** update module github.com/onsi/ginkgo/v2 (v2.27.2 → v2.27.3) ([#45](https://github.com/home-operations/tuppr/issues/45)) ([c6d9e78](https://github.com/home-operations/tuppr/commit/c6d9e789e220d70356227481eb8770a5121370b1))
* **deps:** update module github.com/onsi/ginkgo/v2 (v2.27.3 → v2.27.4) ([#69](https://github.com/home-operations/tuppr/issues/69)) ([68f4bf3](https://github.com/home-operations/tuppr/commit/68f4bf381269614565e7d7c26adef1ab475a7998))
* **deps:** update module github.com/onsi/ginkgo/v2 (v2.27.4 → v2.27.5) ([#74](https://github.com/home-operations/tuppr/issues/74)) ([d511b0b](https://github.com/home-operations/tuppr/commit/d511b0b030361cbea32ebc072c63ebc99cd56bb8))
* **deps:** update module github.com/onsi/ginkgo/v2 (v2.28.0 → v2.28.1) ([#82](https://github.com/home-operations/tuppr/issues/82)) ([8123542](https://github.com/home-operations/tuppr/commit/812354297650bb6c1733fdec12921d54cb2d7009))
* **deps:** update module github.com/onsi/ginkgo/v2 (v2.28.1 → v2.28.2) ([#217](https://github.com/home-operations/tuppr/issues/217)) ([b73d974](https://github.com/home-operations/tuppr/commit/b73d974800ffc02ac504f1a4a8c5d12a60a5fc3d))
* **deps:** update module github.com/onsi/gomega (v1.38.2 → v1.38.3) ([#46](https://github.com/home-operations/tuppr/issues/46)) ([4fcfeae](https://github.com/home-operations/tuppr/commit/4fcfeae03113d47a2c4e6eebe5c4d0b824f7ab62))
* **deps:** update module github.com/onsi/gomega (v1.39.0 → v1.39.1) ([#80](https://github.com/home-operations/tuppr/issues/80)) ([942bc3a](https://github.com/home-operations/tuppr/commit/942bc3acf61ef77ffd9e1d9893925c73e52d0bd9))
* **deps:** update module github.com/siderolabs/talos/pkg/machinery (v1.11.1 → v1.11.2) ([#19](https://github.com/home-operations/tuppr/issues/19)) ([e8136a0](https://github.com/home-operations/tuppr/commit/e8136a0587435bb5d13abc20c703ef614a87532f))
* **deps:** update module github.com/siderolabs/talos/pkg/machinery (v1.11.2 → v1.11.3) ([#26](https://github.com/home-operations/tuppr/issues/26)) ([e4ed948](https://github.com/home-operations/tuppr/commit/e4ed9483fdfe35c56846afe2093ce241a45e7926))
* **deps:** update module github.com/siderolabs/talos/pkg/machinery (v1.11.3 → v1.11.4) ([#35](https://github.com/home-operations/tuppr/issues/35)) ([6dd249c](https://github.com/home-operations/tuppr/commit/6dd249c94d3eeffa08ced9051a33f3068218432d))
* **deps:** update module github.com/siderolabs/talos/pkg/machinery (v1.11.4 → v1.11.5) ([#36](https://github.com/home-operations/tuppr/issues/36)) ([c3601a9](https://github.com/home-operations/tuppr/commit/c3601a9c6dcc5d386aca2a004de87e38990f0b91))
* **deps:** update module github.com/siderolabs/talos/pkg/machinery (v1.11.5 → v1.11.6) ([#52](https://github.com/home-operations/tuppr/issues/52)) ([dd19198](https://github.com/home-operations/tuppr/commit/dd19198e4ce4d95d9c3e46da7f0e021c4f77bf54))
* **deps:** update module github.com/siderolabs/talos/pkg/machinery (v1.12.0 → v1.12.1) ([#64](https://github.com/home-operations/tuppr/issues/64)) ([ab72217](https://github.com/home-operations/tuppr/commit/ab722179aa0ef70f9eafcf0aba18564483ce8e00))
* **deps:** update module github.com/siderolabs/talos/pkg/machinery (v1.12.1 → v1.12.3) ([#77](https://github.com/home-operations/tuppr/issues/77)) ([62f738d](https://github.com/home-operations/tuppr/commit/62f738d26691420ef3ca2961feaf07aafd711407))
* **deps:** update module github.com/siderolabs/talos/pkg/machinery (v1.12.3 → v1.12.4) ([#87](https://github.com/home-operations/tuppr/issues/87)) ([714f2da](https://github.com/home-operations/tuppr/commit/714f2da45d8f64286501604e53948e4ca8b843ff))
* **deps:** update module github.com/siderolabs/talos/pkg/machinery (v1.12.4 → v1.12.5) ([#174](https://github.com/home-operations/tuppr/issues/174)) ([e693896](https://github.com/home-operations/tuppr/commit/e693896abe47891a6ac4f8ff86d959a35e2524d0))
* **deps:** update module github.com/siderolabs/talos/pkg/machinery (v1.12.5 → v1.12.6) ([#188](https://github.com/home-operations/tuppr/issues/188)) ([60a301c](https://github.com/home-operations/tuppr/commit/60a301c2db25ab6552fc2dae0bd17db24429cf98))
* **deps:** update module github.com/siderolabs/talos/pkg/machinery (v1.12.6 → v1.12.7) ([#212](https://github.com/home-operations/tuppr/issues/212)) ([9656f16](https://github.com/home-operations/tuppr/commit/9656f16680691eda7c0c06a666fb479d71c11046))
* **deps:** update module google.golang.org/grpc (v1.79.1 → v1.79.2) ([#171](https://github.com/home-operations/tuppr/issues/171)) ([35a4af8](https://github.com/home-operations/tuppr/commit/35a4af8611008a57efccbb77bc4bf68defbf7ae1))
* **deps:** update module google.golang.org/grpc (v1.79.2 → v1.79.3) ([#186](https://github.com/home-operations/tuppr/issues/186)) ([21406a0](https://github.com/home-operations/tuppr/commit/21406a03c94b82320fdd2e2062f88bda26c7a15f))
* **deps:** update module sigs.k8s.io/controller-runtime (v0.22.1 → v0.22.2) ([#23](https://github.com/home-operations/tuppr/issues/23)) ([4dcc78b](https://github.com/home-operations/tuppr/commit/4dcc78bbef340cf24c7038cf3df479ec3884fd3d))
* **deps:** update module sigs.k8s.io/controller-runtime (v0.22.2 → v0.22.3) ([#25](https://github.com/home-operations/tuppr/issues/25)) ([879b22b](https://github.com/home-operations/tuppr/commit/879b22b0f5565cb4f654f52d58eb905b42effbd6))
* **deps:** update module sigs.k8s.io/controller-runtime (v0.22.3 → v0.22.4) ([#33](https://github.com/home-operations/tuppr/issues/33)) ([2fdd4d1](https://github.com/home-operations/tuppr/commit/2fdd4d110a52b34e2109d109f24bcb84ea069f19))
* **deps:** update module sigs.k8s.io/controller-runtime (v0.23.1 → v0.23.2) ([#167](https://github.com/home-operations/tuppr/issues/167)) ([395dd7c](https://github.com/home-operations/tuppr/commit/395dd7ca854ab50092a4bdade402f10edb94d6f6))
* **deps:** update module sigs.k8s.io/controller-runtime (v0.23.2 → v0.23.3) ([#168](https://github.com/home-operations/tuppr/issues/168)) ([15a8a60](https://github.com/home-operations/tuppr/commit/15a8a60f13ef1fae7c628020dabe2dba5adf5196))
* disable secure metrics by default ([7a40fa5](https://github.com/home-operations/tuppr/commit/7a40fa5e23eb8e6886b28528fe26699028047af9))
* ensure pod is deleted when job is deleted ([92f2312](https://github.com/home-operations/tuppr/commit/92f23121bcd784f337c37d314569056fedd0dfec))
* error on talosctl version detection failure instead of falling back to latest ([#130](https://github.com/home-operations/tuppr/issues/130)) ([277a836](https://github.com/home-operations/tuppr/commit/277a8360299617ba49d4e4657151e426daa2b9c0))
* fix lint and tests ([249acd9](https://github.com/home-operations/tuppr/commit/249acd9ff5128b1ddffa6ea59c9353e7a29eb04b))
* forgot to save file ([6be4cfc](https://github.com/home-operations/tuppr/commit/6be4cfc35b8ef6fcb581452cb97603b2a6d4b3db))
* getTalosVersion func ([c5a7372](https://github.com/home-operations/tuppr/commit/c5a73720ace16d365268933438756233160f2aa9))
* handle correctly maintenance window when not all node are updated ([#148](https://github.com/home-operations/tuppr/issues/148)) ([0c7a735](https://github.com/home-operations/tuppr/commit/0c7a735d4795eb7e07036b042127a59587ba85ee))
* handle missing cert during startup ([#93](https://github.com/home-operations/tuppr/issues/93)) ([ea6eb3b](https://github.com/home-operations/tuppr/commit/ea6eb3bdedfe9b39feb25216a48c4f79348e2058))
* handling of which talosctl image to use ([53aea04](https://github.com/home-operations/tuppr/commit/53aea049f46c96767e090b2f4a74dc0f7661230f))
* harden e2e-test resources cleanup ([#131](https://github.com/home-operations/tuppr/issues/131)) ([ad04a99](https://github.com/home-operations/tuppr/commit/ad04a999461a980b52c00934a7c26ff16bf64f4e))
* **helm:** add missing brackets in prometheus rule template ([#177](https://github.com/home-operations/tuppr/issues/177)) ([7c56d14](https://github.com/home-operations/tuppr/commit/7c56d14af897cf70bbe92464a814643e550e9c3e))
* improve phases ([#115](https://github.com/home-operations/tuppr/issues/115)) ([234399f](https://github.com/home-operations/tuppr/commit/234399fbfd3165b062ce1ed7d7fe5e53e96c51d2))
* increase affinity weights and decrease resource requests ([ff1eda0](https://github.com/home-operations/tuppr/commit/ff1eda038e3d11454982c88c03a9ca1581db05c7))
* it is placement not placementPreset ([7c6161a](https://github.com/home-operations/tuppr/commit/7c6161ab7de6a2791b5fb77d82dc8b3a1fb42ead))
* **k8s-upgrade:** reprocess partial update ([#109](https://github.com/home-operations/tuppr/issues/109)) ([99cbe9e](https://github.com/home-operations/tuppr/commit/99cbe9ec4dde0fe5c99afbe2a6d2b14e21f1fbc8))
* keep k8s update waiting in case a talos update is pending ([#101](https://github.com/home-operations/tuppr/issues/101)) ([e6214a0](https://github.com/home-operations/tuppr/commit/e6214a012a3893ce47e0ca9af27b20eb046ed590))
* **kubernetesupgrade:** inject hostAliases for controlPlane endpoint hostname ([8d0bb2a](https://github.com/home-operations/tuppr/commit/8d0bb2a7f029099aeaa0af18190f220088885a39))
* **kubeupgrade:** continue to next node after partial job success ([#142](https://github.com/home-operations/tuppr/issues/142)) ([35fbe82](https://github.com/home-operations/tuppr/commit/35fbe82438b8f8cf13662ca2f446398fa94fe887))
* look for active jobs first ([fa8bcfe](https://github.com/home-operations/tuppr/commit/fa8bcfe8b78f723a5dd4a0644b3744a4fcbe11d5))
* **main:** show new version number after successful update ([#219](https://github.com/home-operations/tuppr/issues/219)) ([046989e](https://github.com/home-operations/tuppr/commit/046989e1106abe084fa57908ad03803ee085e130))
* make lint happy ([6d222c0](https://github.com/home-operations/tuppr/commit/6d222c043ee96a3c83334dfef43a9c284b4c9a8d))
* **metrics:** record previously unused job metrics and clean up on deletion ([#160](https://github.com/home-operations/tuppr/issues/160)) ([30a2c88](https://github.com/home-operations/tuppr/commit/30a2c88720f7f15cdcd89d72713c5e79770bd9f4))
* **metrics:** replace numeric phase encoding with state-set gauge pattern ([a2ebdc7](https://github.com/home-operations/tuppr/commit/a2ebdc7ebc30e654f75d71ba40414ff7a553aea3))
* **metrics:** replace numeric phase encoding with state-set gauge pattern ([#172](https://github.com/home-operations/tuppr/issues/172)) ([affd321](https://github.com/home-operations/tuppr/commit/affd3216df8eb3a7aa06e79071876b8332ea50ac))
* **mise:** update tool go (1.26.0 → 1.26.1) ([3136f73](https://github.com/home-operations/tuppr/commit/3136f7335cc860f25978549b4d9d54138ca4de79))
* **mise:** update tool go (1.26.1 → 1.26.2) ([8b7e148](https://github.com/home-operations/tuppr/commit/8b7e148bb2d35ab78ab7d7f17f608bcfbd2e3087))
* **mise:** update tool helm (4.1.3 → 4.1.4) ([508010b](https://github.com/home-operations/tuppr/commit/508010b426694b8b76a2fbfef3d5976ad7a7222e))
* modify resource requests and limits in job ([#40](https://github.com/home-operations/tuppr/issues/40)) ([c23d26f](https://github.com/home-operations/tuppr/commit/c23d26f4bf5559e7b2f1dfd03f054f90df322e5c))
* only reg k8s controller once ([b2a3701](https://github.com/home-operations/tuppr/commit/b2a3701f27e437bbcfd3b83227d245ac34f3fed9))
* oomkills and missing current image ([053fc0c](https://github.com/home-operations/tuppr/commit/053fc0c9e2c6d5acfa43c636d55def5c8d248441))
* **README:** fix talos upgrade troubleshooting command ([#79](https://github.com/home-operations/tuppr/issues/79)) ([740203e](https://github.com/home-operations/tuppr/commit/740203ed4a3a4a30334f9660948f483e72b5a55d))
* rebooting phase would never be activated ([#122](https://github.com/home-operations/tuppr/issues/122)) ([1c1240b](https://github.com/home-operations/tuppr/commit/1c1240b816ee5c5b6917fb34006e1ca8b44a24cd))
* register cache index at startup for drain ([#133](https://github.com/home-operations/tuppr/issues/133)) ([75dc830](https://github.com/home-operations/tuppr/commit/75dc8306c642da6519683a3fa5e552e5fdea90c9))
* **release-please:** always update pr with the latest changes ([1ad0add](https://github.com/home-operations/tuppr/commit/1ad0add6796d165233f4f9c573b07e17d8f203af))
* **release:** exclude v from release tag ([#146](https://github.com/home-operations/tuppr/issues/146)) ([a914a39](https://github.com/home-operations/tuppr/commit/a914a39244f78eebe8f3b38c723bb92e49d3dd8b))
* remove duplicated metrics ([dc8d1ca](https://github.com/home-operations/tuppr/commit/dc8d1caf88d3a49fd172eb628241da6e285a7681))
* Remove init container 'health' from controllers ([#17](https://github.com/home-operations/tuppr/issues/17)) ([172427f](https://github.com/home-operations/tuppr/commit/172427f462e46ded77e8c36b8097e541e73f8343))
* retry with new cert when Talos client certificate expires ([#138](https://github.com/home-operations/tuppr/issues/138)) ([b046f89](https://github.com/home-operations/tuppr/commit/b046f89f7d2d6b88a770ea7436f19ac4c8c47e5f))
* secret permission in job ([a967cd5](https://github.com/home-operations/tuppr/commit/a967cd51fb7d7e77cc1be41416c4db30fec5994e))
* secrets can indeed rotate, add refreshing client ([0dba1f5](https://github.com/home-operations/tuppr/commit/0dba1f58e3d360ea482998cecbdf492f2ab44dae))
* set default tolerations in helm chart ([90a8b07](https://github.com/home-operations/tuppr/commit/90a8b077d8f3121b4ed07ca7beaf8f7520c9f804))
* set TALOSCONFIG as env in the job ([169a8f1](https://github.com/home-operations/tuppr/commit/169a8f137a8c80dadd7dd776c09ff65226acde52))
* set talosconfig env var in job ([#51](https://github.com/home-operations/tuppr/issues/51)) ([f2d7ca7](https://github.com/home-operations/tuppr/commit/f2d7ca77554bfc176805f0908e7ac4ae5945767e))
* single node clusters would fail to upgrade talos ([#99](https://github.com/home-operations/tuppr/issues/99)) ([19dffd8](https://github.com/home-operations/tuppr/commit/19dffd8715cdf31929dc75f5e58932035499a86e))
* sync over rbac changes to the helm chart ([154e734](https://github.com/home-operations/tuppr/commit/154e7344915706c06ea34fd257dd2016b4b869e3))
* talos sa changes ([6f9676e](https://github.com/home-operations/tuppr/commit/6f9676e01716871e57dbaa0a8efe10c5468bcb28))
* **talosupgrade:** fix image patching ([52611f9](https://github.com/home-operations/tuppr/commit/52611f934158b11c81da75f051709854778a7f06))
* **talosupgrade:** preserve factory installer flavor across upgrades ([fd10c26](https://github.com/home-operations/tuppr/commit/fd10c2617d25c937c551291fe5fd5fefd2670829))
* **talosupgrade:** retry on transient errors ([#113](https://github.com/home-operations/tuppr/issues/113)) ([f13cd3e](https://github.com/home-operations/tuppr/commit/f13cd3e557e21fad21391a28bc07d4d90b2adea2))
* tofu flag ordering in e2e cleanup ([e007b08](https://github.com/home-operations/tuppr/commit/e007b08a5dfea7a9fae5638f9e87188cad933194))
* try to account for single node clusters and set PriorityClassName ([6021e42](https://github.com/home-operations/tuppr/commit/6021e425fa828ff1bab653c9877be3412d9b3e0d))
* update talosclient to remove needless logs and simplify checks ([180edda](https://github.com/home-operations/tuppr/commit/180edda912b2be54d313cc115244b065006f96b1))
* update talosconfig secret perms to match others ([fc1cae1](https://github.com/home-operations/tuppr/commit/fc1cae1e36eed7a259bcd6c1ec8fd6d893a4a426))
* update timeouts across the board ([50fd4a6](https://github.com/home-operations/tuppr/commit/50fd4a6831484fef0ca85535d8151b85fb595ffa))
* use github app token to create a new tag to fix ga loop protection ([#151](https://github.com/home-operations/tuppr/issues/151)) ([61936a9](https://github.com/home-operations/tuppr/commit/61936a967c8461568a24a549e041027fb80f0262))
* use mise for ci ([088df70](https://github.com/home-operations/tuppr/commit/088df70926b74d39ca6c9bf17ac7706d41a1f71b))
* use new imager approach for e2e bootstrap ([#190](https://github.com/home-operations/tuppr/issues/190)) ([5f429b6](https://github.com/home-operations/tuppr/commit/5f429b661d321d89fb48552a7cc61c4f6ef0ede4))
* wait for node to be ready before verifying upgrade ([c069ec5](https://github.com/home-operations/tuppr/commit/c069ec5344dfc1198329850d07b99acf031e6aa9))
* weight range is onlt 1-100 ([6219d9a](https://github.com/home-operations/tuppr/commit/6219d9ace9a64460523fb6a0b9ac057089a56ce0))
* workaround for tofu cleanup state lock ([#139](https://github.com/home-operations/tuppr/issues/139)) ([89f8b07](https://github.com/home-operations/tuppr/commit/89f8b07c01655fb903380b64b4482ab6236d0b30))


### Documentation

* remove list of metrics ([fab1f4b](https://github.com/home-operations/tuppr/commit/fab1f4be25d468c37fa692065b9afa458ac2346e))
* simplify the contributors.md ([be7794c](https://github.com/home-operations/tuppr/commit/be7794c493d9b7599dccaf1844deb058b738ba8c))
* update readme ([d2f9d16](https://github.com/home-operations/tuppr/commit/d2f9d16c2b9cdc80d0c7efca9e4ce880d5b7a980))


### Miscellaneous Chores

* add in locking ([a62a770](https://github.com/home-operations/tuppr/commit/a62a7703a5375b582807f68f1d7f9ca3bfcecbd2))
* add in safer job handling ([da1ffe5](https://github.com/home-operations/tuppr/commit/da1ffe561fd691112a5a55ff82b559f9016d936b))
* add kubernetes controller logic ([ec29a97](https://github.com/home-operations/tuppr/commit/ec29a97e3b7150457849d38e288e51c1054a6da1))
* add kubernetes controller logic ([4f87819](https://github.com/home-operations/tuppr/commit/4f87819d4aa957ba79daa9cd513bd1a12c2edd8d))
* add mise config ([4b4f45c](https://github.com/home-operations/tuppr/commit/4b4f45cfa23515e128f6a6349f1933133281e667))
* add TerminatingOrFailed podreplacementstrat ([0fa1f8e](https://github.com/home-operations/tuppr/commit/0fa1f8eb212a6853db8e41be87fd02a6ed3faec7))
* add TerminatingOrFailed podreplacementstrat ([7ab5dfe](https://github.com/home-operations/tuppr/commit/7ab5dfec5aa4e86d2edb7625dedf8db9da6da3cc))
* add workflow_dispatch trigger to release workflow ([f50742e](https://github.com/home-operations/tuppr/commit/f50742e7da04796010b3a09b7c88a7cc60edfde4))
* another commit ([2087188](https://github.com/home-operations/tuppr/commit/2087188c0be081ab6a8abcfbd7e3caa587c76644))
* antiaffinity ([7dab5d8](https://github.com/home-operations/tuppr/commit/7dab5d8e44b43487dc7069e7d35ec59b33d858b9))
* backOffLimit is a thing too claude ([eb5368a](https://github.com/home-operations/tuppr/commit/eb5368a123350e190cfb885b6c81ded3519c3e27))
* change draft configuration to draft-pull-request ([5475a40](https://github.com/home-operations/tuppr/commit/5475a402d0fa7fc75027c38dfc63ef86e2864f44))
* charts dir ([20b57f1](https://github.com/home-operations/tuppr/commit/20b57f1d6982d3a8e66f31a26b667553cf1816d6))
* **charts:** expose priorityClassName ([eca344f](https://github.com/home-operations/tuppr/commit/eca344f8d658d9e585c650344f11a5857dedf8fa))
* **ci:** tidy up github actions ([d7ef6e3](https://github.com/home-operations/tuppr/commit/d7ef6e31d919000f0a1cca419abfd25cc58f7e73))
* clean up helm chart crd folder ([006d2c1](https://github.com/home-operations/tuppr/commit/006d2c13190d0ce09ddd3b1cc731a32caa22ea52))
* controller fixes ([a4f1131](https://github.com/home-operations/tuppr/commit/a4f11310383da9c2a705b15831a32d0a1a332fd5))
* **dashboard:** import the file as json ([d1021ed](https://github.com/home-operations/tuppr/commit/d1021ed971f28167bfae94b3de97a66aec0f7f6f))
* **deps:** update k8s.io/utils digest (0af2bda → bc988d5) ([#21](https://github.com/home-operations/tuppr/issues/21)) ([f8b9433](https://github.com/home-operations/tuppr/commit/f8b94333cb10ff8bd46f4447632f3b5377993d62))
* **deps:** update k8s.io/utils digest (0fe9cd7 → 914a6e7) ([#71](https://github.com/home-operations/tuppr/issues/71)) ([9d9d10c](https://github.com/home-operations/tuppr/commit/9d9d10c772752e10d74685d61f54854c8403fd24))
* **deps:** update k8s.io/utils digest (383b50a → 718f0e5) ([#60](https://github.com/home-operations/tuppr/issues/60)) ([7dc7d30](https://github.com/home-operations/tuppr/commit/7dc7d30b2d44edf12d1e1104ddea949980aa0bfc))
* **deps:** update k8s.io/utils digest (3ea5e8c → 0af2bda) ([9bfe55f](https://github.com/home-operations/tuppr/commit/9bfe55f8c7c1d6b8472f2e97e3a8f0e8aedfa246))
* **deps:** update k8s.io/utils digest (718f0e5 → 0fe9cd7) ([#67](https://github.com/home-operations/tuppr/issues/67)) ([0561a71](https://github.com/home-operations/tuppr/commit/0561a718e05e6d660b189bddce4b289fd3c3bc8d))
* **deps:** update k8s.io/utils digest (914a6e7 → b8788ab) ([#83](https://github.com/home-operations/tuppr/issues/83)) ([6d8cfb7](https://github.com/home-operations/tuppr/commit/6d8cfb707b01f76c94296debe9c62b2e0d482113))
* **deps:** update k8s.io/utils digest (98d557b → 9d40a56) ([#57](https://github.com/home-operations/tuppr/issues/57)) ([1b422b1](https://github.com/home-operations/tuppr/commit/1b422b1a4a6f48a90ee7b5813152316a2c8ab1cd))
* **deps:** update k8s.io/utils digest (9d40a56 → 383b50a) ([#59](https://github.com/home-operations/tuppr/issues/59)) ([8bb68e3](https://github.com/home-operations/tuppr/commit/8bb68e3409d57b14d2cb6e4f4232d0b91c6677bd))
* **deps:** update k8s.io/utils digest (b8788ab → 28399d8) ([#189](https://github.com/home-operations/tuppr/issues/189)) ([2b31588](https://github.com/home-operations/tuppr/commit/2b31588bdb3f22ae8a86131ec76bed3ba1e2bbf1))
* **deps:** update k8s.io/utils digest (bc988d5 → 98d557b) ([#55](https://github.com/home-operations/tuppr/issues/55)) ([8d5067a](https://github.com/home-operations/tuppr/commit/8d5067a1137019850f41c859c8789c9aaceced3d))
* **helm:** support envs, volumes and volume mounts ([#104](https://github.com/home-operations/tuppr/issues/104)) ([7d72425](https://github.com/home-operations/tuppr/commit/7d72425bb66ea085a91834267d437ded064c9d47))
* implement nodeSelectorExprs ([4819f82](https://github.com/home-operations/tuppr/commit/4819f82c6c4383cfd2b825b3b8207eb2d344a349))
* increase number of retries ([00ea6d6](https://github.com/home-operations/tuppr/commit/00ea6d6b461a8ae54dbad9ca0eb0bad13ab52dae))
* initial commit ([4a79c84](https://github.com/home-operations/tuppr/commit/4a79c84b31fe0055ad1105e15faafddca9a82cfb))
* **main:** release 0.0.76 ([#145](https://github.com/home-operations/tuppr/issues/145)) ([87e3a49](https://github.com/home-operations/tuppr/commit/87e3a492ea2aa47b56346eb644a55dadf4015456))
* **main:** release 0.0.77 ([#147](https://github.com/home-operations/tuppr/issues/147)) ([4b0ac8c](https://github.com/home-operations/tuppr/commit/4b0ac8cd7f54759760093febbef636fd7bd7c908))
* **main:** release 0.0.78 ([#149](https://github.com/home-operations/tuppr/issues/149)) ([1522e54](https://github.com/home-operations/tuppr/commit/1522e54859bda51f86764a42153a5f3487fa04be))
* **main:** release 0.0.79 ([#152](https://github.com/home-operations/tuppr/issues/152)) ([2feb097](https://github.com/home-operations/tuppr/commit/2feb097cc8584d9bddb0af9abc9c0602bafb82fa))
* **main:** release 0.0.80 ([#159](https://github.com/home-operations/tuppr/issues/159)) ([b47d689](https://github.com/home-operations/tuppr/commit/b47d689f2657e5cc32da029534fd1dc98f9245ee))
* **main:** release 0.1.0 ([#162](https://github.com/home-operations/tuppr/issues/162)) ([1dfb6eb](https://github.com/home-operations/tuppr/commit/1dfb6eb28b40d52fdc0f7cb5b5a06816513c6401))
* **main:** release 0.1.1 ([#176](https://github.com/home-operations/tuppr/issues/176)) ([6dc1936](https://github.com/home-operations/tuppr/commit/6dc1936b6995d0ab96a9290774a03c1a9db1af55))
* **main:** release 0.1.10 ([#222](https://github.com/home-operations/tuppr/issues/222)) ([bc7d023](https://github.com/home-operations/tuppr/commit/bc7d0230644b1b727fa4e7019f6ec82d72ea9985))
* **main:** release 0.1.11 ([#232](https://github.com/home-operations/tuppr/issues/232)) ([4611092](https://github.com/home-operations/tuppr/commit/46110920ba4e1aec241df2293a94337705813470))
* **main:** release 0.1.12 ([#234](https://github.com/home-operations/tuppr/issues/234)) ([767fb55](https://github.com/home-operations/tuppr/commit/767fb55dcbd816ef5da4c1ef2fdae33cd756b82b))
* **main:** release 0.1.13 ([#239](https://github.com/home-operations/tuppr/issues/239)) ([094c393](https://github.com/home-operations/tuppr/commit/094c393559e6c096a1275d95ce4b4bfd0231f5d9))
* **main:** release 0.1.14 ([#240](https://github.com/home-operations/tuppr/issues/240)) ([38c73af](https://github.com/home-operations/tuppr/commit/38c73af0b9e566a76bd475bfa7ef01af96d8430c))
* **main:** release 0.1.15 ([#242](https://github.com/home-operations/tuppr/issues/242)) ([5674c8a](https://github.com/home-operations/tuppr/commit/5674c8a46dba77b29c8506e0cbad5d7711f9025f))
* **main:** release 0.1.16 ([#243](https://github.com/home-operations/tuppr/issues/243)) ([c201a21](https://github.com/home-operations/tuppr/commit/c201a2109d0c564c82af8216812c95f598587776))
* **main:** release 0.1.17 ([#244](https://github.com/home-operations/tuppr/issues/244)) ([113e0d7](https://github.com/home-operations/tuppr/commit/113e0d786f000de8a34be684c73f5bed1744fec7))
* **main:** release 0.1.18 ([#245](https://github.com/home-operations/tuppr/issues/245)) ([e2298e0](https://github.com/home-operations/tuppr/commit/e2298e073620211adf87a8fc8594c3f48dbc70bf))
* **main:** release 0.1.19 ([#246](https://github.com/home-operations/tuppr/issues/246)) ([208626c](https://github.com/home-operations/tuppr/commit/208626cb79a251b2f4fb001bd40eb4be1d318b17))
* **main:** release 0.1.2 ([#180](https://github.com/home-operations/tuppr/issues/180)) ([bffb1fb](https://github.com/home-operations/tuppr/commit/bffb1fbd80c42273f72ef1b321434b534589ee23))
* **main:** release 0.1.20 ([#247](https://github.com/home-operations/tuppr/issues/247)) ([1b0bfe2](https://github.com/home-operations/tuppr/commit/1b0bfe2f031230051b6e51c91a4e8ce8fe25af55))
* **main:** release 0.1.21 ([#249](https://github.com/home-operations/tuppr/issues/249)) ([c8aedf3](https://github.com/home-operations/tuppr/commit/c8aedf37f3143eee7d55b6bf01a3de746142eca9))
* **main:** release 0.1.22 ([#251](https://github.com/home-operations/tuppr/issues/251)) ([dea7b21](https://github.com/home-operations/tuppr/commit/dea7b212f9792d5351a6172d2978865ed1ae0b0a))
* **main:** release 0.1.23 ([#253](https://github.com/home-operations/tuppr/issues/253)) ([81e6804](https://github.com/home-operations/tuppr/commit/81e6804198891e387a5c2e2483505a6c1da4e549))
* **main:** release 0.1.3 ([#182](https://github.com/home-operations/tuppr/issues/182)) ([1456ad2](https://github.com/home-operations/tuppr/commit/1456ad29c62a3e5bba5bfd0e6606b9a6fe366fc9))
* **main:** release 0.1.4 ([#195](https://github.com/home-operations/tuppr/issues/195)) ([6ccdfc3](https://github.com/home-operations/tuppr/commit/6ccdfc349cadcd6e5a680dac61b000089097b408))
* **main:** release 0.1.5 ([#200](https://github.com/home-operations/tuppr/issues/200)) ([693afbf](https://github.com/home-operations/tuppr/commit/693afbfe312c1c8304d622b38d187b1a55a2d73e))
* **main:** release 0.1.6 ([#204](https://github.com/home-operations/tuppr/issues/204)) ([720c4d2](https://github.com/home-operations/tuppr/commit/720c4d2bbd716fd21cd37cfa1109a2b96d49300e))
* **main:** release 0.1.7 ([#207](https://github.com/home-operations/tuppr/issues/207)) ([5621094](https://github.com/home-operations/tuppr/commit/56210946fdb3dad21c241302abeeb6f41ce65f3c))
* **main:** release 0.1.8 ([#210](https://github.com/home-operations/tuppr/issues/210)) ([81da60d](https://github.com/home-operations/tuppr/commit/81da60d500938dbbb46fc48ea41e6f8ff07ed505))
* **main:** release 0.1.9 ([#218](https://github.com/home-operations/tuppr/issues/218)) ([4f600d7](https://github.com/home-operations/tuppr/commit/4f600d72c1640cad2eba4217e3185af2048074d7))
* make the linter happyv2 ([fcdf126](https://github.com/home-operations/tuppr/commit/fcdf126b24578d27a7cc0c353b59ed521b879f61))
* **metrics:** add empty data at startup ([bc96eb0](https://github.com/home-operations/tuppr/commit/bc96eb0fcd608d2fbaa61fd890cd9ea43fb89398))
* modify E2E workflow triggers for pull requests ([0108dcb](https://github.com/home-operations/tuppr/commit/0108dcbf1a966dc37393e0bc0f8dc33df9823e06))
* more workflows ([143d26c](https://github.com/home-operations/tuppr/commit/143d26cc30d7a2795422dfc2df3e0ef8918c5a31))
* more workflows ([122e4e1](https://github.com/home-operations/tuppr/commit/122e4e1c27097ea32fdb0cd19bdc744eb775dde7))
* move deps to mise ([ecdcc52](https://github.com/home-operations/tuppr/commit/ecdcc52ed78915f5f30344b71cb79a97678e65d0))
* preserve is not a thing anymore ([2101471](https://github.com/home-operations/tuppr/commit/2101471b4bb4cb1c12808dd0534c54bbc5e4b442))
* reduce github action usage by improving starting event ([#116](https://github.com/home-operations/tuppr/issues/116)) ([c5737e4](https://github.com/home-operations/tuppr/commit/c5737e4c03f4a0681c0699466f65b8ec8cb153a0))
* refactor coordinator rules ([#114](https://github.com/home-operations/tuppr/issues/114)) ([fc500dd](https://github.com/home-operations/tuppr/commit/fc500dd73e275e2843d515c01903119f7c05ff62))
* refactor renovate workflow to use gh CLI commands ([098fa4a](https://github.com/home-operations/tuppr/commit/098fa4a27967d4f8cb4556e7c96ac20bc585bb37))
* refactor to talosupgrade ([ce3644c](https://github.com/home-operations/tuppr/commit/ce3644cba3ef647bdb4836b2b28fb2209cbf9ea3))
* release 0.1.16 ([63247a2](https://github.com/home-operations/tuppr/commit/63247a2886568f7b027e98e649617d57b04c557a))
* release 0.1.24 ([6bd6ce4](https://github.com/home-operations/tuppr/commit/6bd6ce4cc89618ff430f7034c63f4d1a59925782))
* release 0.1.3 ([de7b94e](https://github.com/home-operations/tuppr/commit/de7b94ea7336c9fee956ab142e8dc8292a493f29))
* release 0.1.8 ([6e148f1](https://github.com/home-operations/tuppr/commit/6e148f1b3a2fc47d365b9751f32022eb433c0042))
* **release-please:** include a bunch of sections for now ([4434848](https://github.com/home-operations/tuppr/commit/4434848260d9e6ed745cb832988a2dfe23c39f2f))
* remove deprecated functions usage ([d6050da](https://github.com/home-operations/tuppr/commit/d6050dace5f447966ef3a409ae2f6afc2bcef566))
* remove unused func ([b967ef7](https://github.com/home-operations/tuppr/commit/b967ef769f7808f83a9e5a10169c80dabb96c8dc))
* rename group from upgrade to talup ([1b0cd2b](https://github.com/home-operations/tuppr/commit/1b0cd2b346f8163069ece0cba7d853d637c6352d))
* rename to tuppr ([a670181](https://github.com/home-operations/tuppr/commit/a67018129256843c574f7a8964efcf9dfb69c288))
* Rename volume and volume mount from 'talos' to 'talosconfig' ([ebe590b](https://github.com/home-operations/tuppr/commit/ebe590b13ffa766f18b53a496346818f370ec584))
* requeue when waiting for node to be ready ([#106](https://github.com/home-operations/tuppr/issues/106)) ([36e1a2e](https://github.com/home-operations/tuppr/commit/36e1a2e5a9de6c469fce270aa577fb73cfe5701c))
* set release please PRs to draft ([f6836ae](https://github.com/home-operations/tuppr/commit/f6836aee7d88c4899e356b8c8ef929f1fbe8dbab))
* thomas suggestions ([e546e9e](https://github.com/home-operations/tuppr/commit/e546e9ed326a2abb64797f979c6138a167d55c0a))
* thomas suggestions ([daa831a](https://github.com/home-operations/tuppr/commit/daa831afd33c9139559612435ed2a7732bc287e3))
* thomas suggestions ([b9e31cb](https://github.com/home-operations/tuppr/commit/b9e31cb563a12179c0fd9a8bceecb5ed046c3630))
* update chart ([7610bb4](https://github.com/home-operations/tuppr/commit/7610bb4049e33156063757077906735404c6aa75))
* update chart ([dc432c4](https://github.com/home-operations/tuppr/commit/dc432c4b8323521148611ac9f7d2a57ec6702f4d))
* update controller logic, add release workflow ([ad99acb](https://github.com/home-operations/tuppr/commit/ad99acb816c5008bce448df1424e326d43db9f18))
* update controller logic, add release workflow ([686c633](https://github.com/home-operations/tuppr/commit/686c6337e0f21bd50e6f2d7c07b4330190ddb787))
* update generated files ([6c078fb](https://github.com/home-operations/tuppr/commit/6c078fb780c315d76d5dfc4500f2935dbe190d1d))
* update manifests ([7c45ce0](https://github.com/home-operations/tuppr/commit/7c45ce0062d6696d4272fa47537b74a8431d6a5b))
* update paths in e2e workflow ([cff0cbc](https://github.com/home-operations/tuppr/commit/cff0cbcbebf5cffa0650fab9e453458dcd5cff1e))
* update PROJECT ([3dde36a](https://github.com/home-operations/tuppr/commit/3dde36a27e609012f52527216a14c5a2362a5877))
* update readme ([afdc7d6](https://github.com/home-operations/tuppr/commit/afdc7d6e7ebab9d07bed087073505e97824b7356))
* update readme ([889ce2e](https://github.com/home-operations/tuppr/commit/889ce2e61c762debf5a835bd6417b097518746d5))
* update readme ([d53e7ec](https://github.com/home-operations/tuppr/commit/d53e7ec95905619c6d8149ad51c7cc5e54fc216f))
* update readme ([e79a3e5](https://github.com/home-operations/tuppr/commit/e79a3e55ccaa91fc47a86e61a0b39026dbd2b7e0))
* update readme ([a2e266b](https://github.com/home-operations/tuppr/commit/a2e266b2bbeb2f5fa65d68efa9675b6b972f5579))
* update readme ([e3c4a39](https://github.com/home-operations/tuppr/commit/e3c4a39025cb2251674146aac9947cf56f473fb0))
* update readme ([9d3fec7](https://github.com/home-operations/tuppr/commit/9d3fec746db6e0b33c3dd19f7ccd009cf5d0a234))
* update readme ([ab8c6d1](https://github.com/home-operations/tuppr/commit/ab8c6d16a729ef65c0d41dd5baeb477d2b56951e))
* update readme ([56c2037](https://github.com/home-operations/tuppr/commit/56c20375c9515f53036db0e8a4245695f14716c1))
* update readme ([3183ea6](https://github.com/home-operations/tuppr/commit/3183ea6fdfaa212dc051be2bdbd0441fab381049))
* update readme ([ec66bd9](https://github.com/home-operations/tuppr/commit/ec66bd93f82126f2752cb6613a541f240d19c9e1))
* update readme ([74c0f2d](https://github.com/home-operations/tuppr/commit/74c0f2dcfcacf3186825ac8d6e283b8ed8ae58bf))
* update readme ([e7503e7](https://github.com/home-operations/tuppr/commit/e7503e75adfb183caecb8509fcc45efaa5442ef7))
* update readme ([4709f4a](https://github.com/home-operations/tuppr/commit/4709f4a9576ec3e32db2bd8861c7340281b9122f))
* update readme with safe upgrade paths ([#118](https://github.com/home-operations/tuppr/issues/118)) ([3f64754](https://github.com/home-operations/tuppr/commit/3f64754a95ef5660d0b1377f8ad33e1646e7e953))
* update workflows ([b02ba22](https://github.com/home-operations/tuppr/commit/b02ba2201466eded1e2936354a759b758451b2f6))
* update workflows ([2c907e4](https://github.com/home-operations/tuppr/commit/2c907e48e032c51d94c85d322ad0cda310cc4c56))
* update workflows ([bfdc9b8](https://github.com/home-operations/tuppr/commit/bfdc9b8e43af6dfc0ff63ca51c8d6e048ed7e78a))
* update workflows ([c346517](https://github.com/home-operations/tuppr/commit/c3465171c813795010d6a57a895034e60c8bebff))
* update workflows ([e7c9781](https://github.com/home-operations/tuppr/commit/e7c978177eae112815993b4f979812a84340cdd3))
* updates ([7bad574](https://github.com/home-operations/tuppr/commit/7bad574607704a12c36c828c36743c104764ef5f))
* updates ([9971571](https://github.com/home-operations/tuppr/commit/9971571ba5e363c563ffebae4745fff1da536bd9))
* updates ([c353f99](https://github.com/home-operations/tuppr/commit/c353f999775a07fbe2d2af1a90f2c24c042d6062))
* updates ([0709b3e](https://github.com/home-operations/tuppr/commit/0709b3e421b15a8007c33b4bb1f86d10e0d813ad))
* updates ([02bdd67](https://github.com/home-operations/tuppr/commit/02bdd67fe1a9c1ebc5f820e4fe28bccd7a747970))
* updates ([7ea8523](https://github.com/home-operations/tuppr/commit/7ea85235053a32a92b0c8246d9c6f349c3b3994e))
* updates ([5c39dbb](https://github.com/home-operations/tuppr/commit/5c39dbb7e63d6b4f5406c69f0d42132507ab2694))
* updates ([22b4f64](https://github.com/home-operations/tuppr/commit/22b4f64d211e633da2b3366c44b9d0800e7d1960))
* updates ([99b9dbe](https://github.com/home-operations/tuppr/commit/99b9dbe0e9c2b3b8448f7ed55616433e3f82f51c))
* updates ([c0d4ef2](https://github.com/home-operations/tuppr/commit/c0d4ef2cf967df4944f50eb5ae93dcc42fd09fbc))
* updates ([2384c00](https://github.com/home-operations/tuppr/commit/2384c00b465f52dd923e8741fc61d2a71d2df1e9))
* updates ([760ab32](https://github.com/home-operations/tuppr/commit/760ab329073d3c0515004c548e039c3734502b40))
* we almost linux kernel now ([6809435](https://github.com/home-operations/tuppr/commit/68094352a54d4abdd64aaf2dec769ff17abaaea0))
* workflow updates ([7723868](https://github.com/home-operations/tuppr/commit/772386831dcec03c7cab10e39c5a8f50735fa58d))
* working talos client? ([6a02c2f](https://github.com/home-operations/tuppr/commit/6a02c2f433641c264aff1f120ea2f5a866386942))
* working talos client? ([9c70947](https://github.com/home-operations/tuppr/commit/9c70947ecf0d3b84676808c85b1fc6fc66d80be1))


### Code Refactoring

* crd spec again ([25af77a](https://github.com/home-operations/tuppr/commit/25af77a345ed6edd64e77459ce707a24a235a760))
* crds - see README.md ([11497fd](https://github.com/home-operations/tuppr/commit/11497fd6b61ffaeffb3f06a4e85cf216dd0588aa))
* extract audit to a shared package ([d8c76d7](https://github.com/home-operations/tuppr/commit/d8c76d733db8fbf362b5b2e70a8321dafb27e4ae))
* healthCheckExprs to healthChecks ([1fe8fe5](https://github.com/home-operations/tuppr/commit/1fe8fe5cd59a763e16a8fb4ad03d8f3bbfd75010))
* **jobs:** move duplication into a single package ([#154](https://github.com/home-operations/tuppr/issues/154)) ([083b80b](https://github.com/home-operations/tuppr/commit/083b80b543f3e4f63d170ee0b906cb14db8d2e17))
* make upgrader follow the same architecture ([#156](https://github.com/home-operations/tuppr/issues/156)) ([274a12c](https://github.com/home-operations/tuppr/commit/274a12cad102723b8608ddd95d65a064b941ede1))
* make webhook validation logic DRY ([#98](https://github.com/home-operations/tuppr/issues/98)) ([f88d067](https://github.com/home-operations/tuppr/commit/f88d06765777ea3b8dd98a3bb0bdf65967cd65a2))
* nodeSelectorExprs to nodeSelectorTerms ([b52abd2](https://github.com/home-operations/tuppr/commit/b52abd2aced4d039ef3f35b7238b9f2316772e3d))
* nodeSelectorTerms to matchNodes ([eba5bb7](https://github.com/home-operations/tuppr/commit/eba5bb790d2e836b3c4af542bd38a2197cf0488f))
* only allow one talosupgrade and one kubernetesupgrade per cluster ([ebdc17d](https://github.com/home-operations/tuppr/commit/ebdc17ddbaacfa30c50e213924d1a51560644839))
* remove image field from crd and replace with version, also refactor with using the talos sdk instead of soley relying on kubernetes to give us the info we need ([211e7dd](https://github.com/home-operations/tuppr/commit/211e7dd60cd2db69f290cab6a6a2dfa1b7d11dc3))
* schematic id no longer present in TalosUpgrade ([4c06fa3](https://github.com/home-operations/tuppr/commit/4c06fa338d8f7e52c55498f23055159dab9519e0))
* split files structure ([#111](https://github.com/home-operations/tuppr/issues/111)) ([d1d7677](https://github.com/home-operations/tuppr/commit/d1d7677ef5bcce053a6f66724234aad1d7270717))
* update talosclient to use default talosconfig found in controller ([71211aa](https://github.com/home-operations/tuppr/commit/71211aa6eea743f87b19248cb92b2b6671bedf4e))


### Continuous Integration

* **github-action:** Update action actions/checkout (v5.0.1 → v6.0.0) ([#42](https://github.com/home-operations/tuppr/issues/42)) ([63e8105](https://github.com/home-operations/tuppr/commit/63e8105a692d5c644ea9a0fc4a471a8b6e49b445))
* **github-action:** Update action actions/create-github-app-token (v2.2.2 → v3.0.0) ([#184](https://github.com/home-operations/tuppr/issues/184)) ([bd32345](https://github.com/home-operations/tuppr/commit/bd32345b12387a927c766b65db10fa376de13985))
* **github-action:** Update action azure/setup-helm (v4.3.1 → v5.0.0) ([#191](https://github.com/home-operations/tuppr/issues/191)) ([6c1aa35](https://github.com/home-operations/tuppr/commit/6c1aa359d62998b4ba8fdd4fbbf84e1e5317ef06))
* **github-action:** Update action codex-/return-dispatch (v2.1.0 → v3.0.0) ([#44](https://github.com/home-operations/tuppr/issues/44)) ([f2531bb](https://github.com/home-operations/tuppr/commit/f2531bb19475c7ffd892b8397667d5e1674f8be4))
* **github-action:** Update action docker/build-push-action (v6.19.2 → v7.0.0) ([#170](https://github.com/home-operations/tuppr/issues/170)) ([6577219](https://github.com/home-operations/tuppr/commit/6577219ab2fbe52632687043e180174382a4543b))
* **github-action:** Update action docker/login-action (v3.7.0 → v4.0.0) ([#161](https://github.com/home-operations/tuppr/issues/161)) ([325e55b](https://github.com/home-operations/tuppr/commit/325e55bfa4de170625f7d9a5c5f69347f060027e))
* **github-action:** Update action docker/metadata-action (v5.10.0 → v6.0.0) ([#169](https://github.com/home-operations/tuppr/issues/169)) ([fdfc061](https://github.com/home-operations/tuppr/commit/fdfc06130b14d4590df49b33df66a1c0c011d593))
* **github-action:** Update action docker/setup-buildx-action (v3.12.0 → v4.0.0) ([#166](https://github.com/home-operations/tuppr/issues/166)) ([14ac54e](https://github.com/home-operations/tuppr/commit/14ac54e11a9c71c49c60dda17dc6a866bff7829a))
* **github-action:** Update action golangci/golangci-lint-action (v8.0.0 → v9.0.0) ([#38](https://github.com/home-operations/tuppr/issues/38)) ([0f9cea5](https://github.com/home-operations/tuppr/commit/0f9cea56383842530381ea3bf60378a40cb5820c))
* **github-action:** Update action googleapis/release-please-action (v4.4.1 → v5) ([#208](https://github.com/home-operations/tuppr/issues/208)) ([870ee06](https://github.com/home-operations/tuppr/commit/870ee06ea69a247f2203ee61c340e05ab2b7d52a))
* **github-action:** Update action jdx/mise-action (v3.6.3 → v4.0.0) ([#183](https://github.com/home-operations/tuppr/issues/183)) ([79badd3](https://github.com/home-operations/tuppr/commit/79badd34530377d6f2c74c75da5e9e7e04cef22e))
* **github-action:** Update GitHub Artifact Actions (major) ([#136](https://github.com/home-operations/tuppr/issues/136)) ([d8e721f](https://github.com/home-operations/tuppr/commit/d8e721f63435472a75e473ef7b8dc88404cd6945))
* **github-action:** Update GitHub Artifact Actions (major) ([#30](https://github.com/home-operations/tuppr/issues/30)) ([a00ea59](https://github.com/home-operations/tuppr/commit/a00ea59192c1c054cfab61ce26d4ce5a736b6c16))
* **github-action:** Update GitHub Artifact Actions (major) ([#49](https://github.com/home-operations/tuppr/issues/49)) ([700d1b3](https://github.com/home-operations/tuppr/commit/700d1b35cd4e6bb015a20440fba9a18a24c81e32))

## [0.1.23](https://github.com/home-operations/tuppr/compare/0.1.22...0.1.23) (2026-05-07)


### Features

* **kubernetesupgrade:** removes superflous version ([#250](https://github.com/home-operations/tuppr/issues/250)) ([2a037ff](https://github.com/home-operations/tuppr/commit/2a037ff98db2026e1e1a6d1ff0a306bad2590a1b))
* **mise:** update tool kustomize (5.6.0 → 5.8.1) ([82ad48f](https://github.com/home-operations/tuppr/commit/82ad48f5648d73c0b5cc201b12e90e077bff8205))
* **mise:** update tool operator-sdk (1.41.1 → 1.42.2) ([160df5c](https://github.com/home-operations/tuppr/commit/160df5cfac86b4fd5c957a47dd336d2f1f54280e))

## [0.1.22](https://github.com/home-operations/tuppr/compare/0.1.21...0.1.22) (2026-05-06)


### Features

* add node watching for kubernetesupgrade ([1cd9cb7](https://github.com/home-operations/tuppr/commit/1cd9cb76b025845a96db3ec45a2a80a8176e6459))
* **mise:** update tool aqua:operator-framework/operator-registry (1.55.0 → 1.67.0) ([ac02ccd](https://github.com/home-operations/tuppr/commit/ac02ccd3f4f935abf46eff97578a0b923876ec2b))
* **mise:** update tool kube-controller-tools (v0.18.0 → v0.21.0) ([e7f7689](https://github.com/home-operations/tuppr/commit/e7f7689508d401fb7de3f4ba1fbeb015474465df))


### Bug Fixes

* use mise for ci ([e03f69a](https://github.com/home-operations/tuppr/commit/e03f69a729e75a28f17c2e84087d6f6680368c07))


### Miscellaneous Chores

* move deps to mise ([a9e3ad1](https://github.com/home-operations/tuppr/commit/a9e3ad1f14b5d5702a632a7129a66a53b990a730))


### Code Refactoring

* extract audit to a shared package ([2f4e8b5](https://github.com/home-operations/tuppr/commit/2f4e8b5dbfe4900bb64dd52e9f4fcec07f5dcbbb))

## [0.1.21](https://github.com/home-operations/tuppr/compare/0.1.20...0.1.21) (2026-05-05)


### Features

* watch nodes instead ([8f46e37](https://github.com/home-operations/tuppr/commit/8f46e37185cb75ce88442de5f367d88df720a226))


### Miscellaneous Chores

* increase number of retries ([8d07361](https://github.com/home-operations/tuppr/commit/8d07361daa27f390af6905155b5775e3d3763d8a))

## [0.1.20](https://github.com/home-operations/tuppr/compare/0.1.19...0.1.20) (2026-05-05)


### Features

* add the build version ([2595b05](https://github.com/home-operations/tuppr/commit/2595b05a35349177df93efafc1f219577ac2cab9))
* check for drift in talosUpgrade or kubernetesUpgrade ([caf239b](https://github.com/home-operations/tuppr/commit/caf239bd818616f9487b155d339c23fd16d0c616))
* **dashboards:** add hooks to dashboards ([c7c1e8e](https://github.com/home-operations/tuppr/commit/c7c1e8e1c9fd938ed565d3b698c65726f1976100))
* improve talos upgrade alerting rules ([a40c7cd](https://github.com/home-operations/tuppr/commit/a40c7cda333ae6243b6cc4d72f7093e3e80e2a03))


### Bug Fixes

* fix lint and tests ([b1269bc](https://github.com/home-operations/tuppr/commit/b1269bcc88560715d36e0eec103078a1d52ca261))
* remove duplicated metrics ([1f5c48c](https://github.com/home-operations/tuppr/commit/1f5c48caeaaa51237c5555e5f3eb075f338f1832))


### Documentation

* remove list of metrics ([931045a](https://github.com/home-operations/tuppr/commit/931045a17dd9774c7188e8e69f462149afe22f30))

## [0.1.19](https://github.com/home-operations/tuppr/compare/0.1.18...0.1.19) (2026-05-05)


### Features

* improve metrics and grafanadashboard ([0aef4fa](https://github.com/home-operations/tuppr/commit/0aef4fac60513033a2d8a17edb2c82a1234bc231))


### Miscellaneous Chores

* **metrics:** add empty data at startup ([f79df1f](https://github.com/home-operations/tuppr/commit/f79df1f6522e3bbc09cf19a62594dc7268a03dc7))

## [0.1.18](https://github.com/home-operations/tuppr/compare/0.1.17...0.1.18) (2026-05-04)


### Bug Fixes

* add missing reporter ([ba40626](https://github.com/home-operations/tuppr/commit/ba4062678b2373b84c6f97bd9a05d2110a62d022))

## [0.1.17](https://github.com/home-operations/tuppr/compare/0.1.16...0.1.17) (2026-05-04)


### Bug Fixes

* correct indent of templates ([63e88ec](https://github.com/home-operations/tuppr/commit/63e88eca2a28fc482f7625cf70bfd01449ed2a94))

## [0.1.16](https://github.com/home-operations/tuppr/compare/0.1.15...0.1.16) (2026-05-04)


### Miscellaneous Chores

* release 0.1.16 ([6092c72](https://github.com/home-operations/tuppr/commit/6092c721b3e898b3462ef1023ed8d9827e9e36e2))

## [0.1.15](https://github.com/home-operations/tuppr/compare/0.1.14...0.1.15) (2026-05-04)


### Bug Fixes

* correct helm template for dashboard ([bcbbc84](https://github.com/home-operations/tuppr/commit/bcbbc84bcd83b3e26bbaae6faf16f19e93b391f5))


### Miscellaneous Chores

* **dashboard:** import the file as json ([d5b86a5](https://github.com/home-operations/tuppr/commit/d5b86a516b1d256657860cbe1ce81a9bfedc4209))

## [0.1.14](https://github.com/home-operations/tuppr/compare/0.1.13...0.1.14) (2026-05-04)


### Features

* add grafanadashboard ([6aaf23e](https://github.com/home-operations/tuppr/commit/6aaf23e2668d49034da218fb45435e88711452eb))


### Documentation

* simplify the contributors.md ([44d9078](https://github.com/home-operations/tuppr/commit/44d90787eccd2cf21c3b2f5354452da56d7ea4c8))

## [0.1.13](https://github.com/home-operations/tuppr/compare/0.1.12...0.1.13) (2026-05-04)


### Features

* add basic contributing/issues/PR template ([339015d](https://github.com/home-operations/tuppr/commit/339015d2e20ea6799b090ac70db2cc16076b46b8))
* **talosupgrade:** preserve any registry's install image across upgrades ([30c662c](https://github.com/home-operations/tuppr/commit/30c662cc69a9771aa34dd88fc72474c057a376ea))

## [0.1.12](https://github.com/home-operations/tuppr/compare/0.1.11...0.1.12) (2026-05-04)


### Features

* **deps:** update module google.golang.org/grpc (v1.80.0 → v1.81.0) ([#236](https://github.com/home-operations/tuppr/issues/236)) ([ff6be1c](https://github.com/home-operations/tuppr/commit/ff6be1c4f22d2c5c0940ef44c1d98618f08b42b8))
* improve metrics ([2413b78](https://github.com/home-operations/tuppr/commit/2413b780f7cc51999f1d41ae70681ea11daa1b92))


### Bug Fixes

* **talosupgrade:** preserve factory installer flavor across upgrades ([aaf1ea5](https://github.com/home-operations/tuppr/commit/aaf1ea5c43228839f7627b3c1b30e0cac73feec7))

## [0.1.11](https://github.com/home-operations/tuppr/compare/0.1.10...0.1.11) (2026-05-01)


### Features

* **deps:** update module sigs.k8s.io/controller-runtime (v0.23.3 → v0.24.0) ([#231](https://github.com/home-operations/tuppr/issues/231)) ([22ac172](https://github.com/home-operations/tuppr/commit/22ac172e92c55cdea6853c4903c6c055740510ac))
* **kubernetesupgrade:** allow private registry for component images via spec.kubernetes.imageRepository ([68bba9e](https://github.com/home-operations/tuppr/commit/68bba9e9aeab9c8586849c0f17b5677f48a4af70))
* **talosupgrade:** allow per-node factory URL override via tuppr.home-operations.com/factory-url annotation ([8154539](https://github.com/home-operations/tuppr/commit/81545397ccf17e2e6ceea326f4a37b53ba745c2a))
* **talosupgrade:** auto-detect schematic from ([ef2a5fa](https://github.com/home-operations/tuppr/commit/ef2a5fa8f2281e04251695fb72cba6c00577bfdc))
* **talosupgrade:** pre/post-upgrade hooks via spec.hooks ([9982679](https://github.com/home-operations/tuppr/commit/9982679db0e66d7b84e59c817fbeba529b81042b))


### Bug Fixes

* **kubernetesupgrade:** inject hostAliases for controlPlane endpoint hostname ([9fe8f47](https://github.com/home-operations/tuppr/commit/9fe8f47c2110780776aeb80fd93fcb5d1384e91d))


### Documentation

* update readme ([51ab315](https://github.com/home-operations/tuppr/commit/51ab3158e6b176b29f8c734faf8adfecac56e3f1))


### Miscellaneous Chores

* remove deprecated functions usage ([ba649c0](https://github.com/home-operations/tuppr/commit/ba649c01315bf423420242bcb4d304abac6ce941))

## [0.1.10](https://github.com/home-operations/tuppr/compare/0.1.9...0.1.10) (2026-04-28)


### Bug Fixes

* **talosupgrade:** fix image patching ([cf58dee](https://github.com/home-operations/tuppr/commit/cf58deec79423dab444f9a2e3f0b445bf05703a3))

## [0.1.9](https://github.com/home-operations/tuppr/compare/0.1.8...0.1.9) (2026-04-27)


### Features

* **deps:** update module github.com/siderolabs/talos/pkg/machinery (v1.12.7 → v1.13.0) ([#221](https://github.com/home-operations/tuppr/issues/221)) ([95e2e57](https://github.com/home-operations/tuppr/commit/95e2e57f1510b8a209b6c9cad8780556488beca4))


### Bug Fixes

* **deps:** update module github.com/onsi/ginkgo/v2 (v2.28.1 → v2.28.2) ([#217](https://github.com/home-operations/tuppr/issues/217)) ([17d7d90](https://github.com/home-operations/tuppr/commit/17d7d90d07c02a8b71783458049a23f7bb557932))
* **main:** show new version number after successful update ([#219](https://github.com/home-operations/tuppr/issues/219)) ([6629716](https://github.com/home-operations/tuppr/commit/6629716d72a04ee2d950ab622a79b64f8de0b485))

## [0.1.8](https://github.com/home-operations/tuppr/compare/0.1.7...0.1.8) (2026-04-25)


### ⚠ BREAKING CHANGES

* **github-action:** Update action googleapis/release-please-action (v4.4.1 → v5) ([#208](https://github.com/home-operations/tuppr/issues/208))

### Bug Fixes

* delete failed jobs and record out-of-band upgraded nodes ([32504cb](https://github.com/home-operations/tuppr/commit/32504cb02bf243e71c8083a3d65840c642a98594))
* **deps:** update module github.com/siderolabs/talos/pkg/machinery (v1.12.6 → v1.12.7) ([#212](https://github.com/home-operations/tuppr/issues/212)) ([c42bd38](https://github.com/home-operations/tuppr/commit/c42bd384d081b84a278254a0fb32b17a94e3b03e))


### Miscellaneous Chores

* release 0.1.8 ([4ac25e5](https://github.com/home-operations/tuppr/commit/4ac25e5e35c8cf5c4fbc3d5a8b619029b30b9f07))


### Continuous Integration

* **github-action:** Update action googleapis/release-please-action (v4.4.1 → v5) ([#208](https://github.com/home-operations/tuppr/issues/208)) ([e43a99e](https://github.com/home-operations/tuppr/commit/e43a99e364560206ae5fd43c6109d4e852c1ef2c))

## [0.1.7](https://github.com/home-operations/tuppr/compare/0.1.6...0.1.7) (2026-04-21)


### Features

* record update history ([65d19e2](https://github.com/home-operations/tuppr/commit/65d19e21d6dbf3ca874b316bc93ff378a598e4a9))


### Bug Fixes

* use new imager approach for e2e bootstrap ([#190](https://github.com/home-operations/tuppr/issues/190)) ([4d637e2](https://github.com/home-operations/tuppr/commit/4d637e29d297146ce79bdf48b7ae70a8d41358f5))

## [0.1.6](https://github.com/home-operations/tuppr/compare/0.1.5...0.1.6) (2026-04-17)


### Features

* **deps:** update module github.com/netresearch/go-cron (v0.13.4 → v0.14.0) ([#205](https://github.com/home-operations/tuppr/issues/205)) ([519314c](https://github.com/home-operations/tuppr/commit/519314c5cd4ae57080d3739a8748ca11abb4059d))
* **talosupgrade:** add parallelism support for concurrent node upgrades ([#201](https://github.com/home-operations/tuppr/issues/201)) ([7b476f0](https://github.com/home-operations/tuppr/commit/7b476f0ae7bd24fa5701e24dc53626743da7e601))


### Bug Fixes

* **deps:** update kubernetes monorepo (v0.35.3 → v0.35.4) ([#203](https://github.com/home-operations/tuppr/issues/203)) ([16970c4](https://github.com/home-operations/tuppr/commit/16970c4adc7ebadcb04d62abaa83888cc1255e4b))

## [0.1.5](https://github.com/home-operations/tuppr/compare/0.1.4...0.1.5) (2026-04-11)


### Features

* **deps:** update module google.golang.org/grpc (v1.79.3 → v1.80.0) ([#193](https://github.com/home-operations/tuppr/issues/193)) ([e4b5bb3](https://github.com/home-operations/tuppr/commit/e4b5bb3987a0a2a9316f748e8c8f5fca49bc6db6))
* **talos:** sync machine.install.image in stored config after upgrade ([75b539a](https://github.com/home-operations/tuppr/commit/75b539a9e36a9d060baaf421c2cb263d51f84092))


### Bug Fixes

* **deps:** update module github.com/google/go-containerregistry (v0.21.4 → v0.21.5) ([#202](https://github.com/home-operations/tuppr/issues/202)) ([149de93](https://github.com/home-operations/tuppr/commit/149de938c44cf6ee69cdef97a7047467653dc0e3))
* **mise:** update tool helm (4.1.3 → 4.1.4) ([a049a31](https://github.com/home-operations/tuppr/commit/a049a31eee0085922277f90f754c7c2c48d04a49))


### Miscellaneous Chores

* refactor renovate workflow to use gh CLI commands ([54b0aba](https://github.com/home-operations/tuppr/commit/54b0aba6129854bb402f67ac3efde00c25e3f124))

## [0.1.4](https://github.com/home-operations/tuppr/compare/0.1.3...0.1.4) (2026-04-08)


### Features

* **deps:** update module github.com/google/cel-go (v0.27.0 → v0.28.0) ([#199](https://github.com/home-operations/tuppr/issues/199)) ([ff82096](https://github.com/home-operations/tuppr/commit/ff82096c2d8a59594ce959252fd36da16b990c0c))


### Bug Fixes

* **ci:** fix helm lint and pin version ([42f40a9](https://github.com/home-operations/tuppr/commit/42f40a997d950fbf301d0b02e602fcaafe7835c6))
* **deps:** update module github.com/google/go-containerregistry (v0.21.3 → v0.21.4) ([#198](https://github.com/home-operations/tuppr/issues/198)) ([013da09](https://github.com/home-operations/tuppr/commit/013da09e5feab50a2c29cd56e95f94ce97933301))
* **deps:** update module github.com/netresearch/go-cron (v0.13.1 → v0.13.4) ([#196](https://github.com/home-operations/tuppr/issues/196)) ([7fbfaab](https://github.com/home-operations/tuppr/commit/7fbfaabddf1bd6525e728ff92b3036fd5db8b1a8))
* **mise:** update tool go (1.26.1 → 1.26.2) ([66ba006](https://github.com/home-operations/tuppr/commit/66ba00674115ebf32dd4325771d39446a01f0ae1))


### Miscellaneous Chores

* **ci:** tidy up github actions ([3397887](https://github.com/home-operations/tuppr/commit/339788728cb7dbfe7f15159fd3fee52c7bd647a9))

## [0.1.3](https://github.com/home-operations/tuppr/compare/0.1.2...0.1.3) (2026-04-01)


### ⚠ BREAKING CHANGES

* **github-action:** Update action azure/setup-helm (v4.3.1 → v5.0.0) ([#191](https://github.com/home-operations/tuppr/issues/191))
* **github-action:** Update action actions/create-github-app-token (v2.2.2 → v3.0.0) ([#184](https://github.com/home-operations/tuppr/issues/184))
* **github-action:** Update action jdx/mise-action (v3.6.3 → v4.0.0) ([#183](https://github.com/home-operations/tuppr/issues/183))

### Features

* **deps:** update module github.com/open-policy-agent/cert-controller (v0.15.0 → v0.16.0) ([#181](https://github.com/home-operations/tuppr/issues/181)) ([f747a7f](https://github.com/home-operations/tuppr/commit/f747a7f869cfc2c2d000f0bd6283e56a09b37b99))


### Bug Fixes

* **deps:** update kubernetes packages (v0.35.2 → v0.35.3) ([#187](https://github.com/home-operations/tuppr/issues/187)) ([499e804](https://github.com/home-operations/tuppr/commit/499e804aad59ebcf78ecbb2a179ccafe88358a5b))
* **deps:** update module github.com/cosi-project/runtime (v1.14.0 → v1.14.1) ([#192](https://github.com/home-operations/tuppr/issues/192)) ([4289459](https://github.com/home-operations/tuppr/commit/4289459e14d53da7c64288a2cb82aefe81f32761))
* **deps:** update module github.com/google/go-containerregistry (v0.21.2 → v0.21.3) ([#185](https://github.com/home-operations/tuppr/issues/185)) ([831fe23](https://github.com/home-operations/tuppr/commit/831fe233e328ae0573a701e4fc56e68370643de6))
* **deps:** update module github.com/siderolabs/talos/pkg/machinery (v1.12.5 → v1.12.6) ([#188](https://github.com/home-operations/tuppr/issues/188)) ([6d99117](https://github.com/home-operations/tuppr/commit/6d9911733eef801c9e5366990c2deebc400dd763))
* **deps:** update module google.golang.org/grpc (v1.79.2 → v1.79.3) ([#186](https://github.com/home-operations/tuppr/issues/186)) ([7c91009](https://github.com/home-operations/tuppr/commit/7c91009746f05d0c457d8e163de0f4b5c1c2ade5))


### Miscellaneous Chores

* **charts:** expose priorityClassName ([678d288](https://github.com/home-operations/tuppr/commit/678d288a405f122717a9903fd75dd992b096d964))
* **deps:** update k8s.io/utils digest (b8788ab → 28399d8) ([#189](https://github.com/home-operations/tuppr/issues/189)) ([584968f](https://github.com/home-operations/tuppr/commit/584968f07de565de35855315aad7731227df6f53))
* release 0.1.3 ([9259009](https://github.com/home-operations/tuppr/commit/925900965c667883a67df5fce6b40635a222cf3a))


### Continuous Integration

* **github-action:** Update action actions/create-github-app-token (v2.2.2 → v3.0.0) ([#184](https://github.com/home-operations/tuppr/issues/184)) ([ec9b3da](https://github.com/home-operations/tuppr/commit/ec9b3da1295be08b0a1157c4ce842b2849891c22))
* **github-action:** Update action azure/setup-helm (v4.3.1 → v5.0.0) ([#191](https://github.com/home-operations/tuppr/issues/191)) ([23b760a](https://github.com/home-operations/tuppr/commit/23b760a3d62227ef991f3ff7bb58172bcec2e244))
* **github-action:** Update action jdx/mise-action (v3.6.3 → v4.0.0) ([#183](https://github.com/home-operations/tuppr/issues/183)) ([a3253db](https://github.com/home-operations/tuppr/commit/a3253dba881d9ee0aa2126d6d61b86b7de6ddcdd))

## [0.1.2](https://github.com/home-operations/tuppr/compare/0.1.1...0.1.2) (2026-03-12)


### Bug Fixes

* **controller:** reset observedGeneration on job failure to allow retry ([#178](https://github.com/home-operations/tuppr/issues/178)) ([5e4ef5b](https://github.com/home-operations/tuppr/commit/5e4ef5b81ce47539b36ef57a7a245af3d93e15be))

## [0.1.1](https://github.com/home-operations/tuppr/compare/0.1.0...0.1.1) (2026-03-09)


### Bug Fixes

* **helm:** add missing brackets in prometheus rule template ([#177](https://github.com/home-operations/tuppr/issues/177)) ([b12bcb6](https://github.com/home-operations/tuppr/commit/b12bcb6b267dff00f25352bf8e920b5ae6241c45))


### Miscellaneous Chores

* change draft configuration to draft-pull-request ([238b2fb](https://github.com/home-operations/tuppr/commit/238b2fb105da5f17e06d0f74255587a840073926))

## [0.1.0](https://github.com/home-operations/tuppr/compare/0.0.80...0.1.0) (2026-03-09)


### ⚠ BREAKING CHANGES

* **github-action:** Update action docker/build-push-action (v6.19.2 → v7.0.0) ([#170](https://github.com/home-operations/tuppr/issues/170))
* **github-action:** Update action docker/metadata-action (v5.10.0 → v6.0.0) ([#169](https://github.com/home-operations/tuppr/issues/169))
* **github-action:** Update action docker/setup-buildx-action (v3.12.0 → v4.0.0) ([#166](https://github.com/home-operations/tuppr/issues/166))
* **github-action:** Update action docker/login-action (v3.7.0 → v4.0.0) ([#161](https://github.com/home-operations/tuppr/issues/161))

### Features

* **monitoring:** add prometheus rule to helm charts to alert failed upgrade ([#163](https://github.com/home-operations/tuppr/issues/163)) ([7cccece](https://github.com/home-operations/tuppr/commit/7cccece5e302335743b59a4d42276a697e362659))


### Bug Fixes

* **deps:** update module github.com/netresearch/go-cron (v0.13.0 → v0.13.1) ([#173](https://github.com/home-operations/tuppr/issues/173)) ([928f4c6](https://github.com/home-operations/tuppr/commit/928f4c6148cbc046686dd30db9291d153aba36ce))
* **deps:** update module github.com/siderolabs/talos/pkg/machinery (v1.12.4 → v1.12.5) ([#174](https://github.com/home-operations/tuppr/issues/174)) ([47c3bf9](https://github.com/home-operations/tuppr/commit/47c3bf979c6c26a570107a503e68b846a273b67f))
* **deps:** update module google.golang.org/grpc (v1.79.1 → v1.79.2) ([#171](https://github.com/home-operations/tuppr/issues/171)) ([d218224](https://github.com/home-operations/tuppr/commit/d21822432ba2a56806e7c529521e685ef5822cdb))
* **deps:** update module sigs.k8s.io/controller-runtime (v0.23.1 → v0.23.2) ([#167](https://github.com/home-operations/tuppr/issues/167)) ([6a812d8](https://github.com/home-operations/tuppr/commit/6a812d837659badbb8dfa23d68bda148e5958488))
* **deps:** update module sigs.k8s.io/controller-runtime (v0.23.2 → v0.23.3) ([#168](https://github.com/home-operations/tuppr/issues/168)) ([b9a876a](https://github.com/home-operations/tuppr/commit/b9a876a68ecf86a9c647ef496fafcd3f97bede41))
* **metrics:** replace numeric phase encoding with state-set gauge pattern ([69f3940](https://github.com/home-operations/tuppr/commit/69f39402b911c0e7081fca8e29a10c83fa4c3068))
* **metrics:** replace numeric phase encoding with state-set gauge pattern ([#172](https://github.com/home-operations/tuppr/issues/172)) ([67a72ee](https://github.com/home-operations/tuppr/commit/67a72ee1a11b50ab0b0bb4a8c0ec14edbfa71c7c))
* **mise:** update tool go (1.26.0 → 1.26.1) ([8da7632](https://github.com/home-operations/tuppr/commit/8da7632b67b1dcaba41d395a110e9470394a5d50))


### Miscellaneous Chores

* set release please PRs to draft ([1119902](https://github.com/home-operations/tuppr/commit/11199023672cb4fb3d3a41d8d6bfb559617f6c7d))


### Continuous Integration

* **github-action:** Update action docker/build-push-action (v6.19.2 → v7.0.0) ([#170](https://github.com/home-operations/tuppr/issues/170)) ([75054f8](https://github.com/home-operations/tuppr/commit/75054f873e0d497ef6fb28dad2bc9b3bd90bd95c))
* **github-action:** Update action docker/login-action (v3.7.0 → v4.0.0) ([#161](https://github.com/home-operations/tuppr/issues/161)) ([c51a0df](https://github.com/home-operations/tuppr/commit/c51a0df6f0670a5438da03146229148440711c9e))
* **github-action:** Update action docker/metadata-action (v5.10.0 → v6.0.0) ([#169](https://github.com/home-operations/tuppr/issues/169)) ([6833297](https://github.com/home-operations/tuppr/commit/683329738aec5b274543c231c7b91191113c0f42))
* **github-action:** Update action docker/setup-buildx-action (v3.12.0 → v4.0.0) ([#166](https://github.com/home-operations/tuppr/issues/166)) ([9f1c9f9](https://github.com/home-operations/tuppr/commit/9f1c9f9ef63657793b0feb66d87affeee073bf69))

## [0.0.80](https://github.com/home-operations/tuppr/compare/0.0.79...0.0.80) (2026-03-03)


### Bug Fixes

* **deps:** update module github.com/google/go-containerregistry (v0.21.1 → v0.21.2) ([#153](https://github.com/home-operations/tuppr/issues/153)) ([c26aca4](https://github.com/home-operations/tuppr/commit/c26aca4729dd545a6524501878b182d2c2177dd2))
* **metrics:** record previously unused job metrics and clean up on deletion ([#160](https://github.com/home-operations/tuppr/issues/160)) ([1875c37](https://github.com/home-operations/tuppr/commit/1875c373c099b7edcb7088efe20dd2ae51b6b347))
* **release-please:** always update pr with the latest changes ([5f4ff63](https://github.com/home-operations/tuppr/commit/5f4ff630c25ca67d5111c9f059b62c6df9cb8772))


### Miscellaneous Chores

* add workflow_dispatch trigger to release workflow ([9751b8a](https://github.com/home-operations/tuppr/commit/9751b8ac155cf5ec3f3759266b54c6bc1b90c4cf))
* **release-please:** include a bunch of sections for now ([405264f](https://github.com/home-operations/tuppr/commit/405264f3bab1cfaa8f5e0f00258d7b62bf996f6f))


### Code Refactoring

* **jobs:** move duplication into a single package ([#154](https://github.com/home-operations/tuppr/issues/154)) ([dd14644](https://github.com/home-operations/tuppr/commit/dd14644391c0893b534872d7380806ebd9802f60))
* make upgrader follow the same architecture ([#156](https://github.com/home-operations/tuppr/issues/156)) ([3e5a6b2](https://github.com/home-operations/tuppr/commit/3e5a6b243a3ebaa4c8a61e302004a59969e29590))

## [0.0.79](https://github.com/home-operations/tuppr/compare/0.0.78...0.0.79) (2026-03-02)


### Bug Fixes

* use github app token to create a new tag to fix ga loop protection ([#151](https://github.com/home-operations/tuppr/issues/151)) ([c264c4a](https://github.com/home-operations/tuppr/commit/c264c4a8b76b97b679d1df2fb8fac7142cb06c4a))

## [0.0.78](https://github.com/home-operations/tuppr/compare/0.0.77...0.0.78) (2026-03-02)


### Bug Fixes

* handle correctly maintenance window when not all node are updated ([#148](https://github.com/home-operations/tuppr/issues/148)) ([8cf89a7](https://github.com/home-operations/tuppr/commit/8cf89a7ef01e1ddba32e428c3962098bdced4bb1))

## [0.0.77](https://github.com/home-operations/tuppr/compare/v0.0.76...0.0.77) (2026-03-02)


### Bug Fixes

* **release:** exclude v from release tag ([#146](https://github.com/home-operations/tuppr/issues/146)) ([12f39ba](https://github.com/home-operations/tuppr/commit/12f39ba6b177fa6839f386c239987f2d87c0bd5c))

## [0.0.76](https://github.com/home-operations/tuppr/compare/0.0.75...v0.0.76) (2026-03-02)


### Features

* **release:** use release-please ([#143](https://github.com/home-operations/tuppr/issues/143)) ([a9af0aa](https://github.com/home-operations/tuppr/commit/a9af0aa8814490669dcc519b1c86a278f50c598b))


### Bug Fixes

* (hopefully) stop e2e test dns flakes ([#144](https://github.com/home-operations/tuppr/issues/144)) ([8b6ec55](https://github.com/home-operations/tuppr/commit/8b6ec5558393c7532ea849d584513ed4dbddd34e))
* **crds:** remove duplicated enum for kubernetes upgrade job phase ([#141](https://github.com/home-operations/tuppr/issues/141)) ([2f62f50](https://github.com/home-operations/tuppr/commit/2f62f502c3b6c272089033e2990498d373ecd8c0))
* **kubeupgrade:** continue to next node after partial job success ([#142](https://github.com/home-operations/tuppr/issues/142)) ([367d108](https://github.com/home-operations/tuppr/commit/367d108999b21422f2058c14afe7c2da6e4aa7df))
* tofu flag ordering in e2e cleanup ([f19bcae](https://github.com/home-operations/tuppr/commit/f19bcae320371b3e4bdf04598101376315a1d0a7))
