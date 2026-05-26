#!/bin/bash
# Pass when the repo is cloned and key directories exist.
export HOME=/root
[[ -f /root/reference-package/zarf.yaml ]] || exit 1
[[ -d /root/reference-package/chart ]] || exit 1
[[ -d /root/reference-package/bundle ]] || exit 1
exit 0
