#!/bin/bash
set -e

APP_NAME="medcare"
VERSION=$(cat VERSION | tr -d 'v\n')
BUILD_TIME=$(date +%Y-%m-%dT%H:%M:%S)
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")

echo "Building Docker image..."
docker build \
  --build-arg VERSION=${VERSION} \
  --build-arg BUILD_TIME=${BUILD_TIME} \
  --build-arg GIT_COMMIT=${GIT_COMMIT} \
  -t ${APP_NAME}:${VERSION} \
  -t ${APP_NAME}:latest \
  .

echo "Docker image built: ${APP_NAME}:${VERSION}"
