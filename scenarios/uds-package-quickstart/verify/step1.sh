#!/bin/bash
# Pass when docker, k3d, and uds are all installed.
command -v docker >/dev/null 2>&1 || exit 1
command -v k3d >/dev/null 2>&1 || exit 1
command -v uds >/dev/null 2>&1 || exit 1
exit 0
