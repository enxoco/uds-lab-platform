# Zarf Injector Cleanup Race POC

This reproduces the Kubernetes behavior behind a Zarf injector cleanup
failure. It lists a payload ConfigMap, deletes it between the list and delete
calls, then repeats the plain delete used by Zarf. Kubernetes returns
`NotFound`, even though the desired end state has already been reached.

The idempotent form, `--ignore-not-found`, succeeds for the same state.

## Requirements

- `kubectl`
- Access to a disposable Kubernetes cluster

## Run

```bash
sh zarf-injector-cleanup-race.sh
```

## Script

```sh
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
```

## Expected output

The plain delete should report that the ConfigMap was not found, followed by:

```text
POC passed: list-then-delete returns NotFound when the object disappears between calls.
```

## Relevance to Zarf

Zarf injector cleanup performs a list followed by individual deletes for
legacy `zarf-payload-*` ConfigMaps. If a payload ConfigMap disappears between
those operations, cleanup returns `NotFound` and can cause `zarf init` to fail
after the seed registry has already become healthy.
