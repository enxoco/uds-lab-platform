#!/bin/bash
# Pass when the Package CR phase is Ready.
export HOME=/root
export KUBECONFIG=/root/.kube/config
PHASE=$(uds zarf tools kubectl get package reference-package -n reference-package \
  -o jsonpath='{.status.phase}' 2>/dev/null)
[[ "$PHASE" == "Ready" ]]
