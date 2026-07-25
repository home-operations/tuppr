# Terraform Provider: HTTP

The HTTP provider interacts with generic HTTP servers.
It provides a data source that issues an HTTP request exposing the response headers and body
for use within a Terraform deployment.

## Documentation, questions and discussions

Official documentation on how to use this provider can be found on the
[Terraform Registry](https://registry.terraform.io/providers/hashicorp/http/latest/docs).
In case of specific questions or discussions, please use the
HashiCorp [Terraform Providers Discuss forums](https://discuss.hashicorp.com/c/terraform-providers/31),
in accordance with HashiCorp [Community Guidelines](https://www.hashicorp.com/community-guidelines).

We also provide:

- [Support](.github/SUPPORT.md) page for help when using the provider
- [Contributing](.github/CONTRIBUTING.md) guidelines in case you want to help this project
- [Design](DESIGN.md) documentation to understand the scope and maintenance decisions

The remainder of this document will focus on the development aspects of the provider.

## Compatibility

Compatibility table between this provider,
the [Terraform Plugin Protocol](https://www.terraform.io/plugin/how-terraform-works#terraform-plugin-protocol)
version it implements, and Terraform:

|     HTTP Provider      | Terraform Plugin Protocol | Terraform |
| :--------------------: | :-----------------------: | :-------: |
|        `>= 2.x`        |            `5`            | `>= 0.12` |
| `>= 1.1.x`, `<= 1.2.x` |         `4`, `5`          | `>= 0.11` |
|       `<= 1.0.x`       |            `4`            | `<= 0.11` |

## Requirements

- [Terraform](https://www.terraform.io/downloads)
- [Go](https://go.dev/doc/install) (1.23)
- [GNU Make](https://www.gnu.org/software/make/)
- [golangci-lint](https://golangci-lint.run/welcome/install/#local-installation) (optional)

## Development

### Building

1. `git clone` this repository and `cd` into its directory
2. `make` will trigger the Golang build

The provided `GNUmakefile` defines additional commands generally useful during development,
like for running tests, generating documentation, code formatting and linting.
Taking a look at it's content is recommended.

### Testing

In order to test the provider, you can run

- `make test` to run provider tests
- `make testacc` to run provider acceptance tests

It's important to note that acceptance tests (`testacc`) will actually spawn
`terraform` and the provider. Read more about they work on the
[official page](https://www.terraform.io/plugin/sdkv2/testing/acceptance-tests).

### Generating documentation

This provider uses [terraform-plugin-docs](https://github.com/hashicorp/terraform-plugin-docs/)
to generate documentation and store it in the `docs/` directory.
Once a release is cut, the Terraform Registry will download the documentation from `docs/`
and associate it with the release version. Read more about how this works on the
[official page](https://www.terraform.io/registry/providers/docs).

Use `make generate` to ensure the documentation is regenerated with any changes.

### Using a development build

If [running tests and acceptance tests](#testing) isn't enough, it's possible to set up a local terraform configuration
to use a development builds of the provider. This can be achieved by leveraging the Terraform CLI
[configuration file development overrides](https://www.terraform.io/cli/config/config-file#development-overrides-for-provider-developers).

First, use `make install` to place a fresh development build of the provider in your
[`${GOBIN}`](https://pkg.go.dev/cmd/go#hdr-Compile_and_install_packages_and_dependencies)
(defaults to `${GOPATH}/bin` or `${HOME}/go/bin` if `${GOPATH}` is not set). Repeat
this every time you make changes to the provider locally.

Then, setup your environment following [these instructions](https://www.terraform.io/plugin/debugging#terraform-cli-development-overrides)
to make your local terraform use your local build.

### Testing GitHub Actions

This project uses [GitHub Actions](https://docs.github.com/en/actions/automating-builds-and-tests) to realize its CI.

Sometimes it might be helpful to locally reproduce the behaviour of those actions,
and for this we use [act](https://github.com/nektos/act). Once installed, you can _simulate_ the actions executed
when opening a PR with:

```shell
# List of workflows for the 'pull_request' action
$ act -l pull_request

# Execute the workflows associated with the `pull_request' action
$ act pull_request
```

## Releasing

The releasable builds are generated from the [build GH workflow](./.github/workflows/build.yml) and the release/promotion process
is completed via internal HashiCorp deployment tooling. Prior to release, the changelog should be updated in `main` with
the changie tool, example:

```sh
changie batch 3.7.2 && changie merge
```

## License

[Mozilla Public License v2.0](./LICENSE)
