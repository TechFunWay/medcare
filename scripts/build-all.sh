#!/bin/bash
# 全平台编译打包——产物格式对齐 release/<版本>/ 发行目录规范：
#   techfunway-medcare-v<版本>-{linux,darwin}-{amd64,arm64}.tar.gz
#   techfunway-medcare-v<版本>-windows-amd64.zip
#   docker-compose.yml（镜像 tag 钉死当前版本）/ docker-compose.latest.yml
#   CHANGELOG.md / screenshots/ + screenshots-<版本>.zip
# 飞牛 fpk 由 scripts/build-fnpack-local.sh 单独产出（顺序：先 build-all，再 fnpack）。
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

APP_NAME="medcare"
PKG_PREFIX="techfunway-medcare"
VERSION=$(cat VERSION | tr -d "v\n")
BUILD_TIME=$(date +%Y-%m-%dT%H:%M:%S)
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS="-X smallgo/server/version.Version=$VERSION -X smallgo/server/version.BuildTime=$BUILD_TIME -X smallgo/server/version.GitCommit=$GIT_COMMIT -X smallgo/server/version.AppName=$APP_NAME"

# go 构建缓存：HOME 可能被重定向到不可写目录，显式给一个可写缓存
export GOCACHE="${GOCACHE:-/tmp/gocache-medcare}"

echo "Building frontend..."
cd web && npm ci >/dev/null && npm run build >/dev/null && cd ..

echo "Copying frontend..."
rm -rf server/static/dist
mkdir -p server/static
cp -r web/dist server/static/dist

# 发行目录统一带 v 前缀（与 release/v<版本>/ 约定、fpk 产物一致）
BUILD_DIR="release/v$VERSION"
rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR"

PLATFORMS=(
  "linux/amd64"
  "linux/arm64"
  "darwin/amd64"
  "darwin/arm64"
  "windows/amd64"
)

for PLATFORM in "${PLATFORMS[@]}"; do
  IFS="/" read -r GOOS GOARCH <<< "$PLATFORM"
  OUTPUT_NAME="$PKG_PREFIX-v$VERSION-$GOOS-$GOARCH"

  echo "Building $OUTPUT_NAME..."

  if [ "$GOOS" = "linux" ]; then
    # Linux 目标在容器内编译（CGO 静态链接）
    docker run --rm \
      -v "$ROOT_DIR/server:/src" \
      -v "go-build-cache:/root/.cache/go-build" \
      -v "go-mod-cache:/go/pkg/mod" \
      -w /src \
      --platform "linux/$GOARCH" \
      -e "LDFLAGS=$LDFLAGS" \
      -e "GOARCH=$GOARCH" \
      -e "APP_NAME=$APP_NAME" \
      golang:1.26-alpine \
      sh -c 'apk add --no-cache gcc musl-dev >/dev/null && CGO_ENABLED=1 go build -ldflags "$LDFLAGS -extldflags -static" -o "$APP_NAME-linux-$GOARCH" .'
    # 容器内产物名是 medcare-linux-<arch>，重命名成发行用的带版本名
    mv "server/$APP_NAME-linux-$GOARCH" "server/$OUTPUT_NAME"
  elif [ "$GOOS" = "windows" ]; then
    # Windows 必须开 CGO（SQLite 驱动是 mattn/go-sqlite3），用 mingw-w64 编真二进制。
    if ! command -v x86_64-w64-mingw32-gcc >/dev/null 2>&1; then
      echo "错误：未找到 x86_64-w64-mingw32-gcc，请先安装：brew install mingw-w64"
      exit 1
    fi
    cd server
    CGO_ENABLED=1 GOOS=$GOOS GOARCH=$GOARCH CC=x86_64-w64-mingw32-gcc \
      go build -ldflags "$LDFLAGS -extldflags -static" -o "$OUTPUT_NAME.exe" .
    cd "$ROOT_DIR"
  else
    cd server
    CGO_ENABLED=1 GOOS=$GOOS GOARCH=$GOARCH go build -ldflags "$LDFLAGS" -o "$OUTPUT_NAME" .
    cd "$ROOT_DIR"
  fi

  mkdir -p "$BUILD_DIR/$OUTPUT_NAME"

  if [ "$GOOS" = "windows" ]; then
    cp "server/$OUTPUT_NAME.exe" "$BUILD_DIR/$OUTPUT_NAME/$APP_NAME.exe"
    rm "server/$OUTPUT_NAME.exe"
  else
    cp "server/$OUTPUT_NAME" "$BUILD_DIR/$OUTPUT_NAME/$APP_NAME"
    rm "server/$OUTPUT_NAME"
  fi

  cp -r server/static/dist "$BUILD_DIR/$OUTPUT_NAME/www"
  find "$BUILD_DIR/$OUTPUT_NAME" -name ".DS_Store" -delete

  cd "$ROOT_DIR/$BUILD_DIR"
  if [ "$GOOS" = "windows" ]; then
    zip -qr "$OUTPUT_NAME.zip" "$OUTPUT_NAME"
  else
    COPYFILE_DISABLE=1 tar czf "$OUTPUT_NAME.tar.gz" "$OUTPUT_NAME"
  fi
  cd "$ROOT_DIR"

  rm -rf "$BUILD_DIR/$OUTPUT_NAME"
  echo "Built $OUTPUT_NAME"
done

# ---- 随发行分发的部署文件与更新日志 ----
echo "Copying release extras..."
sed "s|techfunways/medcare:latest|techfunways/medcare:v$VERSION|g" \
  deploy/docker-compose.yml > "$BUILD_DIR/docker-compose.yml"
cp deploy/docker-compose.latest.yml "$BUILD_DIR/docker-compose.latest.yml"
cp CHANGELOG.md "$BUILD_DIR/CHANGELOG.md"

if [ -d screenshots ] && [ -n "$(ls -A screenshots 2>/dev/null)" ]; then
  rm -rf "$BUILD_DIR/screenshots"
  cp -R screenshots "$BUILD_DIR/screenshots"
  ( cd "$BUILD_DIR" && zip -qr "screenshots-v$VERSION.zip" screenshots )
fi

echo "All builds completed in $BUILD_DIR/"
ls -lh "$BUILD_DIR"
