#!/usr/bin/env bash
# e2e-test.sh — 用药管理端到端测试用例一键执行
#
# 用法: ./scripts/e2e-test.sh [端口]
# 默认端口 18980；自动构建后端、起一个全新数据目录的临时实例、跑用例、清理。
# 不会碰用户的 8913 实例与数据。
#
# 若本机装有 playwright（node_modules/playwright），界面用例会一并执行；
# 否则界面用例标记为「跳过」，可用 NODE_PATH 指向 playwright 所在目录。
set -uo pipefail

PORT="${1:-18980}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP="$(mktemp -d /tmp/medcare-e2e.XXXXXX)"
BIN="$TMP/medcare"
BASE="http://127.0.0.1:${PORT}"
SRV_PID=""

cleanup() {
  if [ -n "$SRV_PID" ]; then kill "$SRV_PID" 2>/dev/null; wait "$SRV_PID" 2>/dev/null; fi
  rm -rf "$TMP"
}
trap cleanup EXIT

echo "构建后端…"
( cd "$ROOT/server" && go build -o "$BIN" . ) || { echo "构建失败"; exit 1; }

if [ ! -d "$ROOT/web/dist" ]; then
  echo "构建前端…"
  ( cd "$ROOT/web" && npm run build ) || { echo "前端构建失败"; exit 1; }
fi

echo "启动临时实例 :${PORT}（数据目录 $TMP/data）…"
"$BIN" -port "${PORT}" -data-dir "$TMP/data" -web-dir "$ROOT/web/dist" -rate-limit 0 -disable-stats >"$TMP/server.log" 2>&1 &
SRV_PID=$!

for _ in $(seq 1 60); do
  if curl -fsS "$BASE/api/version" >/dev/null 2>&1; then break; fi
  sleep 0.25
done
if ! curl -fsS "$BASE/api/version" >/dev/null 2>&1; then
  echo "实例启动失败："; tail -20 "$TMP/server.log"; exit 1
fi

# playwright：优先用项目内依赖，其次用 NODE_PATH 指向的全局目录
PW_DIR=""
if [ -d "$ROOT/web/node_modules/playwright" ]; then
  PW_DIR="$ROOT/web/node_modules"
elif [ -n "${NODE_PATH:-}" ] && [ -d "${NODE_PATH%%:*}/playwright" ]; then
  PW_DIR="${NODE_PATH%%:*}"
fi
[ -z "$PW_DIR" ] && [ -d "/tmp/pwtest/node_modules/playwright" ] && PW_DIR="/tmp/pwtest/node_modules"
export NODE_PATH="${PW_DIR:+${PW_DIR}:${NODE_PATH:-}}"

# 用例需要直接向临时数据库种入识别记录（TC-95~TC-99 与界面删除用例）；不设置则相关用例自动跳过。
export E2E_DATA_DIR="$TMP/data"
export E2E_DB="$TMP/data/db/medcare.db"

node "$ROOT/scripts/e2e-test.js" "$BASE"
CODE=$?

if [ "$CODE" -ne 0 ]; then
  echo ""
  echo "实例日志尾部："
  tail -30 "$TMP/server.log"
fi
exit $CODE
