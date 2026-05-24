#!/bin/bash
# Pass when UDS Core key pods are Running.
RUNNING=$(kubectl get pods -n uds-dev-stack --no-headers 2>/dev/null \
  | awk '$3=="Running"' | wc -l)
[[ "$RUNNING" -ge 3 ]]
