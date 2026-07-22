# Development

Planned work lives in the
[issue tracker](https://github.com/home-operations/tuppr/issues).

Tooling is pinned with [mise](https://mise.jdx.dev); `mise tasks` lists
everything, and the everyday tasks are:

```bash
mise run test              # unit tests (regenerates manifests first)
mise run test-integration  # envtest apiserver: CRD schema + CEL rules
mise run test-e2e          # full kind-based e2e (needs docker)
mise run lint              # golangci-lint
mise run build             # bin/manager
mise run manifests         # CRD + RBAC generation
mise run helm-test         # helm-unittest against the chart
mise run docs-serve        # this docs site, live-reloading
```

The docs site is MkDocs Material: pages live under `docs/`, the nav lives in
`mkdocs.yml`, and `mise run docs` builds the deployable site with `--strict`
link checking (CI runs it on every PR). The MkDocs toolchain is pinned in
`pyproject.toml` / `uv.lock` and run through `uv`.

Contributions are welcome via
[issues and pull requests](https://github.com/home-operations/tuppr/issues).
tuppr is licensed under
[AGPL-3.0](https://github.com/home-operations/tuppr/blob/main/LICENSE).
