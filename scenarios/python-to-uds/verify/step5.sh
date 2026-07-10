#!/bin/bash
# Pass when the bundle and tasks files exist with correct content.
[[ -f /root/myapp/bundle/uds-bundle.yaml ]] || exit 1
[[ -f /root/myapp/tasks.yaml ]] || exit 1
grep -q "UDSBundle" /root/myapp/bundle/uds-bundle.yaml || exit 1
grep -q "uds-common" /root/myapp/tasks.yaml || exit 1
grep -q "setup:" /root/myapp/tasks.yaml || exit 1
grep -q "name: dev" /root/myapp/tasks.yaml || exit 1
exit 0
