#!/usr/bin/env bash
# Thin wrapper — delegates to `uds run cluster-up`.
# Kept for backward compatibility with existing muscle memory and CI scripts.
#
# Preferred: uds run cluster-up [--with KEY=VALUE ...]
#
# Examples:
#   uds run cluster-up
#   uds run cluster-up --with SKIP_WIPE=1
#   uds run cluster-up --with SKIP_WIPE=1 --with SKIP_GOLDEN_PVC=0

set -euo pipefail
cd "$(dirname "$0")/.."

exec uds run cluster-up \
  --with SKIP_WIPE="${SKIP_WIPE:-0}" \
  --with SKIP_GOLDEN_PVC="${SKIP_GOLDEN_PVC:-1}" \
  --with KUBEVIRT_PKG_DIR="${KUBEVIRT_PKG_DIR:-$HOME/src/github.com/uds-packages/kubevirt}"
