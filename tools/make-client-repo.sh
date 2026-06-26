#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
TARGET_ROOT="${1:-${HOME}/mactl}"

echo "Create mactl client repository"
echo "  source: ${REPO_ROOT}"
echo "  target: ${TARGET_ROOT}"

if ! command -v rsync >/dev/null 2>&1; then
    echo "error: rsync command is required" >&2
    exit 1
fi

mkdir -p "${TARGET_ROOT}" \
         "${TARGET_ROOT}/api" \
         "${TARGET_ROOT}/cmd/mactl" \
         "${TARGET_ROOT}/pkg/client" \
         "${TARGET_ROOT}/pkg/config" \
         "${TARGET_ROOT}/pkg/db" \
         "${TARGET_ROOT}/pkg/types" \
         "${TARGET_ROOT}/pkg/util"

# Root files for standalone client repo.
cp "${REPO_ROOT}/TAG" "${TARGET_ROOT}/TAG"
cp "${SCRIPT_DIR}/go.mod.root" "${TARGET_ROOT}/go.mod"
cp "${SCRIPT_DIR}/Makefile.root" "${TARGET_ROOT}/Makefile"
cp "${SCRIPT_DIR}/Makefile.windows.root" "${TARGET_ROOT}/Makefile.windows"

# Submodule go.mod files and mactl build Makefile.
cp "${SCRIPT_DIR}/go.mod.api" "${TARGET_ROOT}/api/go.mod"
cp "${SCRIPT_DIR}/go.mod.client" "${TARGET_ROOT}/pkg/client/go.mod"
cp "${SCRIPT_DIR}/go.mod.config" "${TARGET_ROOT}/pkg/config/go.mod"
cp "${SCRIPT_DIR}/go.mod.types" "${TARGET_ROOT}/pkg/types/go.mod"
cp "${SCRIPT_DIR}/Makefile.mactl" "${TARGET_ROOT}/cmd/mactl/Makefile"
cp "${SCRIPT_DIR}/Makefile.windows.mactl" "${TARGET_ROOT}/cmd/mactl/Makefile.windows"

# Sync source trees to follow the latest upstream layout.
rsync -a --delete --exclude='go.mod' "${REPO_ROOT}/api/" "${TARGET_ROOT}/api/"
rsync -a --delete --exclude='go.mod' "${REPO_ROOT}/pkg/client/" "${TARGET_ROOT}/pkg/client/"
rsync -a --delete --exclude='go.mod' "${REPO_ROOT}/pkg/config/" "${TARGET_ROOT}/pkg/config/"
rsync -a --delete "${REPO_ROOT}/pkg/db/" "${TARGET_ROOT}/pkg/db/"
rsync -a --delete --exclude='go.mod' "${REPO_ROOT}/pkg/types/" "${TARGET_ROOT}/pkg/types/"
rsync -a --delete "${REPO_ROOT}/pkg/util/" "${TARGET_ROOT}/pkg/util/"
rsync -a --delete "${REPO_ROOT}/cmd/mactl/cmd/" "${TARGET_ROOT}/cmd/mactl/cmd/"
cp "${REPO_ROOT}/cmd/mactl/main.go" "${TARGET_ROOT}/cmd/mactl/main.go"

echo "done"
echo "next:"
echo "  cd ${TARGET_ROOT}"
echo "  make setup"
echo "  make"
