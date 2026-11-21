#!/usr/bin/env bash
# build-fnpack-local.sh — 不用 Docker，直接用 zig cc 交叉编译并打飞牛 fpk
#
# 用法: ./scripts/build-fnpack-local.sh [amd64|arm64|both]
# 产物: release/<版本>/techfunway-medcare_v<版本>_{x86,arm64}.fpk
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

TARGETS="${1:-both}"
ZIG="${ZIG:-$ROOT_DIR/../tmp/tools/zig-macos-aarch64-0.13.0/zig}"
[ -x "$ZIG" ] || ZIG="$(command -v zig || true)"
[ -n "$ZIG" ] && [ -x "$ZIG" ] || { echo "未找到 zig，请设置 ZIG 指向 zig 可执行文件"; exit 1; }

APP_NAME=$(python3 -c "import json; print(json.load(open('app.json'))['appname'])")
FNOS_PKG_NAME=$(awk -F'=' '/^appname/ {gsub(/^[ \t]+|[ \t]+$/, "", $2); print $2}' fnpack/manifest)
VERSION=$(cat VERSION | tr -d '\n')
BUILD_TIME=$(date +%Y-%m-%dT%H:%M:%S)
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS="-X smallgo/server/version.Version=${VERSION} -X smallgo/server/version.BuildTime=${BUILD_TIME} -X smallgo/server/version.GitCommit=${GIT_COMMIT} -X smallgo/server/version.AppName=${APP_NAME}"

# 桌面主入口必须是统一网关 + type=url（新页面打开）
python3 - <<'PY'
import json
with open("fnpack/app/ui/config", encoding="utf-8") as source:
    config = json.load(source)
entries = config[".url"]
main = entries.get("techfunway-medcare.main")
login = entries.get("techfunway-medcare.FnOSLogin")
if not main or main.get("type") != "url":
    raise SystemExit("fnOS main entry must use type=url")
if main.get("protocol") != "" or main.get("gatewayPrefix") != "/app/techfunway-medcare" or main.get("gatewaySocket") != "app.sock":
    raise SystemExit("fnOS main entry must be a unified-gateway entry")
if main.get("url") != "/app/techfunway-medcare":
    raise SystemExit("fnOS main entry must land on the gateway-prefixed SPA root")
if not login or login.get("gatewayPrefix") != "/app/techfunway-medcare" or not login.get("noDisplay") or login.get("url") != "/app/techfunway-medcare/fnos-entry.html":
    raise SystemExit("fnOS hidden entry must point to the login trampoline and stay hidden")
PY

echo "构建前端（网关前缀版）…"
VITE_FNOS_APP=true VITE_BASE_PATH="/app/${FNOS_PKG_NAME}/" npm --prefix web ci --no-audit --no-fund >/dev/null
VITE_FNOS_APP=true VITE_BASE_PATH="/app/${FNOS_PKG_NAME}/" npm --prefix web run build >/dev/null

BUILD_DIR="release/${VERSION}"
mkdir -p "${BUILD_DIR}"
mkdir -p tmp

build_one() {
  local ARCH="$1" FILE_ARCH="$2" ZTARGET="$3" FNOS_PLATFORM="$4"
  echo "编译 linux/${ARCH}（zig cc）…"
  ( cd server && CGO_ENABLED=1 CC="${ZIG} cc -target ${ZTARGET}" GOOS=linux GOARCH="${ARCH}" \
      go build -ldflags "${LDFLAGS} -linkmode external -extldflags -static" -o "../tmp/medcare-linux-${ARCH}" . )

  local BUILD_PACK="${BUILD_DIR}/${APP_NAME}_${ARCH}"
  rm -rf "${BUILD_PACK}"
  mkdir -p "${BUILD_PACK}/app"
  cp -r fnpack/cmd fnpack/config fnpack/wizard "${BUILD_PACK}/"
  cp -r fnpack/app/ui "${BUILD_PACK}/app/"
  cp fnpack/ICON.PNG fnpack/ICON_256.PNG "${BUILD_PACK}/"
  cp "tmp/medcare-linux-${ARCH}" "${BUILD_PACK}/app/medcare"
  chmod +x "${BUILD_PACK}/app/medcare"
  cp -R web/dist/. "${BUILD_PACK}/app/ui/"
  sed "s/^platform.*/platform                   = ${FNOS_PLATFORM}/" fnpack/manifest > "${BUILD_PACK}/manifest"
  sed -i '' "s/^version.*/version                    = ${VERSION#v}/" "${BUILD_PACK}/manifest" 2>/dev/null || \
    sed -i "s/^version.*/version                    = ${VERSION#v}/" "${BUILD_PACK}/manifest"
  find "${BUILD_PACK}" -name '.DS_Store' -delete

  ( cd "${BUILD_PACK}" && fnpack build )
  if [ -f "${BUILD_PACK}/${FNOS_PKG_NAME}.fpk" ]; then
    mv "${BUILD_PACK}/${FNOS_PKG_NAME}.fpk" "${BUILD_DIR}/${FNOS_PKG_NAME}_${VERSION}_${FILE_ARCH}.fpk"
  elif [ -f "${BUILD_PACK}/../${FNOS_PKG_NAME}.fpk" ]; then
    mv "${BUILD_PACK}/../${FNOS_PKG_NAME}.fpk" "${BUILD_DIR}/${FNOS_PKG_NAME}_${VERSION}_${FILE_ARCH}.fpk"
  else
    echo "打包失败：未找到 fpk"; exit 1
  fi
  rm -rf "${BUILD_PACK}"
  echo "完成 ${FNOS_PKG_NAME}_${VERSION}_${FILE_ARCH}.fpk"
}

case "${TARGETS}" in
  amd64|x86) build_one amd64 x86 x86_64-linux-musl x86 ;;
  arm64)     build_one arm64 arm64 aarch64-linux-musl arm ;;
  both)      build_one amd64 x86 x86_64-linux-musl x86; build_one arm64 arm64 aarch64-linux-musl arm ;;
  *) echo "用法: $0 [amd64|arm64|both]"; exit 1 ;;
esac

echo "恢复标准前端构建（直连端口用）…"
VITE_FNOS_APP=false npm --prefix web run build >/dev/null
rm -rf server/static/dist
mkdir -p server/static
cp -R web/dist/. server/static/dist/

echo "产物："
ls -lh "${BUILD_DIR}"/*.fpk
