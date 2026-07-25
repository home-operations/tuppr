## 2.7.0 (February 16, 2026)

FEATURES:

* ephemeral/local_command: New ephemeral resource that runs an executable on the local machine and returns the exit code, standard output data, and standard error data. ([#457](https://github.com/hashicorp/terraform-provider-local/issues/457))

## 2.6.2 (January 28, 2026)

NOTES:

* all: This release introduces no functional changes. It was built using the latest 1.24 patch version of Go and re-released to address a certificate handshake issue. ([#461](https://github.com/hashicorp/terraform-provider-local/issues/461))

## 2.6.1 (November 17, 2025)

BUG FIXES:

* Fixed documentation header for `local_command` action ([#456](https://github.com/hashicorp/terraform-provider-local/issues/456))

## 2.6.0 (November 17, 2025)

FEATURES:

* action/local_command: New action that invokes an executable on the local machine. ([#450](https://github.com/hashicorp/terraform-provider-local/issues/450))
* data/local_command: New data source that runs an executable on the local machine and returns the exit code, standard output data, and standard error data. ([#452](https://github.com/hashicorp/terraform-provider-local/issues/452))

## 2.5.3 (May 08, 2025)

NOTES:

* Update dependencies ([#404](https://github.com/hashicorp/terraform-provider-local/issues/404))## 2.5.3-alpha1 (April 24, 2025)

NOTES:

* This release is being used to test new build and release actions. ([#405](https://github.com/hashicorp/terraform-provider-local/issues/405))

## 2.5.2 (September 11, 2024)

NOTES:

* all: This release introduces no functional changes. It does however include dependency updates which address upstream CVEs. ([#348](https://github.com/hashicorp/terraform-provider-local/issues/348))

## 2.5.1 (March 11, 2024)

NOTES:

* No functional changes from v2.5.0. Minor documentation fixes. ([#303](https://github.com/hashicorp/terraform-provider-local/issues/303))

## 2.5.0 (March 11, 2024)

FEATURES:

* functions/direxists: Added a new `direxists` function that checks for the existence of a directory, similar to the built-in `fileexists` function. ([#285](https://github.com/hashicorp/terraform-provider-local/issues/285))

## 2.4.1 (December 12, 2023)

NOTES:

* This release introduces no functional changes. It does however include dependency updates which address upstream CVEs. ([#273](https://github.com/hashicorp/terraform-provider-local/issues/273))

## 2.4.0 (March 08, 2023)

NOTES:

* This Go module has been updated to Go 1.19 per the [Go support policy](https://golang.org/doc/devel/release.html#policy). Any consumers building on earlier Go versions may experience errors. ([#184](https://github.com/hashicorp/terraform-provider-local/issues/184))

FEATURES:

* resource/local_file: added support for `MD5`, `SHA1`, `SHA256`, and `SHA512` checksum outputs. ([#142](https://github.com/hashicorp/terraform-provider-local/issues/142))
* resource/local_sensitive_file: added support for `MD5`, `SHA1`, `SHA256`, and `SHA512` checksum outputs. ([#142](https://github.com/hashicorp/terraform-provider-local/issues/142))
* data-source/local_file: added support for `MD5`, `SHA1`, `SHA256`, and `SHA512` checksum outputs. ([#142](https://github.com/hashicorp/terraform-provider-local/issues/142))
* data-source/local_sensitive-file: added support for `MD5`, `SHA1`, `SHA256`, and `SHA512` checksum outputs. ([#142](https://github.com/hashicorp/terraform-provider-local/issues/142))

## 2.3.0 (January 11, 2023)

NOTES:

* provider: Rewritten to use the [`terraform-plugin-framework`](https://www.terraform.io/plugin/framework) ([#155](https://github.com/hashicorp/terraform-provider-local/issues/155))

## 2.2.3 (May 18, 2022)

NOTES:

* resource/local_file: Update docs to prevent confusion that exactly one of the arguments `content`,
  `sensitive_content`, `content_base64`, and `source` needs to be specified ([#123](https://github.com/hashicorp/terraform-provider-local/pull/123)).

* resource/local_sensitive_file: Update docs to prevent confusion that exactly one of the arguments `content`,
  `content_base64`, and `source` needs to be specified ([#123](https://github.com/hashicorp/terraform-provider-local/pull/123)).

* No functional changes from 2.2.2.

## 2.2.2 (March 11, 2022)

NOTES:

* resource/local_sensitive_file: Fixed typo in documentation (default permission is `"0700"`, not `"0777"`).
* No functional changes from 2.2.1.

## 2.2.1 (March 10, 2022)

NOTES:

* This release is a republishing of the 2.2.0 release to fix release asset checksum errors. It is identical otherwise.

## 2.2.0 (March 10, 2022)

NOTES:

* resource/local_file: Argument `sensitive_content` is `Deprecated`. For creating or accessing files containing sensitive data,
  please use the new resource and data source `local_sensitive_file`.
  Both are identical to their `local_file` counterparts, but `content` and `content_base64` attributes are marked as _sensitive_.

FEATURES:

* **New Data Source:** `local_sensitive_file` ([#101](https://github.com/hashicorp/terraform-provider-local/pull/101) and [#106](https://github.com/hashicorp/terraform-provider-local/pull/106))
* **New Resource:** `local_sensitive_file` ([#106](https://github.com/hashicorp/terraform-provider-local/pull/106))

## 2.1.0 (February 19, 2021)

NOTES:

* Binary releases of this provider now include the` darwin-arm64` platform.
* This version contains no further changes.

## 2.0.0 (October 14, 2020)

NOTES:

* Binary releases of this provider now include the `linux-arm64` platform.

BREAKING CHANGES:

* Upgrade to version 2 of the Terraform Plugin SDK, which drops support for Terraform 0.11.
  This provider will continue to work as expected for users of Terraform 0.11, which will not download the new version.
  ([#42](https://github.com/terraform-providers/terraform-provider-local/issues/42))

FEATURES:

* resource/local_file: Added `source` attribute as alternative way to provide content
  for the `local_file` resource.
  ([#44](https://github.com/terraform-providers/terraform-provider-local/issues/44))

## 1.4.0 (September 30, 2019)

NOTES:

* The provider has switched to the standalone TF SDK, there should be no noticeable impact on compatibility.
  ([#32](https://github.com/terraform-providers/terraform-provider-local/issues/32))

FEATURES:

* resource/local_file: Added support for configurable permissions
  ([#30](https://github.com/terraform-providers/terraform-provider-local/issues/30))

## 1.3.0 (June 26, 2019)

FEATURES:

* resource/local_file: Added support for base64 encoded content
  ([#29](https://github.com/terraform-providers/terraform-provider-local/issues/29))
* data-source/local_file: Added support for base64 encoded content
  ([#29](https://github.com/terraform-providers/terraform-provider-local/issues/29))

## 1.2.2 (May 01, 2019)

NOTES:

* This releases includes another Terraform SDK upgrade intended to align with that being used for other providers
  as we prepare for the Core `v0.12.0` release. It should have no significant changes in behavior for this provider.

## 1.2.1 (April 11, 2019)

NOTES:

* This releases includes only a Terraform SDK upgrade intended to align with that being used for other providers
  as we prepare for the Core `v0.12.0` release. It should have no significant changes in behavior for this provider.

## 1.2.0 (March 20, 2019)

FEATURES:

* The provider is now compatible with Terraform v0.12, while retaining compatibility with prior versions.
* resource/local_file: added optional `sensitive_content` attribute, which can be used instead of `content`
  in situations where the content contains sensitive information that should not be displayed in a rendered diff.
  ([#9](https://github.com/terraform-providers/terraform-provider-local/issues/9))

## 1.1.0 (January 04, 2018)

FEATURES:

* data-source/local_file: Added for reading files in a way that participates in Terraform's dependency graph,
  which allows reading of files that are created dynamically during `terraform apply`.
  ([#6](https://github.com/terraform-providers/terraform-provider-local/issues/6))

## 1.0.0 (September 15, 2017)

NOTES:

* No changes from 0.1.0; just adjusting to
  [the new version numbering scheme](https://www.hashicorp.com/blog/hashicorp-terraform-provider-versioning/).

## 0.1.0 (June 21, 2017)

NOTES:

* Same functionality as that of Terraform 0.9.8.
  Repacked as part of [Provider Splitout](https://www.hashicorp.com/blog/upcoming-provider-changes-in-terraform-0-10/)


