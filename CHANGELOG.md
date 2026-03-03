# Changelog

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
