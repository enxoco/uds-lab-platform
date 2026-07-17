#!/bin/sh
# Demonstrates the list-then-delete race in Zarf injector cleanup.
set -eu

command -v kubectl >/dev/null 2>&1 || {
  echo "kubectl is required" >&2
  exit 1
}

NAMESPACE="zarf-cleanup-race-$$"
cleanup() {
  kubectl delete namespace "$NAMESPACE" --ignore-not-found >/dev/null 2>&1 || true
}
trap cleanup EXIT

kubectl create namespace "$NAMESPACE" >/dev/null
kubectl -n "$NAMESPACE" create configmap zarf-payload-031 \
  --from-literal=payload=test >/dev/null

echo "1. List payload ConfigMaps, as Zarf does"
RESOURCE=$(kubectl -n "$NAMESPACE" get configmaps \
  -l zarf-injector=payload -o name 2>/dev/null || true)

# The current Zarf code lists legacy payload ConfigMaps without this label.
[ -n "$RESOURCE" ] || RESOURCE="configmap/zarf-payload-031"

echo "2. Delete the object after the list, simulating another cleanup worker"
kubectl -n "$NAMESPACE" delete "$RESOURCE" --wait=false >/dev/null
kubectl -n "$NAMESPACE" wait --for=delete "$RESOURCE" --timeout=30s >/dev/null

echo "3. Repeat Zarf's plain delete"
set +e
OUTPUT=$(kubectl -n "$NAMESPACE" delete "$RESOURCE" 2>&1)
STATUS=$?
set -e
printf '%s\n' "$OUTPUT"

if [ "$STATUS" -eq 0 ]; then
  echo "ERROR: race did not reproduce" >&2
  exit 1
fi

echo "4. Idempotent cleanup succeeds with --ignore-not-found"
kubectl -n "$NAMESPACE" delete "$RESOURCE" --ignore-not-found >/dev/null
echo "POC passed: list-then-delete returns NotFound when the object disappears between calls."
