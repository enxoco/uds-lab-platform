#!/usr/bin/env bash
set -euo pipefail

[[ -z "${HCLOUD_TOKEN:-}" ]] && { echo "error: HCLOUD_TOKEN not set" >&2; exit 1; }
command -v jq   >/dev/null 2>&1 || { echo "error: jq required"   >&2; exit 1; }
command -v curl >/dev/null 2>&1 || { echo "error: curl required" >&2; exit 1; }

USE_FZF=false
command -v fzf >/dev/null 2>&1 && USE_FZF=true

API="https://api.hetzner.cloud/v1"
STACK="${1:-dev}"

hcloud_get() {
  curl -sf -H "Authorization: Bearer $HCLOUD_TOKEN" "$API/$1"
}

pick() {
  local header="$1"
  if $USE_FZF; then
    fzf --header="$header" --height=50% --reverse --no-sort
  else
    local lines=() i=1
    while IFS= read -r line; do lines+=("$line"); done
    echo "" >&2
    echo "$header" >&2
    for line in "${lines[@]}"; do printf "  %2d) %s\n" "$i" "$line" >&2; ((i++)); done
    echo "" >&2
    while true; do
      read -rp "Choice [1-${#lines[@]}]: " idx >&2
      [[ "$idx" =~ ^[0-9]+$ ]] && (( idx >= 1 && idx <= ${#lines[@]} )) && break
      echo "  Invalid — enter a number between 1 and ${#lines[@]}" >&2
    done
    echo "${lines[$((idx-1))]}"
  fi
}

# --- Pick location ---
echo "Fetching Hetzner locations..." >&2
LOC_LINES=$(hcloud_get "locations" | jq -r '
  .locations[]
  | "\(.name)  \(.city), \(.country)  [\(.network_zone)]"
')

LOCATION=$(echo "$LOC_LINES" | pick "Select datacenter location:" | awk '{print $1}')
echo "→ Location: $LOCATION" >&2

# --- Pick server type (filtered by location, sorted by price) ---
echo "Fetching server types available in $LOCATION..." >&2
TYPE_LINES=$(hcloud_get "server_types?per_page=50" | jq -r --arg loc "$LOCATION" '
  .server_types[]
  | select(.deprecation == null)
  | select(any(.available_locations[]?; .name == $loc))
  | . as $t
  | (first(.prices[]? | select(.location == $loc)) // null) as $price_entry
  | select($price_entry != null)
  | ($price_entry.price_hourly.gross | tonumber) as $price
  | [$price, "\($t.name)  \($t.cores)vCPU \($t.memory | floor)GB RAM  \($t.cpu_type)  ~$\($price)/hr"]
  | @tsv
' | sort -n | cut -f2-)

[[ -z "$TYPE_LINES" ]] && { echo "error: no server types available in $LOCATION" >&2; exit 1; }

SERVER_TYPE=$(echo "$TYPE_LINES" | pick "Select server type (sorted cheapest first):" | awk '{print $1}')
echo "→ Server type: $SERVER_TYPE" >&2
echo "" >&2

export TF_VAR_location="$LOCATION"
export TF_VAR_server_type="$SERVER_TYPE"

atmos terraform apply coder-server --stack "$STACK"
