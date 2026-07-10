#!/bin/bash
# Pass when the UDS Package CR exists with the required fields.
[[ -f /root/myapp/chart/templates/uds-package.yaml ]] || exit 1
grep -q "kind: Package" /root/myapp/chart/templates/uds-package.yaml || exit 1
grep -q "enableAuthserviceSelector" /root/myapp/chart/templates/uds-package.yaml || exit 1
grep -q "network" /root/myapp/chart/templates/uds-package.yaml || exit 1
grep -q "expose" /root/myapp/chart/templates/uds-package.yaml || exit 1
exit 0
