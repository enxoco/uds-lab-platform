#!/usr/bin/env bash
# Thin wrapper — delegates to `uds run dev`.
# Kept for backward compatibility with muscle memory and CI scripts.
#
# Preferred: uds run dev [--with KEY=VALUE ...]
#
# Examples:
#   uds run dev
#   uds run dev --with BUILD_IMAGES=1
#   uds run dev --with WIPE_CLUSTER=0
#   uds run dev --with BUILD_IMAGES=1 --with WIPE_CLUSTER=0

set -euo pipefail
cd "$(dirname "$0")/.."

exec uds run dev \
  --with BUILD_IMAGES="${BUILD_IMAGES:-0}" \
  --with WIPE_CLUSTER="${WIPE_CLUSTER:-1}" \
  --with SKIP_BASE="${SKIP_BASE:-0}" \
  --with SKIP_UDS_CORE="${SKIP_UDS_CORE:-0}" \
  --with LOCAL_VM_IMAGES="${LOCAL_VM_IMAGES:-0}"
