#!/bin/sh
#
# 卸载 scamper-client-go 的 scamper systemd 服务（Linux，需 root）。
#
#   sudo sh scripts/systemd/uninstall.sh
#
set -eu

UNIT="scamper-client-go-scamperd.service"
DST="/etc/systemd/system/$UNIT"

if [ "$(id -u)" -ne 0 ]; then
    echo "uninstall.sh: 需要 root，请用: sudo sh $0" >&2
    exit 1
fi

if command -v systemctl >/dev/null 2>&1; then
    systemctl disable --now "$UNIT" 2>/dev/null || true
    rm -f "$DST"
    systemctl daemon-reload
else
    rm -f "$DST"
fi

rm -rf /run/scamper

echo "uninstalled: $UNIT"
