# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository. It is symlinked from `AGENTS.md` so other coding agents (Codex, Cursor, OpenCode, Zed) read the same instructions.

## AI usage policy

This project does not accept pull requests that are fully or predominantly AI-generated. AI tools may be used in an assistive capacity only, for corrections or for expanding repetitive variations on code a human has already designed. Treat your role as assistive: small, reviewable edits to human-authored work, never wholesale generation of features. This policy is stricter than the [org-wide contribution guidelines](https://github.com/home-operations/.github/blob/main/CONTRIBUTING.md) and takes precedence.

## What this is

`tuppr` is a Kubernetes operator (Go, Kubebuilder, controller-runtime) that orchestrates automated upgrades of Talos Linux nodes and the Kubernetes control plane. Two CRDs in `api/v1alpha1/` drive everything: `TalosUpgrade` (multiple per cluster, queued) and `KubernetesUpgrade` (singleton, enforced by validating webhook).

End-user behavior, CRD spec, and metrics are documented in `README.md`. Read it when a question is about what the operator does rather than how the code is laid out.

## How to work on it

The `Makefile` is the source of truth (`make help` lists targets). Non-obvious rules:

- After any edit under `api/`, run `make manifests generate helm-crds` and commit the regenerated files.
- `make lint test` must pass before opening a PR.
- Lefthook formats staged files on commit — don't bypass it.
- Use [Conventional Commits](https://www.conventionalcommits.org/); release-please drives versioning and the changelog from them.

See the [org-wide contribution guidelines](https://github.com/home-operations/.github/blob/main/CONTRIBUTING.md) for general PR etiquette.

## Coding conventions

- Reconcilers must be idempotent and never panic — return errors and let controller-runtime requeue.
- Wrap errors with context: `fmt.Errorf("...: %w", err)`. Never silently swallow them.
- Log via the structured `logr` logger. Never log secrets or kubeconfig contents.
- Avoid third-party dependencies unless justified. Prefer clear, boring Go over clever abstractions.
- Idiomatic Go names: `MixedCaps` exported, `mixedCaps` unexported, no underscores. Acronyms keep case (`nodeID`, `talosAPIClient`). CRD kinds are `PascalCase` and singular (`TalosUpgrade`); JSON tags are `camelCase`. Reconciler types are named `<Kind>Reconciler`.

Do not run `git add`, `git commit`, or `git push` unless explicitly asked.
