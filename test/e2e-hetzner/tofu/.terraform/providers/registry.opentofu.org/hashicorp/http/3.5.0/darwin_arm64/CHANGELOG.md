## 3.5.0 (April 23, 2025)

FEATURES:

* Add mTLS support (client cert & key) to http datasource ([#211](https://github.com/hashicorp/terraform-provider-http/issues/211))

## 3.4.5 (September 10, 2024)

NOTES:

* all: This release introduces no functional changes. It does however include dependency updates which address upstream CVEs. ([#452](https://github.com/hashicorp/terraform-provider-http/issues/452))

## 3.4.4 (July 31, 2024)

NOTES:

* data-source/http: Previous versions of this provider ignored any `Host` headers specified in the `request_headers` attribute when setting the HTTP request. Any specified `Host` request header will now be set on the HTTP request.

For example, in the following configuration:
```hcl
data "http" "example" {
  url      = "https://www.example.com"
  request_headers = {
    Host = "www.differentexample.com"
  }
}
```
The HTTP request URL host is still `www.example.com` but the HTTP request `Host` header will now be `www.differentexample.com` instead of `www.example.com`.
 ([#440](https://github.com/hashicorp/terraform-provider-http/issues/440))

BUG FIXES:

* data-source/http: Allow `Host` header in `request_headers` to be set on HTTP request ([#440](https://github.com/hashicorp/terraform-provider-http/issues/440))

## 3.4.3 (June 03, 2024)

BUG FIXES:

* data-source/http: Avoid potentially leaking URL-embedded basic authentication credentials in logs and error messages ([#429](https://github.com/hashicorp/terraform-provider-http/issues/429))

## 3.4.2 (February 29, 2024)

NOTES:

* data-source/http: Previously the HTTP request would unexpectedly always contain a body for all requests. Certain HTTP server implementations are sensitive to this data existing if it is not expected. Requests now only contain a request body if the `request_body` attribute is explicitly set. To exactly preserve the previous behavior, set `request_body = ""`. ([#388](https://github.com/hashicorp/terraform-provider-http/issues/388))

BUG FIXES:

* data-source/http: Ensured HTTP request body is not sent unless configured ([#388](https://github.com/hashicorp/terraform-provider-http/issues/388))

## 3.4.1 (December 19, 2023)

BUG FIXES:

* data-source/http: Includes update to go-retryablehttp fixing preservation of request body on temporary redirects or re-established HTTP/2 connections ([#346](https://github.com/hashicorp/terraform-provider-http/issues/346))

## 3.4.0 (June 21, 2023)

ENHANCEMENTS:

* data-source/http: `response_body_base64` has been added and contains a standard base64 encoding of the response body ([#158](https://github.com/hashicorp/terraform-provider-http/issues/158))
* data-source/http: Replaced issuing warning on the basis of possible non-text `Content-Type` with issuing warning if response body does not contain valid UTF-8. ([#158](https://github.com/hashicorp/terraform-provider-http/issues/158))

## 3.3.0 (April 25, 2023)

NOTES:

* This Go module has been updated to Go 1.19 per the [Go support policy](https://golang.org/doc/devel/release.html#policy). Any consumers building on earlier Go versions may experience errors. ([#245](https://github.com/hashicorp/terraform-provider-http/issues/245))

ENHANCEMENTS:

* data-source/http: Added `retry` with nested `attempts`, `max_delay_ms` and `min_delay_ms` ([#151](https://github.com/hashicorp/terraform-provider-http/issues/151))
* data-source/http: Added `request_timeout_ms` ([#151](https://github.com/hashicorp/terraform-provider-http/issues/151))

## 3.2.1 (November 7, 2022)

BUG FIXES

* data-source/http: Using DefaultTransport to reinstate previous behavior (e.g., ProxyFromEnvironment) ([#198](https://github.com/hashicorp/terraform-provider-http/pull/198)).

## 3.2.0 (October 31, 2022)

ENHANCEMENTS:

* data-source/http: Added `ca_cert_pem` attribute which allows PEM encoded certificate(s) to be included in the set of root certificate authorities used when verifying server certificates ([#125](https://github.com/hashicorp/terraform-provider-http/pull/125)).
* data-source/http: Added `insecure` attribute to allow disabling the verification of a server's certificate chain and host name. Defaults to `false` ([#125](https://github.com/hashicorp/terraform-provider-http/pull/125)).

## 3.1.0 (August 30, 2022)

ENHANCEMENTS:

* data-source/http: Allow optionally specifying HTTP request method and body ([#21](https://github.com/hashicorp/terraform-provider-http/issues/21)).

## 3.0.1 (July 27, 2022)

BUG FIXES

* data-source/http: Reinstated previously deprecated and removed `body` attribute ([#166](https://github.com/hashicorp/terraform-provider-http/pull/166)).


## 3.0.0 (July 27, 2022)

NOTES:

* Provider has been re-written using the new [`terraform-plugin-framework`](https://www.terraform.io/plugin/framework) ([#177](https://github.com/hashicorp/terraform-provider-http/pull/142)).

BREAKING CHANGES:

* data-source/http: Response status code is not checked anymore. A new read-only attribute, `status_code`, has been added. It can be used either with
  [precondition and postcondition](https://www.terraform.io/language/expressions/custom-conditions#preconditions-and-postconditions) checks (Terraform >= 1.2.0), or, for instance, 
  with [local-exec Provisioner](https://www.terraform.io/language/resources/provisioners/local-exec) ([114](https://github.com/hashicorp/terraform-provider-http/pull/114)).
* data-source/http: Deprecated `body` has been removed ([#137](https://github.com/hashicorp/terraform-provider-http/pull/137)).

## 2.2.0 (June 02, 2022)

ENHANCEMENTS:

* data-source/http: `body` is now deprecated and has been superseded by `response_body`. `body` will be removed in the next major release ([#137](https://github.com/hashicorp/terraform-provider-http/pull/137)).  

NOTES:

* "Uplift" aligned with Utility Providers Upgrade ([#135](https://github.com/hashicorp/terraform-provider-http/issues/135)).

## 2.1.0 (February 19, 2021)

Binary releases of this provider now include the darwin-arm64 platform. This version contains no further changes.

## 2.0.0 (October 14, 2020)

Binary releases of this provider now include the linux-arm64 platform.

BREAKING CHANGES:

* Upgrade to version 2 of the Terraform Plugin SDK, which drops support for Terraform 0.11. This provider will continue to work as expected for users of Terraform 0.11, which will not download the new version. ([#47](https://github.com/terraform-providers/terraform-provider-http/issues/47))

IMPROVEMENTS:

* Relaxed error on non-text `Content-Type` headers to be a warning instead ([#50](https://github.com/terraform-providers/terraform-provider-http/issues/50))

BUG FIXES:

* Modified some of the documentation to work a bit better in the registry ([#42](https://github.com/terraform-providers/terraform-provider-http/issues/42))
* Allowed the `us-ascii` charset in addition to `utf-8` ([#43](https://github.com/terraform-providers/terraform-provider-http/issues/43))

## 1.2.0 (March 17, 2020)

IMPROVEMENTS:

* Switch to v1.7.0 of the standalone plugin SDK ([#35](https://github.com/terraform-providers/terraform-provider-http/issues/35))
* Added response_headers to datasource ([#31](https://github.com/terraform-providers/terraform-provider-http/issues/31))

BUG FIXES:

* Fix request error message to include the `err` and not just url ([#26](https://github.com/terraform-providers/terraform-provider-http/issues/26))

## 1.1.1 (May 01, 2019)

* This release includes an upgrade to the Terraform SDK, in an effort to help align with what other providers are releasing with, as we lead up to Core v0.12. It should have no noticeable impact on the provider.

## 1.1.0 (April 18, 2019)

IMPROVEMENTS:

* The provider is now compatible with Terraform v0.12, while retaining compatibility with prior versions.

## 1.0.1 (January 03, 2018)

* Allow `charset` argument on `Content-Type` ([#5](https://github.com/terraform-providers/terraform-provider-http/issues/5))

## 1.0.0 (September 14, 2017)

* add content type for ADFS FederationMetadata.xml ([#4](https://github.com/terraform-providers/terraform-provider-http/issues/4))

## 0.1.0 (June 20, 2017)

NOTES:

* Same functionality as that of Terraform 0.9.8. Repacked as part of [Provider Splitout](https://www.hashicorp.com/blog/upcoming-provider-changes-in-terraform-0-10/)
