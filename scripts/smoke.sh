#!/usr/bin/env bash
# smoke.sh — 用药管理端到端冒烟
#
# 用法: ./scripts/smoke.sh [端口]
# 默认端口 18913，临时数据目录，跑完即清理；不会碰用户的 8913 实例。
set -uo pipefail

PORT="${1:-18913}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP="$(mktemp -d /tmp/medcare-smoke.XXXXXX)"
BIN="$TMP/medcare"
BASE="http://127.0.0.1:${PORT}"
FAILS=0
SRV_PID=""

say()  { printf '%s\n' "$*"; }
pass() { say "PASS  $*"; }
fail() { say "FAIL  $*"; FAILS=$((FAILS+1)); }

cleanup() {
  if [ -n "${SRV_PID}" ]; then kill "${SRV_PID}" 2>/dev/null; wait "${SRV_PID}" 2>/dev/null; fi
  rm -rf "$TMP"
}
trap cleanup EXIT

say "构建后端…"
( cd "$ROOT/server" && go build -o "$BIN" . ) || { fail "go build"; exit 1; }

# 前端产物：优先用 make 复制到 server/static/dist 的产物，没有就用直接在
# web 下构建的 dist。原先只认 server/static/dist，干净检出上那个目录并不存在，
# 「SPA 可访问」会恒返回 404。
say "准备前端产物…"
WEB_DIST="$ROOT/server/static/dist"
if [ ! -f "$WEB_DIST/index.html" ]; then
  if [ ! -f "$ROOT/web/dist/index.html" ]; then
    ( cd "$ROOT/web" && npm run build >/dev/null ) || { fail "前端构建"; exit 1; }
  fi
  WEB_DIST="$ROOT/web/dist"
fi

say "启动临时实例 :${PORT}…"
"$BIN" -port "${PORT}" -data-dir "$TMP/data" -web-dir "$WEB_DIST" -rate-limit 0 >"$TMP/server.log" 2>&1 &
SRV_PID=$!

for _ in $(seq 1 40); do
  if curl -fsS "$BASE/api/health" >/dev/null 2>&1; then break; fi
  sleep 0.25
done
if ! curl -fsS "$BASE/api/health" >/dev/null 2>&1; then
  fail "健康检查"; cat "$TMP/server.log"; exit 1
fi
pass "健康检查"

json_get() { python3 -c "import json,sys; d=json.load(sys.stdin); print(eval(sys.argv[1]))" "$1"; }

REG=$(curl -sS -X POST "$BASE/api/auth/register" -H 'Content-Type: application/json' -d '{"username":"smoke","password":"smoke1234"}')
TOKEN=$(printf '%s' "$REG" | json_get "d['data']['token']")
AUTH="Authorization: Bearer $TOKEN"
[ -n "$TOKEN" ] && pass "注册并登录" || { fail "注册"; exit 1; }

BOOK=$(curl -sS -X POST "$BASE/api/medcare/books" -H "$AUTH" -H 'Content-Type: application/json' -d '{"name":"冒烟用药本","care_recipient_name":"测试","timezone":"Asia/Shanghai"}')
BOOK_ID=$(printf '%s' "$BOOK" | json_get "d['data']['id']")
[ "$BOOK_ID" = "1" ] && pass "创建用药本" || fail "创建用药本"

MED=$(curl -sS -X POST "$BASE/api/medcare/books/$BOOK_ID/medicines" -H "$AUTH" -H 'Content-Type: application/json' -d '{"name":"测试药片","category":"oral","unit":"片","spec":"10mg×20片"}')
MED_ID=$(printf '%s' "$MED" | json_get "d['data']['id']")
[ -n "$MED_ID" ] && pass "创建药品" || fail "创建药品"

STOCK=$(curl -sS -X POST "$BASE/api/medcare/books/$BOOK_ID/medicines/$MED_ID/stock-in" -H "$AUTH" -H 'Content-Type: application/json' -d '{"packages":2,"units_per_package":20,"expiry_date":"2027-06-30"}')
QTY=$(printf '%s' "$STOCK" | json_get "d['data']['medicine']['stock_quantity']")
[ "$QTY" = "40" ] && pass "入库换算为 40" || fail "入库（得到 ${QTY}）"

