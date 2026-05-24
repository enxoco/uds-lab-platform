#!/bin/bash
export HOME=/root
PHASE=$(uds zarf tools kubectl get package hello-uds -n hello-uds \
  -o jsonpath='{.status.phase}' 2>/dev/null)
[[ "$PHASE" == "Ready" ]]
