# Kubernetes "kubectl" Provider

![Build Status](https://github.com/alekc/terraform-provider-kubectl/actions/workflows/tests.yaml/badge.svg) [![user guide](https://img.shields.io/badge/-user%20guide-blue)](https://registry.terraform.io/providers/alekc/kubectl)

This provider offers the most effective method for handling Kubernetes resources in Terraform. It empowers you to leverage what Kubernetes values most - YAML!

At the heart of this provider lies the kubectl_manifest resource, enabling the processing and application of free-form YAML directly to Kubernetes. This YAML object is meticulously monitored and manages the entire lifecycle, from creation and updates to seamless deletion, including drift detection.

The terraform-provider-kubectl has gained widespread adoption in numerous extensive Kubernetes installations, serving as the primary tool for orchestrating the complete lifecycle of Kubernetes resources.

## Supported Kubernetes and Terraform versions

At the moment, the acceptance tests cover a combination of the last 7 Kubernetes releases and the last 4 stable
Terraform versions (plus 0.15). This doesn't necessarily mean it won't work with other combinations, but your mileage may vary

## Installation

### Terraform 0.13+

The provider can be installed and managed automatically by Terraform. Sample `versions.tf` file :

```hcl
terraform {
  required_version = ">= 0.13"

  required_providers {
    kubectl = {
      source  = "alekc/kubectl"
      version = "~> 2.0"
    }
  }
}
```

### Install manually

If you don't want to use the one-liner above, you can download a binary for your system from the [release page](https://github.com/alekc/terraform-provider-kubectl/releases),
then either place it at the root of your Terraform folder or in the Terraform plugin folder on your system.

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

See [User Guide](https://registry.terraform.io/providers/alekc/kubectl/latest) for details on installation and all the provided data and resource types.

## Changing providers for existing resources

When you used another fork of this provider in the past, it is possible to change the provider on all existing resources within your state. A common use-case of this is to switch from `gavinbunney/kubectl` towards this fork.

Change the `required_providers` sections in your main code and in all used modules to reflect the usage of `alekc/kubectl` as shown above. Once this is done, use the `state replace-provider` to make the switch on all existing resources in your state.

```
terraform state replace-provider gavinbunney/kubectl alekc/kubectl
```

You should then `terraform init`, and the next terraform actions will use this provider.

---

## Development Guide

If you wish to work on the provider, you'll first need [Go](http://www.golang.org) installed on your machine (version 1.12+ is _required_).
You'll also need to correctly setup a [GOPATH](http://golang.org/doc/code.html#GOPATH), as well as adding `$GOPATH/bin` to your `$PATH`.

To compile the provider, run `make build`. This will build the provider and put the provider binary in the `$GOPATH/bin` directory.

### Building The Provider

You can build the master branch of the provider by running

```sh
git clone github.com/alekc/terraform-provider-kubectl
cd terraform-provider-kubectl
make build
```

This will build an executable `terraform-provider-kubectl` in your `${GOPATH}/bin/` directory.
Now we need to tell Terraform to override remote versions with our local build. To do so create/edit `~/.terraformrc/` file and add following content to it:

```hcl
 provider_installation {
  dev_overrides {
    "alekc/kubectl" = "/Users/alekc/go/bin/"
  }
  direct {}
}
```

change "/Users/alekc/go/bin/" with the path where your go has placed built executable. After that all you have to do is run
`terraform init` and you will be using the new version.

If all went well, you should see a following message during the apply:

```text
╷
│ Warning: Provider development overrides are in effect
│
│ The following provider development overrides are set in the CLI configuration:
│  - alekc/kubectl in /Users/alekc/go/bin

```

### Testing

In order to test the provider, you can simply run `make test`.

```sh
$ make test
```

The provider uses k3s to run integration tests. These tests look for any `*.tf` files in the `_examples` folder and run an `plan`, `apply`, `refresh` and `plan` loop over each file.

Inside each file the string `name-here` is replaced with a unique name during test execution. This is a simple string replace before the TF is applied to ensure that tests don't fail due to naming clashes.

Each scenario can be placed in a folder, to help others navigate and use the examples, and added to the [README.MD](./_examples/README.MD).

> Note: The test infrastructure doesn't support multi-file TF configurations so ensure your test scenario is in a single file.

In order to run the full suite of Acceptance tests, run `make testacc`.

_Note:_ Acceptance tests create real resources, and often cost money to run.

```sh
$ make testacc
```

### Inspiration

Thanks to the original provider by [gavinbunney](https://github.com/gavinbunney/terraform-provider-kubectl) on the original base of this provider. Current version has been forked from 1.14
