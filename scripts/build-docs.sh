#!/usr/bin/env bash
# build-docs.sh — assemble the tuppr documentation site.
#
# Produces one directory ready for GitHub Pages:
#   site/    ← MkDocs Material user docs (site root)
#
# `mkdocs build --strict` fails on a broken intra-site link or a missing nav
# file (validation config in mkdocs.yml), so this script doubles as our doc
# lint.
#
# Run via `mise run docs` so uv (and therefore the uv.lock-pinned MkDocs +
# Material + pymdown-extensions) resolves to the versions pinned in the repo.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

OUT="site"
# Custom domain the site is served from. Written into the artifact as a CNAME
# so the domain survives every deploy (must match Settings -> Pages custom
# domain).
SITE_DOMAIN="tuppr.home-operations.com"

echo "==> checking snippet SECTION references (--strict only guards file paths)"
# pymdownx.snippets' check_paths fails the build on a missing FILE, but a
# `--8<-- "file.yaml:section"` whose section marker was renamed/removed renders
# as a silently EMPTY code block — exactly the docs/manifest drift the snippet
# convention exists to prevent. Verify every section reference resolves.
uv run python - <<'PYEOF'
import pathlib, re, sys

errors = []
for md in pathlib.Path("docs").rglob("*.md"):
    for ref in re.findall(r'--8<--\s+"([^":]+):([^"]+)"', md.read_text()):
        path, section = pathlib.Path(ref[0]), ref[1]
        if re.fullmatch(r"\d*(:\d*)?", section):
            continue  # a LINE-RANGE include (file:5:8), not a named section
        if not path.is_file():
            continue  # missing files are check_paths' job; don't double-report
        if f"--8<-- [start:{section}]" not in path.read_text():
            errors.append(f"{md}: section '{section}' not found in {path} "
                          f"(expected a '# --8<-- [start:{section}]' marker)")
if errors:
    print("snippet section check FAILED:", *errors, sep="\n  ", file=sys.stderr)
    sys.exit(1)
PYEOF

echo "==> mkdocs build (--strict: broken link or missing nav file fails here)"
# uv run resolves MkDocs + plugins from the committed uv.lock into a managed venv.
uv run mkdocs build --strict --site-dir "$OUT"

# GitHub Pages deploys via Actions do not run Jekyll; .nojekyll keeps it
# explicit and future-proof.
touch "${OUT}/.nojekyll"

# Pin the custom domain in the published artifact.
echo "${SITE_DOMAIN}" > "${OUT}/CNAME"

echo "==> docs site assembled at ${OUT}/ for https://${SITE_DOMAIN}/"
