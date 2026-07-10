#!/usr/bin/env bash
# patch-demo-routes.sh — Exempt /demo and /api/demo-sessions from Istio authservice / JWT auth.
#
# WHY THIS EXISTS
# ───────────────
# The UDS Package CR has no per-path authservice bypass.  Pepr generates two
# Istio AuthorizationPolicies for the lab-platform SSO client:
#
#   uds-lab-platform-authservice  (action: Custom)
#     Routes unauthenticated requests to authservice → Keycloak SSO redirect.
#
#   uds-lab-platform-jwt-authz   (action: Deny)
#     Denies requests that lack a valid SSO JWT from the UDS realm.
#
# The self-service demo flow needs /demo and /api/demo-sessions to be
# reachable without SSO so prospects can access them via HMAC-signed links.
# The Go server validates the HMAC token itself; SSO must not intercept first.
#
# WHEN TO RUN
# ───────────
# Run once after initial cluster deploy.  Re-run whenever Pepr reconciles the
# Package CR (e.g. after a Helm upgrade), because Pepr will overwrite both
# policies and lose the notPaths additions.
#
# USAGE
#   ./scripts/patch-demo-routes.sh [--namespace <ns>]
#
# OPTIONS
#   --namespace, -n   Namespace of the lab-platform Package CR (default: lab-platform)
#   --wait            Wait up to 2 min for the policies to exist before patching
#   --dry-run         Print the patch commands without applying them

set -euo pipefail

NS="lab-platform"
WAIT=0
DRY_RUN=0

while [ $# -gt 0 ]; do
  case "$1" in
    --namespace|-n) NS="$2"; shift 2 ;;
    --wait)         WAIT=1; shift ;;
    --dry-run)      DRY_RUN=1; shift ;;
    *) echo "Unknown option: $1" >&2; exit 1 ;;
  esac
done

AUTHSERVICE_POLICY="uds-lab-platform-authservice"
JWT_AUTHZ_POLICY="uds-lab-platform-jwt-authz"
DEMO_PATHS='["/demo","/demo/*","/api/demo-sessions"]'

# JSON Patch targets — ambient/waypoint mode:
#
# In ambient mode, both policies target the waypoint Gateway (not a sidecar
# workload), so Pepr does NOT add the port-15020 Prometheus exemption.  Neither
# policy has a `to` field at generation time.  We ADD the `to` field from
# scratch so the SSO rule only fires on non-demo paths.
#
# Confirmed structure (kubectl get authorizationpolicy … -o json):
#   authservice: rules[0] has `when` (notValues: "*") but no `to`
#   jwt-authz:   rules[0] has `from` (notRequestPrincipals) but no `to`

AUTHSERVICE_PATCH="[{\"op\":\"add\",\"path\":\"/spec/rules/0/to\",\"value\":[{\"operation\":{\"notPaths\":${DEMO_PATHS}}}]}]"
JWT_AUTHZ_PATCH="[{\"op\":\"add\",\"path\":\"/spec/rules/0/to\",\"value\":[{\"operation\":{\"notPaths\":${DEMO_PATHS}}}]}]"

maybe_run() {
  if [ "$DRY_RUN" = "1" ]; then
    echo "[dry-run] $*"
  else
    "$@"
  fi
}

wait_for_policy() {
  local name="$1" deadline=$(( $(date +%s) + 120 ))
  if [ "$WAIT" = "0" ]; then
    kubectl get authorizationpolicy "$name" -n "$NS" &>/dev/null || {
      echo "ERROR: AuthorizationPolicy '$name' not found in namespace '$NS'." >&2
      echo "       Run with --wait to poll until it exists, or check that Pepr has processed the Package CR." >&2
      exit 1
    }
    return
  fi
  echo "Waiting for AuthorizationPolicy/$name in $NS..."
  until kubectl get authorizationpolicy "$name" -n "$NS" &>/dev/null; do
    [ "$(date +%s)" -gt "$deadline" ] && {
      echo "ERROR: timed out waiting for $name" >&2; exit 1
    }
    sleep 3
  done
}

wait_for_policy "$AUTHSERVICE_POLICY"
wait_for_policy "$JWT_AUTHZ_POLICY"

echo "Patching $AUTHSERVICE_POLICY in $NS..."
maybe_run kubectl patch authorizationpolicy "$AUTHSERVICE_POLICY" -n "$NS" \
  --type=json -p="$AUTHSERVICE_PATCH"

echo "Patching $JWT_AUTHZ_POLICY in $NS..."
maybe_run kubectl patch authorizationpolicy "$JWT_AUTHZ_POLICY" -n "$NS" \
  --type=json -p="$JWT_AUTHZ_PATCH"

echo ""
echo "Demo route exemptions applied.  Verify:"
echo "  kubectl get authorizationpolicy -n $NS -o yaml | grep -A4 notPaths"
echo ""
echo "NOTE: Pepr re-applies these policies on every Package CR reconciliation."
echo "      Re-run this script after any Helm upgrade of uds-lab-platform."
