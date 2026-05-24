#!/bin/bash
# Pass when zarf.yaml and values/values.yaml exist with expected content.
BASE=/root/hello-uds
[[ -f "$BASE/zarf.yaml" ]] || exit 1
[[ -f "$BASE/values/values.yaml" ]] || exit 1
grep -q "ZarfPackageConfig" "$BASE/zarf.yaml" || exit 1
grep -q "hello-uds" "$BASE/zarf.yaml" || exit 1
grep -q "ZARF_VAR_DOMAIN" "$BASE/values/values.yaml" || exit 1
exit 0
