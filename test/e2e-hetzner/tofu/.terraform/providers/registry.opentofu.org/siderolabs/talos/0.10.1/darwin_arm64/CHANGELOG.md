## [terraform-provider-talos 0.10.1](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.10.1) (2026-01-19)

Welcome to the v0.10.1 release of terraform-provider-talos!  
*This is a pre-release of terraform-provider-talos*



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### Contributors

* Dennis Witt
* Nikita COEUR
* Noel Georgi
* purajit

### Changes
<details><summary>4 commits</summary>
<p>

* [`bbf6d87`](https://github.com/siderolabs/terraform-provider-talos/commit/bbf6d87deb43ef2c46ae679bf28a61dd59d5e33a) fix(conform): allow multiple commits
* [`c696a93`](https://github.com/siderolabs/terraform-provider-talos/commit/c696a93a67777a04bc762558af443d9c3e024cb5) feat: add staged_if_needing_reboot apply mode for automatic reboot prevention
* [`9313f8e`](https://github.com/siderolabs/terraform-provider-talos/commit/9313f8e373428c5ca414047397f7f604ff428ace) feat: add exact_filters attribute to talos_image_factory_extensions_versions
* [`3c508e1`](https://github.com/siderolabs/terraform-provider-talos/commit/3c508e13869622bdd345ad431de3175b79add753) fix: gracefully handle Unknown config_patches values
</p>
</details>

### Dependency Changes

This release has no dependency changes

Previous release can be found at [v0.10.0](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.10.0)

## [terraform-provider-talos 0.10.0](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.10.0) (2025-12-23)

Welcome to the v0.10.0 release of terraform-provider-talos!  
*This is a pre-release of terraform-provider-talos*



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### config patches

JSON6502 patches are no longer supported, use strategic patches instead.


### Component Updates

Talos sdk: v1.12.0


### Contributors

* Noel Georgi

### Changes
<details><summary>3 commits</summary>
<p>

* [`36facc4`](https://github.com/siderolabs/terraform-provider-talos/commit/36facc45be98895d71b6502f8f1a527d492bf632) chore: bump deps
* [`b45422f`](https://github.com/siderolabs/terraform-provider-talos/commit/b45422f6aa8d84c2097d1b603954c4860197a2b4) release(v0.10.0-beta.0): prepare release
* [`11063bc`](https://github.com/siderolabs/terraform-provider-talos/commit/11063bc25f97c7abb8ae9ceb099bbc523c8655b8) chore: bump deps
</p>
</details>

### Changes since v0.10.0-beta.0
<details><summary>1 commit</summary>
<p>

* [`36facc4`](https://github.com/siderolabs/terraform-provider-talos/commit/36facc45be98895d71b6502f8f1a527d492bf632) chore: bump deps
</p>
</details>

### Dependency Changes

* **github.com/hashicorp/terraform-plugin-docs**                  v0.22.0 -> v0.24.0
* **github.com/hashicorp/terraform-plugin-framework**             v1.15.1 -> v1.17.0
* **github.com/hashicorp/terraform-plugin-framework-timeouts**    v0.5.0 -> v0.7.0
* **github.com/hashicorp/terraform-plugin-framework-validators**  v0.18.0 -> v0.19.0
* **github.com/hashicorp/terraform-plugin-go**                    v0.28.0 -> v0.29.0
* **github.com/hashicorp/terraform-plugin-log**                   v0.9.0 -> v0.10.0
* **github.com/hashicorp/terraform-plugin-sdk/v2**                v2.37.0 -> v2.38.1
* **github.com/hashicorp/terraform-plugin-testing**               v1.13.3 -> v1.14.0
* **github.com/siderolabs/crypto**                                v0.6.3 -> v0.6.4
* **github.com/siderolabs/gen**                                   v0.8.5 -> v0.8.6
* **github.com/siderolabs/image-factory**                         v0.8.3 -> v0.9.0
* **github.com/siderolabs/talos**                                 v1.11.0 -> v1.12.0
* **github.com/siderolabs/talos/pkg/machinery**                   v1.11.0 -> v1.12.0
* **go.yaml.in/yaml/v4**                                          v4.0.0-rc.3 **_new_**
* **golang.org/x/mod**                                            v0.27.0 -> v0.31.0
* **k8s.io/client-go**                                            v0.34.0 -> v0.35.0

Previous release can be found at [v0.9.0](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.9.0)

## [terraform-provider-talos 0.10.0-beta.0](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.10.0-beta.0) (2025-11-28)

Welcome to the v0.10.0-beta.0 release of terraform-provider-talos!  
*This is a pre-release of terraform-provider-talos*



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### config patches

JSON6502 patches are no longer supported, use strategic patches instead.


### Component Updates

Talos sdk: v1.12.0-beta.0


### Contributors

* Noel Georgi

### Changes
<details><summary>1 commit</summary>
<p>

* [`11063bc`](https://github.com/siderolabs/terraform-provider-talos/commit/11063bc25f97c7abb8ae9ceb099bbc523c8655b8) chore: bump deps
</p>
</details>

### Dependency Changes

* **github.com/hashicorp/terraform-plugin-docs**                  v0.22.0 -> v0.24.0
* **github.com/hashicorp/terraform-plugin-framework**             v1.15.1 -> v1.16.1
* **github.com/hashicorp/terraform-plugin-framework-timeouts**    v0.5.0 -> v0.7.0
* **github.com/hashicorp/terraform-plugin-framework-validators**  v0.18.0 -> v0.19.0
* **github.com/hashicorp/terraform-plugin-go**                    v0.28.0 -> v0.29.0
* **github.com/hashicorp/terraform-plugin-log**                   v0.9.0 -> v0.10.0
* **github.com/hashicorp/terraform-plugin-sdk/v2**                v2.37.0 -> v2.38.1
* **github.com/siderolabs/crypto**                                v0.6.3 -> v0.6.4
* **github.com/siderolabs/gen**                                   v0.8.5 -> v0.8.6
* **github.com/siderolabs/image-factory**                         v0.8.3 -> v0.9.0
* **github.com/siderolabs/talos**                                 v1.11.0 -> v1.12.0-beta.0
* **github.com/siderolabs/talos/pkg/machinery**                   v1.11.0 -> v1.12.0-beta.0
* **go.yaml.in/yaml/v4**                                          v4.0.0-rc.3 **_new_**
* **golang.org/x/mod**                                            v0.27.0 -> v0.30.0
* **k8s.io/client-go**                                            v0.34.0 -> v0.35.0-alpha.3

Previous release can be found at [v0.9.0](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.9.0)

## [terraform-provider-talos 0.9.0-alpha.0](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.9.0-alpha.0) (2025-05-19)

Welcome to the v0.9.0-alpha.0 release of terraform-provider-talos!  
*This is a pre-release of terraform-provider-talos*



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### talos_machine_disks Data Source

The `talos_machine_disks` data source has been updated to use the better CEL expression language.
The resource attributes and selector have been updated to use the new syntax.
The user would need to update the data source in their configuration to use the new syntax.
The expression syntax is documented in the [CEL documentation](https://www.talos.dev/latest/talos-guides/configuration/disk-management/#disk-selector).

This also brings in consistency with how disks are reported in Talos.


### Component Updates

Talos sdk: v1.11.0-alpha.0


### Contributors

* Noel Georgi
* Halvdan Hoem Grelland
* obvionaoe

### Changes
<details><summary>7 commits</summary>
<p>

* [`4016c0c`](https://github.com/siderolabs/terraform-provider-talos/commit/4016c0cc890585c6343829b99d23a22c4fb4bb42) fix: secureboot installer urls for non-metal platform
* [`34f3f1e`](https://github.com/siderolabs/terraform-provider-talos/commit/34f3f1e74a5e7b65cc82a5c46b5929fbde530790) chore: simplify disk selector code
* [`93070aa`](https://github.com/siderolabs/terraform-provider-talos/commit/93070aaa166aa2ba81a3322bac2de4b9ef927319) feat: use CEL expression filters for `talos_machine_disks`
* [`f70e10e`](https://github.com/siderolabs/terraform-provider-talos/commit/f70e10e97d81a1b211c7d09dd3b04156ece70d1a) fix: allow talos_version to be used without a `v` prefix
* [`fa8002d`](https://github.com/siderolabs/terraform-provider-talos/commit/fa8002d47d35c558ae810e50f6ed0beaa759454a) chore: bump deps
* [`e76002d`](https://github.com/siderolabs/terraform-provider-talos/commit/e76002d6bf47b39d474cc5bc01c0b919afb20046) fix: factory installer urls
* [`44eec1c`](https://github.com/siderolabs/terraform-provider-talos/commit/44eec1cc87eeae3d5eaaf45ea742807eea32367a) fix: image factory examples and docs
</p>
</details>

### Dependency Changes

* **github.com/hashicorp/terraform-plugin-framework-validators**  v0.17.0 -> v0.18.0
* **github.com/siderolabs/gen**                                   v0.8.0 -> v0.8.1
* **github.com/siderolabs/image-factory**                         v0.6.9 -> v0.7.0
* **github.com/siderolabs/talos**                                 v1.10.0 -> v1.11.0-alpha.0
* **github.com/siderolabs/talos/pkg/machinery**                   v1.10.0 -> v1.11.0-alpha.0

Previous release can be found at [v0.8.1](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.8.1)

## [terraform-provider-talos 0.8.0-alpha.0](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.8.0-alpha.0) (2025-04-18)

Welcome to the v0.8.0-alpha.0 release of terraform-provider-talos!  
*This is a pre-release of terraform-provider-talos*



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### Component Updates

Talos sdk: v1.10.0-beta.0


### Contributors

* Noel Georgi
* Andrey Smirnov
* Matt Willsher

### Changes
<details><summary>5 commits</summary>
<p>

* [`2cfcf88`](https://github.com/siderolabs/terraform-provider-talos/commit/2cfcf8802e61550ad7570aa8cbd44439700fd677) chore: bump deps
* [`46ab81c`](https://github.com/siderolabs/terraform-provider-talos/commit/46ab81ca8b799e5390cf398a4a65210d2425a41e) fix: drop talos<->k8s compatibility check
* [`69596f1`](https://github.com/siderolabs/terraform-provider-talos/commit/69596f1e4b74a1d0f95e663315724d9e8150b5b0) fix: skip if aggregator certs are nil
* [`d3214dc`](https://github.com/siderolabs/terraform-provider-talos/commit/d3214dc15659de2906c1aaf2912e77a90dfaca7b) refactor: pull platform metadata from Talos machinery
* [`5c0ff77`](https://github.com/siderolabs/terraform-provider-talos/commit/5c0ff772a72339827bbd63d3fe59c52fb73148de) feat: add secure boot support to non-metal image factory urls
</p>
</details>

### Dependency Changes

* **github.com/hashicorp/terraform-plugin-docs**                  v0.20.1 -> v0.21.0
* **github.com/hashicorp/terraform-plugin-framework**             v1.13.0 -> v1.14.1
* **github.com/hashicorp/terraform-plugin-framework-timeouts**    v0.4.1 -> v0.5.0
* **github.com/hashicorp/terraform-plugin-framework-validators**  v0.16.0 -> v0.17.0
* **github.com/hashicorp/terraform-plugin-go**                    v0.25.0 -> v0.26.0
* **github.com/hashicorp/terraform-plugin-sdk/v2**                v2.35.0 -> v2.36.1
* **github.com/hashicorp/terraform-plugin-testing**               v1.11.0 -> v1.12.0
* **github.com/siderolabs/image-factory**                         v0.6.4 -> v0.6.8
* **github.com/siderolabs/talos**                                 v1.9.2 -> v1.10.0-beta.0
* **github.com/siderolabs/talos/pkg/machinery**                   v1.9.2 -> v1.10.0-beta.0
* **golang.org/x/mod**                                            v0.22.0 -> v0.24.0
* **k8s.io/client-go**                                            v0.32.0 -> v0.33.0-rc.0

Previous release can be found at [v0.7.1](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.7.1)

## [terraform-provider-talos 0.8.0-alpha.0](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.8.0-alpha.0) (2025-04-18)

Welcome to the v0.8.0-alpha.0 release of terraform-provider-talos!



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### Component Updates

Talos sdk: v1.10.0-beta.0


### Contributors

* Noel Georgi
* Andrey Smirnov
* Matt Willsher

### Changes
<details><summary>5 commits</summary>
<p>

* [`2cfcf88`](https://github.com/siderolabs/terraform-provider-talos/commit/2cfcf8802e61550ad7570aa8cbd44439700fd677) chore: bump deps
* [`46ab81c`](https://github.com/siderolabs/terraform-provider-talos/commit/46ab81ca8b799e5390cf398a4a65210d2425a41e) fix: drop talos<->k8s compatibility check
* [`69596f1`](https://github.com/siderolabs/terraform-provider-talos/commit/69596f1e4b74a1d0f95e663315724d9e8150b5b0) fix: skip if aggregator certs are nil
* [`d3214dc`](https://github.com/siderolabs/terraform-provider-talos/commit/d3214dc15659de2906c1aaf2912e77a90dfaca7b) refactor: pull platform metadata from Talos machinery
* [`5c0ff77`](https://github.com/siderolabs/terraform-provider-talos/commit/5c0ff772a72339827bbd63d3fe59c52fb73148de) feat: add secure boot support to non-metal image factory urls
</p>
</details>

### Dependency Changes

* **github.com/hashicorp/terraform-plugin-docs**                  v0.20.1 -> v0.21.0
* **github.com/hashicorp/terraform-plugin-framework**             v1.13.0 -> v1.14.1
* **github.com/hashicorp/terraform-plugin-framework-timeouts**    v0.4.1 -> v0.5.0
* **github.com/hashicorp/terraform-plugin-framework-validators**  v0.16.0 -> v0.17.0
* **github.com/hashicorp/terraform-plugin-go**                    v0.25.0 -> v0.26.0
* **github.com/hashicorp/terraform-plugin-sdk/v2**                v2.35.0 -> v2.36.1
* **github.com/hashicorp/terraform-plugin-testing**               v1.11.0 -> v1.12.0
* **github.com/siderolabs/image-factory**                         v0.6.4 -> v0.6.8
* **github.com/siderolabs/talos**                                 v1.9.2 -> v1.10.0-beta.0
* **github.com/siderolabs/talos/pkg/machinery**                   v1.9.2 -> v1.10.0-beta.0
* **golang.org/x/mod**                                            v0.22.0 -> v0.24.0
* **k8s.io/client-go**                                            v0.32.0 -> v0.33.0-rc.0

Previous release can be found at [v0.7.1](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.7.1)

## [terraform-provider-talos 0.7.0-alpha.0](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.7.0-alpha.0) (2024-10-21)

Welcome to the v0.7.0-alpha.0 release of terraform-provider-talos!



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### Component Updates

Talos sdk: v1.9.0-alpha.0


### Contributors

* Noel Georgi

### Changes
<details><summary>1 commit</summary>
<p>

* [`beb5e2b`](https://github.com/siderolabs/terraform-provider-talos/commit/beb5e2b38466116da1d212dea33f5d7926655442) chore: bump deps
</p>
</details>

### Dependency Changes

* **github.com/hashicorp/terraform-plugin-framework-validators**  v0.13.0 -> v0.14.0
* **github.com/siderolabs/talos**                                 v1.8.1 -> v1.9.0-alpha.0
* **github.com/siderolabs/talos/pkg/machinery**                   v1.8.1 -> v1.9.0-alpha.0

Previous release can be found at [v0.6.1](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.6.1)

## [terraform-provider-talos 0.6.1](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.6.1) (2024-10-18)

Welcome to the v0.6.1 release of terraform-provider-talos!



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### Talos Cluster Kubeconfig

The `talos_cluster_kubeconfig` resource now supports confiuring the certificate renewal check time.


### Component Updates

Talos sdk: v1.8.1


### Contributors

* Noel Georgi

### Changes
<details><summary>3 commits</summary>
<p>

* [`daddfb7`](https://github.com/siderolabs/terraform-provider-talos/commit/daddfb76cec78135ad62e8e46f4d965eb6eed52a) feat: configurable cert refresh time for kubeconfig
* [`96c9a85`](https://github.com/siderolabs/terraform-provider-talos/commit/96c9a857f9df8a28b7f32f1d0cf948a74a8acfcb) fix: resourceplanmodifiers for kubeconfig resource
* [`800573b`](https://github.com/siderolabs/terraform-provider-talos/commit/800573b841c901f562b8b2b151e58d86516944d0) fix: disks wipe on destroy
</p>
</details>

### Dependency Changes

* **github.com/hashicorp/terraform-plugin-framework**  v1.11.0 -> v1.12.0
* **github.com/hashicorp/terraform-plugin-go**         v0.23.0 -> v0.24.0
* **github.com/siderolabs/crypto**                     v0.4.4 -> v0.5.0
* **github.com/siderolabs/go-blockdevice**             v0.4.7 -> v0.4.8
* **github.com/siderolabs/talos**                      v1.8.0 -> v1.8.1
* **github.com/siderolabs/talos/pkg/machinery**        v1.8.0 -> v1.8.1

Previous release can be found at [v0.6.0](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.6.0)


## [terraform-provider-talos 0.6.0](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.6.0) (2024-09-23)

Welcome to the v0.6.0 release of terraform-provider-talos!



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### Image Factory

Support for querying info from Image Factory and registering schematics is now supported via new Terraform resources.


### Talos Cluster Health

`talos_cluster_health` data source now has a way to skip running the Kubernetes components health check by setting `skip_kubernetes_checks` to `true`.


### Talos Cluster Kubeconfig

`talos_cluster_kubeconfig` data source is now deprecated and will be removed in the next minor release.
Use `talos_cluster_kubeconfig` resource instead.
The `talos_cluster_kubeconfig` resource will regenerate kubernetes client config when the time to expiry is less than a month.


### Talos Machine Configuration Data Source

`talos_machine_configuration` data source now defaults to generating config with documentation and examples disabled.

To restore the previous behavior, set `docs` and `examples` attributes to `true`.


### Talos Machine Configuration Apply

`talos_machine_configuration_apply` resource now optionally supports resetting the machine back to maintenance mode.


### Talos Machine Secrets

`talos_machine_secrets` resource now regenerates client config when the time to expiry is less than a month.


### Component Updates

Talos sdk: v1.8.0-alpha.1


### Contributors

* Noel Georgi
* Bruno Bigras
* Hippie Hacker

### Changes
<details><summary>20 commits</summary>
<p>

* [`758df67`](https://github.com/siderolabs/terraform-provider-talos/commit/758df676873a53d0833b79f74f74c1d0cfbb1f12) docs: machine_configuration fix typo
* [`4e01809`](https://github.com/siderolabs/terraform-provider-talos/commit/4e0180983319098412faab94b18a97ca43cae192) release(v0.6.0-beta.0): prepare release
* [`8dcab7b`](https://github.com/siderolabs/terraform-provider-talos/commit/8dcab7bc766edf0b386c1a061a71f3541c75b56d) chore: bump deps
* [`3b59c4a`](https://github.com/siderolabs/terraform-provider-talos/commit/3b59c4a34fc3b1b824bf104981ac70ebc5e6e722) release(v0.6.0-alpha.2): prepare release
* [`858f7f3`](https://github.com/siderolabs/terraform-provider-talos/commit/858f7f3de9368604b8f18125420d6888606b21be) chore: bump deps
* [`7af49b3`](https://github.com/siderolabs/terraform-provider-talos/commit/7af49b315e4b37b7a165c6ef5d5dc2830dc00a6f) chore: better health checks from talos sdk
* [`063b7ae`](https://github.com/siderolabs/terraform-provider-talos/commit/063b7aeb9dc69782ec7cf541433f7b1eb92e4197) chore: default talos_machine_configuration docs and examples to false
* [`80c5534`](https://github.com/siderolabs/terraform-provider-talos/commit/80c5534fbd876506201f21b50c950ce053402a55) chore: support filtering stable version
* [`b9c7f5f`](https://github.com/siderolabs/terraform-provider-talos/commit/b9c7f5f8aedaca50a4e694e022cae456cd44d065) release(v0.6.0-alpha.1): prepare release
* [`92fcb70`](https://github.com/siderolabs/terraform-provider-talos/commit/92fcb70cd9c1d6cfb5bd3ea159e56fa493fe62db) feat: add `talos_image_factory_url` data source
* [`ffc8102`](https://github.com/siderolabs/terraform-provider-talos/commit/ffc8102dd7f7b0929dc54fa1f50afd9b2cfda6be) feat: add factory support
* [`d6c2a0b`](https://github.com/siderolabs/terraform-provider-talos/commit/d6c2a0bc939de41252d97f09b8e9dfcff3377450) chore: fix goreleaser config
* [`dcdcee6`](https://github.com/siderolabs/terraform-provider-talos/commit/dcdcee62f8d9429cc0416dfdb92dfc27391da0ee) release(v0.6.0-alpha.0): prepare release
* [`d962913`](https://github.com/siderolabs/terraform-provider-talos/commit/d9629133c03ef09d5a6136b0e6cc1624a7ef4c28) chore: reset options for `machine_configuration_apply` resource
* [`f26a591`](https://github.com/siderolabs/terraform-provider-talos/commit/f26a5911bdd243fd384a353de1c0140de256211c) chore: data source -> resource `talos_cluster_kubeconfig`
* [`78fd0d3`](https://github.com/siderolabs/terraform-provider-talos/commit/78fd0d369ff401fbd795609cbbc2275f7d108bb0) chore: ignore version prefix for `talos_version`
* [`11ae330`](https://github.com/siderolabs/terraform-provider-talos/commit/11ae33002bee7a3e319bc7f9ea7555ca3ebaa120) feat: support skipping k8s health checks
* [`0fe1a6f`](https://github.com/siderolabs/terraform-provider-talos/commit/0fe1a6fe8d4440f72d7c553cf961b0de7267404b) docs: update description of `talos_cluster_health`
* [`f6f1811`](https://github.com/siderolabs/terraform-provider-talos/commit/f6f1811e90c9eef91f562f1f8b15f78e984315a0) feat: regenerate talosconfig
* [`501c78e`](https://github.com/siderolabs/terraform-provider-talos/commit/501c78eb7403012c90229bfc24399d0e1603289b) chore: bump deps
</p>
</details>

### Changes since v0.6.0-beta.0
<details><summary>1 commit</summary>
<p>

* [`758df67`](https://github.com/siderolabs/terraform-provider-talos/commit/758df676873a53d0833b79f74f74c1d0cfbb1f12) docs: machine_configuration fix typo
</p>
</details>

### Dependency Changes

* **github.com/blang/semver/v4**                                  v4.0.0 **_new_**
* **github.com/hashicorp/terraform-plugin-docs**                  v0.19.0 -> v0.19.4
* **github.com/hashicorp/terraform-plugin-framework**             v1.7.0 -> v1.11.0
* **github.com/hashicorp/terraform-plugin-framework-validators**  v0.12.0 -> v0.13.0
* **github.com/hashicorp/terraform-plugin-go**                    v0.22.1 -> v0.23.0
* **github.com/hashicorp/terraform-plugin-log**                   v0.9.0 **_new_**
* **github.com/hashicorp/terraform-plugin-sdk/v2**                v2.33.0 -> v2.34.0
* **github.com/hashicorp/terraform-plugin-testing**               v1.7.0 -> v1.10.0
* **github.com/siderolabs/gen**                                   v0.4.8 -> v0.5.0
* **github.com/siderolabs/image-factory**                         v0.5.0 **_new_**
* **github.com/siderolabs/talos**                                 v1.8.0-beta.0 **_new_**
* **github.com/siderolabs/talos/pkg/machinery**                   v1.7.0 -> v1.8.0-beta.0
* **golang.org/x/mod**                                            v0.17.0 -> v0.21.0
* **k8s.io/client-go**                                            v0.29.3 -> v0.31.0

Previous release can be found at [v0.5.0](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.5.0)

## [terraform-provider-talos 0.6.0-beta.0](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.6.0-beta.0) (2024-09-10)

Welcome to the v0.6.0-beta.0 release of terraform-provider-talos!  
*This is a pre-release of terraform-provider-talos*



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### Image Factory

Support for querying info from Image Factory and registering schematics is now supported via new Terraform resources.


### Talos Cluster Health

`talos_cluster_health` data source now has a way to skip running the Kubernetes components health check by setting `skip_kubernetes_checks` to `true`.


### Talos Cluster Kubeconfig

`talos_cluster_kubeconfig` data source is now deprecated and will be removed in the next minor release.
Use `talos_cluster_kubeconfig` resource instead.
The `talos_cluster_kubeconfig` resource will regenerate kubernetes client config when the time to expiry is less than a month.


### Talos Machine Configuration Data Source

`talos_machine_configuration` data source now defaults to generating config with documentation and examples disabled.

To restore the previous behavior, set `docs` and `examples` attributes to `true`.


### Talos Machine Configuration Apply

`talos_machine_configuration_apply` resource now optionally supports resetting the machine back to maintenance mode.


### Talos Machine Secrets

`talos_machine_secrets` resource now regenerates client config when the time to expiry is less than a month.


### Component Updates

Talos sdk: v1.8.0-alpha.1


### Contributors

* Noel Georgi
* Hippie Hacker

### Changes
<details><summary>18 commits</summary>
<p>

* [`8dcab7b`](https://github.com/siderolabs/terraform-provider-talos/commit/8dcab7bc766edf0b386c1a061a71f3541c75b56d) chore: bump deps
* [`3b59c4a`](https://github.com/siderolabs/terraform-provider-talos/commit/3b59c4a34fc3b1b824bf104981ac70ebc5e6e722) release(v0.6.0-alpha.2): prepare release
* [`858f7f3`](https://github.com/siderolabs/terraform-provider-talos/commit/858f7f3de9368604b8f18125420d6888606b21be) chore: bump deps
* [`7af49b3`](https://github.com/siderolabs/terraform-provider-talos/commit/7af49b315e4b37b7a165c6ef5d5dc2830dc00a6f) chore: better health checks from talos sdk
* [`063b7ae`](https://github.com/siderolabs/terraform-provider-talos/commit/063b7aeb9dc69782ec7cf541433f7b1eb92e4197) chore: default talos_machine_configuration docs and examples to false
* [`80c5534`](https://github.com/siderolabs/terraform-provider-talos/commit/80c5534fbd876506201f21b50c950ce053402a55) chore: support filtering stable version
* [`b9c7f5f`](https://github.com/siderolabs/terraform-provider-talos/commit/b9c7f5f8aedaca50a4e694e022cae456cd44d065) release(v0.6.0-alpha.1): prepare release
* [`92fcb70`](https://github.com/siderolabs/terraform-provider-talos/commit/92fcb70cd9c1d6cfb5bd3ea159e56fa493fe62db) feat: add `talos_image_factory_url` data source
* [`ffc8102`](https://github.com/siderolabs/terraform-provider-talos/commit/ffc8102dd7f7b0929dc54fa1f50afd9b2cfda6be) feat: add factory support
* [`d6c2a0b`](https://github.com/siderolabs/terraform-provider-talos/commit/d6c2a0bc939de41252d97f09b8e9dfcff3377450) chore: fix goreleaser config
* [`dcdcee6`](https://github.com/siderolabs/terraform-provider-talos/commit/dcdcee62f8d9429cc0416dfdb92dfc27391da0ee) release(v0.6.0-alpha.0): prepare release
* [`d962913`](https://github.com/siderolabs/terraform-provider-talos/commit/d9629133c03ef09d5a6136b0e6cc1624a7ef4c28) chore: reset options for `machine_configuration_apply` resource
* [`f26a591`](https://github.com/siderolabs/terraform-provider-talos/commit/f26a5911bdd243fd384a353de1c0140de256211c) chore: data source -> resource `talos_cluster_kubeconfig`
* [`78fd0d3`](https://github.com/siderolabs/terraform-provider-talos/commit/78fd0d369ff401fbd795609cbbc2275f7d108bb0) chore: ignore version prefix for `talos_version`
* [`11ae330`](https://github.com/siderolabs/terraform-provider-talos/commit/11ae33002bee7a3e319bc7f9ea7555ca3ebaa120) feat: support skipping k8s health checks
* [`0fe1a6f`](https://github.com/siderolabs/terraform-provider-talos/commit/0fe1a6fe8d4440f72d7c553cf961b0de7267404b) docs: update description of `talos_cluster_health`
* [`f6f1811`](https://github.com/siderolabs/terraform-provider-talos/commit/f6f1811e90c9eef91f562f1f8b15f78e984315a0) feat: regenerate talosconfig
* [`501c78e`](https://github.com/siderolabs/terraform-provider-talos/commit/501c78eb7403012c90229bfc24399d0e1603289b) chore: bump deps
</p>
</details>

### Changes since v0.6.0-alpha.2
<details><summary>1 commit</summary>
<p>

* [`8dcab7b`](https://github.com/siderolabs/terraform-provider-talos/commit/8dcab7bc766edf0b386c1a061a71f3541c75b56d) chore: bump deps
</p>
</details>

### Dependency Changes

* **github.com/blang/semver/v4**                                  v4.0.0 **_new_**
* **github.com/hashicorp/terraform-plugin-docs**                  v0.19.0 -> v0.19.4
* **github.com/hashicorp/terraform-plugin-framework**             v1.7.0 -> v1.11.0
* **github.com/hashicorp/terraform-plugin-framework-validators**  v0.12.0 -> v0.13.0
* **github.com/hashicorp/terraform-plugin-go**                    v0.22.1 -> v0.23.0
* **github.com/hashicorp/terraform-plugin-log**                   v0.9.0 **_new_**
* **github.com/hashicorp/terraform-plugin-sdk/v2**                v2.33.0 -> v2.34.0
* **github.com/hashicorp/terraform-plugin-testing**               v1.7.0 -> v1.10.0
* **github.com/siderolabs/gen**                                   v0.4.8 -> v0.5.0
* **github.com/siderolabs/image-factory**                         v0.5.0 **_new_**
* **github.com/siderolabs/talos**                                 v1.8.0-beta.0 **_new_**
* **github.com/siderolabs/talos/pkg/machinery**                   v1.7.0 -> v1.8.0-beta.0
* **golang.org/x/mod**                                            v0.17.0 -> v0.21.0
* **k8s.io/client-go**                                            v0.29.3 -> v0.31.0

Previous release can be found at [v0.5.0](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.5.0)

## [terraform-provider-talos 0.6.0-alpha.2](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.6.0-alpha.2) (2024-09-02)

Welcome to the v0.6.0-alpha.2 release of terraform-provider-talos!  
*This is a pre-release of terraform-provider-talos*



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### Image Factory

Support for querying info from Image Factory and registering schematics is now supported via new Terraform resources.


### Talos Cluster Health

`talos_cluster_health` data source now has a way to skip running the Kubernetes components health check by setting `skip_kubernetes_checks` to `true`.


### Talos Cluster Kubeconfig

`talos_cluster_kubeconfig` data source is now deprecated and will be removed in the next minor release.
Use `talos_cluster_kubeconfig` resource instead.
The `talos_cluster_kubeconfig` resource will regenerate kubernetes client config when the time to expiry is less than a month.


### Talos Machine Configuration Data Source

`talos_machine_configuration` data source now defaults to generating config with documentation and examples disabled.

To restore the previous behavior, set `docs` and `examples` attributes to `true`.


### Talos Machine Configuration Apply

`talos_machine_configuration_apply` resource now optionally supports resetting the machine back to maintenance mode.


### Talos Machine Secrets

`talos_machine_secrets` resource now regenerates client config when the time to expiry is less than a month.


### Component Updates

Talos sdk: v1.8.0-alpha.1


### Contributors

* Noel Georgi
* Hippie Hacker

### Changes
<details><summary>16 commits</summary>
<p>

* [`858f7f3`](https://github.com/siderolabs/terraform-provider-talos/commit/858f7f3de9368604b8f18125420d6888606b21be) chore: bump deps
* [`7af49b3`](https://github.com/siderolabs/terraform-provider-talos/commit/7af49b315e4b37b7a165c6ef5d5dc2830dc00a6f) chore: better health checks from talos sdk
* [`063b7ae`](https://github.com/siderolabs/terraform-provider-talos/commit/063b7aeb9dc69782ec7cf541433f7b1eb92e4197) chore: default talos_machine_configuration docs and examples to false
* [`80c5534`](https://github.com/siderolabs/terraform-provider-talos/commit/80c5534fbd876506201f21b50c950ce053402a55) chore: support filtering stable version
* [`b9c7f5f`](https://github.com/siderolabs/terraform-provider-talos/commit/b9c7f5f8aedaca50a4e694e022cae456cd44d065) release(v0.6.0-alpha.1): prepare release
* [`92fcb70`](https://github.com/siderolabs/terraform-provider-talos/commit/92fcb70cd9c1d6cfb5bd3ea159e56fa493fe62db) feat: add `talos_image_factory_url` data source
* [`ffc8102`](https://github.com/siderolabs/terraform-provider-talos/commit/ffc8102dd7f7b0929dc54fa1f50afd9b2cfda6be) feat: add factory support
* [`d6c2a0b`](https://github.com/siderolabs/terraform-provider-talos/commit/d6c2a0bc939de41252d97f09b8e9dfcff3377450) chore: fix goreleaser config
* [`dcdcee6`](https://github.com/siderolabs/terraform-provider-talos/commit/dcdcee62f8d9429cc0416dfdb92dfc27391da0ee) release(v0.6.0-alpha.0): prepare release
* [`d962913`](https://github.com/siderolabs/terraform-provider-talos/commit/d9629133c03ef09d5a6136b0e6cc1624a7ef4c28) chore: reset options for `machine_configuration_apply` resource
* [`f26a591`](https://github.com/siderolabs/terraform-provider-talos/commit/f26a5911bdd243fd384a353de1c0140de256211c) chore: data source -> resource `talos_cluster_kubeconfig`
* [`78fd0d3`](https://github.com/siderolabs/terraform-provider-talos/commit/78fd0d369ff401fbd795609cbbc2275f7d108bb0) chore: ignore version prefix for `talos_version`
* [`11ae330`](https://github.com/siderolabs/terraform-provider-talos/commit/11ae33002bee7a3e319bc7f9ea7555ca3ebaa120) feat: support skipping k8s health checks
* [`0fe1a6f`](https://github.com/siderolabs/terraform-provider-talos/commit/0fe1a6fe8d4440f72d7c553cf961b0de7267404b) docs: update description of `talos_cluster_health`
* [`f6f1811`](https://github.com/siderolabs/terraform-provider-talos/commit/f6f1811e90c9eef91f562f1f8b15f78e984315a0) feat: regenerate talosconfig
* [`501c78e`](https://github.com/siderolabs/terraform-provider-talos/commit/501c78eb7403012c90229bfc24399d0e1603289b) chore: bump deps
</p>
</details>

### Changes since v0.6.0-alpha.1
<details><summary>4 commits</summary>
<p>

* [`858f7f3`](https://github.com/siderolabs/terraform-provider-talos/commit/858f7f3de9368604b8f18125420d6888606b21be) chore: bump deps
* [`7af49b3`](https://github.com/siderolabs/terraform-provider-talos/commit/7af49b315e4b37b7a165c6ef5d5dc2830dc00a6f) chore: better health checks from talos sdk
* [`063b7ae`](https://github.com/siderolabs/terraform-provider-talos/commit/063b7aeb9dc69782ec7cf541433f7b1eb92e4197) chore: default talos_machine_configuration docs and examples to false
* [`80c5534`](https://github.com/siderolabs/terraform-provider-talos/commit/80c5534fbd876506201f21b50c950ce053402a55) chore: support filtering stable version
</p>
</details>

### Dependency Changes

* **github.com/blang/semver/v4**                                  v4.0.0 **_new_**
* **github.com/hashicorp/terraform-plugin-docs**                  v0.19.0 -> v0.19.4
* **github.com/hashicorp/terraform-plugin-framework**             v1.7.0 -> v1.11.0
* **github.com/hashicorp/terraform-plugin-framework-validators**  v0.12.0 -> v0.13.0
* **github.com/hashicorp/terraform-plugin-go**                    v0.22.1 -> v0.23.0
* **github.com/hashicorp/terraform-plugin-log**                   v0.9.0 **_new_**
* **github.com/hashicorp/terraform-plugin-sdk/v2**                v2.33.0 -> v2.34.0
* **github.com/hashicorp/terraform-plugin-testing**               v1.7.0 -> v1.10.0
* **github.com/siderolabs/gen**                                   v0.4.8 -> v0.5.0
* **github.com/siderolabs/image-factory**                         9687413a9a85 **_new_**
* **github.com/siderolabs/talos**                                 v1.8.0-alpha.2 **_new_**
* **github.com/siderolabs/talos/pkg/machinery**                   v1.7.0 -> v1.8.0-alpha.2
* **golang.org/x/mod**                                            v0.17.0 -> v0.20.0
* **google.golang.org/grpc**                                      v1.63.2 -> v1.66.0
* **k8s.io/client-go**                                            v0.29.3 -> v0.31.0

Previous release can be found at [v0.5.0](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.5.0)

## [terraform-provider-talos 0.6.0-alpha.1](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.6.0-alpha.1) (2024-07-18)

Welcome to the v0.6.0-alpha.1 release of terraform-provider-talos!  
*This is a pre-release of terraform-provider-talos*



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### Image Factory

Support for querying info from Image Factory and registering schematics is now supported via new Terraform resources.


### Talos Cluster Health

`talos_cluster_health` data source now has a way to skip running the Kubernetes components health check by setting `skip_kubernetes_checks` to `true`.


### Talos Cluster Kubeconfig

`talos_cluster_kubeconfig` data source is now deprecated and will be removed in the next minor release.
Use `talos_cluster_kubeconfig` resource instead.
The `talos_cluster_kubeconfig` resource will regenerate kubernetes client config when the time to expiry is less than a month.


### Talos Machine Configuration Apply

`talos_machine_configuration_apply` resource now optionally supports resetting the machine back to maintenance mode.


### Talos Machine Secrets

`talos_machine_secrets` resource now regenerates client config when the time to expiry is less than a month.


### Component Updates

Talos sdk: v1.8.0-alpha.1


### Contributors

* Noel Georgi

### Changes
<details><summary>11 commits</summary>
<p>

* [`92fcb70`](https://github.com/siderolabs/terraform-provider-talos/commit/92fcb70cd9c1d6cfb5bd3ea159e56fa493fe62db) feat: add `talos_image_factory_url` data source
* [`ffc8102`](https://github.com/siderolabs/terraform-provider-talos/commit/ffc8102dd7f7b0929dc54fa1f50afd9b2cfda6be) feat: add factory support
* [`d6c2a0b`](https://github.com/siderolabs/terraform-provider-talos/commit/d6c2a0bc939de41252d97f09b8e9dfcff3377450) chore: fix goreleaser config
* [`dcdcee6`](https://github.com/siderolabs/terraform-provider-talos/commit/dcdcee62f8d9429cc0416dfdb92dfc27391da0ee) release(v0.6.0-alpha.0): prepare release
* [`d962913`](https://github.com/siderolabs/terraform-provider-talos/commit/d9629133c03ef09d5a6136b0e6cc1624a7ef4c28) chore: reset options for `machine_configuration_apply` resource
* [`f26a591`](https://github.com/siderolabs/terraform-provider-talos/commit/f26a5911bdd243fd384a353de1c0140de256211c) chore: data source -> resource `talos_cluster_kubeconfig`
* [`78fd0d3`](https://github.com/siderolabs/terraform-provider-talos/commit/78fd0d369ff401fbd795609cbbc2275f7d108bb0) chore: ignore version prefix for `talos_version`
* [`11ae330`](https://github.com/siderolabs/terraform-provider-talos/commit/11ae33002bee7a3e319bc7f9ea7555ca3ebaa120) feat: support skipping k8s health checks
* [`0fe1a6f`](https://github.com/siderolabs/terraform-provider-talos/commit/0fe1a6fe8d4440f72d7c553cf961b0de7267404b) docs: update description of `talos_cluster_health`
* [`f6f1811`](https://github.com/siderolabs/terraform-provider-talos/commit/f6f1811e90c9eef91f562f1f8b15f78e984315a0) feat: regenerate talosconfig
* [`501c78e`](https://github.com/siderolabs/terraform-provider-talos/commit/501c78eb7403012c90229bfc24399d0e1603289b) chore: bump deps
</p>
</details>

### Changes since v0.6.0-alpha.0
<details><summary>2 commits</summary>
<p>

* [`92fcb70`](https://github.com/siderolabs/terraform-provider-talos/commit/92fcb70cd9c1d6cfb5bd3ea159e56fa493fe62db) feat: add `talos_image_factory_url` data source
* [`ffc8102`](https://github.com/siderolabs/terraform-provider-talos/commit/ffc8102dd7f7b0929dc54fa1f50afd9b2cfda6be) feat: add factory support
</p>
</details>

### Dependency Changes

* **github.com/hashicorp/terraform-plugin-docs**                  v0.19.0 -> v0.19.4
* **github.com/hashicorp/terraform-plugin-framework**             v1.7.0 -> v1.10.0
* **github.com/hashicorp/terraform-plugin-framework-validators**  v0.12.0 -> v0.13.0
* **github.com/hashicorp/terraform-plugin-go**                    v0.22.1 -> v0.23.0
* **github.com/hashicorp/terraform-plugin-log**                   v0.9.0 **_new_**
* **github.com/hashicorp/terraform-plugin-sdk/v2**                v2.33.0 -> v2.34.0
* **github.com/hashicorp/terraform-plugin-testing**               v1.7.0 -> v1.9.0
* **github.com/siderolabs/gen**                                   v0.4.8 -> v0.5.0
* **github.com/siderolabs/image-factory**                         8b4e0d9e9819 **_new_**
* **github.com/siderolabs/talos**                                 980f9ebc0725 **_new_**
* **github.com/siderolabs/talos/pkg/machinery**                   v1.7.0 -> v1.8.0-alpha.1
* **golang.org/x/mod**                                            v0.17.0 -> v0.19.0
* **google.golang.org/grpc**                                      v1.63.2 -> v1.65.0
* **k8s.io/client-go**                                            v0.29.3 -> v0.31.0-beta.0

Previous release can be found at [v0.5.0](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.5.0)

## [terraform-provider-talos 0.6.0-alpha.0](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.6.0-alpha.0) (2024-07-11)

Welcome to the v0.6.0-alpha.0 release of terraform-provider-talos!  
*This is a pre-release of terraform-provider-talos*



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### Talos Cluster Health

`talos_cluster_health` data source now has a way to skip running the Kubernetes components health check by setting `skip_kubernetes_checks` to `true`.


### Talos Cluster Kubeconfig

`talos_cluster_kubeconfig` data source is now deprecated and will be removed in the next minor release.
Use `talos_cluster_kubeconfig` resource instead.
The `talos_cluster_kubeconfig` resource will regenerate kubernetes client config when the time to expiry is less than a month.


### Talos Machine Configuration Apply

`talos_machine_configuration_apply` resource now optionally supports resetting the machine back to maintenance mode.


### Talos Machine Secrets

`talos_machine_secrets` resource now regenerates client config when the time to expiry is less than a month.


### Component Updates

Talos sdk: v1.8.0-alpha.1


### Contributors

* Noel Georgi
* Dmitriy Matrenichev

### Changes
<details><summary>8 commits</summary>
<p>

* [`1908d9e`](https://github.com/siderolabs/terraform-provider-talos/commit/1908d9e085d4f0d55aa8d55888a01ed7b52e8295) release(v0.6.0-alpha.0): prepare release
* [`d962913`](https://github.com/siderolabs/terraform-provider-talos/commit/d9629133c03ef09d5a6136b0e6cc1624a7ef4c28) chore: reset options for `machine_configuration_apply` resource
* [`f26a591`](https://github.com/siderolabs/terraform-provider-talos/commit/f26a5911bdd243fd384a353de1c0140de256211c) chore: data source -> resource `talos_cluster_kubeconfig`
* [`78fd0d3`](https://github.com/siderolabs/terraform-provider-talos/commit/78fd0d369ff401fbd795609cbbc2275f7d108bb0) chore: ignore version prefix for `talos_version`
* [`11ae330`](https://github.com/siderolabs/terraform-provider-talos/commit/11ae33002bee7a3e319bc7f9ea7555ca3ebaa120) feat: support skipping k8s health checks
* [`0fe1a6f`](https://github.com/siderolabs/terraform-provider-talos/commit/0fe1a6fe8d4440f72d7c553cf961b0de7267404b) docs: update description of `talos_cluster_health`
* [`f6f1811`](https://github.com/siderolabs/terraform-provider-talos/commit/f6f1811e90c9eef91f562f1f8b15f78e984315a0) feat: regenerate talosconfig
* [`501c78e`](https://github.com/siderolabs/terraform-provider-talos/commit/501c78eb7403012c90229bfc24399d0e1603289b) chore: bump deps
</p>
</details>

### Changes from siderolabs/gen
<details><summary>2 commits</summary>
<p>

* [`7654108`](https://github.com/siderolabs/gen/commit/7654108fe6ae15d4765584342709bc0bced6b3d6) chore: add hashtriemap implementation
* [`8485864`](https://github.com/siderolabs/gen/commit/84858640dc9c3032219380885283b995d4f2b0d1) chore: optimize maps.Values and maps.Keys
</p>
</details>

### Dependency Changes

* **github.com/hashicorp/terraform-plugin-docs**       v0.19.0 -> v0.19.4
* **github.com/hashicorp/terraform-plugin-framework**  v1.7.0 -> v1.9.0
* **github.com/hashicorp/terraform-plugin-go**         v0.22.1 -> v0.23.0
* **github.com/hashicorp/terraform-plugin-log**        v0.9.0 **_new_**
* **github.com/hashicorp/terraform-plugin-sdk/v2**     v2.33.0 -> v2.34.0
* **github.com/hashicorp/terraform-plugin-testing**    v1.7.0 -> v1.8.0
* **github.com/siderolabs/gen**                        v0.4.8 -> v0.5.0
* **github.com/siderolabs/talos/pkg/machinery**        v1.7.0 -> v1.8.0-alpha.1
* **golang.org/x/mod**                                 v0.17.0 -> v0.19.0
* **google.golang.org/grpc**                           v1.63.2 -> v1.65.0
* **k8s.io/client-go**                                 v0.29.3 -> v0.31.0-alpha.3

Previous release can be found at [v0.5.0](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.5.0)

## [terraform-provider-talos 0.4.0-alpha.0](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.4.0-alpha.0) (2023-08-30)

Welcome to the v0.4.0-alpha.0 release of terraform-provider-talos!  
*This is a pre-release of terraform-provider-talos*



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### Talos Cluster Health data source

`talos_cluster_health` data source has been added and the `wait` parameter from the `talos_cluster_kubeconfig` data source is now deprecated.


### Component Updates

Talos sdk: v1.6.0-alpha.0


### Contributors

* Noel Georgi
* Dmitriy Matrenichev

### Changes
<details><summary>4 commits</summary>
<p>

* [`1c918e6`](https://github.com/siderolabs/terraform-provider-talos/commit/1c918e65b10764b1ed5286193ac661f3daec51d9) chore: add conform
* [`ed36726`](https://github.com/siderolabs/terraform-provider-talos/commit/ed3672669b20c7fd911088609952f8e036f38a1f) feat: add `talos_cluster_health` data source.
* [`5ac7183`](https://github.com/siderolabs/terraform-provider-talos/commit/5ac7183f33a425e17821f1a294a3133daa09e7fa) fix: node/endpoint were swapped for some resources.
* [`713ac46`](https://github.com/siderolabs/terraform-provider-talos/commit/713ac4686a3e00e135cab7ea533da7319522dddd) fix: creation of talos client
</p>
</details>

### Changes from siderolabs/gen
<details><summary>1 commit</summary>
<p>

* [`36a3ae3`](https://github.com/siderolabs/gen/commit/36a3ae312ce03876b2c961a1bcb4ef4c221593d7) feat: update module
</p>
</details>

### Dependency Changes

* **github.com/hashicorp/terraform-plugin-framework**  v1.3.4 -> v1.3.5
* **github.com/hashicorp/terraform-plugin-sdk/v2**     v2.27.0 -> v2.28.0
* **github.com/siderolabs/gen**                        v0.4.5 -> v0.4.6
* **github.com/siderolabs/talos/pkg/machinery**        v1.5.0 -> v1.6.0-alpha.0
* **k8s.io/client-go**                                 v0.28.0 -> v0.28.1

Previous release can be found at [v0.3.2](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.3.2)

## [terraform-provider-talos 0.3.0-beta.0](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.3.0-beta.0) (2023-08-07)

Welcome to the 0.3.0-beta.0 release of terraform-provider-talos!  
*This is a pre-release of terraform-provider-talos*



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### Component Updates

Talos sdk: v1.5.0-beta.0


### Contributors

* Noel Georgi
* Ole-Martin Bratteng
* Spencer Smith

### Changes
<details><summary>5 commits</summary>
<p>

* [`3f02af3`](https://github.com/siderolabs/terraform-provider-talos/commit/3f02af32747ab97d56d274eecbb3cc12bdaa7d1c) feat: update to talos 1.5 sdk
* [`ff0e2ad`](https://github.com/siderolabs/terraform-provider-talos/commit/ff0e2adec13192716b9cf2180baa1bff2843387d) fix: ci failures due to TF state removal
* [`ee150ce`](https://github.com/siderolabs/terraform-provider-talos/commit/ee150ce9925aac49e324ba4909104cba5a9ad50e) docs: update link to contrib repo
* [`df4f876`](https://github.com/siderolabs/terraform-provider-talos/commit/df4f876ce18e8239bb1cabec7437a0f62ed1f5f7) docs: replace `type` with `machine_type`
* [`f6c8715`](https://github.com/siderolabs/terraform-provider-talos/commit/f6c871516635dbb402bfe24bd47759537c7fee46) chore: bump deps
</p>
</details>

### Dependency Changes

* **github.com/siderolabs/talos/pkg/machinery**  v1.4.7 -> v1.5.0-beta.0

Previous release can be found at [v0.2.1](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.2.1)

## [terraform-provider-talos 0.2.0-alpha.2](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.2.0-alpha.2) (2023-04-14)

Welcome to the v0.2.0-alpha.2 release of terraform-provider-talos!  
*This is a pre-release of terraform-provider-talos*



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### Data Sources

`talos_machine_disks` data source is added to list disks on a machine.


### Provider Changes

This version of the provider includes some breaking changes. Make sure to follow the provider upgrade guide at https://registry.terraform.io/providers/siderolabs/talos/latest/docs/guides/version-0.2-upgrade.html


### Component Updates

Talos sdk: v1.4.0-beta.1


### Contributors

* Andrey Smirnov
* Andrey Smirnov
* Artem Chernyshev
* Dmitriy Matrenichev
* Artem Chernyshev
* Noel Georgi
* Serge Logvinov
* Andrew Rynhard
* Andrew Rynhard
* Matt Zahorik
* Olli Janatuinen
* Seán C McCord
* Spencer Smith

### Changes
<details><summary>2 commits</summary>
<p>

* [`187e434`](https://github.com/siderolabs/terraform-provider-talos/commit/187e434235da1107eaefc1656147900fa8c1d082) feat: `talos_machine_disks` data source
* [`a29e1e7`](https://github.com/siderolabs/terraform-provider-talos/commit/a29e1e7894776a4c796712163e64084e73b1ffa7) fix: handle unknown types at plan time
</p>
</details>

### Changes from siderolabs/gen
<details><summary>9 commits</summary>
<p>

* [`214c1ef`](https://github.com/siderolabs/gen/commit/214c1efe795cf426e5ebcc48cb305bfc7a16fdb8) chore: set `slice.Filter` result slice cap to len
* [`8e89b1e`](https://github.com/siderolabs/gen/commit/8e89b1ede9f35ff4c18a41ee44a69259181c892b) feat: add GetOrCreate and GetOrCall methods
* [`7c7ccc3`](https://github.com/siderolabs/gen/commit/7c7ccc3d973621b2fa7adfef10241ecc1f7a644d) feat: introduce channel SendWithContext
* [`b3b6db8`](https://github.com/siderolabs/gen/commit/b3b6db858cb6ce46005edeb70776608e3f9bc402) fix: fix Copy documentation and implementation
* [`521f737`](https://github.com/siderolabs/gen/commit/521f7371f40556ddce7f730c8de5e1888e40b621) feat: add xerrors package which contains additions to the std errors
* [`726e066`](https://github.com/siderolabs/gen/commit/726e066dcb35c86f82866097bed806f22b936292) fix: rename tuples.go to pair.go and set proper package name
* [`d8d7d25`](https://github.com/siderolabs/gen/commit/d8d7d25ce9a588609c00cb798206a01a866bf7a6) chore: minor additions
* [`338a650`](https://github.com/siderolabs/gen/commit/338a65065f92eb6426a66c4a88a0cc02cc02e529) chore: add initial implementation and documentation
* [`4fd8667`](https://github.com/siderolabs/gen/commit/4fd866707052c792a6adccbc28efec5debdd18a8) Initial commit
</p>
</details>

### Changes from siderolabs/go-blockdevice
<details><summary>59 commits</summary>
<p>

* [`b4386f3`](https://github.com/siderolabs/go-blockdevice/commit/b4386f37510bc25e39b231fa587288ad0abf0b68) feat: make disk utils read subsystem information from the `/sys/block`
* [`8c7ea19`](https://github.com/siderolabs/go-blockdevice/commit/8c7ea1910b27e0660e3e1a6f98b9f7e24bc11ff0) fix: blockdevice size is reported by Linux in 512 blocks always
* [`e52e012`](https://github.com/siderolabs/go-blockdevice/commit/e52e012a6935a99a1b344a898f281cf7d6a78e69) feat: add ext4 filesystem detection logic
* [`694ac62`](https://github.com/siderolabs/go-blockdevice/commit/694ac62b3dcf995beea95a77659fdc6064b457b3) chore: update imports to siderolabs, rekres
* [`dcf6044`](https://github.com/siderolabs/go-blockdevice/commit/dcf6044c906b36f183e11b6553458c680126d1d9) chore: rekres and rename
* [`9c4af49`](https://github.com/siderolabs/go-blockdevice/commit/9c4af492cc17279f0281fcd271e7423be78442bb) fix: cryptsetup remove slot
* [`74ea471`](https://github.com/siderolabs/go-blockdevice/commit/74ea47109c4525bec139640fed6354ad3097f5fb) feat: add freebsd stubs
* [`9fa801c`](https://github.com/siderolabs/go-blockdevice/commit/9fa801cf4da184e3560b9a18ba43d13316f172f9) feat: add ReadOnly attribute to Disk
* [`fccee8b`](https://github.com/siderolabs/go-blockdevice/commit/fccee8bb082b105cb60db40cb01636efc3241b5f) chore: rekres the source, fix issues
* [`d9c3a27`](https://github.com/siderolabs/go-blockdevice/commit/d9c3a273886113e24809ef1e9930fc982318217d) feat: support probing FAT12/FAT16 filesystems
* [`b374eb4`](https://github.com/siderolabs/go-blockdevice/commit/b374eb48148dc92a82d8bf9540432bb8531f73f3) fix: align partition to 1M boundary by default
* [`ec428fe`](https://github.com/siderolabs/go-blockdevice/commit/ec428fed2ecd5a389833a88f8dc333762816db99) fix: lookup filesystem labels on the actual device path
* [`7b9de26`](https://github.com/siderolabs/go-blockdevice/commit/7b9de26bc6bc3d54b95bd8e8fb3aade4b45adc6c) feat: read symlink fullpath in block device list function
* [`6928ee4`](https://github.com/siderolabs/go-blockdevice/commit/6928ee43c3034549e32f000f8b7bc16a6ebb7ed4) refactor: rewrite GPT serialize/deserialize functions
* [`0c7e429`](https://github.com/siderolabs/go-blockdevice/commit/0c7e4296e01b3df815a935db3e30de6b9d4cc1d1) refactor: simplify middle endian functions
* [`15b182d`](https://github.com/siderolabs/go-blockdevice/commit/15b182db0cd233b163ed83d1724c7e28cf29d71a) fix: return partition table not exist when trying to read an empty dev
* [`b9517d5`](https://github.com/siderolabs/go-blockdevice/commit/b9517d51120d385f97b0026f99ce3c4782940c37) fix: resize partition
* [`70d2865`](https://github.com/siderolabs/go-blockdevice/commit/70d28650b398a14469cbb5356417355b0ba62956) fix: try to find cdrom disks
* [`667bf53`](https://github.com/siderolabs/go-blockdevice/commit/667bf539b99ac34b629a0103ef7a7278a5a5f35d) fix: revert gpt partition not found
* [`d7d4cdd`](https://github.com/siderolabs/go-blockdevice/commit/d7d4cdd7ac56c82caab19246b5decd59f12195eb) fix: gpt partition not found
* [`33afba3`](https://github.com/siderolabs/go-blockdevice/commit/33afba347c0dce38a436c46a0aac26d2f99427c1) fix: also open in readonly mode when running `All` lookup method
* [`e367f9d`](https://github.com/siderolabs/go-blockdevice/commit/e367f9dc7fa935f11672de0fdc8a89429285a07a) feat: make probe always open blockdevices in readonly mode
* [`d981156`](https://github.com/siderolabs/go-blockdevice/commit/d9811569588ba44be878a00ce316f59a37abed8b) fix: allow Build for Windows
* [`fe24303`](https://github.com/siderolabs/go-blockdevice/commit/fe2430349e9d734ce6dbf4e7b2e0f8a37bb22679) fix: perform correct PMBR partition calculations
* [`2ec0c3c`](https://github.com/siderolabs/go-blockdevice/commit/2ec0c3cc0ff5ff705ed5c910ca1bcd5d93c7b102) fix: preserve the PMBR bootable flag when opening GPT partition
* [`87816a8`](https://github.com/siderolabs/go-blockdevice/commit/87816a81cefc728cfe3cb221b476d8ed4b609fd8) feat: align partition to minimum I/O size
* [`c34b59f`](https://github.com/siderolabs/go-blockdevice/commit/c34b59fb33a7ad8be18bb19bc8c8d8294b4b3a78) feat: expose more encryption options in the LUKS module
* [`30c2bc3`](https://github.com/siderolabs/go-blockdevice/commit/30c2bc3cb62af52f0aea9ce347923b0649fb7928) feat: mark MBR bootable
* [`1292574`](https://github.com/siderolabs/go-blockdevice/commit/1292574643e06512255fb0f45107e0c296eb5a3b) fix: make disk type matcher parser case insensitive
* [`b77400e`](https://github.com/siderolabs/go-blockdevice/commit/b77400e0a7261bf25da77c1f28c2f393f367bfa9) fix: properly detect nvme and sd card disk types
* [`1d830a2`](https://github.com/siderolabs/go-blockdevice/commit/1d830a25f64f6fb96a1bedd800c0b40b107dc833) fix: revert mark the EFI partition in PMBR as bootable
* [`bec914f`](https://github.com/siderolabs/go-blockdevice/commit/bec914ffdda42abcfe642bc2cdfc9fcda56a74ee) fix: mark the EFI partition in PMBR as bootable
* [`776b37d`](https://github.com/siderolabs/go-blockdevice/commit/776b37d31de0781f098f5d9d1894fbea3f2dfa1d) feat: add options to probe disk by various sysblock parameters
* [`bb3ad73`](https://github.com/siderolabs/go-blockdevice/commit/bb3ad73f69836acc2785ec659435e24a531359e7) fix: align partition start to physical sector size
* [`8f976c2`](https://github.com/siderolabs/go-blockdevice/commit/8f976c2031108651738ebd4db69fb09758754a28) feat: replace exec.Command with go-cmd module
* [`1cf7f25`](https://github.com/siderolabs/go-blockdevice/commit/1cf7f252c38cf11ef07723de2debc27d1da6b520) fix: properly handle no child processes error from cmd.Wait
* [`04a9851`](https://github.com/siderolabs/go-blockdevice/commit/04a98510c07fe8477f598befbfe6eaec4f4b73a2) feat: implement luks encryption provider
* [`b0375e4`](https://github.com/siderolabs/go-blockdevice/commit/b0375e4267fdc6108bd9ff7a5dc97b80cd924b1d) feat: add an option to open block device with exclusive flock
* [`5a1c7f7`](https://github.com/siderolabs/go-blockdevice/commit/5a1c7f768e016c93f6c0be130ffeaf34109b5b4d) refactor: add devname into gpt.Partition, refactor probe package
* [`f2728a5`](https://github.com/siderolabs/go-blockdevice/commit/f2728a581972be977d863d5d9177a873b8f3fc7b) fix: keep contents of PMBR when writing it
* [`2878460`](https://github.com/siderolabs/go-blockdevice/commit/2878460b54e8b8c3846c6a882ca9e1472c8b6b3b) fix: write second copy of partition entries
* [`943b08b`](https://github.com/siderolabs/go-blockdevice/commit/943b08bc32a2156cffb23e92b8be9288de4a7421) fix: blockdevice reset should read partition table from disk
* [`5b4ee44`](https://github.com/siderolabs/go-blockdevice/commit/5b4ee44cfd434a03ec2d7167bcc56d0f164c3fa2) fix: ignore `/dev/ram` devices
* [`98754ec`](https://github.com/siderolabs/go-blockdevice/commit/98754ec2bb200acc9e9e573fa766754d60e25ff2) refactor: rewrite GPT library
* [`2a1baad`](https://github.com/siderolabs/go-blockdevice/commit/2a1baadffdf8c9b65355e9af6e744aeab838c9db) fix: correctly build paths for `mmcblk` devices
* [`8076344`](https://github.com/siderolabs/go-blockdevice/commit/8076344a95021f25ab5d1fbf5ea4fefc790f6c3c) fix: return proper disk size from GetDisks function
* [`8742133`](https://github.com/siderolabs/go-blockdevice/commit/874213371a3fb0925aab45cbba68a957e3319525) chore: add common method to list available disks using /sys/block
* [`c4b5833`](https://github.com/siderolabs/go-blockdevice/commit/c4b583363d63503ed7e4adb9a9fa64335f7e198d) feat: implement "fast" wipe
* [`b4e67d7`](https://github.com/siderolabs/go-blockdevice/commit/b4e67d73d70d8dc06aa2b4986622dcb854dfc40c) feat: return resize status from Resize() function
* [`ceae64e`](https://github.com/siderolabs/go-blockdevice/commit/ceae64edb3a591c6f6bbd75b1149d1cfe426dd8e) fix: sync kernel partition table incrementally
* [`2cb9516`](https://github.com/siderolabs/go-blockdevice/commit/2cb95165aa67b0b839863b5ad89920c3ac7e2c82) fix: return correct error value from blkpg functions
* [`cebe43d`](https://github.com/siderolabs/go-blockdevice/commit/cebe43d1fdc1e509437198e578faa9d5a804cc37) refactor: expose `InsertAt` method via interface
* [`c40dcd8`](https://github.com/siderolabs/go-blockdevice/commit/c40dcd80c50b41c1f2a60ea6aa9d5fb3d3b180a3) fix: properly inform kernel about partition deletion
* [`bb8ac5d`](https://github.com/siderolabs/go-blockdevice/commit/bb8ac5d6a25e279e16213f585dc8d02ba6ed645f) feat: implement disk wiping via several methods
* [`23fb7dc`](https://github.com/siderolabs/go-blockdevice/commit/23fb7dc755325cfe12e48c8e8e31bebab9ddc2bc) feat: expose partition name (label)
* [`ff3a821`](https://github.com/siderolabs/go-blockdevice/commit/ff3a8210be999b8bfb2019f19f8a8b50901c64cc) feat: implement 'InsertAt' method to insert partitions at any position
* [`3d1ce4f`](https://github.com/siderolabs/go-blockdevice/commit/3d1ce4fc859fa614a4c5c54a10c0f5f4fce38bb6) fix: calculate last lba of partition correctly
* [`b71540f`](https://github.com/siderolabs/go-blockdevice/commit/b71540f6c398e958bdb7c118396a736419f735d4) feat: copy initial version from talos-systems/talos
* [`ca3c078`](https://github.com/siderolabs/go-blockdevice/commit/ca3c078da95e6497c9d41667dc242e32682e517d) Initial commit
</p>
</details>

### Dependency Changes

* **github.com/dustin/go-humanize**         v1.0.1 **_new_**
* **github.com/siderolabs/gen**             v0.4.3 **_new_**
* **github.com/siderolabs/go-blockdevice**  v0.4.4 **_new_**
* **k8s.io/client-go**                      v0.26.3 -> v0.27.0

Previous release can be found at [v0.2.0-alpha.1](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.2.0-alpha.1)

## [terraform-provider-talos 0.2.0-alpha.1](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.2.0-alpha.1) (2023-04-12)

Welcome to the v0.2.0-alpha.1 release of terraform-provider-talos!  
*This is a pre-release of terraform-provider-talos*



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### Provider Changes

This version of the provider includes some breaking changes. Make sure to follow the provider upgrade guide at https://registry.terraform.io/providers/siderolabs/talos/latest/docs/guides/version-0.2-upgrade.html


### Component Updates

Talos sdk: v1.4.0-beta.1


### Contributors

* Noel Georgi

### Changes
<details><summary>1 commit</summary>
<p>

* [`96aeedd`](https://github.com/siderolabs/terraform-provider-talos/commit/96aeedd4bca46ced42bc98220e809940b773ce5d) docs: fix rendering of website
</p>
</details>

### Dependency Changes

* **github.com/siderolabs/talos/pkg/machinery**  v1.4.0-beta.0 -> v1.4.0-beta.1

Previous release can be found at [v0.2.0-alpha.0](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.2.0-alpha.0)


## [terraform-provider-talos 0.2.0-alpha.0](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.2.0-alpha.0) (2023-04-10)

Welcome to the v0.2.0-alpha.0 release of terraform-provider-talos!  
*This is a pre-release of terraform-provider-talos*



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### Provider Changes

This version of the provider includes some breaking changes. Make sure to follow the provider upgrade guide at https://registry.terraform.io/providers/siderolabs/talos/latest/docs/guides/version-0.2-upgrade


### Component Updates

Talos sdk: v1.4.0-beta.0


### Contributors

* Andrey Smirnov
* Noel Georgi
* Alexey Palazhchenko
* Andrey Smirnov
* Spencer Smith
* Andrew Rynhard
* Artem Chernyshev
* Robert Wunderer
* Serge Logvinov

### Changes
<details><summary>10 commits</summary>
<p>

* [`6b2182b`](https://github.com/siderolabs/terraform-provider-talos/commit/6b2182be4015eb21d936930cfa108159b3fa16f6) chore: ci cleanup
* [`ea07caa`](https://github.com/siderolabs/terraform-provider-talos/commit/ea07caa6f39d159b286db76c525ed7cb1892f1e6) feat: code cleanup and tests
* [`4e5c210`](https://github.com/siderolabs/terraform-provider-talos/commit/4e5c210c219ddeb0ccf68cd80cb2988ce0ab5ccd) feat: use new tf sdk
* [`d1438b7`](https://github.com/siderolabs/terraform-provider-talos/commit/d1438b71d94b0ee0d0f3f5feb12317344c6e2a3e) chore: bump talos machinery
* [`aed502e`](https://github.com/siderolabs/terraform-provider-talos/commit/aed502e51945bad2493c521672a52111fcd1eca6) fix: state update required two runs
* [`dc00baa`](https://github.com/siderolabs/terraform-provider-talos/commit/dc00baad8d85e2786789ea03ac8e35d0b613bd70) fix: force new talosconfig if endpoints or nodes change
* [`b7d84ba`](https://github.com/siderolabs/terraform-provider-talos/commit/b7d84ba4e297271b1599de20310640c4ebffccb9) chore: update registry index file, remove example
* [`158dbbd`](https://github.com/siderolabs/terraform-provider-talos/commit/158dbbde42326fed9ada2a97a2a25c625c0a783e) docs: re-word `talos_version`
* [`f3b4a5b`](https://github.com/siderolabs/terraform-provider-talos/commit/f3b4a5b24362c264a642e5bc5a28bba0e70a86fc) chore: bump deps
* [`d2b6df0`](https://github.com/siderolabs/terraform-provider-talos/commit/d2b6df03e44a9150445209d3bd118c5d4417547c) docs: clarify meaning of `talos_version` in `machine_configuration` resources
</p>
</details>

### Changes from siderolabs/crypto
<details><summary>27 commits</summary>
<p>

* [`c3225ee`](https://github.com/siderolabs/crypto/commit/c3225eee603a8d1218c67e1bfe33ddde7953ed74) feat: allow CSR template subject field to be overridden
* [`8570669`](https://github.com/siderolabs/crypto/commit/85706698dac8cddd0e9f41006bed059347d2ea26) chore: rename to siderolabs/crypto
* [`e9df1b8`](https://github.com/siderolabs/crypto/commit/e9df1b8ca74c6efdc7f72191e5d2613830162fd5) feat: add support for generating keys from RSA-SHA256 CAs
* [`510b0d2`](https://github.com/siderolabs/crypto/commit/510b0d2753a89170d0c0f60e052a66484997a5b2) chore: add json tags
* [`6fa2d93`](https://github.com/siderolabs/crypto/commit/6fa2d93d0382299d5471e0de8e831c923398aaa8) fix: deepcopy nil fields as `nil`
* [`9a63cba`](https://github.com/siderolabs/crypto/commit/9a63cba8dabd278f3080fa8c160613efc48c43f8) fix: add back support for generating ECDSA keys with P-256 and SHA512
* [`893bc66`](https://github.com/siderolabs/crypto/commit/893bc66e4716a4cb7d1d5e66b5660ffc01f22823) fix: use SHA256 for ECDSA-P256
* [`deec8d4`](https://github.com/siderolabs/crypto/commit/deec8d47700e10e3ea813bdce01377bd93c83367) chore: implement DeepCopy methods for PEMEncoded* types
* [`d3cb772`](https://github.com/siderolabs/crypto/commit/d3cb77220384b3a3119a6f3ddb1340bbc811f1d1) feat: make possible to change KeyUsage
* [`6bc5bb5`](https://github.com/siderolabs/crypto/commit/6bc5bb50c52767296a1b1cab6580e3fcf1358f34) chore: remove unused argument
* [`cd18ef6`](https://github.com/siderolabs/crypto/commit/cd18ef62eb9f65d8b6730a2eb73e47e629949e1b) feat: add support for several organizations
* [`97c888b`](https://github.com/siderolabs/crypto/commit/97c888b3924dd5ac70b8d30dd66b4370b5ab1edc) chore: add options to CSR
* [`7776057`](https://github.com/siderolabs/crypto/commit/7776057f5086157873f62f6a21ec23fa9fd86e05) chore: fix typos
* [`80df078`](https://github.com/siderolabs/crypto/commit/80df078327030af7e822668405bb4853c512bd7c) chore: remove named result parameters
* [`15bdd28`](https://github.com/siderolabs/crypto/commit/15bdd282b74ac406ab243853c1b50338a1bc29d0) chore: minor updates
* [`4f80b97`](https://github.com/siderolabs/crypto/commit/4f80b976b640d773fb025d981bf85bcc8190815b) fix: verify CSR signature before issuing a certificate
* [`39584f1`](https://github.com/siderolabs/crypto/commit/39584f1b6e54e9966db1f16369092b2215707134) feat: support for key/certificate types RSA, Ed25519, ECDSA
* [`cf75519`](https://github.com/siderolabs/crypto/commit/cf75519cab82bd1b128ae9b45107c6bb422bd96a) fix: function NewKeyPair should create certificate with proper subject
* [`751c95a`](https://github.com/siderolabs/crypto/commit/751c95aa9434832a74deb6884cff7c5fd785db0b) feat: add 'PEMEncodedKey' which allows to transport keys in YAML
* [`562c3b6`](https://github.com/siderolabs/crypto/commit/562c3b66f89866746c0ba47927c55f41afed0f7f) feat: add support for public RSA key in RSAKey
* [`bda0e9c`](https://github.com/siderolabs/crypto/commit/bda0e9c24e80c658333822e2002e0bc671ac53a3) feat: enable more conversions between encoded and raw versions
* [`e0dd56a`](https://github.com/siderolabs/crypto/commit/e0dd56ac47456f85c0b247999afa93fb87ebc78b) feat: add NotBefore option for x509 cert creation
* [`12a4897`](https://github.com/siderolabs/crypto/commit/12a489768a6bb2c13e16e54617139c980f99a658) feat: add support for SPKI fingerprint generation and matching
* [`d0c3eef`](https://github.com/siderolabs/crypto/commit/d0c3eef149ec9b713e7eca8c35a6214bd0a64bc4) fix: implement NewKeyPair
* [`196679e`](https://github.com/siderolabs/crypto/commit/196679e9ec77cb709db54879ddeddd4eaafaea01) feat: move `pkg/grpc/tls` from `github.com/talos-systems/talos` as `./tls`
* [`1ff6242`](https://github.com/siderolabs/crypto/commit/1ff6242c91bb298ceeb4acd65685cba952fe4178) chore: initial version as imported from talos-systems/talos
* [`835063e`](https://github.com/siderolabs/crypto/commit/835063e055b28a525038b826a6d80cbe76402414) chore: initial commit
</p>
</details>

### Dependency Changes

* **github.com/hashicorp/terraform-plugin-framework**             v1.2.0 **_new_**
* **github.com/hashicorp/terraform-plugin-framework-timeouts**    v0.3.1 **_new_**
* **github.com/hashicorp/terraform-plugin-framework-validators**  v0.10.0 **_new_**
* **github.com/hashicorp/terraform-plugin-go**                    v0.15.0 **_new_**
* **github.com/hashicorp/terraform-plugin-sdk/v2**                v2.25.0 -> v2.26.1
* **github.com/hashicorp/terraform-plugin-testing**               v1.2.0 **_new_**
* **github.com/siderolabs/crypto**                                v0.4.0 **_new_**
* **github.com/siderolabs/talos/pkg/machinery**                   v1.3.6 -> v1.4.0-beta.0
* **golang.org/x/mod**                                            v0.10.0 **_new_**
* **google.golang.org/grpc**                                      v1.51.0 -> v1.54.0
* **k8s.io/client-go**                                            v0.26.3 **_new_**

Previous release can be found at [v0.1.2](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.2)

## [terraform-provider-talos 0.1.0-alpha.12](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.12) (2022-12-16)

Welcome to the v0.1.0-alpha.12 release of terraform-provider-talos!  
*This is a pre-release of terraform-provider-talos*



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### Fixes

Fixed an issue with the provider when using a secure Talos client.


### Contributors

* Noel Georgi

### Changes
<details><summary>1 commit</summary>
<p>

* [`3c80f59`](https://github.com/siderolabs/terraform-provider-talos/commit/3c80f59ad7a8514b5481c84ca0d210c62ea06bcc) fix: handling talos secure client
</p>
</details>

### Dependency Changes

This release has no dependency changes

Previous release can be found at [v0.1.0-alpha.11](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.11)

## [terraform-provider-talos 0.1.0-alpha.11](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.11) (2022-12-09)

Welcome to the v0.1.0-alpha.11 release of terraform-provider-talos!  
*This is a pre-release of terraform-provider-talos*



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### Component Updates

Talos sdk: v1.2.7


### Contributors


### Changes
<details><summary>0 commit</summary>
<p>

</p>
</details>

### Dependency Changes

This release has no dependency changes

Previous release can be found at [v0.1.0-alpha.10](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.10)

## [terraform-provider-talos 0.1.0-alpha.10](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.10) (2022-11-02)

Welcome to the v0.1.0-alpha.10 release of terraform-provider-talos!  
*This is a pre-release of terraform-provider-talos*



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### Component Updates

Talos sdk: v1.2.6


### Contributors


### Changes
<details><summary>0 commit</summary>
<p>

</p>
</details>

### Dependency Changes

This release has no dependency changes

Previous release can be found at [v0.1.0-alpha.9](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.9)

## [terraform-provider-talos 0.1.0-alpha.9](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.9) (2022-10-18)

Welcome to the v0.1.0-alpha.9 release of terraform-provider-talos!  
*This is a pre-release of terraform-provider-talos*



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### Component Updates

Talos sdk: v1.2.5


### Contributors


### Changes
<details><summary>0 commit</summary>
<p>

</p>
</details>

### Dependency Changes

This release has no dependency changes

Previous release can be found at [v0.1.0-alpha.8](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.8)

## [terraform-provider-talos 1.2.4](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v1.2.4) (2022-10-18)

Welcome to the v1.2.4 release of terraform-provider-talos!  
*This is a pre-release of terraform-provider-talos*



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### Component Updates

Talos sdk: v1.2.4


### Contributors

* Dmitriy Matrenichev
* Noel Georgi

### Changes
<details><summary>1 commit</summary>
<p>

* [`1c7975d`](https://github.com/siderolabs/terraform-provider-talos/commit/1c7975d1392406fa10a1a225e426e005d699c73e) chore: move to `gen` go pkg
</p>
</details>

### Changes from siderolabs/gen
<details><summary>2 commits</summary>
<p>

* [`338a650`](https://github.com/siderolabs/gen/commit/338a65065f92eb6426a66c4a88a0cc02cc02e529) chore: add initial implementation and documentation
* [`4fd8667`](https://github.com/siderolabs/gen/commit/4fd866707052c792a6adccbc28efec5debdd18a8) Initial commit
</p>
</details>

### Dependency Changes

* **github.com/siderolabs/gen**  v0.1.0 **_new_**

Previous release can be found at [v0.1.0-alpha.7](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.7)

## [terraform-provider-talos 0.1.0-alpha.7](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.7) (2022-09-20)

Welcome to the v0.1.0-alpha.7 release of terraform-provider-talos!  
*This is a pre-release of terraform-provider-talos*



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### Contributors

* Noel Georgi

### Changes
<details><summary>1 commit</summary>
<p>

* [`79594a6`](https://github.com/siderolabs/terraform-provider-talos/commit/79594a60b231966f68be2e39c01990e209176d6b) chore: bump talos to v1.2.3
</p>
</details>

### Dependency Changes

This release has no dependency changes

Previous release can be found at [v0.1.0-alpha.6](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.6)

## [terraform-provider-talos 0.1.0-alpha.6](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.6) (2022-09-15)

Welcome to the v0.1.0-alpha.6 release of terraform-provider-talos!  
*This is a pre-release of terraform-provider-talos*



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### Talos Provider

The Talos provider now requires `endpoint` and `node` to be set for `talos_machine_configuration_apply`, `talos_machine_bootstrap`, `talos_cluster_kubeconfig` resources.
The `endpoints` and `nodes` arguments are removed for the above resources.

This release also fixes a bug when multiple endpoitns were specified in the Talos client config.



### Contributors


### Changes
<details><summary>0 commit</summary>
<p>

</p>
</details>

### Dependency Changes

This release has no dependency changes

Previous release can be found at [v0.1.0-alpha.5](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.5)

## [terraform-provider-talos 0.1.0-alpha.5](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.5) (2022-09-15)

Welcome to the v0.1.0-alpha.5 release of terraform-provider-talos!  
*This is a pre-release of terraform-provider-talos*



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### Talos Provider

The Talos provider now requires `endpoint` and `node` to be set for `talos_machine_configuration_apply`, `talos_machine_bootstrap`, `talos_cluster_kubeconfig` resources.
The `endpoints` and `nodes` arguments are removed for the above resources.

This release also fixes a bug when multiple endpoitns were specified in the Talos client config.



### Contributors


### Changes
<details><summary>0 commit</summary>
<p>

</p>
</details>

### Dependency Changes

This release has no dependency changes

Previous release can be found at [v0.1.0-alpha.4](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.4)

## [terraform-provider-talos 0.1.0-alpha.4](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.4) (2022-09-15)

Welcome to the v0.1.0-alpha.4 release of terraform-provider-talos!  
*This is a pre-release of terraform-provider-talos*



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### Talos Provider

The Talos provider now requires `endpoint` and `node` to be set for `talos_machine_configuration_apply`, `talos_machine_bootstrap`, `talos_cluster_kubeconfig` resources.
The `endpoints` and `nodes` arguments are removed for the above resources.

This release also fixes a bug when multiple endpoitns were specified in the Talos client config.



### Contributors

* Noel Georgi
* Nahuel Pastorale

### Changes
<details><summary>3 commits</summary>
<p>

* [`ea5a4ba`](https://github.com/siderolabs/terraform-provider-talos/commit/ea5a4bab1d86f20fc832fd4ffacf5978699a716e) release(v0.1.0-alpha.4): prepare release
* [`b9111db`](https://github.com/siderolabs/terraform-provider-talos/commit/b9111db42dade59000b26b313f7b7f0d5e6dc083) fix: client operations
* [`e2d04ac`](https://github.com/siderolabs/terraform-provider-talos/commit/e2d04acbf10eb092effa60a1cbc0eb27325d4b19) docs: fix wrong resource reference
</p>
</details>

### Dependency Changes

This release has no dependency changes

Previous release can be found at [v0.1.0-alpha.3](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.3)

## [terraform-provider-talos 0.1.0-alpha.3](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.3) (2022-09-08)

Welcome to the v0.1.0-alpha.3 release of terraform-provider-talos!  
*This is a pre-release of terraform-provider-talos*



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### Talos Provider

The Talos provider supports generating configs, applying them and bootstrap the nodes.

Resources supported:

* `talos_machine_secrets`
* `talos_client_configuration`
* `talos_machine_configuration_controlplane`
* `talos_machine_configuration_worker`
* `talos_machine_configuration_apply`
* `talos_machine_bootstrap`
* `talos_cluster_kubeconfig`

Data sources supported:

* `talos_client_configuration`
* `talos_cluster_kubeconfig`

Data sources will always create a diff and might be removed in a future release.


### Contributors

* Andrew Rynhard
* Andrey Smirnov
* Dmitriy Matrenichev
* Noel Georgi

### Changes
<details><summary>2 commits</summary>
<p>

* [`e29e302`](https://github.com/siderolabs/terraform-provider-talos/commit/e29e3028b0b5dc3c56757a7724d1400d4d98b3e8) chore: add release workflow
* [`4ecfd4f`](https://github.com/siderolabs/terraform-provider-talos/commit/4ecfd4f52353495a8304b0530d85a73b757861c9) feat: add new resources
</p>
</details>

### Changes from siderolabs/go-pointer
<details><summary>2 commits</summary>
<p>

* [`71ccdf0`](https://github.com/siderolabs/go-pointer/commit/71ccdf0d65330596f4def36da37625e4f362f2a9) chore: implement main functionality
* [`c1c3b23`](https://github.com/siderolabs/go-pointer/commit/c1c3b235d30cb0de97ed0645809f2b21af3b021e) Initial commit
</p>
</details>

### Dependency Changes

* **github.com/cosi-project/runtime**   v0.1.1 **_new_**
* **github.com/siderolabs/go-pointer**  v1.0.0 **_new_**
* **google.golang.org/grpc**            v1.48.0 **_new_**

Previous release can be found at [v0.1.0-alpha.2](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.2)

