#!/bin/bash
export HOME=/root
KUBECTL="uds zarf tools kubectl"
$KUBECTL cluster-info &>/dev/null || exit 1
$KUBECTL get namespace uds-dev-stack &>/dev/null || exit 1
$KUBECTL get namespace istio-system &>/dev/null || exit 1
RUNNING=$($KUBECTL get pods -n uds-dev-stack --no-headers 2>/dev/null \
  | awk '$3=="Running"' | wc -l)
[[ "$RUNNING" -ge 1 ]]
