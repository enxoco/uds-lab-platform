#!/bin/bash
# Pass when at least one pod is Running in the myapp namespace.
export HOME=/root
export KUBECONFIG=/root/.kube/config
KUBECTL="uds zarf tools kubectl"
$KUBECTL get namespace myapp &>/dev/null || exit 1
RUNNING=$($KUBECTL get pods -n myapp --no-headers 2>/dev/null \
  | awk '$3=="Running"' | wc -l)
[[ "$RUNNING" -ge 1 ]]
