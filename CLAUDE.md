# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository. It is symlinked from `AGENTS.md` so other coding agents (Codex, Cursor, OpenCode, Zed) read the same instructions.

## AI usage policy

This project does not accept pull requests that are fully or predominantly AI-generated. AI tools may be used in an assistive capacity only, for corrections or for expanding repetitive variations on code a human has already designed. See `CONTRIBUTING.md` for the full policy. Treat your role as assistive: small, reviewable edits to human-authored work, never wholesale generation of features.

## What this is

`tuppr` is a Kubernetes operator (Go, Kubebuilder, controller-runtime) that orchestrates automated upgrades of Talos Linux nodes and the Kubernetes control plane. Two CRDs in `api/v1alpha1/` drive everything: `TalosUpgrade` (multiple per cluster, queued) and `KubernetesUpgrade` (singleton, enforced by validating webhook).

End-user behavior, CRD spec, and metrics are documented in `README.md`. Read it when a question is about what the operator does rather than how the code is laid out.

## How to work on it

The `Makefile` is the source of truth (`make help` lists targets). The one non-obvious rule:

- After any edit under `api/`, run `make manifests generate helm-crds` and commit the regenerated files.

`CONTRIBUTING.md` has the PR workflow: Conventional Commits, `make lint test` must pass, lefthook handles formatting on staged files (don't bypass it).

Do not run `git add`, `git commit`, or `git push` unless explicitly asked.
