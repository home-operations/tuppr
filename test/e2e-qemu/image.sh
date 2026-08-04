#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
# shellcheck source=common.sh
source "${SCRIPT_DIR}/common.sh"

: "${CONTROLLER_IMAGE:?must be set to the tag to build and push}"

cd "$REPO_ROOT"

GO_VERSION="${GO_VERSION:-$(mise config get tools.go)}"

log "Building controller image $CONTROLLER_IMAGE..."
docker build --build-arg "GO_VERSION=${GO_VERSION}" -t "$CONTROLLER_IMAGE" .

log "Pushing controller image..."
docker push "$CONTROLLER_IMAGE"

log "Controller image ready: $CONTROLLER_IMAGE"
