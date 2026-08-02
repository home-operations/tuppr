# AGENTS.md: tuppr

Guidance for AI coding agents (and humans) writing Go in this repository. This
file is meant to be identical, or nearly so, across every Go service/CLI in
the home-operations fleet: treat it as a template, copy it verbatim into a
new Go repo, and only add repo-specific detail if something here genuinely
doesn't apply. Where a fact could differ per repo (exact versions, task
commands, variable names, CI steps), this file points at where to check
rather than asserting a specific value, since restating it here just goes
stale.

## Working in this repo: AI usage, commits, and safety

This repo doesn't carry its own `CONTRIBUTING.md`; GitHub serves the org-wide
one from [`home-operations/.github`](https://github.com/home-operations/.github/blob/main/CONTRIBUTING.md),
which includes an AI Usage Policy that applies to any AI coding agent here:
assistive use only, a human must author the majority of any change, AI use
must be disclosed, a human reviews every line before submission, and the
contributor must be able to explain any line a reviewer asks about. AI must
never write the PR description, an issue, or a reply to a human on the
contributor's behalf. Read the policy itself rather than trusting this
summary; it can change without this file being updated to match.

- PR titles follow [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/):
  `<type>[(scope)][!]: <description>` (e.g. `fix(config): reject a negative
timeout`), which is what drives release-please's version bumps. Individual
  commit messages don't have to follow the format, though matching it is
  fine. Sign off commits: `git commit -s`.
- Never `git commit`, `git push`, or open a PR unless asked to. Ask before
  any destructive or hard-to-reverse action (force-push, `git reset --hard`,
  deleting a branch, rewriting history) instead of defaulting to it.
- Never touch secrets or gitignored files. Check this repo's actual
  `.gitignore` before assuming it already excludes something like
  `*.key`/`*.crt`/`.env`; don't assume gitignore coverage that isn't
  actually configured. This fleet generally passes signing keys and webhook
  secrets by path or env var specifically so they're never committed; don't
  be the exception.
- Don't state a library's API, flags, or defaults from memory: verify
  against `pkg.go.dev`, the vendored source in the module cache, or this
  project's own code. Dependency behavior changes between versions in ways
  that are easy to get subtly wrong from recollection alone.
- After a change, run this repo's actual test and lint tasks (see "Build,
  lint, test" below) before calling it done. Don't claim untested code
  works.

## Baseline

- **Idiomatic.** Follow [Effective Go](https://go.dev/doc/effective_go) and
  the [Code Review Comments](https://go.dev/wiki/CodeReviewComments) wiki.
  `gofmt -s` runs on every staged `.go` file via lefthook and again in CI:
  never hand-format, and don't fight it with inline exceptions. Comments
  explain non-obvious constraints only (a hidden invariant, why a workaround
  exists, what would surprise a reader); don't narrate what good naming
  already says, and don't reference the current change or past behavior in
  a comment: that belongs in the PR description and rots as the code moves
  on.
- **Go 1.26.** Check `go.mod`'s `go` directive and `.mise/config.toml`'s
  `tools.go` for the exact pinned versions. Renovate bumps them together
  in one grouped Go-toolchain PR, patch releases included, so the two
  should match; a lingering gap means a toolchain PR hasn't merged yet
  (or something is off) and is worth flagging, not routine drift. When a newer construct is
  genuinely more idiomatic, use it: Go 1.26 added `errors.AsType[T](err)`, a
  generic type-safe replacement for the `var t *T; errors.As(err, &t)`
  two-step; prefer it in new code. `go fix` (rebuilt in 1.26 as a
  modernizer runner on `go vet`'s analysis) surfaces these mechanical
  migrations; run it after a toolchain bump.
- **Idempotent.** Reconcilers, code generators (`mise run generate`), and
  CLI subcommands must be safe to re-run: identical input yields identical
  output/state, with no accumulating side effects on a second invocation.
  The strongest version of this is a stateless service: if every response
  is re-derivable from its inputs or upstream, a restart or an extra
  replica can't affect correctness, only latency.
- **DRY and minimal, without premature abstraction.** Three similar call
  sites are fine as-is; don't introduce an interface, options struct, or
  generic helper until a real third caller needs the variance it buys.
  Touch only what the task requires: don't refactor or "improve" adjacent
  code, and match the existing style even where you'd do it differently.
  Remove imports, variables, and functions your own change orphaned; leave
  pre-existing dead code alone and mention it instead of deleting it
  unprompted.
- **Unit tested**, table-driven via `t.Run` subtests. Match whatever
  framework the package you're touching already uses instead of assuming:
  plain stdlib `testing` is the fleet default and is sufficient for most
  tests (config parsing, HTTP handlers, pure functions); `testify`
  (`assert`/`require`) is common where a table's per-case assertions get
  repetitive; `controller-runtime` operators scaffolded by kubebuilder often
  keep Ginkgo/Gomega for `test/e2e`/`test/integration`, but that doesn't
  necessarily extend to unit tests under `internal/`, check the actual test
  files before assuming. Don't introduce a second framework into a package
  that already has one. `go test -race` is the floor for anything touching
  goroutines; check `.mise/config.toml`'s `test` task for whether `-race`
  and coverage flags are already wired in.
- **`log/slog`**, JSON handler to stdout by default (a text-format escape
  hatch via config is fine for local runs), never a third-party logging
  library. Call `slog.SetDefault` once in `main`, then use package-level
  `slog.Info`/`slog.Error`/etc., or thread a `*slog.Logger` through
  constructors that are genuinely reused outside `main`; don't pass a
  logger through call chains that don't need one.
- **`github.com/caarlos0/env/v11`** for env-var-driven configuration: one
  `Config` struct (commonly in `internal/config`), populated by
  `env.Parse`/`env.ParseAs`, behind a `Load()` that also derives any
  computed fields and validates: fail fast on invalid config at startup
  instead of letting a bad value surface later as a runtime error.
  Doc-comment every field with what it does and why its `envDefault` is
  what it is; the struct doubles as the config reference. If this repo is
  primarily a CLI tool already using `pflag`/`cobra` with its own
  env-var-binding convention, match that existing pattern instead of
  introducing a second, competing config path.
- **`github.com/spf13/pflag`, only when the app has a real CLI surface**:
  subcommands, flags a human types, anything beyond "read env vars and
  serve." Wire it through `github.com/spf13/cobra` rather than a bare
  `pflag.FlagSet` once there's more than a couple of flags. A service
  that's entirely env-configured shouldn't take a flags dependency just to
  have one. Exception: `controller-runtime` operators keep the kubebuilder
  scaffold's stdlib `flag` + `opts.BindFlags(flag.CommandLine)` wiring;
  don't convert generated operator boilerplate to pflag.

## Project layout

`cmd/<app>/main.go` is the entrypoint; everything else lives under
`internal/` unless another repo needs to import this one as a library, in
which case the exported package lives outside `internal/` at the module
root. Keep `main.go` to wiring: parse config, build the logger, construct
dependencies, run, translate the top-level error into an exit code.
Business logic belongs in `internal/<package>`, not in `main`.

## Errors

Wrap with `fmt.Errorf("<component>: %w", err)` so a caller gets context
without losing the original error for `errors.Is`/`errors.As`/`errors.AsType`.
A `"<package>: %w"`-style prefix is the fleet convention: grep an error
message and you know which package raised it. Define sentinel errors
(`errors.New`, package-level `Err*`) for conditions a caller branches on,
and classify with a single `errors.Is` switch at the boundary that turns an
internal error into an HTTP status / exit code / CR condition, rather than
scattering classification through business logic. Never discard an error
silently: `_ = someCall()` is only for genuinely fire-and-forget calls (e.g.
best-effort metrics), and say why in a comment when that's not obvious.

## Context & shutdown

Every function that does I/O takes a `context.Context` as its first
parameter, and propagates one it's already given rather than building a
fresh `context.Background()` partway down the call stack, unless there's a
documented reason cancellation shouldn't propagate there. Long-running
processes derive their root context from
`signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)`
and re-arm the default handler on a second signal so a stuck drain can
still be force-killed. Prefer `golang.org/x/sync/errgroup` over raw
`sync.WaitGroup` + channels when fanning out goroutines that can fail: it
propagates the first error and cancels the group's context for you.

## Build, lint, test (via mise)

Mise is mandatory: it pins the exact Go and golangci-lint versions
(`.mise/config.toml`), so running `go build`/`go test` outside `mise run`
risks a toolchain mismatch with CI. Run `mise tasks` to see what's actually
defined in this repo; don't copy another repo's task names, flags, or
output paths (`-race`, `$(go list ./...)` vs `./...`, build output
location, `-ldflags` version stamping all vary) without checking
`.mise/config.toml` first. Fleet-wide, the common tasks are some subset of
`build`, `fmt`, `vet`, `test`, `lint`, `lint-fix`, plus repo-specific ones
like `generate`/`generate-check`, `test-integration`, `test-e2e`, `bench`,
or `helm-*` for repos that ship a chart; not every repo has every task.

`lefthook` (`.lefthook.toml`, extending the shared `home-operations/.github`
config) runs `gofmt -s -w` on staged `.go` files pre-commit; that part is
shared fleet-wide. What CI actually enforces beyond that (`go vet`, a `go
mod tidy` diff check, a generated-file diff check) varies per repo: check
`.golangci.yml` and `.github/workflows/` here rather than assuming every
repo enforces the same set. Lint rules themselves live in `.golangci.yml`;
read it instead of trusting a restated list, since the two can drift.

## Containers

Static binaries are the fleet default: `CGO_ENABLED=0`, `-trimpath`, and
`-ldflags` stamping build metadata into exported `main` package variables,
though the exact variable names differ per repo (`main.version`/
`main.commit`, `main.Version`/`main.Gitsha`, etc.): check `main.go` before
copying an `-ldflags -X` example verbatim. The base image pattern is
building `FROM golang:<pinned>-alpine` and running `FROM
gcr.io/distroless/static:nonroot`. Only drop `CGO_ENABLED=0` if a
dependency genuinely requires cgo, and justify it in a comment. Long-running
services in containers commonly set `GOMEMLIMIT` via
`github.com/KimMachineGun/automemlimit` so the GC reclaims before the
cgroup OOM-kills the process; check whether this repo already does before
adding it. Expose Prometheus metrics via `github.com/prometheus/client_golang`;
whether health/readiness probes share that port or use a separate one is a
per-repo decision, check the manager/server setup in `main.go` before
assuming.

## Security

`govulncheck ./...` runs in CI as the `Go Vulncheck` job, via `mise run
vulncheck` with the tool version pinned in `.mise/config.toml`. Run the
same task locally before cutting a release; a new advisory that fails the
job is an action item (upgrade the module or the Go toolchain), not noise
to suppress.
