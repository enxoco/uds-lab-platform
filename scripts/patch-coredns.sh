#!/usr/bin/env bash
# Patch CoreDNS NodeHosts so *.uds.dev resolves to MetalLB gateways inside
# the cluster. Required because public *.uds.dev DNS wildcards to 127.0.0.1.
# Idempotent — safe to rerun.
set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
log()  { echo -e "${GREEN}▶${NC} $*"; }
warn() { echo -e "${YELLOW}⚠${NC} $*"; }
die()  { echo -e "${RED}✗${NC} $*" >&2; exit 1; }

log "Detecting MetalLB gateway IPs..."

ADMIN_IP=$(kubectl get svc -n istio-admin-gateway admin-ingressgateway \
  -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || true)
TENANT_IP=$(kubectl get svc -n istio-tenant-gateway tenant-ingressgateway \
  -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || true)

[ -n "$ADMIN_IP" ]  || die "Could not detect admin gateway IP (is UDS Core deployed?)"
[ -n "$TENANT_IP" ] || die "Could not detect tenant gateway IP (is UDS Core deployed?)"

log "Admin gateway:  $ADMIN_IP"
log "Tenant gateway: $TENANT_IP"

# Patch NodeHosts in the main coredns configmap.
# NodeHosts is the block coredns uses for static host entries (same as /etc/hosts).
# We append our overrides; duplicate entries are harmless — last write wins for same key.
PATCH=$(cat <<EOF
${ADMIN_IP} keycloak.admin.uds.dev
${TENANT_IP} sso.uds.dev
${TENANT_IP} lab.uds.dev
EOF
)

log "Patching CoreDNS NodeHosts..."

# Read the current NodeHosts, strip any existing uds.dev lines, append fresh ones.
CURRENT=$(kubectl get configmap coredns -n kube-system \
  -o jsonpath='{.data.NodeHosts}')

CLEANED=$(echo "$CURRENT" | grep -v '\.uds\.dev' || true)
NEW_HOSTS="${CLEANED}
${PATCH}"

kubectl patch configmap coredns -n kube-system --type merge \
  -p "{\"data\":{\"NodeHosts\":$(echo "$NEW_HOSTS" | python3 -c 'import sys,json; print(json.dumps(sys.stdin.read()))')}}"

log "Restarting CoreDNS to pick up changes..."
kubectl rollout restart deployment/coredns -n kube-system
kubectl rollout status deployment/coredns -n kube-system --timeout=60s

# Authservice starts during bundle deploy before CoreDNS is patched, so it
# caches a failed OIDC discovery from keycloak.admin.uds.dev (resolves to
# 127.0.0.1 via public DNS). Restart forces it to re-fetch JWKS keys now
# that CoreDNS resolves the hostname correctly.
if kubectl get deployment authservice -n authservice &>/dev/null; then
  log "Restarting Authservice to re-fetch Keycloak OIDC config..."
  kubectl rollout restart deployment/authservice -n authservice
  kubectl rollout status deployment/authservice -n authservice --timeout=120s
else
  warn "Authservice deployment not found in namespace 'authservice' — skipping restart"
fi

log "CoreDNS patched. Verifications:"
log "  kubectl exec -n kube-system deploy/coredns -- nslookup sso.uds.dev"
