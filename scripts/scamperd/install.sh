#!/bin/sh
#
# 安装 scamper-client-go 的 scamper LaunchDaemon（需 root）。
#
#   sudo sh scripts/scamperd/install.sh
#
# 幂等：重复执行会先卸载再重新加载，便于更新 plist。
set -eu

LABEL="com.scamper-client-go.scamperd"
HERE="$(cd "$(dirname "$0")" && pwd)"
SRC="$HERE/$LABEL.plist"
DST="/Library/LaunchDaemons/$LABEL.plist"

if [ "$(id -u)" -ne 0 ]; then
    echo "install.sh: 需要 root，请用: sudo sh $0" >&2
    exit 1
fi

if [ ! -f "$SRC" ]; then
    echo "install.sh: 找不到 $SRC" >&2
    exit 1
fi

plutil -lint "$SRC" >/dev/null

cp "$SRC" "$DST"
chown root:wheel "$DST"
chmod 0644 "$DST"

# 先卸载旧实例（未加载时忽略错误），再加载，保证幂等。
launchctl bootout system "$DST" >/dev/null 2>&1 || true
launchctl bootstrap system "$DST"
launchctl enable system/"$LABEL"
# bootstrap 会按 RunAtLoad 启动；kickstart 确保立即拉起。
launchctl kickstart -k system/"$LABEL"

echo "installed: $DST"
echo "socket:    /var/run/scamper/scamper.sock"
echo "status:    launchctl print system/$LABEL"
