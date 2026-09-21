#!/usr/bin/env bash
#
# 交叉编译 ddns-go（含 DNSHE）→ 打包为飞牛 fnOS .fpk
#
# 用法（在 ddns-go 仓库根目录执行，仓库内需包含本文件与 fnos/ 目录）：
#   ./build-fnos.sh                 # 默认 arm64 + 版本 6.17.7
#   ./build-fnos.sh arm64 6.17.7    # 指定架构与版本
#   ./build-fnos.sh amd64 6.17.7    # 也可出 x86 版
#
set -euo pipefail

APPNAME=ddns-go
ARCH="${1:-arm64}"            # arm64 | amd64
VERSION="${2:-6.17.7}"

ROOT="$(cd "$(dirname "$0")" && pwd)"
FNOS_DIR="$ROOT/fnos"
OUT="$ROOT/${APPNAME}_${ARCH}.fpk"

echo "==> 交叉编译 ddns-go (linux/$ARCH, version $VERSION)"
CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH" \
  go build -trimpath -ldflags "-s -w" -o "$FNOS_DIR/ddns-go" ./

chmod +x "$FNOS_DIR/ddns-go"

echo "==> 写入版本号到 manifest"
# 替换 manifest 中的 version 行（保留字段对齐）
sed -i -E "s/^version[[:space:]]*=.*/version         = $VERSION/" "$FNOS_DIR/manifest"

echo "==> 打包 $OUT"
rm -f "$OUT"
if command -v fnpack >/dev/null 2>&1; then
  # 官方 fnpack（若已安装）：在 fnos 目录内执行
  ( cd "$FNOS_DIR" && fnpack build ) || true
  # 把 fnpack 生成的 .fpk 搬到根目录并重命名（若存在）
  find "$FNOS_DIR" -maxdepth 1 -name '*.fpk' -exec mv -f {} "$OUT" \;
fi
if [ ! -f "$OUT" ]; then
  # 社区已验证：.fpk 本质就是按目录结构打的 tar.gz
  tar -czf "$OUT" -C "$FNOS_DIR" .
fi

echo "==> 完成: $OUT"
ls -lh "$OUT"
