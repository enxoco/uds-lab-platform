#!/bin/bash
# Pass when uds-package.yaml exists with the Package CR.
MANIFEST=/root/hello-uds/manifests/uds-package.yaml
[[ -f "$MANIFEST" ]] || exit 1
grep -q "kind: Package" "$MANIFEST" || exit 1
grep -q "uds.dev/v1alpha1" "$MANIFEST" || exit 1
grep -q "hello-uds" "$MANIFEST" || exit 1
exit 0
