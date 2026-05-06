#!/usr/bin/env bash
# 在项目根目录执行：编译 gping 并安装到本机用户路径（适合 macOS Apple Silicon，原生 arm64）。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT"

# 安装目录：可通过第一个参数覆盖，例如 ./build.sh /opt/homebrew/bin
INSTALL_DIR="${1:-$HOME/bin}"
BINARY_NAME="gping"
TARGET="$INSTALL_DIR/$BINARY_NAME"

if [[ "$(uname -s)" != "Darwin" ]]; then
	printf '提示: 当前不是 macOS，仍会继续编译为当前系统架构。\n' >&2
fi
ARCH="$(uname -m)"
if [[ "$(uname -s)" == "Darwin" && "$ARCH" != "arm64" ]]; then
	printf '提示: Apple Silicon 一般为 arm64；本机为 %s，将编译为当前 CPU 架构。\n' "$ARCH" >&2
fi

mkdir -p "$INSTALL_DIR"
go build -o "$TARGET" .
chmod +x "$TARGET"

printf '已安装: %s\n' "$TARGET"
case ":$PATH:" in
*":$INSTALL_DIR:"*) printf 'PATH 已包含 %s\n' "$INSTALL_DIR" ;;
*)
	printf '\n若命令找不到，可把下面一行加入 ~/.zshrc 后执行 source ~/.zshrc：\n'
	printf '  export PATH="%s:$PATH"\n' "$INSTALL_DIR"
	;;
esac
