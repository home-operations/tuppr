# Kubernetes "kubectl" Provider

![Build Status](https://github.com/alekc/terraform-provider-kubectl/actions/workflows/tests.yaml/badge.svg) [![user guide](https://img.shields.io/badge/-user%20guide-blue)](https://registry.terraform.io/providers/alekc/kubectl)

This provider offers the most effective method for handling Kubernetes resources in Terraform. It empowers you to leverage what Kubernetes values most — YAML.

At the heart of this provider lies the `kubectl_manifest` resource, enabling the processing and application of free-form YAML directly to Kubernetes. This YAML object is monitored across its full lifecycle — creation, updates, drift detection and deletion.

For reads, the provider exposes both a `data "kubectl_manifest"` source for ordinary lookups and an `ephemeral "kubectl_manifest"` resource (Terraform 1.10+) that fetches Secret payloads, freshly-minted tokens, and any other sensitive data without ever writing the value to `terraform.tfstate`.

The `terraform-provider-kubectl` has gained widespread adoption in numerous large Kubernetes installations, serving as the primary tool for orchestrating the complete lifecycle of Kubernetes resources.

## What's in this provider

| Type               | Name                                                                      | Purpose                                                                                                                                                                                                                                                             |
| ------------------ | ------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Resource           | [`kubectl_manifest`](./docs/resources/kubectl_manifest.md)                | Apply a raw YAML manifest to the cluster (full create / update / delete + drift detection).                                                                                                                                                                         |
| Resource           | [`kubectl_server_version`](./docs/resources/kubectl_server_version.md)    | Read API-server version info, with `triggers` for use in `depends_on` chains.                                                                                                                                                                                       |
| Data source        | [`kubectl_manifest`](./docs/data-sources/kubectl_manifest.md)             | Read any object from the cluster by GVK + name (+ namespace) and extract fields by dot-path.                                                                                                                                                                        |
| Data source        | [`kubectl_server_version`](./docs/data-sources/kubectl_server_version.md) | Read API-server version info.                                                                                                                                                                                                                                       |
| Data source        | [`kubectl_file_documents`](./docs/data-sources/kubectl_file_documents.md) | Split a multi-document YAML string into individual documents.                                                                                                                                                                                                       |
| Data source        | [`kubectl_filename_list`](./docs/data-sources/kubectl_filename_list.md)   | Glob a directory for YAML files.                                                                                                                                                                                                                                    |
| Data source        | [`kubectl_path_documents`](./docs/data-sources/kubectl_path_documents.md) | Glob a directory and split every matched file into individual documents.                                                                                                                                                                                            |
| Ephemeral resource | [`kubectl_manifest`](./docs/ephemeral-resources/kubectl_manifest.md)      | Read any cluster object without ever writing the value to `terraform.tfstate` or the plan file. Required for Secret payloads, freshly-minted tokens, private keys, and anything else you must keep out of state. Re-fetched on every plan / apply. Terraform 1.10+. |

## Supported Kubernetes and Terraform versions

Every PR is exercised against the matrix below on `kind`. The matrix is regenerated from [endoflife.date](https://endoflife.date) on each CI run, so it tracks the four most recent active Kubernetes release cycles and the four most recent stable Terraform minors, plus a legacy `1.5.7` cell (the last MPL-licensed Terraform release).

|                 | Terraform 1.15 | Terraform 1.14 | Terraform 1.13 | Terraform 1.12 | Terraform 1.5.7 |
| --------------- | :------------: | :------------: | :------------: | :------------: | :-------------: |
| Kubernetes 1.36 |   smoke + ✅   |       ✅       |       ✅       |       ✅       |       ✅        |
| Kubernetes 1.35 |       ✅       |       ✅       |       ✅       |       ✅       |       ✅        |
| Kubernetes 1.34 |       ✅       |       ✅       |       ✅       |       ✅       |       ✅        |
| Kubernetes 1.33 |       ✅       |       ✅       |       ✅       |       ✅       |       ✅        |

The versions in the table are the snapshot resolved at the time of writing; the live matrix moves with the upstream release cadence. The newest pair (latest Kubernetes × latest Terraform) is run first as a single **smoke** job; the remaining 19 combinations fan out only after smoke passes. Combinations outside this grid may still work — your mileage may vary.

## Installation

### Terraform 0.13+

The provider can be installed and managed automatically by Terraform. Sample `versions.tf` file:

```hcl
terraform {
  required_version = ">= 1.0"

  required_providers {
    kubectl = {
      source  = "alekc/kubectl"
      version = "~> 2.3"
    }
  }
}
```

The provider itself works back to Terraform 0.13. The example above pins `>= 1.0` because most users will be on a supported Terraform release; if you need to run older Terraform, you can drop the constraint to `>= 0.13`.

If your configuration uses the `ephemeral "kubectl_manifest"` block (covered below), the floor moves up to **Terraform 1.10**, because ephemeral resources are a 1.10+ language feature regardless of the provider version. Bump `required_version` to `">= 1.10"` in that case.

### Install manually

If you don't want to use the one-liner above, you can download a binary for your system from the [release page](https://github.com/alekc/terraform-provider-kubectl/releases), then either place it at the root of your Terraform folder or in the Terraform plugin folder on your system.

## Quick Start

```hcl
provider "kubectl" {
  host                   = var.eks_cluster_endpoint
  cluster_ca_certificate = base64decode(var.eks_cluster_ca)
  token                  = data.aws_eks_cluster_auth.main.token
  load_config_file       = false
}

resource "kubectl_manifest" "test" {
    yaml_body = <<YAML
apiVersion: couchbase.com/v1
kind: CouchbaseCluster
metadata:
  name: name-here-cluster
spec:
  baseImage: name-here-image
  version: name-here-image-version
  authSecret: name-here-operator-secret-name
  exposeAdminConsole: true
  adminConsoleServices:
    - data
  cluster:
    dataServiceMemoryQuota: 256
    indexServiceMemoryQuota: 256
    searchServiceMemoryQuota: 256
    eventingServiceMemoryQuota: 256
    analyticsServiceMemoryQuota: 1024
    indexStorageSetting: memory_optimized
    autoFailoverTimeout: 120
    autoFailoverMaxCount: 3
    autoFailoverOnDataDiskIssues: true
    autoFailoverOnDataDiskIssuesTimePeriod: 120
    autoFailoverServerGroup: false
YAML
}
```

See the [User Guide](https://registry.terraform.io/providers/alekc/kubectl/latest) for details on installation and all the provided data and resource types.

### Reading sensitive data without persisting it

For data that must never reach `terraform.tfstate` (Secret payloads, freshly-minted tokens, private keys), use the `ephemeral "kubectl_manifest"` resource. It has the same lookup shape as the data source, but the value is re-fetched on every plan / apply and never persisted. Requires Terraform 1.10+.

Ephemeral values cannot flow through `output` blocks. They are consumed during apply through a resource's write-only attribute (Terraform 1.11+), a provisioner, or a `check` block:

```hcl
ephemeral "kubectl_manifest" "db_creds" {
  api_version = "v1"
  kind        = "Secret"
  name        = "postgres-credentials"
  namespace   = "data"

  fields = {
    password = "data.password"
  }
}

# Example consumer: stage the password into a sibling tool's config
# during apply, never to state. `content_wo` is a write-only attribute
# (Terraform 1.11+); the value is forgotten after the file is written.
resource "local_file" "db_password" {
  filename            = "${path.module}/.db-password"
  content_wo          = ephemeral.kubectl_manifest.db_creds.results["password"]
  content_wo_revision = 1
}
```

See [`docs/ephemeral-resources/kubectl_manifest.md`](./docs/ephemeral-resources/kubectl_manifest.md) for the full reference, additional consumer patterns (including `check` blocks for cluster invariants), and behaviour notes.

### Reading existing objects

A `data "kubectl_manifest"` block reads any cluster object by `api_version` + `kind` + `name` (+ `namespace`) and extracts user-named fields via gojsonq dot-paths. The fetched object is also exposed as raw `yaml` and `json`. Use this when the value is non-sensitive and you want it cached in state; reach for the ephemeral resource above when it is not.

```hcl
data "kubectl_manifest" "ns" {
  api_version = "v1"
  kind        = "Namespace"
  name        = "kube-system"
  fields = {
    phase = "status.phase"
  }
}
```

See [`docs/data-sources/kubectl_manifest.md`](./docs/data-sources/kubectl_manifest.md) for the full reference.

---

## Contributing

Building from source, pointing Terraform at a local build with
`dev_overrides`, running the test suites, and the PR workflow are all
documented in [CONTRIBUTING.md](./CONTRIBUTING.md). Start there if you
want to try an unreleased fix from `master` or submit a change.

## Changing providers for existing resources

When you used another fork of this provider in the past, you can switch the provider on all existing resources within your state. A common use-case is moving from `gavinbunney/kubectl` to this fork.

Change the `required_providers` block in your root module and all child modules to use `alekc/kubectl` as shown in the [Installation](#terraform-013) section above. Then use `state replace-provider` to update existing state:

```sh
terraform state replace-provider gavinbunney/kubectl alekc/kubectl
```

Run `terraform init` afterwards; subsequent terraform actions will use this provider.

### Inspiration

Thanks to the original provider by [gavinbunney](https://github.com/gavinbunney/terraform-provider-kubectl) — this fork was originally based on version 1.14 and has followed a separate development path since.
