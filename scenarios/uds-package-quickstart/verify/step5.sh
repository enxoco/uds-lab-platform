#!/bin/bash
# Pass when the hello-uds Package CR phase is Ready.
PHASE=$(kubectl get package hello-uds -n hello-uds \
  -o jsonpath='{.status.phase}' 2>/dev/null)
[[ "$PHASE" == "Ready" ]]
