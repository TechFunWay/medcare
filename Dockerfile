ARG VERSION=dev
ARG BUILD_TIME=unknown
ARG GIT_COMMIT=unknown

FROM node:20-alpine AS frontend-builder
WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26-alpine AS backend-builder
ARG VERSION
ARG BUILD_TIME
ARG GIT_COMMIT
# SQLite 驱动需要 CGO，alpine 镜像默认没有 gcc，先装编译工具链
RUN apk add --no-cache gcc musl-dev
WORKDIR /app/server
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
RUN CGO_ENABLED=1 GOOS=linux go build \
	  -ldflags "-X smallgo/server/version.Version=${VERSION} -X smallgo/server/version.BuildTime=${BUILD_TIME} -X smallgo/server/version.GitCommit=${GIT_COMMIT} -X smallgo/server/version.AppName=medcare" \
	  -o /app/server/medcare-bin .

FROM alpine:3.20
ENV TZ=Asia/Shanghai
RUN apk add --no-cache ca-certificates tzdata
RUN adduser -D -u 1000 appuser
WORKDIR /app
COPY --from=backend-builder /app/server/medcare-bin ./medcare
COPY --from=frontend-builder /app/web/dist ./static/dist
RUN mkdir -p /app/data && chown -R appuser:appuser /app
USER appuser
EXPOSE 8913
VOLUME ["/app/data"]
HEALTHCHECK --interval=30s --timeout=3s --retries=3 \
  CMD wget -qO- http://localhost:8913/api/version || exit 1
ENTRYPOINT ["./medcare"]
CMD ["-data-dir", "/app/data", "-web-dir", "./static/dist"]
