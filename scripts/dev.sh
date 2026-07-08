#!/usr/bin/env bash
# Thin wrapper — delegates to `uds run dev`.
# Kept for backward compatibility with muscle memory and CI scripts.
#
# Preferred: uds run dev [--with KEY=VALUE ...]
#
# Examples:
#   uds run dev
#   uds run dev --with SKIP_IMAGES=1
#   uds run dev --with SKIP_WIPE=1
#   uds run dev --with SKIP_IMAGES=1 --with SKIP_WIPE=1

set -euo pipefail
cd "$(dirname "$0")/.."

exec uds run dev \
  --with SKIP_IMAGES="${SKIP_IMAGES:-0}" \
  --with SKIP_WIPE="${SKIP_WIPE:-0}" \
  --with SKIP_BASE="${SKIP_BASE:-0}" \
  --with SKIP_TOOLS="${SKIP_TOOLS:-0}" \
  --with SKIP_UDS_CORE="${SKIP_UDS_CORE:-0}"
