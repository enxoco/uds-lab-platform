#!/usr/bin/env bash
# Generates hack/proxy/nginx.conf from live MetalLB IPs, then starts the proxy.
set -euo pipefail

export KUBECONFIG="${KUBECONFIG:-$HOME/.kube/config}"
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
NGINX_CONF="$REPO_ROOT/hack/proxy/nginx.conf"

get_lb_ip() {
  local svc="$1" ns="$2"
  kubectl get svc "$svc" -n "$ns" --request-timeout=10s \
    -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null
}

echo "Detecting MetalLB gateway IPs..."
ADMIN_IP=$(get_lb_ip admin-ingressgateway istio-admin-gateway)
TENANT_IP=$(get_lb_ip tenant-ingressgateway istio-tenant-gateway)

[ -n "$ADMIN_IP" ]  || { echo "ERROR: admin-ingressgateway has no LoadBalancer IP" >&2; exit 1; }
[ -n "$TENANT_IP" ] || { echo "ERROR: tenant-ingressgateway has no LoadBalancer IP" >&2; exit 1; }

echo "  admin  gateway: $ADMIN_IP"
echo "  tenant gateway: $TENANT_IP"

cat > "$NGINX_CONF" <<EOF
worker_processes 1;
events { worker_connections 1024; }

stream {
    map \$ssl_preread_server_name \$backend_443 {
        ~*\\.admin\\.uds\\.dev  ${ADMIN_IP}:443;
        default              ${TENANT_IP}:443;
    }

    map \$ssl_preread_server_name \$backend_80 {
        ~*\\.admin\\.uds\\.dev  ${ADMIN_IP}:80;
        default              ${TENANT_IP}:80;
    }

    server {
        listen 443;
        proxy_pass \$backend_443;
        ssl_preread on;
    }

    server {
        listen 80;
        proxy_pass \$backend_80;
    }
}
EOF

echo "nginx.conf written. Starting proxy..."
docker compose -f "$REPO_ROOT/hack/proxy/docker-compose.yaml" up -d
echo "Proxy started."
