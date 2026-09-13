#!/bin/sh
#
# 安装 scamper-client-go 的 scamper systemd 服务（Linux，需 root）。
#
#   sudo sh scripts/systemd/install.sh
#
# 幂等：重复执行会覆盖 unit 并 reload/重启。
set -eu

UNIT="scamper-client-go-scamperd.service"
HERE="$(cd "$(dirname "$0")" && pwd)"
SRC="$HERE/$UNIT"
DST="/etc/systemd/system/$UNIT"

if [ "$(id -u)" -ne 0 ]; then
    echo "install.sh: 需要 root，请用: sudo sh $0" >&2
    exit 1
fi

if ! command -v systemctl >/dev/null 2>&1; then
    echo "install.sh: 未找到 systemctl，本脚本仅适用于 systemd 系统" >&2
    exit 1
fi

if [ ! -f "$SRC" ]; then
    echo "install.sh: 找不到 $SRC" >&2
    exit 1
fi

# 探测 scamper 二进制路径（apt/yum 包多在 /usr/bin，源码安装在 /usr/local/bin）。
SCAMPER_BIN="$(command -v scamper 2>/dev/null || true)"
if [ -z "$SCAMPER_BIN" ]; then
    SCAMPER_BIN="/usr/local/bin/scamper"
    echo "install.sh: PATH 中未找到 scamper，默认使用 $SCAMPER_BIN" >&2
fi
if [ ! -x "$SCAMPER_BIN" ]; then
    echo "install.sh: 警告: $SCAMPER_BIN 不存在或不可执行" >&2
fi

sed "s#^ExecStart=.*#ExecStart=$SCAMPER_BIN -U /run/scamper/scamper.sock -w 100 -p 10000#" "$SRC" > "$DST"
chown root:root "$DST"
chmod 0644 "$DST"

systemctl daemon-reload
systemctl enable --now "$UNIT"
systemctl --no-pager --full status "$UNIT" || true

echo
echo "installed: $DST"
echo "binary:    $SCAMPER_BIN"
echo "socket:    /run/scamper/scamper.sock  (= /var/run/scamper/scamper.sock)"
echo "status:    systemctl status $UNIT"
echo "logs:      journalctl -u $UNIT -f"
