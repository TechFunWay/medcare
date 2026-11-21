#!/bin/bash
set -e

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

APP_NAME=$(python3 -c "import json; print(json.load(open('app.json'))['appname'])")
FNOS_PKG_NAME=$(awk -F'=' '/^appname/ {gsub(/^[ \t]+|[ \t]+$/, "", $2); print $2}' fnpack/manifest)
VERSION=$(cat VERSION | tr -d '\n')
BUILD_TIME=$(date +%Y-%m-%dT%H:%M:%S)
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS="-X smallgo/server/version.Version=${VERSION} -X smallgo/server/version.BuildTime=${BUILD_TIME} -X smallgo/server/version.GitCommit=${GIT_COMMIT} -X smallgo/server/version.AppName=${APP_NAME}"

# 可见的桌面主入口必须走统一网关，且必须是 type=url 新页面打开
# （2026-10-11 用户定调：应用中心点开要新页面打开，不要 fnOS 桌面内嵌窗口；
# contract 2026-09-28 同款裁定）。应用页面跑在网关域上，飞牛会话同源，飞牛
# 授权登录零跳转；网关域上应用自身的 Authorization 凭证会被接入层拦截，登录
# 态由服务端种下的 HttpOnly 会话 Cookie 维持（网关域不发送该头）。网关另保留
# 一个隐藏入口，供直连端口（旧书签）的访问者走一次性票据跳板。
python3 - <<'PY'
import json

with open("fnpack/app/ui/config", encoding="utf-8") as source:
    config = json.load(source)
entries = config[".url"]
main = entries.get("techfunway-medcare.main")
login = entries.get("techfunway-medcare.FnOSLogin")
if not main or main.get("type") != "url":
    raise SystemExit("fnOS main entry must use type=url (opens in a new page, not an in-desktop window)")
if main.get("protocol") != "" or main.get("gatewayPrefix") != "/app/techfunway-medcare" or main.get("gatewaySocket") != "app.sock":
    raise SystemExit("fnOS main entry must be a unified-gateway entry (protocol='' + gatewayPrefix + gatewaySocket)")
if main.get("url") != "/app/techfunway-medcare":
    raise SystemExit("fnOS main entry must land on the gateway-prefixed SPA root")
if not login or login.get("gatewayPrefix") != "/app/techfunway-medcare" or login.get("gatewaySocket") != "app.sock":
    raise SystemExit("fnOS hidden entry must use the medcare unified gateway")
if not login.get("noDisplay") or login.get("url") != "/app/techfunway-medcare/fnos-entry.html":
    raise SystemExit("fnOS hidden entry must point to the login trampoline and stay hidden")
PY

# Verify Docker is available (required for CGO cross-compilation)
if ! command -v docker &>/dev/null; then
  echo "Error: Docker is required for fnOS package builds (CGO cross-compilation)."
  echo "Install Docker Desktop and try again."
  exit 1
fi

echo "Building frontend..."
VITE_FNOS_APP=true VITE_BASE_PATH="/app/${FNOS_PKG_NAME}/" npm --prefix web ci
VITE_FNOS_APP=true VITE_BASE_PATH="/app/${FNOS_PKG_NAME}/" npm --prefix web run build

echo "Copying frontend..."
rm -rf server/static/dist
cp -r web/dist server/static/dist

BUILD_DIR="release/${VERSION}"
mkdir -p ${BUILD_DIR}

# Save original manifest
cp fnpack/manifest fnpack/manifest.bak

