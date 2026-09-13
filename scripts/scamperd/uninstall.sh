#!/bin/sh
#
# 卸载 scamper-client-go 的 scamper LaunchDaemon（需 root）。
#
#   sudo sh scripts/scamperd/uninstall.sh
#
set -eu

LABEL="com.scamper-client-go.scamperd"
DST="/Library/LaunchDaemons/$LABEL.plist"

if [ "$(id -u)" -ne 0 ]; then
    echo "uninstall.sh: 需要 root，请用: sudo sh $0" >&2
    exit 1
fi

launchctl bootout system "$DST" >/dev/null 2>&1 || true
rm -f "$DST"
rm -f /var/run/scamper/scamper.sock /var/run/scamper/scamper.pid

echo "uninstalled: $LABEL"
