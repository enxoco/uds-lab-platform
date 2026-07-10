#!/bin/bash
# Pass when the app source files exist in the working directory.
[[ -f /root/myapp/app.py ]] || exit 1
[[ -f /root/myapp/requirements.txt ]] || exit 1
[[ -f /root/myapp/Dockerfile ]] || exit 1
exit 0