PLAN=$(curl -sS -X POST "$BASE/api/medcare/books/$BOOK_ID/plans" -H "$AUTH" -H 'Content-Type: application/json'   --data-binary "{\"medicine_id\":$MED_ID,\"title\":\"每日一次\",\"type\":\"oral\",\"dose_amount\":1,\"dose_unit\":\"片\",\"frequency\":{\"type\":\"daily\",\"times\":[\"08:00\"]},\"start_date\":\"$(date +%F)\",\"timezone\":\"Asia/Shanghai\"}")
PLAN_ID=$(printf '%s' "$PLAN" | json_get "d['data']['plan']['id']")
[ -n "$PLAN_ID" ] && pass "创建计划" || fail "创建计划"

EVENT=$(curl -sS "$BASE/api/medcare/books/$BOOK_ID/dose-events?pageSize=5" -H "$AUTH")
EVENT_ID=$(printf '%s' "$EVENT" | json_get "d['data']['items'][0]['id']" 2>/dev/null)
if [ -n "$EVENT_ID" ] && [ "$EVENT_ID" != "None" ]; then
  pass "生成日程事件"
  DONE=$(curl -sS -X POST "$BASE/api/medcare/books/$BOOK_ID/dose-events/$EVENT_ID/complete" -H "$AUTH" -H 'Content-Type: application/json' -d "{\"deduct_stock\":true}")
  STATUS=$(printf '%s' "$DONE" | json_get "d['data']['event']['status']")
  [ "$STATUS" = "done" ] && pass "完成打卡" || fail "完成打卡（${STATUS}）"
  MED2=$(curl -sS "$BASE/api/medcare/books/$BOOK_ID/medicines/$MED_ID" -H "$AUTH")
  QTY2=$(printf '%s' "$MED2" | json_get "d['data']['stock_quantity']")
  [ "$QTY2" = "39" ] && pass "完成扣减库存 39" || fail "库存扣减（${QTY2}）"
  UNDO=$(curl -sS -X POST "$BASE/api/medcare/books/$BOOK_ID/dose-events/$EVENT_ID/undo" -H "$AUTH")
  QTY3=$(curl -sS "$BASE/api/medcare/books/$BOOK_ID/medicines/$MED_ID" -H "$AUTH" | json_get "d['data']['stock_quantity']")
  [ "$QTY3" = "40" ] && pass "撤销恢复库存 40" || fail "撤销恢复（${QTY3}）"
else
  fail "生成日程事件"
fi

INVITE=$(curl -sS -X POST "$BASE/api/medcare/books/$BOOK_ID/invites" -H "$AUTH" -H 'Content-Type: application/json' -d '{"role":"manager","expires_hours":168,"max_uses":5}')
TOKEN2=$(printf '%s' "$INVITE" | json_get "d['data']['token']")
[ -n "$TOKEN2" ] && pass "生成邀请" || fail "生成邀请"
REG2=$(curl -sS -X POST "$BASE/api/auth/register" -H 'Content-Type: application/json' -d '{"username":"smoke2","password":"smoke1234"}')
T2=$(printf '%s' "$REG2" | json_get "d['data']['token']")
JOIN=$(curl -sS -X POST "$BASE/api/medcare/invites/$TOKEN2/join" -H "Authorization: Bearer $T2")
JOINED=$(printf '%s' "$JOIN" | json_get "d['data']['id']" 2>/dev/null)
[ "$JOINED" = "$BOOK_ID" ] && pass "受邀加入用药本" || fail "受邀加入"

DENY=$(curl -sS -o /dev/null -w '%{http_code}' "$BASE/api/medcare/books/$BOOK_ID/medicines" -H "Authorization: Bearer $TOKEN")
[ "$DENY" = "200" ] && pass "成员可读药品" || fail "成员读药品（${DENY}）"

STATS=$(curl -sS "$BASE/api/medcare/books/$BOOK_ID/stats/overview" -H "$AUTH")
RATE=$(printf '%s' "$STATS" | json_get "d['data']['completion_rate']")
[ -n "$RATE" ] && pass "统计概览" || fail "统计概览"

SHELL_CODE=$(curl -sS -o /dev/null -w '%{http_code}' "$BASE/today")
[ "$SHELL_CODE" = "200" ] && pass "SPA 可访问" || fail "SPA（${SHELL_CODE}）"

say ""
if [ "$FAILS" -eq 0 ]; then say "全部通过"; exit 0; else say "失败 $FAILS 项"; exit 1; fi
