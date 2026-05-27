#!/bin/bash
# Pass when postgres-operator is NOT declared in zarf.yaml (user has confirmed correct structure).
# Grep returns exit 0 if found — invert with !
export HOME=/root
! grep -q "postgres-operator" /root/reference-package/zarf.yaml
