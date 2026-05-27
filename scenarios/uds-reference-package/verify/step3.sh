#!/bin/bash
# Pass when the UDS Package CR template exists and declares SSO and network expose.
export HOME=/root
CR=/root/reference-package/chart/templates/uds-package.yaml
[[ -f "$CR" ]] || exit 1
grep -q "expose" "$CR" || exit 1
grep -q "sso" "$CR" || exit 1
exit 0
