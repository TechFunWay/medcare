#!/bin/bash
# 构建 docker 多平台合并镜像（linux/amd64 + linux/arm64），输出本地 OCI 归档，不推远端。
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

IMAGE="techfunways/medcare"
BUILDER="techfunway-medcare-multiarch"
VERSION="$(cat VERSION | tr -d "v\n")"
BUILD_TIME="$(date +%Y-%m-%dT%H:%M:%S)"
GIT_COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
# 发行目录统一带 v 前缀（与 build-all.sh、fpk 产物一致）
OUTPUT_DIR="release/v$VERSION"
OUTPUT_FILE="$OUTPUT_DIR/techfunway-medcare-v$VERSION-multiarch.oci.tar"

if ! command -v docker >/dev/null 2>&1; then
  echo "Error: Docker is required to build the multi-platform image."
  exit 1
fi

mkdir -p "$OUTPUT_DIR"
rm -f "$OUTPUT_FILE"

# 单一 OCI 归档需要 docker-container 驱动；默认驱动无法可靠导出合并 manifest。
if ! docker buildx inspect "$BUILDER" >/dev/null 2>&1; then
  docker buildx create --name "$BUILDER" --driver docker-container --use >/dev/null
fi

echo "Building $IMAGE:v$VERSION for linux/amd64,linux/arm64..."
docker buildx build \
  --builder "$BUILDER" \
  --platform linux/amd64,linux/arm64 \
  --build-arg "VERSION=$VERSION" \
  --build-arg "BUILD_TIME=$BUILD_TIME" \
  --build-arg "GIT_COMMIT=$GIT_COMMIT" \
  --build-arg "APK_MIRROR=${APK_MIRROR:-}" \
  -t "$IMAGE:v$VERSION" \
  -t "$IMAGE:latest" \
  --output "type=oci,dest=$OUTPUT_FILE" \
  .

echo "Multi-platform OCI image archive completed: $OUTPUT_FILE"
echo "Load it on a target with: docker load -i $OUTPUT_FILE"
