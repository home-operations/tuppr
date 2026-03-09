# Changelog

## [0.2.0](https://github.com/home-operations/tuppr/compare/0.1.0...0.2.0) (2026-03-09)


### ⚠ BREAKING CHANGES

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

* add annotation to force reset failed upgrade ([31c7a9f](https://github.com/home-operations/tuppr/commit/31c7a9fe61fc65b1dbe170a19c5349315f913731))
* add configurable talos upgrade timeout ([#61](https://github.com/home-operations/tuppr/issues/61)) ([64cbb6a](https://github.com/home-operations/tuppr/commit/64cbb6a932aec1fdd63139f47c8f262eaa46cb2b))
* add maintenance window support ([#91](https://github.com/home-operations/tuppr/issues/91)) ([2e7ffcf](https://github.com/home-operations/tuppr/commit/2e7ffcfd29d359b7673b6cd8da4d277b1219a036))
* add node labels during upgrades ([#110](https://github.com/home-operations/tuppr/issues/110)) ([5339f14](https://github.com/home-operations/tuppr/commit/5339f1418bcac4b4d57d5a0c524cd1124728c000))
* Add node selector for talosupgrade ([#103](https://github.com/home-operations/tuppr/issues/103)) ([a49b545](https://github.com/home-operations/tuppr/commit/a49b545b184af78b0c907f4ed173b0ff33b3d243))
* add placementPreset to upgradePolicy ([fee6414](https://github.com/home-operations/tuppr/commit/fee6414b8144de2eea11faec4f080da4a8b0362d))
* add stage support ([#48](https://github.com/home-operations/tuppr/issues/48)) ([5fc6f6a](https://github.com/home-operations/tuppr/commit/5fc6f6ad19cc6c0677294a0e732e850768e35537))
* add suspend annotation support ([aa235e2](https://github.com/home-operations/tuppr/commit/aa235e223c4f6c4981278fd1c7bcbb7b74182849))
* Allow overriding talos schematic and version through node annotations ([#102](https://github.com/home-operations/tuppr/issues/102)) ([a19c77f](https://github.com/home-operations/tuppr/commit/a19c77fe8104e48f0376eec521f2def70f79f5dc))
* block upgrades when talos version or schematic happen outside cr ([0e40009](https://github.com/home-operations/tuppr/commit/0e40009296ef0e850abdfa126bf7c0a005b4b86d))
* Check image availability before creating a new job ([#100](https://github.com/home-operations/tuppr/issues/100)) ([0ddf680](https://github.com/home-operations/tuppr/commit/0ddf680d7e3db171beda4bbe4a7f54abddcf2417))
* clean up completed jobs after successful node upgrade ([4d72df5](https://github.com/home-operations/tuppr/commit/4d72df52da639bf497567b963235482a2ac4967c))
* configure drain behavior ([#112](https://github.com/home-operations/tuppr/issues/112)) ([8dc798f](https://github.com/home-operations/tuppr/commit/8dc798f51e42e1873501cdcdfe7e3b32c521458d))
* **container:** update image golang (1.24 → 1.25) ([#3](https://github.com/home-operations/tuppr/issues/3)) ([ce062d9](https://github.com/home-operations/tuppr/commit/ce062d9f98e50defc1089b16e215425127390c5a))
* **container:** update image golang (1.25 → 1.26) ([#84](https://github.com/home-operations/tuppr/issues/84)) ([f52110b](https://github.com/home-operations/tuppr/commit/f52110b6af20934e7a713ed55e3118914b0a0303))
* continue to evaluate healthchecks until all pass ([ba0e8fe](https://github.com/home-operations/tuppr/commit/ba0e8fe46c637023b3fe60c852a493791b92061c))
* **deps:** update kubernetes packages (v0.34.3 → v0.35.0) ([#53](https://github.com/home-operations/tuppr/issues/53)) ([9a99963](https://github.com/home-operations/tuppr/commit/9a999631906930dc310e59c6ba0b8353528fe229))
* **deps:** update module github.com/cosi-project/runtime (v1.10.7 → v1.11.0) ([#15](https://github.com/home-operations/tuppr/issues/15)) ([f0ba464](https://github.com/home-operations/tuppr/commit/f0ba464e96bf456caa0b2e874ab6f09ee1e4fd51))
* **deps:** update module github.com/cosi-project/runtime (v1.11.0 → v1.12.0) ([#34](https://github.com/home-operations/tuppr/issues/34)) ([3f8d4e6](https://github.com/home-operations/tuppr/commit/3f8d4e63c90dab644171ab6c454166aa1b5b0edb))
* **deps:** update module github.com/cosi-project/runtime (v1.12.0 → v1.13.0) ([#43](https://github.com/home-operations/tuppr/issues/43)) ([570f001](https://github.com/home-operations/tuppr/commit/570f00132f82c66cd69677ad27810ae5f6aa1941))
* **deps:** update module github.com/cosi-project/runtime (v1.13.0 → v1.14.0) ([#86](https://github.com/home-operations/tuppr/issues/86)) ([708ddff](https://github.com/home-operations/tuppr/commit/708ddff89720b9a2df9165ab52555a6461c1230c))
* **deps:** update module github.com/google/cel-go (v0.26.1 → v0.27.0) ([#81](https://github.com/home-operations/tuppr/issues/81)) ([77a7997](https://github.com/home-operations/tuppr/commit/77a799790cea096cf8e50de91d645830a61ec3ec))
* **deps:** update module github.com/google/go-containerregistry (v0.20.7 → v0.21.0) ([#127](https://github.com/home-operations/tuppr/issues/127)) ([c27cb9e](https://github.com/home-operations/tuppr/commit/c27cb9e8d8681d7080fdc2f728fd1dfb453527c4))
* **deps:** update module github.com/netresearch/go-cron (v0.11.0 → v0.12.0) ([#123](https://github.com/home-operations/tuppr/issues/123)) ([511fcd4](https://github.com/home-operations/tuppr/commit/511fcd4135053977371c7d2e2942ba558a9beaf9))
* **deps:** update module github.com/netresearch/go-cron (v0.12.0 → v0.13.0) ([#132](https://github.com/home-operations/tuppr/issues/132)) ([a179348](https://github.com/home-operations/tuppr/commit/a17934860befab1374abe76421a7e274fe958197))
* **deps:** update module github.com/onsi/ginkgo/v2 (v2.25.3 → v2.26.0) ([#20](https://github.com/home-operations/tuppr/issues/20)) ([b20c5e7](https://github.com/home-operations/tuppr/commit/b20c5e78d0a1b9014fb022980780ae062c5e3d8e))
* **deps:** update module github.com/onsi/ginkgo/v2 (v2.26.0 → v2.27.1) ([#29](https://github.com/home-operations/tuppr/issues/29)) ([7aa82e5](https://github.com/home-operations/tuppr/commit/7aa82e53c1acfbd8b59ec2281fa7153c69dd865b))
* **deps:** update module github.com/onsi/gomega (v1.36.1 → v1.38.2) ([#6](https://github.com/home-operations/tuppr/issues/6)) ([b761b6b](https://github.com/home-operations/tuppr/commit/b761b6baf93a53f249ac77c417c9bb8e15d8870f))
* **deps:** update module github.com/onsi/gomega (v1.38.3 → v1.39.0) ([#70](https://github.com/home-operations/tuppr/issues/70)) ([9f90170](https://github.com/home-operations/tuppr/commit/9f901706d318f16fe0f890115d545f655beb18e7))
* **deps:** update module github.com/prometheus/client_golang (v1.22.0 → v1.23.2) ([#24](https://github.com/home-operations/tuppr/issues/24)) ([598f522](https://github.com/home-operations/tuppr/commit/598f522422b71efac4caed427e39533806f7fe12))
* **deps:** update module github.com/siderolabs/talos/pkg/machinery (v1.11.6 → v1.12.0) ([#58](https://github.com/home-operations/tuppr/issues/58)) ([f9fc435](https://github.com/home-operations/tuppr/commit/f9fc4359d6f4888ec7bb061662df26e7f1a02599))
* **deps:** update module google.golang.org/grpc (v1.78.0 → v1.79.1) ([#124](https://github.com/home-operations/tuppr/issues/124)) ([f2cd254](https://github.com/home-operations/tuppr/commit/f2cd254c62bb71809b33196a363f80197f79b4cd))
* **deps:** update module sigs.k8s.io/controller-runtime (v0.21.0 → v0.22.1) ([7c951ad](https://github.com/home-operations/tuppr/commit/7c951ad2e4a44b66a2c53c4c1a94ca94cf68cde0))
* **deps:** update module sigs.k8s.io/controller-runtime (v0.22.4 → v0.23.1) ([#76](https://github.com/home-operations/tuppr/issues/76)) ([a56a7f2](https://github.com/home-operations/tuppr/commit/a56a7f2e89c63e385f4b17830a64b43dda702bae))
* e2e tests on hetzner cloud ([#125](https://github.com/home-operations/tuppr/issues/125)) ([21d55ea](https://github.com/home-operations/tuppr/commit/21d55ea85033de001a67cb14efff0a0a59a4c66b))
* first pass at support prometheus metrics ([2b625e0](https://github.com/home-operations/tuppr/commit/2b625e02d77e0e59fa041f0521ea562922270481))
* gha workflow for Hetzner e2e tests ([#126](https://github.com/home-operations/tuppr/issues/126)) ([26303ae](https://github.com/home-operations/tuppr/commit/26303ae6e7353825c8a22c4d1575fc7a9265ddf0))
* **helm:** always create talos sa, pass talosconfig as volume to the controller, update rbac ([1546c86](https://github.com/home-operations/tuppr/commit/1546c861babf9e2abb004cf191b26286ea725312))
* implement log level and update existing log messages ([#121](https://github.com/home-operations/tuppr/issues/121)) ([d8b76d6](https://github.com/home-operations/tuppr/commit/d8b76d61853d4cc59e1f38aed065852daa9923d7))
* initial support for KubernetesUpgrade ([85a790d](https://github.com/home-operations/tuppr/commit/85a790d1d79240f944efb245def42df8aa84ad5c))
* **internal,controller,kubernetesupgrade,talosupgradae:** set job la… ([#62](https://github.com/home-operations/tuppr/issues/62)) ([04aa6d1](https://github.com/home-operations/tuppr/commit/04aa6d1e23e9530d4fbddc81da295c91e9f5e529))
* **mise:** update tool go (1.25.7 → 1.26.0) ([97707e8](https://github.com/home-operations/tuppr/commit/97707e8e3a3508464f72c3491c7bcade4f637692))
* **monitoring:** add prometheus rule to helm charts to alert failed upgrade ([#163](https://github.com/home-operations/tuppr/issues/163)) ([7cccece](https://github.com/home-operations/tuppr/commit/7cccece5e302335743b59a4d42276a697e362659))
* **release:** use release-please ([#143](https://github.com/home-operations/tuppr/issues/143)) ([a9af0aa](https://github.com/home-operations/tuppr/commit/a9af0aa8814490669dcc519b1c86a278f50c598b))
* run healthchecks concurrently ([1d5a915](https://github.com/home-operations/tuppr/commit/1d5a915d194bde63e6ee9bd976723de607d763bd))
* try to prevent talos and kube upgrades running at the same time ([fc0b67b](https://github.com/home-operations/tuppr/commit/fc0b67b39832849d6b5662b46eb6fd2a42e232b1))
* use self-signed cert to remove cert-manager deps ([#92](https://github.com/home-operations/tuppr/issues/92)) ([68e3463](https://github.com/home-operations/tuppr/commit/68e3463a63506e427e60d1362f7946f859b3b8f1))


### Bug Fixes

* (hopefully) stop e2e test dns flakes ([#144](https://github.com/home-operations/tuppr/issues/144)) ([8b6ec55](https://github.com/home-operations/tuppr/commit/8b6ec5558393c7532ea849d584513ed4dbddd34e))
* add healthchecking phase ([#117](https://github.com/home-operations/tuppr/issues/117)) ([c9d5c8d](https://github.com/home-operations/tuppr/commit/c9d5c8da25f0bce70d1fc2995ccb89b127c2a901))
* add retry logic to handle certificate regeneration during upgrades ([35b7072](https://github.com/home-operations/tuppr/commit/35b7072f47d95f61776a903091dccf92177a233f))
* allow repo without tags but not the other way around ([#97](https://github.com/home-operations/tuppr/issues/97)) ([a3eab60](https://github.com/home-operations/tuppr/commit/a3eab60a97d2795b8cb8b7fe4f55445266828198))
* assign args with equals for consistency ([b688d13](https://github.com/home-operations/tuppr/commit/b688d13b6acac730da93ce8bf65edbcca2d7085d))
* cel not find the status field ([8f4544e](https://github.com/home-operations/tuppr/commit/8f4544e7b8b1ebb4c51fef0ac5ca1c9ffc8c3a5e))
* consistency in rbac ([a874d3b](https://github.com/home-operations/tuppr/commit/a874d3ba6cbe03d5939bc4358362561982c89717))
* correctly handle cert creation ([#95](https://github.com/home-operations/tuppr/issues/95)) ([7727182](https://github.com/home-operations/tuppr/commit/77271824bb05243616c1df88f1a72307eb31055e))
* **crds:** remove duplicated enum for kubernetes upgrade job phase ([#141](https://github.com/home-operations/tuppr/issues/141)) ([2f62f50](https://github.com/home-operations/tuppr/commit/2f62f502c3b6c272089033e2990498d373ecd8c0))
* **crds:** remove duplicated validation policy for job phase ([#140](https://github.com/home-operations/tuppr/issues/140)) ([170e0e4](https://github.com/home-operations/tuppr/commit/170e0e4f5c7a4ec10ba0f613b44d1596b585e324))
* crufty old name ([c778f73](https://github.com/home-operations/tuppr/commit/c778f73c93d9eb8229e4688bb9f6c599df49607a))
* **deps:** update kubernetes packages (v0.34.0 → v0.34.1) ([#4](https://github.com/home-operations/tuppr/issues/4)) ([f9126f0](https://github.com/home-operations/tuppr/commit/f9126f05b33a5efff08f3d380b6cdaa2456c3156))
* **deps:** update kubernetes packages (v0.34.1 → v0.34.2) ([#41](https://github.com/home-operations/tuppr/issues/41)) ([ef17215](https://github.com/home-operations/tuppr/commit/ef172159d866059fedb69edb7728d4cf0d1194be))
* **deps:** update kubernetes packages (v0.34.2 → v0.34.3) ([#47](https://github.com/home-operations/tuppr/issues/47)) ([d06bbd3](https://github.com/home-operations/tuppr/commit/d06bbd3c815f9254cfefc0b7e48ea9b43a759a7b))
* **deps:** update kubernetes packages (v0.35.0 → v0.35.1) ([#85](https://github.com/home-operations/tuppr/issues/85)) ([7fd1728](https://github.com/home-operations/tuppr/commit/7fd17280ab78305d7e01810eddbaae0143cb5947))
* **deps:** update kubernetes packages (v0.35.1 → v0.35.2) ([#137](https://github.com/home-operations/tuppr/issues/137)) ([9e03b43](https://github.com/home-operations/tuppr/commit/9e03b439046ee01acd905b398ba9a3b4236b2bd9))
* **deps:** update module github.com/google/cel-go (v0.26.0 → v0.26.1) ([#11](https://github.com/home-operations/tuppr/issues/11)) ([9a0eeaa](https://github.com/home-operations/tuppr/commit/9a0eeaacf72a4b731fdbeccddfd5106b9b572597))
* **deps:** update module github.com/google/go-containerregistry (v0.21.0 → v0.21.1) ([#134](https://github.com/home-operations/tuppr/issues/134)) ([2fae2a5](https://github.com/home-operations/tuppr/commit/2fae2a53d44013dfad4b2b19c572ea68d72310f1))
* **deps:** update module github.com/google/go-containerregistry (v0.21.1 → v0.21.2) ([#153](https://github.com/home-operations/tuppr/issues/153)) ([c26aca4](https://github.com/home-operations/tuppr/commit/c26aca4729dd545a6524501878b182d2c2177dd2))
* **deps:** update module github.com/netresearch/go-cron (v0.13.0 → v0.13.1) ([#173](https://github.com/home-operations/tuppr/issues/173)) ([928f4c6](https://github.com/home-operations/tuppr/commit/928f4c6148cbc046686dd30db9291d153aba36ce))
* **deps:** update module github.com/onsi/ginkgo/v2 (v2.25.1 → v2.25.3) ([#5](https://github.com/home-operations/tuppr/issues/5)) ([840a14c](https://github.com/home-operations/tuppr/commit/840a14cf71ff9ff5c5c2bac4d2690e0aa9955104))
* **deps:** update module github.com/onsi/ginkgo/v2 (v2.27.1 → v2.27.2) ([#31](https://github.com/home-operations/tuppr/issues/31)) ([81dca46](https://github.com/home-operations/tuppr/commit/81dca464e120e4b3f7855bfcad5880b77385f20c))
* **deps:** update module github.com/onsi/ginkgo/v2 (v2.27.2 → v2.27.3) ([#45](https://github.com/home-operations/tuppr/issues/45)) ([f613242](https://github.com/home-operations/tuppr/commit/f613242d83207de378f235b0a8962a44709e0859))
* **deps:** update module github.com/onsi/ginkgo/v2 (v2.27.3 → v2.27.4) ([#69](https://github.com/home-operations/tuppr/issues/69)) ([0d4d0aa](https://github.com/home-operations/tuppr/commit/0d4d0aa2524ee110a0e10ebc1f9c63bfa8bd4959))
* **deps:** update module github.com/onsi/ginkgo/v2 (v2.27.4 → v2.27.5) ([#74](https://github.com/home-operations/tuppr/issues/74)) ([6aa9d7e](https://github.com/home-operations/tuppr/commit/6aa9d7e777f953262f139cc2ba62be114ceeeb52))
* **deps:** update module github.com/onsi/ginkgo/v2 (v2.28.0 → v2.28.1) ([#82](https://github.com/home-operations/tuppr/issues/82)) ([1a01259](https://github.com/home-operations/tuppr/commit/1a0125971bee206193b16c75990742d02b3c4bff))
* **deps:** update module github.com/onsi/gomega (v1.38.2 → v1.38.3) ([#46](https://github.com/home-operations/tuppr/issues/46)) ([4976685](https://github.com/home-operations/tuppr/commit/4976685782d0fe556b8634d10a9df8d192568db6))
* **deps:** update module github.com/onsi/gomega (v1.39.0 → v1.39.1) ([#80](https://github.com/home-operations/tuppr/issues/80)) ([a15112f](https://github.com/home-operations/tuppr/commit/a15112fea0d16ecef60934439a32b160c559f625))
* **deps:** update module github.com/siderolabs/talos/pkg/machinery (v1.11.1 → v1.11.2) ([#19](https://github.com/home-operations/tuppr/issues/19)) ([ecdb319](https://github.com/home-operations/tuppr/commit/ecdb319ce5d865a27c316a6ab8441d98b9c5eed7))
* **deps:** update module github.com/siderolabs/talos/pkg/machinery (v1.11.2 → v1.11.3) ([#26](https://github.com/home-operations/tuppr/issues/26)) ([af1fc93](https://github.com/home-operations/tuppr/commit/af1fc93ba6119a64ad24a75bd1f1166c9d499d78))
* **deps:** update module github.com/siderolabs/talos/pkg/machinery (v1.11.3 → v1.11.4) ([#35](https://github.com/home-operations/tuppr/issues/35)) ([849913b](https://github.com/home-operations/tuppr/commit/849913b562a8b949d6c92719dc8e51396171866e))
* **deps:** update module github.com/siderolabs/talos/pkg/machinery (v1.11.4 → v1.11.5) ([#36](https://github.com/home-operations/tuppr/issues/36)) ([9fa2419](https://github.com/home-operations/tuppr/commit/9fa24190e17cc09c2bb5f377b1c8d81444c5419e))
* **deps:** update module github.com/siderolabs/talos/pkg/machinery (v1.11.5 → v1.11.6) ([#52](https://github.com/home-operations/tuppr/issues/52)) ([4a72544](https://github.com/home-operations/tuppr/commit/4a725443a97d5f133fcb6c091534a481de29efdc))
* **deps:** update module github.com/siderolabs/talos/pkg/machinery (v1.12.0 → v1.12.1) ([#64](https://github.com/home-operations/tuppr/issues/64)) ([d45d83b](https://github.com/home-operations/tuppr/commit/d45d83b93a65a3175a11ca69b1e40e9bd4762829))
* **deps:** update module github.com/siderolabs/talos/pkg/machinery (v1.12.1 → v1.12.3) ([#77](https://github.com/home-operations/tuppr/issues/77)) ([006833f](https://github.com/home-operations/tuppr/commit/006833f5600faa59abc9d021ae208e39d3184ae4))
* **deps:** update module github.com/siderolabs/talos/pkg/machinery (v1.12.3 → v1.12.4) ([#87](https://github.com/home-operations/tuppr/issues/87)) ([322a45a](https://github.com/home-operations/tuppr/commit/322a45a392d25a0ce80472d1a295453fa09d0158))
* **deps:** update module github.com/siderolabs/talos/pkg/machinery (v1.12.4 → v1.12.5) ([#174](https://github.com/home-operations/tuppr/issues/174)) ([47c3bf9](https://github.com/home-operations/tuppr/commit/47c3bf979c6c26a570107a503e68b846a273b67f))
* **deps:** update module google.golang.org/grpc (v1.79.1 → v1.79.2) ([#171](https://github.com/home-operations/tuppr/issues/171)) ([d218224](https://github.com/home-operations/tuppr/commit/d21822432ba2a56806e7c529521e685ef5822cdb))
* **deps:** update module sigs.k8s.io/controller-runtime (v0.22.1 → v0.22.2) ([#23](https://github.com/home-operations/tuppr/issues/23)) ([454a16b](https://github.com/home-operations/tuppr/commit/454a16bd8cec2118b9f6e61fa55711bcadec0f5f))
* **deps:** update module sigs.k8s.io/controller-runtime (v0.22.2 → v0.22.3) ([#25](https://github.com/home-operations/tuppr/issues/25)) ([724160b](https://github.com/home-operations/tuppr/commit/724160b407cc28b9a1762ccd7089654288043c3e))
* **deps:** update module sigs.k8s.io/controller-runtime (v0.22.3 → v0.22.4) ([#33](https://github.com/home-operations/tuppr/issues/33)) ([1ec12ab](https://github.com/home-operations/tuppr/commit/1ec12abee5632b77e3024df1f68c1c7dc8c007f9))
* **deps:** update module sigs.k8s.io/controller-runtime (v0.23.1 → v0.23.2) ([#167](https://github.com/home-operations/tuppr/issues/167)) ([6a812d8](https://github.com/home-operations/tuppr/commit/6a812d837659badbb8dfa23d68bda148e5958488))
* **deps:** update module sigs.k8s.io/controller-runtime (v0.23.2 → v0.23.3) ([#168](https://github.com/home-operations/tuppr/issues/168)) ([b9a876a](https://github.com/home-operations/tuppr/commit/b9a876a68ecf86a9c647ef496fafcd3f97bede41))
* disable secure metrics by default ([2cae70f](https://github.com/home-operations/tuppr/commit/2cae70fbbed5c10569fb9b8771f1925c359d2c5a))
* ensure pod is deleted when job is deleted ([d6e9d54](https://github.com/home-operations/tuppr/commit/d6e9d547d3f95154f645e7860eff5f4f456dc5a1))
* error on talosctl version detection failure instead of falling back to latest ([#130](https://github.com/home-operations/tuppr/issues/130)) ([7d512a5](https://github.com/home-operations/tuppr/commit/7d512a5145f1d4d9b25a0bb84705ba42b658a454))
* forgot to save file ([d21c79b](https://github.com/home-operations/tuppr/commit/d21c79b1ee236d5131d5c448c7fdaccfb7906e57))
* getTalosVersion func ([5e2032a](https://github.com/home-operations/tuppr/commit/5e2032a777cfc098a87d0ad03d780d54227a8a0c))
* handle correctly maintenance window when not all node are updated ([#148](https://github.com/home-operations/tuppr/issues/148)) ([8cf89a7](https://github.com/home-operations/tuppr/commit/8cf89a7ef01e1ddba32e428c3962098bdced4bb1))
* handle missing cert during startup ([#93](https://github.com/home-operations/tuppr/issues/93)) ([2f5a4f9](https://github.com/home-operations/tuppr/commit/2f5a4f9495c949890702005d1015f7e312245419))
* handling of which talosctl image to use ([5e3fd85](https://github.com/home-operations/tuppr/commit/5e3fd85559c47d29c7a39c161f74475671a77c92))
* harden e2e-test resources cleanup ([#131](https://github.com/home-operations/tuppr/issues/131)) ([f60e0f4](https://github.com/home-operations/tuppr/commit/f60e0f4b3c2f3539f7212b5c11dfd12f60ee492a))
* improve phases ([#115](https://github.com/home-operations/tuppr/issues/115)) ([47208c7](https://github.com/home-operations/tuppr/commit/47208c7d4a478f3e9ff844d790613b574b294b1c))
* increase affinity weights and decrease resource requests ([24f427b](https://github.com/home-operations/tuppr/commit/24f427b0f3423327594065262f90ae451922867d))
* it is placement not placementPreset ([ee4f112](https://github.com/home-operations/tuppr/commit/ee4f112d3284daf3301ef377b9482dd79e32be9c))
* **k8s-upgrade:** reprocess partial update ([#109](https://github.com/home-operations/tuppr/issues/109)) ([d893911](https://github.com/home-operations/tuppr/commit/d893911d7d92607939e5f14ca39a7051ec5b2546))
* keep k8s update waiting in case a talos update is pending ([#101](https://github.com/home-operations/tuppr/issues/101)) ([4b9ab74](https://github.com/home-operations/tuppr/commit/4b9ab74347e5a2c5581036672ce54e53cc601fd5))
* **kubeupgrade:** continue to next node after partial job success ([#142](https://github.com/home-operations/tuppr/issues/142)) ([367d108](https://github.com/home-operations/tuppr/commit/367d108999b21422f2058c14afe7c2da6e4aa7df))
* look for active jobs first ([8d8cfe5](https://github.com/home-operations/tuppr/commit/8d8cfe556ddd539141fc28e06a3c4fed83cf1710))
* **metrics:** record previously unused job metrics and clean up on deletion ([#160](https://github.com/home-operations/tuppr/issues/160)) ([1875c37](https://github.com/home-operations/tuppr/commit/1875c373c099b7edcb7088efe20dd2ae51b6b347))
* **metrics:** replace numeric phase encoding with state-set gauge pattern ([69f3940](https://github.com/home-operations/tuppr/commit/69f39402b911c0e7081fca8e29a10c83fa4c3068))
* **metrics:** replace numeric phase encoding with state-set gauge pattern ([#172](https://github.com/home-operations/tuppr/issues/172)) ([67a72ee](https://github.com/home-operations/tuppr/commit/67a72ee1a11b50ab0b0bb4a8c0ec14edbfa71c7c))
* **mise:** update tool go (1.26.0 → 1.26.1) ([8da7632](https://github.com/home-operations/tuppr/commit/8da7632b67b1dcaba41d395a110e9470394a5d50))
* modify resource requests and limits in job ([#40](https://github.com/home-operations/tuppr/issues/40)) ([fbc2c2b](https://github.com/home-operations/tuppr/commit/fbc2c2b40b77019d170242873a003e69ec794cdd))
* only reg k8s controller once ([d349225](https://github.com/home-operations/tuppr/commit/d349225d25fe97795e4a1667a75c0b537ecffa28))
* oomkills and missing current image ([f6ecce7](https://github.com/home-operations/tuppr/commit/f6ecce7005ae378cf7c7d54907608d5ca5804d48))
* **README:** fix talos upgrade troubleshooting command ([#79](https://github.com/home-operations/tuppr/issues/79)) ([9e45798](https://github.com/home-operations/tuppr/commit/9e45798eb3d0c547814263612250170fec854eaa))
* rebooting phase would never be activated ([#122](https://github.com/home-operations/tuppr/issues/122)) ([520c6f9](https://github.com/home-operations/tuppr/commit/520c6f9140d98353f4615214469f45ce6cf2d136))
* register cache index at startup for drain ([#133](https://github.com/home-operations/tuppr/issues/133)) ([045fe50](https://github.com/home-operations/tuppr/commit/045fe5087016ff880980668ca6d34b8487c8a623))
* **release-please:** always update pr with the latest changes ([5f4ff63](https://github.com/home-operations/tuppr/commit/5f4ff630c25ca67d5111c9f059b62c6df9cb8772))
* **release:** exclude v from release tag ([#146](https://github.com/home-operations/tuppr/issues/146)) ([12f39ba](https://github.com/home-operations/tuppr/commit/12f39ba6b177fa6839f386c239987f2d87c0bd5c))
* Remove init container 'health' from controllers ([#17](https://github.com/home-operations/tuppr/issues/17)) ([2a10ca3](https://github.com/home-operations/tuppr/commit/2a10ca38b652b825aa649165d48cf66d817c7b3f))
* retry with new cert when Talos client certificate expires ([#138](https://github.com/home-operations/tuppr/issues/138)) ([2e67aea](https://github.com/home-operations/tuppr/commit/2e67aea71e2962fa4c7b1e1683bdcad371f35f3e))
* secret permission in job ([5ad31a6](https://github.com/home-operations/tuppr/commit/5ad31a63271631e8ff206a0c84225ab4487b8b16))
* secrets can indeed rotate, add refreshing client ([66ee764](https://github.com/home-operations/tuppr/commit/66ee764b9f7b951cc5d7c3bf681b67a86cb65439))
* set default tolerations in helm chart ([91990ff](https://github.com/home-operations/tuppr/commit/91990ffb1b4b7f6a96934588898c4c83dd41f8f3))
* set TALOSCONFIG as env in the job ([6493486](https://github.com/home-operations/tuppr/commit/649348632b5301dc9bbc9814f1811228bb46429e))
* set talosconfig env var in job ([#51](https://github.com/home-operations/tuppr/issues/51)) ([25329c9](https://github.com/home-operations/tuppr/commit/25329c9172eb955c19aa9b58a2a92ced09b1fba3))
* single node clusters would fail to upgrade talos ([#99](https://github.com/home-operations/tuppr/issues/99)) ([35c945d](https://github.com/home-operations/tuppr/commit/35c945df46a650bdf2713584349f8bda9b93eed1))
* sync over rbac changes to the helm chart ([504066e](https://github.com/home-operations/tuppr/commit/504066e4bd9ce95ea55bd69751239672b547776d))
* talos sa changes ([bc02ba2](https://github.com/home-operations/tuppr/commit/bc02ba26bb25aa451d7a59ef8b8e8ee03edf0ddd))
* **talosupgrade:** retry on transient errors ([#113](https://github.com/home-operations/tuppr/issues/113)) ([16e04f8](https://github.com/home-operations/tuppr/commit/16e04f82991420b5440f87133eb5323b3f38dee0))
* tofu flag ordering in e2e cleanup ([f19bcae](https://github.com/home-operations/tuppr/commit/f19bcae320371b3e4bdf04598101376315a1d0a7))
* try to account for single node clusters and set PriorityClassName ([0e6f1a4](https://github.com/home-operations/tuppr/commit/0e6f1a4ea3faaee8f7971ceab5a1cb21d26d0866))
* update talosclient to remove needless logs and simplify checks ([0d375bf](https://github.com/home-operations/tuppr/commit/0d375bfc12f4c7e4c0415162673b870e089dcfe9))
* update talosconfig secret perms to match others ([25c5a46](https://github.com/home-operations/tuppr/commit/25c5a46f99d4a3898eb2007d45895cb596b0439e))
* update timeouts across the board ([0c4f096](https://github.com/home-operations/tuppr/commit/0c4f096e79e8f710838c622b99728019c4775575))
* use github app token to create a new tag to fix ga loop protection ([#151](https://github.com/home-operations/tuppr/issues/151)) ([c264c4a](https://github.com/home-operations/tuppr/commit/c264c4a8b76b97b679d1df2fb8fac7142cb06c4a))
* wait for node to be ready before verifying upgrade ([ab8c02e](https://github.com/home-operations/tuppr/commit/ab8c02e1113f39bc8004c3de7bcbbc6561d46473))
* weight range is onlt 1-100 ([780e7f8](https://github.com/home-operations/tuppr/commit/780e7f82aee08a5e55d491442dd91e62fe1f2fcf))
* workaround for tofu cleanup state lock ([#139](https://github.com/home-operations/tuppr/issues/139)) ([133ae62](https://github.com/home-operations/tuppr/commit/133ae620805cd29ec2ddeab21348aa461dfbe17e))


### Miscellaneous Chores

* add in locking ([2c73df2](https://github.com/home-operations/tuppr/commit/2c73df2adf476882daadcc5295a319aad069dcee))
* add in safer job handling ([dec68b7](https://github.com/home-operations/tuppr/commit/dec68b7a1a82bedebd45fc48374288ba557f8830))
* add kubernetes controller logic ([fb0ddea](https://github.com/home-operations/tuppr/commit/fb0ddea9bc57bc0ee37ca6eb41d00e0522026da4))
* add kubernetes controller logic ([5856da2](https://github.com/home-operations/tuppr/commit/5856da2ae2f19fe69bfd481f3941275b496aa123))
* add mise config ([d363fc2](https://github.com/home-operations/tuppr/commit/d363fc222ecf7cf140681408454bd7bf1fd43bb7))
* add TerminatingOrFailed podreplacementstrat ([a653e51](https://github.com/home-operations/tuppr/commit/a653e51a51f1a92ef39b601e9fa49e357030c3a5))
* add TerminatingOrFailed podreplacementstrat ([d3bcb48](https://github.com/home-operations/tuppr/commit/d3bcb486d61431bc5f8be5d2ac403973de7eee9d))
* add workflow_dispatch trigger to release workflow ([9751b8a](https://github.com/home-operations/tuppr/commit/9751b8ac155cf5ec3f3759266b54c6bc1b90c4cf))
* another commit ([5485944](https://github.com/home-operations/tuppr/commit/54859443729e2ff537b5399ec31502b55f9ad603))
* antiaffinity ([3ea093c](https://github.com/home-operations/tuppr/commit/3ea093ceab78c117305f4025db6d72e59060074e))
* backOffLimit is a thing too claude ([3dfa1cf](https://github.com/home-operations/tuppr/commit/3dfa1cfd492abc3945c1135233ef380fd6812abb))
* charts dir ([12a8a47](https://github.com/home-operations/tuppr/commit/12a8a4799d4e2eb8e4541ebbe35dfd0c38eb6160))
* clean up helm chart crd folder ([ca11100](https://github.com/home-operations/tuppr/commit/ca11100bff655e2de7eb5db0f06b27b7412d2773))
* controller fixes ([246fac4](https://github.com/home-operations/tuppr/commit/246fac43a257de6b1b53727bcab5c31816763b00))
* **deps:** update k8s.io/utils digest (0af2bda → bc988d5) ([#21](https://github.com/home-operations/tuppr/issues/21)) ([c4a44af](https://github.com/home-operations/tuppr/commit/c4a44afe09605db53518ed2e9328ffecf1420933))
* **deps:** update k8s.io/utils digest (0fe9cd7 → 914a6e7) ([#71](https://github.com/home-operations/tuppr/issues/71)) ([887972d](https://github.com/home-operations/tuppr/commit/887972d1b6474623b16f444344fbb41a0d216a17))
* **deps:** update k8s.io/utils digest (383b50a → 718f0e5) ([#60](https://github.com/home-operations/tuppr/issues/60)) ([1cf38d2](https://github.com/home-operations/tuppr/commit/1cf38d22d0333c15b6cba16e2f0db789c5d41551))
* **deps:** update k8s.io/utils digest (3ea5e8c → 0af2bda) ([ce023d9](https://github.com/home-operations/tuppr/commit/ce023d900b965f9b04ef07044eedb9348789b655))
* **deps:** update k8s.io/utils digest (718f0e5 → 0fe9cd7) ([#67](https://github.com/home-operations/tuppr/issues/67)) ([eb953d9](https://github.com/home-operations/tuppr/commit/eb953d91ac2179b4465ffd5cc26940f1f5b77608))
* **deps:** update k8s.io/utils digest (914a6e7 → b8788ab) ([#83](https://github.com/home-operations/tuppr/issues/83)) ([c7c9848](https://github.com/home-operations/tuppr/commit/c7c9848e9d6b14da62d754c56c28dd41a39a07b3))
* **deps:** update k8s.io/utils digest (98d557b → 9d40a56) ([#57](https://github.com/home-operations/tuppr/issues/57)) ([8b80e4d](https://github.com/home-operations/tuppr/commit/8b80e4d5fdb1a2b0022c0d7e8ff09dbfcfd2a513))
* **deps:** update k8s.io/utils digest (9d40a56 → 383b50a) ([#59](https://github.com/home-operations/tuppr/issues/59)) ([6fec7fc](https://github.com/home-operations/tuppr/commit/6fec7fc260b7dd85f1b78bce7b430924b8f978b0))
* **deps:** update k8s.io/utils digest (bc988d5 → 98d557b) ([#55](https://github.com/home-operations/tuppr/issues/55)) ([8fc3bb7](https://github.com/home-operations/tuppr/commit/8fc3bb733af0fe2b415dbd153ba6defc8c0b241e))
* **helm:** support envs, volumes and volume mounts ([#104](https://github.com/home-operations/tuppr/issues/104)) ([5a4cf29](https://github.com/home-operations/tuppr/commit/5a4cf29de69e73333a5b34d5bae0c640c1eee7ee))
* implement nodeSelectorExprs ([f3c6912](https://github.com/home-operations/tuppr/commit/f3c69126535bddc3d36342d0d46d045b6c8e32a0))
* initial commit ([b2308d9](https://github.com/home-operations/tuppr/commit/b2308d95cbf350a837c0c3179e9112cee3427740))
* **main:** release 0.0.76 ([#145](https://github.com/home-operations/tuppr/issues/145)) ([5c7c7bf](https://github.com/home-operations/tuppr/commit/5c7c7bf1b742c657d7f0c93264685881bb480eea))
* **main:** release 0.0.77 ([#147](https://github.com/home-operations/tuppr/issues/147)) ([7db9ce3](https://github.com/home-operations/tuppr/commit/7db9ce334d4787440b615a6d861c5a594a95d9d3))
* **main:** release 0.0.78 ([#149](https://github.com/home-operations/tuppr/issues/149)) ([7e0a491](https://github.com/home-operations/tuppr/commit/7e0a4918f5ea00fb0b59de224b3184671cb83bd8))
* **main:** release 0.0.79 ([#152](https://github.com/home-operations/tuppr/issues/152)) ([b98fe82](https://github.com/home-operations/tuppr/commit/b98fe828b45f5f2fccff3e58faed1ea186a0724d))
* **main:** release 0.0.80 ([#159](https://github.com/home-operations/tuppr/issues/159)) ([896d1ab](https://github.com/home-operations/tuppr/commit/896d1ab92a21496a262b552f40512e2376e51dbd))
* **main:** release 0.1.0 ([#162](https://github.com/home-operations/tuppr/issues/162)) ([2aa2535](https://github.com/home-operations/tuppr/commit/2aa25352e2d305f2ba7be8eee65ccbc2eabb5bb8))
* modify E2E workflow triggers for pull requests ([ee5309f](https://github.com/home-operations/tuppr/commit/ee5309f2850022708318af75de9d4395a5a76c84))
* more workflows ([030fcaf](https://github.com/home-operations/tuppr/commit/030fcafd1b96940a2bf6b5e2daa74a6bee5fba03))
* more workflows ([94db277](https://github.com/home-operations/tuppr/commit/94db2770b381324f5ba71cf8a18c3434612d4088))
* preserve is not a thing anymore ([b034a57](https://github.com/home-operations/tuppr/commit/b034a577a4447a027201243bb426bce143300c4a))
* reduce github action usage by improving starting event ([#116](https://github.com/home-operations/tuppr/issues/116)) ([c1e1196](https://github.com/home-operations/tuppr/commit/c1e1196a4566fac46ed7f8adfe9ee8399b6ba734))
* refactor coordinator rules ([#114](https://github.com/home-operations/tuppr/issues/114)) ([2f5d088](https://github.com/home-operations/tuppr/commit/2f5d08844c7b0f760f0494d313f61a71ff853927))
* refactor to talosupgrade ([3d4eaf9](https://github.com/home-operations/tuppr/commit/3d4eaf9b9c888c71f43fbf1a08990b0b3cca80e5))
* **release-please:** include a bunch of sections for now ([405264f](https://github.com/home-operations/tuppr/commit/405264f3bab1cfaa8f5e0f00258d7b62bf996f6f))
* remove unused func ([a64054d](https://github.com/home-operations/tuppr/commit/a64054d51623bc623be724fa06fb7abf38a5aee8))
* rename group from upgrade to talup ([443f170](https://github.com/home-operations/tuppr/commit/443f17017c598d4bc6ff62faddf3394a25c6828d))
* rename to tuppr ([d3bcc83](https://github.com/home-operations/tuppr/commit/d3bcc83e7baf95982d29c622ec3e8142a55b2cfb))
* Rename volume and volume mount from 'talos' to 'talosconfig' ([d27d722](https://github.com/home-operations/tuppr/commit/d27d7221ad19a8109c28b529e9e23f25104b25e6))
* requeue when waiting for node to be ready ([#106](https://github.com/home-operations/tuppr/issues/106)) ([a7853ef](https://github.com/home-operations/tuppr/commit/a7853ef5449572eefa0eeaf8bbe84f9811cccbb3))
* set release please PRs to draft ([1119902](https://github.com/home-operations/tuppr/commit/11199023672cb4fb3d3a41d8d6bfb559617f6c7d))
* thomas suggestions ([d571db5](https://github.com/home-operations/tuppr/commit/d571db52f605efee45d6753bb5730ef1d012e196))
* thomas suggestions ([385face](https://github.com/home-operations/tuppr/commit/385facedc409e5ae2ca161042c15f4d2a03ebf62))
* thomas suggestions ([cb77265](https://github.com/home-operations/tuppr/commit/cb772657ced7c582bd0255b42d1eef78c44883e1))
* update chart ([382f108](https://github.com/home-operations/tuppr/commit/382f10874e15f87a046ff231ce354a858c590f77))
* update chart ([1891c71](https://github.com/home-operations/tuppr/commit/1891c71929097ecf7b6e403234e34893c5e609fe))
* update controller logic, add release workflow ([cc5bf34](https://github.com/home-operations/tuppr/commit/cc5bf34f14b9e6f3057ca84fd8338ce467137e00))
* update controller logic, add release workflow ([8a1439b](https://github.com/home-operations/tuppr/commit/8a1439bf38f04b48edd077489fdac50868936d1b))
* update generated files ([aae7f85](https://github.com/home-operations/tuppr/commit/aae7f8521785a55c518d384d51cf1cac8bf80f22))
* update manifests ([c07a087](https://github.com/home-operations/tuppr/commit/c07a087986ba021376520b4b920d1f058cb340b3))
* update paths in e2e workflow ([4112844](https://github.com/home-operations/tuppr/commit/41128446689d9530f47f3d7606d439fa4d547c3b))
* update PROJECT ([e660ff7](https://github.com/home-operations/tuppr/commit/e660ff705b88e8d1e842f397ba26c9c4ed6f9d88))
* update readme ([ba2f2db](https://github.com/home-operations/tuppr/commit/ba2f2dbe898390685def0187c2ad371904d50b53))
* update readme ([42e1e80](https://github.com/home-operations/tuppr/commit/42e1e8055dc5505580f1028309c87acc0e55c450))
* update readme ([072581a](https://github.com/home-operations/tuppr/commit/072581a8b612619f735f95aac2c6c054a98f835c))
* update readme ([a032253](https://github.com/home-operations/tuppr/commit/a032253b9164cd7ece180dd91f8195971c5d9a36))
* update readme ([c885182](https://github.com/home-operations/tuppr/commit/c885182b40e5fb9c555edea3d04d12a3e819d9f1))
* update readme ([ed26765](https://github.com/home-operations/tuppr/commit/ed267654134f81f96c188050d864a72d1bfc154f))
* update readme ([28f3aad](https://github.com/home-operations/tuppr/commit/28f3aadcc8dac0fd1d418fec43a8eaa1f9c9e865))
* update readme ([c32389c](https://github.com/home-operations/tuppr/commit/c32389c4bb2f90d1bccc6ee29aa17df1df2056c2))
* update readme ([dc72de3](https://github.com/home-operations/tuppr/commit/dc72de3aed14b9850b1ea7b8ff978153eaf20b3d))
* update readme ([9e5c8eb](https://github.com/home-operations/tuppr/commit/9e5c8eb67e31c44d71cc5c6215ab340fae5c046c))
* update readme ([5f5fff9](https://github.com/home-operations/tuppr/commit/5f5fff9d90828352fb4aa73625c610e1e6ecb3ac))
* update readme ([391632c](https://github.com/home-operations/tuppr/commit/391632c7554672bdb77b6c52ecdc7b29007e710c))
* update readme ([c7536b7](https://github.com/home-operations/tuppr/commit/c7536b71fdcc729976ecadfa12dc93cfd33e573c))
* update readme ([2c11c7f](https://github.com/home-operations/tuppr/commit/2c11c7fc502b3b970fe93a17fdbe3ff9057f4c66))
* update readme with safe upgrade paths ([#118](https://github.com/home-operations/tuppr/issues/118)) ([11b0f4f](https://github.com/home-operations/tuppr/commit/11b0f4ffe3f65f48316324704dd72e20dea7eda1))
* update workflows ([3a3ad23](https://github.com/home-operations/tuppr/commit/3a3ad23bf279ec9093566b6dea50b5d7a86dec4f))
* update workflows ([271d564](https://github.com/home-operations/tuppr/commit/271d564e208c60cdcb303eea9b09ba2fbd027463))
* update workflows ([da48e4d](https://github.com/home-operations/tuppr/commit/da48e4d5e3dc28c2b515e516fcf23e73e2c6d415))
* update workflows ([69ea664](https://github.com/home-operations/tuppr/commit/69ea664ee62649f9200f8718a7c9d98c6efb6bca))
* update workflows ([bf24781](https://github.com/home-operations/tuppr/commit/bf24781a6af98d3737ff77da6f2da9e8b7f4570f))
* updates ([6228b91](https://github.com/home-operations/tuppr/commit/6228b91e3321747acecff0596b0cec4758e9418c))
* updates ([a5f11d4](https://github.com/home-operations/tuppr/commit/a5f11d451227b6b7f33afb3a01f6502b9494b336))
* updates ([bc7a6d8](https://github.com/home-operations/tuppr/commit/bc7a6d89c9f5a020a67ffe26ee117936b7b3db2f))
* updates ([6a0c6de](https://github.com/home-operations/tuppr/commit/6a0c6decbb9dd81bb1e1b28163c7e850391668fa))
* updates ([c6ce77c](https://github.com/home-operations/tuppr/commit/c6ce77cfa7194ea8a100478ac2e8f1dd1fbe4a48))
* updates ([fd7b813](https://github.com/home-operations/tuppr/commit/fd7b813df660920cf8a2a230830eee80108ca6df))
* updates ([cf208cf](https://github.com/home-operations/tuppr/commit/cf208cf6366367e615e8cfbaa2a2b999ad67ac21))
* updates ([3e00d3c](https://github.com/home-operations/tuppr/commit/3e00d3cbfb79b5dbb00e238063d9f01bb5cc370a))
* updates ([407b109](https://github.com/home-operations/tuppr/commit/407b109a0f50c78b76e45925320f882b68505034))
* updates ([8815840](https://github.com/home-operations/tuppr/commit/881584099349cdb23d2cd8ab4085a0a7cea87aa4))
* updates ([7e41b57](https://github.com/home-operations/tuppr/commit/7e41b578fc5bcf88875466fa405f37bd583a3a18))
* updates ([367a70d](https://github.com/home-operations/tuppr/commit/367a70dcc2f360df2c1aee5fe4d5605ca425274c))
* we almost linux kernel now ([cc71b35](https://github.com/home-operations/tuppr/commit/cc71b350578c06c3278d49e1850b863de5f50b6e))
* workflow updates ([c47b9fb](https://github.com/home-operations/tuppr/commit/c47b9fb10edf708768faa7d867ac627f8b5bdf46))
* working talos client? ([39541c2](https://github.com/home-operations/tuppr/commit/39541c2a55dd222740a39cd1f8bc3b0bc49305b9))
* working talos client? ([93391ca](https://github.com/home-operations/tuppr/commit/93391ca0b3c0c0ab650c13d99b1a4f407233961c))


### Code Refactoring

* crd spec again ([c7cf714](https://github.com/home-operations/tuppr/commit/c7cf71427e26a34d046c2e8c5dd6c68a9b66a8ac))
* crds - see README.md ([18ea5bd](https://github.com/home-operations/tuppr/commit/18ea5bdf61613b88031335548d960b476cc89d1c))
* healthCheckExprs to healthChecks ([bc67790](https://github.com/home-operations/tuppr/commit/bc677906dfd292470d88788a58399b8aabfbd43f))
* **jobs:** move duplication into a single package ([#154](https://github.com/home-operations/tuppr/issues/154)) ([dd14644](https://github.com/home-operations/tuppr/commit/dd14644391c0893b534872d7380806ebd9802f60))
* make upgrader follow the same architecture ([#156](https://github.com/home-operations/tuppr/issues/156)) ([3e5a6b2](https://github.com/home-operations/tuppr/commit/3e5a6b243a3ebaa4c8a61e302004a59969e29590))
* make webhook validation logic DRY ([#98](https://github.com/home-operations/tuppr/issues/98)) ([4bc2e39](https://github.com/home-operations/tuppr/commit/4bc2e391a2176bc83a7df29085af5a32f86e4ffe))
* nodeSelectorExprs to nodeSelectorTerms ([2d7c085](https://github.com/home-operations/tuppr/commit/2d7c0852fb073ab93493f2f91062f9758120f715))
* nodeSelectorTerms to matchNodes ([a1a7926](https://github.com/home-operations/tuppr/commit/a1a7926cf125caa02693b951e7bdeba831ee235c))
* only allow one talosupgrade and one kubernetesupgrade per cluster ([d0b59de](https://github.com/home-operations/tuppr/commit/d0b59de3128d16500f0c6a5466e04e7a04b13a4b))
* remove image field from crd and replace with version, also refactor with using the talos sdk instead of soley relying on kubernetes to give us the info we need ([62d4ec3](https://github.com/home-operations/tuppr/commit/62d4ec32abb2d8d9941d7c4200f6a131f13b945b))
* schematic id no longer present in TalosUpgrade ([8c11c35](https://github.com/home-operations/tuppr/commit/8c11c357832183874c3e7cc602e020c49cc9aad8))
* split files structure ([#111](https://github.com/home-operations/tuppr/issues/111)) ([871d474](https://github.com/home-operations/tuppr/commit/871d474ed287c1bfee1b7d069a43d212063f08b8))
* update talosclient to use default talosconfig found in controller ([64ffa55](https://github.com/home-operations/tuppr/commit/64ffa55140bbe24d29bb420fdaf23ce25b5711ee))


### Continuous Integration

* **github-action:** Update action actions/checkout (v5.0.1 → v6.0.0) ([#42](https://github.com/home-operations/tuppr/issues/42)) ([c3edc78](https://github.com/home-operations/tuppr/commit/c3edc7877471f3753f63840e38ceb8246731d80d))
* **github-action:** Update action codex-/return-dispatch (v2.1.0 → v3.0.0) ([#44](https://github.com/home-operations/tuppr/issues/44)) ([6158dfb](https://github.com/home-operations/tuppr/commit/6158dfb1b3b3239ba9e4018b32c3f51c865f4ea8))
* **github-action:** Update action docker/build-push-action (v6.19.2 → v7.0.0) ([#170](https://github.com/home-operations/tuppr/issues/170)) ([75054f8](https://github.com/home-operations/tuppr/commit/75054f873e0d497ef6fb28dad2bc9b3bd90bd95c))
* **github-action:** Update action docker/login-action (v3.7.0 → v4.0.0) ([#161](https://github.com/home-operations/tuppr/issues/161)) ([c51a0df](https://github.com/home-operations/tuppr/commit/c51a0df6f0670a5438da03146229148440711c9e))
* **github-action:** Update action docker/metadata-action (v5.10.0 → v6.0.0) ([#169](https://github.com/home-operations/tuppr/issues/169)) ([6833297](https://github.com/home-operations/tuppr/commit/683329738aec5b274543c231c7b91191113c0f42))
* **github-action:** Update action docker/setup-buildx-action (v3.12.0 → v4.0.0) ([#166](https://github.com/home-operations/tuppr/issues/166)) ([9f1c9f9](https://github.com/home-operations/tuppr/commit/9f1c9f9ef63657793b0feb66d87affeee073bf69))
* **github-action:** Update action golangci/golangci-lint-action (v8.0.0 → v9.0.0) ([#38](https://github.com/home-operations/tuppr/issues/38)) ([5061f7c](https://github.com/home-operations/tuppr/commit/5061f7c9ae6e6955016c9f786deb0798f709e148))
* **github-action:** Update GitHub Artifact Actions (major) ([#136](https://github.com/home-operations/tuppr/issues/136)) ([d74be93](https://github.com/home-operations/tuppr/commit/d74be9300ab2a9809850e30fbd87b18141049b58))
* **github-action:** Update GitHub Artifact Actions (major) ([#30](https://github.com/home-operations/tuppr/issues/30)) ([5de6324](https://github.com/home-operations/tuppr/commit/5de632495227104238985a9362b395f98547387a))
* **github-action:** Update GitHub Artifact Actions (major) ([#49](https://github.com/home-operations/tuppr/issues/49)) ([9b3338d](https://github.com/home-operations/tuppr/commit/9b3338d72ac63c180dd4ef30e72303898b097a9e))

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
