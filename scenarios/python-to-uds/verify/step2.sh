#!/bin/bash
# Pass when the Helm chart exists and templates render a Deployment and Service.
export HOME=/root
[[ -f /root/myapp/chart/Chart.yaml ]] || exit 1
[[ -f /root/myapp/chart/templates/deployment.yaml ]] || exit 1
[[ -f /root/myapp/chart/templates/service.yaml ]] || exit 1
OUTPUT=$(uds zarf tools helm template test /root/myapp/chart/ 2>/dev/null)
echo "$OUTPUT" | grep -q "kind: Deployment" || exit 1
echo "$OUTPUT" | grep -q "kind: Service" || exit 1
exit 0
