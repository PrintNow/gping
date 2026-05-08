#!/usr/bin/env bash
# 在项目根目录执行：编译 gping 到 ./bin/gping。
# 不传参数：仅编译，不会安装、不会修改任何系统/用户配置（仅打印安装建议）。
# 传 INSTALL_DIR：编译后把二进制 install 到该目录，例如：
#   ./build.sh ~/.local/bin
#   ./build.sh /opt/homebrew/bin
# 跨 macOS 与 Linux，按当前 Go 工具链架构编译。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT"

BIN_DIR="$ROOT/bin"
BINARY_NAME="gping"
TARGET="$BIN_DIR/$BINARY_NAME"

OS="$(uname -s)"
ARCH="$(uname -m)"
case "$OS" in
    Darwin|Linux) ;;
    *) printf '提示: 未明确支持的系统 %s，将按当前 Go 工具链编译。\n' "$OS" >&2 ;;
esac

mkdir -p "$BIN_DIR"
VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o "$TARGET" .
chmod +x "$TARGET"

printf '已编译: %s (%s/%s)\n' "$TARGET" "$OS" "$ARCH"

# 显式传入 INSTALL_DIR：仅在用户主动指定时才安装到目标目录。
if [[ $# -ge 1 && -n "${1:-}" ]]; then
    INSTALL_DIR="$1"
    mkdir -p "$INSTALL_DIR"
    install -m 0755 "$TARGET" "$INSTALL_DIR/$BINARY_NAME"
    printf '已安装: %s/%s\n' "$INSTALL_DIR" "$BINARY_NAME"
    case ":$PATH:" in
        *":$INSTALL_DIR:"*)
            printf 'PATH 已包含 %s\n' "$INSTALL_DIR"
            ;;
        *)
            printf '提示: %s 不在 PATH 中，需要时把它加入你的 shell rc 文件。\n' "$INSTALL_DIR"
            ;;
    esac
    exit 0
fi

# 未指定 INSTALL_DIR：只列出候选路径，不会自动复制；按你机器的偏好挑一行执行。
suggestions=()
[[ -d "$HOME/.local/bin" ]] && suggestions+=("$HOME/.local/bin")
[[ -d "$HOME/bin" ]] && suggestions+=("$HOME/bin")
if [[ "$OS" == "Darwin" ]]; then
    if [[ "$ARCH" == "arm64" && -d "/opt/homebrew/bin" ]]; then
        suggestions+=("/opt/homebrew/bin")
    elif [[ -d "/usr/local/bin" ]]; then
        suggestions+=("/usr/local/bin")
    fi
elif [[ "$OS" == "Linux" && -d "/usr/local/bin" ]]; then
    suggestions+=("/usr/local/bin")
fi
# 兜底：即使目录不存在也提示 XDG 标准位置
[[ ${#suggestions[@]} -eq 0 ]] && suggestions+=("$HOME/.local/bin")

printf '\n若要安装到 PATH，可任选其一手动执行：\n'
for d in "${suggestions[@]}"; do
    case ":$PATH:" in
        *":$d:"*) note=" (已在 PATH)" ;;
        *)        note=" (需将该目录加入 PATH)" ;;
    esac
    printf '  install -m 0755 %s %s/%s%s\n' "$TARGET" "$d" "$BINARY_NAME" "$note"
done
