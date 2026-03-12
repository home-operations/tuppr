# Changelog

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
