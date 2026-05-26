#!/bin/bash
# Pass when at least one pod is Running in the reference-package namespace.
export HOME=/root
export KUBECONFIG=/root/.kube/config
KUBECTL="uds zarf tools kubectl"
$KUBECTL get namespace reference-package &>/dev/null || exit 1
RUNNING=$($KUBECTL get pods -n reference-package --no-headers 2>/dev/null \
  | awk '$3=="Running"' | wc -l)
[[ "$RUNNING" -ge 1 ]]
