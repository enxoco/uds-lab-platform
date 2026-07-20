#!/usr/bin/env bash
# Thin wrapper — delegates to `uds run cluster-up`.
# Kept for backward compatibility with existing muscle memory and CI scripts.
#
# Preferred: uds run cluster-up [--with KEY=VALUE ...]
#
# Examples:
#   uds run cluster-up
#   uds run cluster-up --with WIPE_CLUSTER=0
#   uds run cluster-up --with WIPE_CLUSTER=0 --with SKIP_GOLDEN_PVC=0

set -euo pipefail
cd "$(dirname "$0")/.."

exec uds run cluster-up \
  --with WIPE_CLUSTER="${WIPE_CLUSTER:-1}" \
  --with SKIP_GOLDEN_PVC="${SKIP_GOLDEN_PVC:-1}" \
  --with LOCAL_VM_IMAGES="${LOCAL_VM_IMAGES:-0}" \
  --with CDI_FLAVOR="${CDI_FLAVOR:-unicorn}" \
  --with CDI_PKG_DIR="${CDI_PKG_DIR:-../cdi-operator}" \
  --with KUBEVIRT_PKG_DIR="${KUBEVIRT_PKG_DIR:-$HOME/src/github.com/uds-packages/kubevirt}"