for ARCH in "amd64" "arm64"; do
  FILE_ARCH="$ARCH"
  if [ "$ARCH" = "amd64" ]; then FILE_ARCH="x86"; fi
  echo "Building fnOS package for ${ARCH}..."

  echo "  Compiling Go binary via Docker (CGO_ENABLED=1, linux/${ARCH})..."
  docker run --rm \
    -v "${ROOT_DIR}/server:/src" \
    -v "go-build-cache:/root/.cache/go-build" \
    -v "go-mod-cache:/go/pkg/mod" \
    -w /src \
    --platform "linux/${ARCH}" \
    -e "LDFLAGS=${LDFLAGS}" \
    -e "ARCH=${ARCH}" \
    golang:1.26-alpine \
    sh -c 'apk add --no-cache gcc musl-dev && CGO_ENABLED=1 go build -ldflags "$LDFLAGS -extldflags -static" -o "medcare-linux-${ARCH}" .'

  # Prepare build directory
  BUILD_PACK="${BUILD_DIR}/${APP_NAME}_${ARCH}"
  rm -rf "${BUILD_PACK}"
  mkdir -p "${BUILD_PACK}"

  # Copy fnpack template (only essential directories)
  cp -r fnpack/cmd "${BUILD_PACK}/"
  cp -r fnpack/config "${BUILD_PACK}/"
  cp -r fnpack/wizard "${BUILD_PACK}/"
  mkdir -p "${BUILD_PACK}/app"
  cp -r fnpack/app/ui "${BUILD_PACK}/app/"
  cp fnpack/ICON.PNG "${BUILD_PACK}/"
  cp fnpack/ICON_256.PNG "${BUILD_PACK}/"

  # Copy binary
  cp server/medcare-linux-${ARCH} "${BUILD_PACK}/app/medcare"
  chmod +x "${BUILD_PACK}/app/medcare"
  rm server/medcare-linux-${ARCH}

  # Copy frontend to app/ui
  cp -r server/static/dist/* "${BUILD_PACK}/app/ui/"

  # Generate manifest with correct platform
  if [ "$ARCH" = "amd64" ]; then
    FNOS_PLATFORM="x86"
  else
    FNOS_PLATFORM="arm"
  fi
  sed "s/^platform.*/platform              = ${FNOS_PLATFORM}/" fnpack/manifest > "${BUILD_PACK}/manifest"

  # Update version in manifest
  sed -i '' "s/^version.*/version               = ${VERSION#v}/" "${BUILD_PACK}/manifest" 2>/dev/null || \
  sed -i "s/^version.*/version               = ${VERSION#v}/" "${BUILD_PACK}/manifest"

  # Strip macOS metadata so it never ships inside the package
  find "${BUILD_PACK}" -name '.DS_Store' -delete

  # Build with fnpack
  cd "${BUILD_PACK}"
  fnpack build
  cd "$ROOT_DIR"

  # Move the built fpk to release directory（文件名带版本号，与 bill/reminders 一致）
  if [ -f "${BUILD_PACK}/${FNOS_PKG_NAME}.fpk" ]; then
    mv "${BUILD_PACK}/${FNOS_PKG_NAME}.fpk" "${BUILD_DIR}/${FNOS_PKG_NAME}_${VERSION}_${FILE_ARCH}.fpk"
  elif [ -f "${BUILD_PACK}/../${FNOS_PKG_NAME}.fpk" ]; then
    mv "${BUILD_PACK}/../${FNOS_PKG_NAME}.fpk" "${BUILD_DIR}/${FNOS_PKG_NAME}_${VERSION}_${FILE_ARCH}.fpk"
  fi

  # Clean up
  rm -rf "${BUILD_PACK}"

  echo "Built ${FNOS_PKG_NAME}_${VERSION}_${FILE_ARCH}.fpk"
done

# Restore original manifest
mv fnpack/manifest.bak fnpack/manifest

# fnpack 构建用的是网关前缀版前端（VITE_BASE_PATH=/app/<包名>/），打完后恢复
# 标准构建，避免直连端口（make dev / 普通二进制）的静态资源被替换成前缀版。
echo "Restoring standard frontend build..."
VITE_FNOS_APP=false npm --prefix web run build
rm -rf server/static/dist
mkdir -p server/static/dist
cp -R web/dist/. server/static/dist/

echo "fnOS packages completed in ${BUILD_DIR}/"
