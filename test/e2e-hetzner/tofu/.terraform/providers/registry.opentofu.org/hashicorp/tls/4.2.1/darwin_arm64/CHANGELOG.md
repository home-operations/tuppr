## 4.2.1 (January 30, 2026)

BUG FIXES:

* fix regression triggering a replace for certs due to max_path_length defaulting to -1 ([#737](https://github.com/hashicorp/terraform-provider-tls/issues/737))

## 4.2.0 (January 28, 2026)

FEATURES:

* ephemeral/tls_public_key: New ephemeral resource that extracts the public key from a private key. ([#668](https://github.com/hashicorp/terraform-provider-tls/issues/668))

ENHANCEMENTS:

* data-source/tls_certificats: Added `max_path_length` attribute ([#301](https://github.com/hashicorp/terraform-provider-tls/issues/301))
* resource/tls_self_signed_cert: Added optional `max_path_length` attribute ([#301](https://github.com/hashicorp/terraform-provider-tls/issues/301))
* resource/tls_locally_signed_cert: Added optional `max_path_length` attribute ([#301](https://github.com/hashicorp/terraform-provider-tls/issues/301))

## 4.1.0 (April 23, 2025)

NOTES:

* New [ephemeral resource](https://developer.hashicorp.com/terraform/language/resources/ephemeral) `tls_private_key` now supports [ephemeral values](https://developer.hashicorp.com/terraform/language/values/variables#exclude-values-from-state). ([#637](https://github.com/hashicorp/terraform-provider-tls/issues/637))

FEATURES:

* ephemeral/tls_private_key: New ephemeral resource that generates a PEM formatted private key. When used in combination with a managed resource write-only attribute, Terraform will not store the private key data in the plan or state file. ([#637](https://github.com/hashicorp/terraform-provider-tls/issues/637))

ENHANCEMENTS:

* tls_cert_request: Added optional `email_address` attribute ([#35](https://github.com/hashicorp/terraform-provider-tls/issues/35))
* tls_self_signed_cert: Added optional `email_address` attribute ([#35](https://github.com/hashicorp/terraform-provider-tls/issues/35))

## 4.0.6 (September 11, 2024)

NOTES:

* all: This release introduces no functional changes. It does however include dependency updates which address upstream CVEs. ([#552](https://github.com/hashicorp/terraform-provider-tls/issues/552))

## 4.0.5 (November 27, 2023)

NOTES:

* This release introduces no functional changes. It does however include dependency updates which address upstream CVEs. ([#432](https://github.com/hashicorp/terraform-provider-tls/issues/432))

## 4.0.4 (October 31, 2022)

BUG FIXES:

* resource/tls_locally_signed_cert: Ensure `terraform refresh` updates state when cert is ready for renewal ([#278](https://github.com/hashicorp/terraform-provider-tls/issues/278)).
* resource/tls_self_signed_cert: Ensure `terraform refresh` updates state when cert is ready for renewal ([#278](https://github.com/hashicorp/terraform-provider-tls/issues/278)).

## 4.0.3 (September 20, 2022)

BUG FIXES:

* resource/tls_locally_signed_cert: Prevented `Config Read Error` with Terraform version 1.3.0 and later
* resource/tls_self_signed_cert: Prevented `Config Read Error` with Terraform version 1.3.0 and later

## 4.0.2 (August 30, 2022)

BUG FIXES:

* resource/tls_cert_request: Fix regexp in attribute plan modifier to correctly match PEM ([#255](https://github.com/hashicorp/terraform-provider-tls/issues/255)).
* resource/tls_locally_signed_cert: Fix regexp in attribute plan modifier to correctly match PEM ([#255](https://github.com/hashicorp/terraform-provider-tls/issues/255)).
* resource/tls_self_signed_cert: Fix regexp in attribute plan modifier to correctly match PEM ([#255](https://github.com/hashicorp/terraform-provider-tls/issues/255)).

## 4.0.1 (July 25, 2022)

BUG FIXES:

* data-source/tls_certificate: Prevented `empty list of object` error with `certificates` attribute ([#244](https://github.com/hashicorp/terraform-provider-tls/issues/244)).

## 4.0.0 (July 21, 2022)

NOTES:

* Provider has been re-written using the new [`terraform-plugin-framework`](https://www.terraform.io/plugin/framework) ([#215](https://github.com/hashicorp/terraform-provider-tls/pull/215)).

* resource/tls_cert_request: `private_key_pem` attribute is now stored in the state _as-is_; first apply may result in an update-in-place ([#87](https://github.com/hashicorp/terraform-provider-tls/issues/87), [#215](https://github.com/hashicorp/terraform-provider-tls/pull/215)).

* resource/tls_self_signed_cert: `private_key_pem` attribute is now stored in the state _as-is_; first apply may result in an update-in-place ([#87](https://github.com/hashicorp/terraform-provider-tls/issues/87), [#215](https://github.com/hashicorp/terraform-provider-tls/pull/215)).

* resource/tls_locally_signed_cert: `cert_request_pem`, `ca_private_key_pem` and `ca_cert_pem` attributes are now stored in the state _as-is_; first apply may result in an update-in-place ([#87](https://github.com/hashicorp/terraform-provider-tls/issues/87), [#215](https://github.com/hashicorp/terraform-provider-tls/pull/215)).
  
* resource/tls_private_key: `private_key_pem_pkcs8`, `private_key_openssh` and `public_key_fingerprint_sha256` attributes are now retro-fitted, depending on version being updated; first apply may result in an update-in-place ([#210](https://github.com/hashicorp/terraform-provider-tls/issues/210), [#225](https://github.com/hashicorp/terraform-provider-tls/pull/225))).

ENHANCEMENTS:

* resource/tls_private_key: New attribute `private_key_pem_pkcs8` ([PKCS#8](https://datatracker.ietf.org/doc/html/rfc5208)) ([#210](https://github.com/hashicorp/terraform-provider-tls/issues/210), [#225](https://github.com/hashicorp/terraform-provider-tls/pull/225))).

BREAKING CHANGES:

* resource/tls_cert_request: Attribute `key_algorithm` is now read-only, as it's inferred from `private_key_pem` ([#174](https://github.com/hashicorp/terraform-provider-tls/issues/174), [#215](https://github.com/hashicorp/terraform-provider-tls/pull/215)).
* resource/tls_self_signed_cert: Attribute `private_key_pem` is stored (and returned) _as-is_ (in accordance with [guidelines](https://www.terraform.io/plugin/sdkv2/best-practices/sensitive-state#don-t-encrypt-state)) ([#87](https://github.com/hashicorp/terraform-provider-tls/issues/87), [#215](https://github.com/hashicorp/terraform-provider-tls/pull/215)).

* resource/tls_self_signed_cert: Attribute `key_algorithm` is now read-only, as it's inferred from `private_key_pem` ([#174](https://github.com/hashicorp/terraform-provider-tls/issues/174), [#215](https://github.com/hashicorp/terraform-provider-tls/pull/215)).
* resource/tls_self_signed_cert: Setting an unsupported value in `allowed_uses` attribute, will now return an error instead of just a warning ([#185](https://github.com/hashicorp/terraform-provider-tls/issues/185), [#215](https://github.com/hashicorp/terraform-provider-tls/pull/215)).
* resource/tls_self_signed_cert: Attribute `private_key_pem` is stored (and returned) _as-is_ (in accordance with [guidelines](https://www.terraform.io/plugin/sdkv2/best-practices/sensitive-state#don-t-encrypt-state)) ([#87](https://github.com/hashicorp/terraform-provider-tls/issues/87), [#215](https://github.com/hashicorp/terraform-provider-tls/pull/215)).

* resource/tls_locally_signed_cert: Attribute `ca_key_algorithm` is now read-only, as it's inferred from `ca_private_key_pem` ([#174](https://github.com/hashicorp/terraform-provider-tls/issues/174), [#215](https://github.com/hashicorp/terraform-provider-tls/pull/215)).
* resource/tls_locally_signed_cert: Setting an unsupported value in `allowed_uses` attribute, will now return an error instead of just a warning ([#185](https://github.com/hashicorp/terraform-provider-tls/issues/185), [#215](https://github.com/hashicorp/terraform-provider-tls/pull/215)).
* resource/tls_locally_signed_cert: Attributes `cert_request_pem`, `ca_private_key_pem`, `ca_cert_pem` are stored (and returned) _as-is_ (in accordance with [guidelines](https://www.terraform.io/plugin/sdkv2/best-practices/sensitive-state#don-t-encrypt-state)) ([#87](https://github.com/hashicorp/terraform-provider-tls/issues/87), [#215](https://github.com/hashicorp/terraform-provider-tls/pull/215)).

* provider: Default value for `proxy.from_env` is now `true`, and relies upon [`httpproxy.FromEnvironment`](https://pkg.go.dev/golang.org/x/net/http/httpproxy#FromEnvironment) ([#224](https://github.com/hashicorp/terraform-provider-tls/pull/224)).

## 3.4.0 (May 16, 2022)

NEW FEATURES:

* data-source/tls_certificate: New attribute `content` that can be used in alternative to `url`, to provide the certificate in PEM format ([#189](https://github.com/hashicorp/terraform-provider-tls/pull/189)).
* data-source/tls_certificate: Objects in the `certificates` chain attribute expose a new attribute `cert_pem` (PEM format) ([#208](https://github.com/hashicorp/terraform-provider-tls/pull/208)).

* resource/tls_self_signed_cert: New attribute `set_authority_key_id` to make the generated certificate include an [authority key identifier](https://datatracker.ietf.org/doc/html/rfc5280#section-4.2.1.1) ([#212](https://github.com/hashicorp/terraform-provider-tls/pull/212)).

ENHANCEMENTS:

* resource/tls_locally_signed_cert: If CA provided via `ca_cert_pem` is not an actual CA, a warning will be raised, but the certificate will still be created ([#209](https://github.com/hashicorp/terraform-provider-tls/pull/209)). 

NOTES:

* data-source/tls_certificate: The `id` attribute has changed to the hashing of all certificates information in the chain. The first apply of this updated data source may show this difference ([#189](https://github.com/hashicorp/terraform-provider-tls/pull/189)).

BUG FIXES:

* data-source/tls_certificate: Prevent plan differences with the `id` attribute ([#79](https://github.com/hashicorp/terraform-provider-tls/issues/79), [#189](https://github.com/hashicorp/terraform-provider-tls/pull/189)).

* resource/tls_cert_request: Allow for absent or empty `subject` block ([#209](https://github.com/hashicorp/terraform-provider-tls/pull/209)).

* resource/tls_self_signed_cert: Allow for absent or empty `subject` block ([#209](https://github.com/hashicorp/terraform-provider-tls/pull/209)).

## 3.3.0 (April 07, 2022)

NEW FEATURES:

* provider: Added (opt-in) HTTP `proxy` configuration ([#179](https://github.com/hashicorp/terraform-provider-tls/pull/179)).

* data-source/tls_certificate: Support for `tls://` scheme in `url` argument. When used, the provider will fetch certificates via a direct Secure Socket (i.e. ignores proxy) ([#179](https://github.com/hashicorp/terraform-provider-tls/pull/179)).

ENHANCEMENTS:

* data-source/tls_certificate: When `proxy` is configured on provider, certificates fetched via `url` with scheme `https://` will go through the specified HTTP proxy ([#179](https://github.com/hashicorp/terraform-provider-tls/pull/179)).

* resource/tls_locally_signed_cert: Validate `allowed_uses` contains documented values, but raise warning instead of error when it does not ([#184](https://github.com/hashicorp/terraform-provider-tls/pull/184)).

## 3.2.1 (April 05, 2022)

BUG FIXES:

* resource/tls_locally_signed_cert: Fix issue preventing the generation of [subject key identifier](https://datatracker.ietf.org/doc/html/rfc5280#section-4.2.1.2) for private keys using ED25519 ([#182](https://github.com/hashicorp/terraform-provider-tls/pull/182)).

* resource/tls_self_signed_cert: Fix issue preventing the generation of [subject key identifier](https://datatracker.ietf.org/doc/html/rfc5280#section-4.2.1.2) for private keys using ED25519 ([#182](https://github.com/hashicorp/terraform-provider-tls/pull/182)).

## 3.2.0 (April 04, 2022)

NEW FEATURES:

* resource/tls_private_key: Added support for [ED25519](https://ed25519.cr.yp.to/) key algorithm ([#151](https://github.com/hashicorp/terraform-provider-tls/pull/151)).

* data-source/tls_public_key: Added support for [ED25519](https://ed25519.cr.yp.to/) key algorithm ([#160](https://github.com/hashicorp/terraform-provider-tls/pull/160)).

* resource/tls_cert_request: Added support for [ED25519](https://ed25519.cr.yp.to/) key algorithm ([#173](https://github.com/hashicorp/terraform-provider-tls/pull/173)).

* resource/tls_self_signed_cert: Added support for [ED25519](https://ed25519.cr.yp.to/) key algorithm ([#173](https://github.com/hashicorp/terraform-provider-tls/pull/173)).

* resource/tls_locally_signed_cert: Added support for [ED25519](https://ed25519.cr.yp.to/) key algorithm ([#173](https://github.com/hashicorp/terraform-provider-tls/pull/173)).

ENHANCEMENTS:

* resource/tls_private_key: New attributes `private_key_openssh` (OpenSSH PEM format) and `public_key_fingerprint_sha256` ([#151](https://github.com/hashicorp/terraform-provider-tls/pull/151)).

* data-source/tls_public_key: Can now be configured by passing a private key either via `private_key_pem` or `private_key_openssh` ([#160](https://github.com/hashicorp/terraform-provider-tls/pull/160)).

* resource/tls_locally_signed_cert: Validate `validity_period_hours` and `early_renewal_hours` are greater or equal then zero ([#169](https://github.com/hashicorp/terraform-provider-tls/pull/169)).
* resource/tls_locally_signed_cert: Validate `allowed_uses` contains documented values, instead of silently ignoring unknowns ([#169](https://github.com/hashicorp/terraform-provider-tls/pull/169)).
* resource/tls_locally_signed_cert: `ca_key_algorithm` is now optional and deprecated, as it's now inferred from `ca_private_key_pem`. It will be read-only in the next major release ([#173](https://github.com/hashicorp/terraform-provider-tls/pull/173)).

* resource/tls_self_signed_cert: Validate `validity_period_hours` and `early_renewal_hours` are greater or equal then zero ([#169](https://github.com/hashicorp/terraform-provider-tls/pull/169)).
* resource/tls_self_signed_cert: Validate `allowed_uses` contains documented values, instead of silently ignoring unknowns ([#169](https://github.com/hashicorp/terraform-provider-tls/pull/169)).
* resource/tls_self_signed_cert: `key_algorithm` is now optional and deprecated, as it's now inferred from `private_key_pem`. It will be read-only in the next major release ([#173](https://github.com/hashicorp/terraform-provider-tls/pull/173)).

* resource/tls_cert_request: `key_algorithm` is now optional and deprecated, as it's now inferred from `private_key_pem`. It will be read-only in the next major release ([#173](https://github.com/hashicorp/terraform-provider-tls/pull/173)).

NOTES:

* Upgraded to Golang 1.17 ([#156](https://github.com/hashicorp/terraform-provider-tls/pull/156))
* Adopted [`golangci-lint`](https://golangci-lint.run/) as part of CI ([#155](https://github.com/hashicorp/terraform-provider-tls/pull/155))
* Acceptance tests now run against all minor versions of Terraform >= 0.12 ([#153](https://github.com/hashicorp/terraform-provider-tls/pull/153))

## 3.1.0 (February 19, 2021)

Binary releases of this provider now include the darwin-arm64 platform. This version contains no further changes.

## 3.0.0 (October 14, 2020)

Binary releases of this provider will now include the linux-arm64 platform.

BREAKING CHANGES:

* Upgrade to version 2 of the Terraform Plugin SDK, which drops support for Terraform 0.11. This provider will continue to work as expected for users of Terraform 0.11, which will not download the new version. ([#83](https://github.com/terraform-providers/terraform-provider-tls/issues/83))

## 2.2.0 (July 24, 2020)

NEW FEATURES:

* Add `tls_certificate` data source ([#62](https://github.com/terraform-providers/terraform-provider-tls/issues/62))

## 2.1.1 (September 25, 2019)

NOTES:

* The provider has switched to the standalone TF SDK, there should be no noticeable impact on compatibility. ([#54](https://github.com/terraform-providers/terraform-provider-tls/issues/54))

## 2.1.0 (August 16, 2019)

ENHANCEMENTS:

* Certificate renewal is now handled as a "replace" action in the plan, rather than by behaving as if the expired certificate had been deleted. Although the effective behavior remains unchanged, renewal will now appear as a `-/+` action in the plan, rather than just as a `+`. ([#34](https://github.com/terraform-providers/terraform-provider-tls/issues/34))
* Certificates can now have URIs as subject alternative names. ([#50](https://github.com/terraform-providers/terraform-provider-tls/issues/50))
* Certificates can now optionally have the Subject Key ID field populated. ([#31](https://github.com/terraform-providers/terraform-provider-tls/issues/31))

BUG FIXES:

* More of the private key arguments are now marked as "sensitive" so that Terraform will know to hide their values when showing plans and state in response to various commands. ([#48](https://github.com/terraform-providers/terraform-provider-tls/issues/48))
* In `tls_public_key`, don't panic if the PEM isn't valid PEM syntax at all. ([#40](https://github.com/terraform-providers/terraform-provider-tls/issues/40))

## 2.0.1 (April 30, 2019)

* This release includes an upgraded Terraform SDK, for the sake of aligning versions of the SDK amongst released providers, as we lead up to Core v0.12. This should have no noticeable impact on the provider.

## 2.0.0 (April 17, 2019)

IMPROVEMENTS:

* The provider is now compatible with Terraform v0.12, while retaining compatibility with prior versions.

## 1.2.0 (August 15, 2018)

FEATURES: 

* `tls_private_key` (both datasource and resource) include MD5 public key fingerprints as read-only attributes.


BUG FIXES:
* `tls_cert_request` and `tls_self_signed_cert`: changes to `subject` now
  correctly force the recreation of the resource, instead of returning an error
  ([#18](https://github.com/terraform-providers/terraform-provider-tls/issues/18))

## 1.1.0 (March 09, 2018)

FEATURES:

* **New Data Source:** `tls_public_key`
  ([#11](https://github.com/terraform-providers/terraform-provider-tls/issues/11))

## 1.0.1 (November 09, 2017)

BUG FIXES:

* `tls_cert_request` and `tls_self_signed_cert` no longer cause a crash when
  `subject` isn't specified.
  ([#7](https://github.com/terraform-providers/terraform-provider-tls/issues/7))
* `tls_cert_request` and `tls_self_signed_cert` no longer generate empty-string
  values for various subject fields when they are not set in configuration.
  ([#10](https://github.com/terraform-providers/terraform-provider-tls/issues/10))

## 1.0.0 (September 15, 2017)

* No changes from 0.1.0; just adjusting to [the new version numbering
  scheme](https://www.hashicorp.com/blog/hashicorp-terraform-provider-versioning/).

## 0.1.0 (June 21, 2017)

NOTES:

* Same functionality as that of Terraform 0.9.8. Repacked as part of [Provider
  Splitout](https://www.hashicorp.com/blog/upcoming-provider-changes-in-terraform-0-10/)
