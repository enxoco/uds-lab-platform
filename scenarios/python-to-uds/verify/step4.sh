#!/bin/bash
# Pass when zarf.yaml exists with the correct kind and image reference.
[[ -f /root/myapp/zarf.yaml ]] || exit 1
grep -q "ZarfPackageConfig" /root/myapp/zarf.yaml || exit 1
grep -q "myapp" /root/myapp/zarf.yaml || exit 1
grep -q "myapp:dev" /root/myapp/zarf.yaml || exit 1
exit 0
