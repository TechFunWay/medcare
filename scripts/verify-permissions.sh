#!/usr/bin/env bash
# verify-permissions.sh — 逐人权限（模块 × 动作）端到端验收
#
# 场景：用药本里的用药人可能是老人、孕妇、初中生或大学生，帮忙的家人各管一段。
# 管理员对每位成员逐格勾选：查看、新增、修改、删除分别是独立的一格，
# 勾了才有、不勾就没有（包括连列表都看不到）。
# 这里验的是后端鉴权与权限下发，不只是前端把按钮藏起来。
#
# 用法: ./scripts/verify-permissions.sh [端口]
# 默认端口 18915，临时数据目录，跑完即清理；不会碰用户的 8913 实例。
set -uo pipefail

PORT="${1:-18915}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP="$(mktemp -d /tmp/medcare-perm.XXXXXX)"
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

json_get() { python3 -c "import json,sys; d=json.load(sys.stdin); print(eval(sys.argv[1]))" "$1"; }
code_of() { curl -sS -o /dev/null -w '%{http_code}' "$@"; }

# 取某位成员的字段（权限按逗号连接），一律用管理员身份读，避免被测者自己没权限
member_field() {
  curl -sS "$BASE/api/medcare/books/$BOOK_ID/members" -H "$AUTH" | python3 -c "
import json, sys
name, field = sys.argv[1], sys.argv[2]
for m in json.load(sys.stdin)['data']:
    if m['username'] == name:
        v = m.get(field)
        print(','.join(sorted(v)) if isinstance(v, list) else v)
        break
" "$1" "$2"
}
member_id() { member_field "$1" user_id; }
member_perms() { member_field "$1" permissions; }
is_custom() { member_field "$1" custom_permissions; }

perm_has() {
  printf '%s' "$1" | python3 -c "
import sys
key = sys.argv[1]
keys = [k for k in sys.stdin.read().split(',') if k]
print('yes' if key in keys else 'no')
" "$2"
}
expect_has() {
  local set="$1" key="$2" label="$3"
  if [ "$(perm_has "$set" "$key")" = "yes" ]; then pass "${label}（含 ${key}）"; else fail "${label}：应含 ${key}，实际 [${set}]"; fi
}
expect_lacks() {
  local set="$1" key="$2" label="$3"
  if [ "$(perm_has "$set" "$key")" = "no" ]; then pass "${label}（不含 ${key}）"; else fail "${label}：不该含 ${key}，实际 [${set}]"; fi
}
expect_eq() {
  local want="$1" got="$2" label="$3"
  if [ "$want" = "$got" ]; then pass "${label}（${got}）"; else fail "${label} 期望 ${want} 得到 ${got}"; fi
}
expect_code() {
  local want="$1" label="$2"; shift 2
  local got; got=$(code_of "$@")
  if [ "$got" = "$want" ]; then pass "${label}（${got}）"; else fail "${label} 期望 ${want} 得到 ${got}"; fi
}
set_perms() { curl -sS -X PUT "$BASE/api/medcare/books/$BOOK_ID/members/$1/permissions" -H "$AUTH" -H 'Content-Type: application/json' -d "$2" >/dev/null; }
count_of() { printf '%s' "$1" | python3 -c "import sys;print(len([k for k in sys.stdin.read().split(',') if k]))"; }

say "构建后端…"
( cd "$ROOT/server" && go build -o "$BIN" . ) || { fail "go build"; exit 1; }

WEB_DIST="$ROOT/server/static/dist"
if [ ! -f "$WEB_DIST/index.html" ]; then
  [ -f "$ROOT/web/dist/index.html" ] || { fail "缺少前端产物（先 cd web && npm run build）"; exit 1; }
  WEB_DIST="$ROOT/web/dist"
fi

say "启动临时实例 :${PORT}…"
"$BIN" -port "${PORT}" -data-dir "$TMP/data" -web-dir "$WEB_DIST" -rate-limit 0 >"$TMP/server.log" 2>&1 &
SRV_PID=$!
for _ in $(seq 1 40); do
  curl -fsS "$BASE/api/health" >/dev/null 2>&1 && break
  sleep 0.25
done
curl -fsS "$BASE/api/health" >/dev/null 2>&1 || { fail "健康检查"; cat "$TMP/server.log"; exit 1; }
pass "健康检查"

# ---------- 管理员建本、备药、备计划 ----------
REG=$(curl -sS -X POST "$BASE/api/auth/register" -H 'Content-Type: application/json' -d '{"username":"perm_admin","password":"smoke-Perm1"}')
AUTH="Authorization: Bearer $(printf '%s' "$REG" | json_get "d['data']['token']")"
BOOK=$(curl -sS -X POST "$BASE/api/medcare/books" -H "$AUTH" -H 'Content-Type: application/json' -d '{"name":"李奶奶的用药本","care_recipient_name":"李奶奶","timezone":"Asia/Shanghai"}')
BOOK_ID=$(printf '%s' "$BOOK" | json_get "d['data']['id']")
[ -n "$BOOK_ID" ] || { fail "创建用药本"; exit 1; }
pass "创建用药本"

MED=$(curl -sS -X POST "$BASE/api/medcare/books/$BOOK_ID/medicines" -H "$AUTH" -H 'Content-Type: application/json' -d '{"name":"苯磺酸氨氯地平片","category":"oral","unit":"片","spec":"5mg×7片"}')
MED_ID=$(printf '%s' "$MED" | json_get "d['data']['id']")
curl -sS -X POST "$BASE/api/medcare/books/$BOOK_ID/medicines/$MED_ID/stock-in" -H "$AUTH" -H 'Content-Type: application/json' -d '{"packages":10,"units_per_package":20,"expiry_date":"2028-06-30"}' >/dev/null
[ -n "$MED_ID" ] && pass "备药与入库" || fail "备药"

# 四个时间点 = 四个待完成事件，供后面逐格验证「点完成」
for T in 08:00 09:00 10:00 11:00; do
  curl -sS -X POST "$BASE/api/medcare/books/$BOOK_ID/plans" -H "$AUTH" -H 'Content-Type: application/json' \
    --data-binary "{\"medicine_id\":$MED_ID,\"title\":\"计划 $T\",\"type\":\"oral\",\"dose_amount\":1,\"dose_unit\":\"片\",\"frequency\":{\"type\":\"daily\",\"times\":[\"$T\"]},\"start_date\":\"$(date +%F)\",\"timezone\":\"Asia/Shanghai\"}" >/dev/null
done
EVENTS=$(curl -sS "$BASE/api/medcare/books/$BOOK_ID/dose-events?pageSize=20" -H "$AUTH" | python3 -c "
import json,sys
items = json.load(sys.stdin)['data']['items']
print(' '.join(str(e['id']) for e in items if e['status'] == 'pending'))")
set -- $EVENTS
E1=${1:-}; E2=${2:-}; E3=${3:-}; E4=${4:-}
[ -n "$E4" ] && pass "生成 4 个待完成事件" || { fail "生成事件（得到 $EVENTS）"; exit 1; }

# ---------- 能力矩阵下发 ----------
CAT=$(curl -sS "$BASE/api/medcare/books/$BOOK_ID/permissions" -H "$AUTH")
expect_eq "7" "$(printf '%s' "$CAT" | json_get "len(d['data']['modules'])")" "矩阵模块数"
expect_eq "22" "$(printf '%s' "$CAT" | json_get "len(d['data']['keys'])")" "能力格总数"
OWNER_PERMS=$(printf '%s' "$BOOK" | json_get "','.join(sorted(d['data']['permissions']))")
expect_has "$OWNER_PERMS" "medicine:delete" "管理员默认全勾"
expect_has "$OWNER_PERMS" "image:view" "管理员默认可看图片"
expect_eq "22" "$(count_of "$OWNER_PERMS")" "管理员下发格数"

# ---------- 成员加入：默认能看、能催，不能改 ----------
INV=$(curl -sS -X POST "$BASE/api/medcare/books/$BOOK_ID/invites" -H "$AUTH" -H 'Content-Type: application/json' -d '{"role":"member","expires_hours":168,"max_uses":5}')
INVT=$(printf '%s' "$INV" | json_get "d['data']['token']")
REG2=$(curl -sS -X POST "$BASE/api/auth/register" -H 'Content-Type: application/json' -d '{"username":"perm_member","password":"smoke-Perm1"}')
M2=$(printf '%s' "$REG2" | json_get "d['data']['token']")
curl -sS -X POST "$BASE/api/medcare/invites/$INVT/join" -H "Authorization: Bearer $M2" >/dev/null
MEMBER_ID=$(member_id perm_member)
[ -n "$MEMBER_ID" ] && pass "成员加入用药本" || { fail "成员加入"; exit 1; }
MAUTH="Authorization: Bearer $M2"
MSET=$(member_perms perm_member)
expect_has "$MSET" "today:view" "成员默认可看今日"
expect_has "$MSET" "medicine:view" "成员默认可看药品"
expect_has "$MSET" "stats:view" "成员默认可看统计"
expect_lacks "$MSET" "today:record" "成员默认不能点完成"
expect_lacks "$MSET" "medicine:create" "成员默认不能新增药品"
expect_eq "False" "$(is_custom perm_member)" "未单独勾选时跟随角色"
expect_code 200 "成员能读今日安排" "$BASE/api/medcare/books/$BOOK_ID/dose-events?pageSize=5" -H "$MAUTH"
expect_code 200 "成员能读药品列表" "$BASE/api/medcare/books/$BOOK_ID/medicines" -H "$MAUTH"
expect_code 200 "成员能点提醒用药" -X POST "$BASE/api/medcare/books/$BOOK_ID/dose-events/$E1/remind" -H "$MAUTH"
expect_code 403 "缺「用药记录·点完成」打卡被拒" -X POST "$BASE/api/medcare/books/$BOOK_ID/dose-events/$E1/complete" -H "$MAUTH" -H 'Content-Type: application/json' -d '{"deduct_stock":true}'
expect_code 403 "缺「药品与库存·新增」建药被拒" -X POST "$BASE/api/medcare/books/$BOOK_ID/medicines" -H "$MAUTH" -H 'Content-Type: application/json' -d '{"name":"布洛芬","category":"oral","unit":"片"}'
expect_code 403 "缺「药品与库存·修改」改药被拒" -X PUT "$BASE/api/medcare/books/$BOOK_ID/medicines/$MED_ID" -H "$MAUTH" -H 'Content-Type: application/json' -d '{"notes":"越权改备注"}'
expect_code 403 "缺「药品与库存·删除」归档被拒" -X DELETE "$BASE/api/medcare/books/$BOOK_ID/medicines/$MED_ID" -H "$MAUTH"
expect_code 403 "缺「用药计划·新建」建计划被拒" -X POST "$BASE/api/medcare/books/$BOOK_ID/plans" -H "$MAUTH" -H 'Content-Type: application/json' -d "{\"medicine_id\":$MED_ID,\"title\":\"越权\",\"type\":\"oral\",\"dose_amount\":1,\"dose_unit\":\"片\",\"frequency\":{\"type\":\"daily\",\"times\":[\"12:00\"]},\"start_date\":\"$(date +%F)\"}"
expect_code 403 "缺「图片资料·上传」传图被拒" -X POST "$BASE/api/medcare/books/$BOOK_ID/images" -H "$MAUTH" -H 'Content-Type: application/json' -d '{"title":"越权图片"}'
expect_code 403 "成员不能改他人权限" -X PUT "$BASE/api/medcare/books/$BOOK_ID/members/$MEMBER_ID/permissions" -H "$MAUTH" -H 'Content-Type: application/json' -d '{"permissions":["medicine:create"]}'

# ---------- 逐格开通：查看+新增+点完成，不给修改与删除 ----------
set_perms "$MEMBER_ID" '{"permissions":["today:view","today:remind","today:record","medicine:view","medicine:create"]}'
MSET=$(member_perms perm_member)
expect_eq "5" "$(count_of "$MSET")" "逐格勾选按提交的格数生效"
expect_has "$MSET" "medicine:create" "已勾药品与库存·新增"
expect_lacks "$MSET" "medicine:edit" "未勾药品与库存·修改"
expect_eq "True" "$(is_custom perm_member)" "单独勾选后标记为已单独设置"
expect_code 200 "开通后能建药品" -X POST "$BASE/api/medcare/books/$BOOK_ID/medicines" -H "$MAUTH" -H 'Content-Type: application/json' -d '{"name":"布洛芬","category":"oral","unit":"片"}'
expect_code 200 "开通后能点完成" -X POST "$BASE/api/medcare/books/$BOOK_ID/dose-events/$E2/complete" -H "$MAUTH" -H 'Content-Type: application/json' -d '{"deduct_stock":true}'
expect_code 403 "只有新增没有修改：改药仍被拒" -X PUT "$BASE/api/medcare/books/$BOOK_ID/medicines/$MED_ID" -H "$MAUTH" -H 'Content-Type: application/json' -d '{"notes":"越权改备注"}'
expect_code 403 "只有新增没有删除：归档仍被拒" -X DELETE "$BASE/api/medcare/books/$BOOK_ID/medicines/$MED_ID" -H "$MAUTH"
expect_code 403 "没勾计划任何一格：建计划仍被拒" -X POST "$BASE/api/medcare/books/$BOOK_ID/plans" -H "$MAUTH" -H 'Content-Type: application/json' -d "{\"medicine_id\":$MED_ID,\"title\":\"越权\",\"type\":\"oral\",\"dose_amount\":1,\"dose_unit\":\"片\",\"frequency\":{\"type\":\"daily\",\"times\":[\"13:00\"]},\"start_date\":\"$(date +%F)\"}"

# 幂等：换顺序提交同一集合
set_perms "$MEMBER_ID" '{"permissions":["medicine:create","today:record","medicine:view","today:view","today:remind"]}'
expect_eq "$MSET" "$(member_perms perm_member)" "重复提交同一勾选集合结果不变"

# ---------- 查看也是一格：取消后连列表都看不到 ----------
set_perms "$MEMBER_ID" '{"permissions":["today:remind"]}'
expect_code 403 "取消「用药记录·查看」后读不到今日" "$BASE/api/medcare/books/$BOOK_ID/dose-events?pageSize=5" -H "$MAUTH"
expect_code 403 "取消「药品与库存·查看」后读不到药品" "$BASE/api/medcare/books/$BOOK_ID/medicines" -H "$MAUTH"
expect_code 403 "取消「统计与日志·查看」后读不到统计" "$BASE/api/medcare/books/$BOOK_ID/stats/overview" -H "$MAUTH"
expect_code 403 "取消「成员与邀请·查看」后读不到成员" "$BASE/api/medcare/books/$BOOK_ID/members" -H "$MAUTH" || true
expect_code 200 "只留「点提醒」仍能催药" -X POST "$BASE/api/medcare/books/$BOOK_ID/dose-events/$E3/remind" -H "$MAUTH"

# ---------- 邀请也是一格 ----------
expect_code 403 "取消「成员与邀请·邀请」后不能发邀请" -X POST "$BASE/api/medcare/books/$BOOK_ID/invites" -H "$MAUTH" -H 'Content-Type: application/json' -d '{"role":"member","expires_hours":24,"max_uses":1}'
set_perms "$MEMBER_ID" '{"permissions":["member:view","member:invite"]}'
expect_code 200 "勾上邀请后能生成邀请" -X POST "$BASE/api/medcare/books/$BOOK_ID/invites" -H "$MAUTH" -H 'Content-Type: application/json' -d '{"role":"member","expires_hours":24,"max_uses":1}'

# ---------- 恢复跟随角色 / 改角色重置 ----------
set_perms "$MEMBER_ID" '{"permissions":null}'
expect_has "$(member_perms perm_member)" "today:view" "传 null 后恢复跟随角色"
expect_lacks "$(member_perms perm_member)" "today:record" "恢复后成员仍不能点完成"
set_perms "$MEMBER_ID" '{"permissions":["today:view"]}'
curl -sS -X PUT "$BASE/api/medcare/books/$BOOK_ID/members/$MEMBER_ID" -H "$AUTH" -H 'Content-Type: application/json' -d '{"role":"manager"}' >/dev/null
MSET=$(member_perms perm_member)
expect_eq "22" "$(count_of "$MSET")" "升为监护人后按新角色全勾"
expect_eq "False" "$(is_custom perm_member)" "改角色会清空单独勾选"

# ---------- 监护人也能被削减（只给查看与删除，不给新增） ----------
SCRATCH=$(curl -sS -X POST "$BASE/api/medcare/books/$BOOK_ID/medicines" -H "$AUTH" -H 'Content-Type: application/json' -d '{"name":"待归档药品","category":"oral","unit":"片"}')
SCRATCH_ID=$(printf '%s' "$SCRATCH" | json_get "d['data']['id']")
set_perms "$MEMBER_ID" '{"permissions":["medicine:view","medicine:delete","today:view"]}'
expect_code 200 "勾了删除就能归档药品" -X DELETE "$BASE/api/medcare/books/$BOOK_ID/medicines/$SCRATCH_ID" -H "$MAUTH"
expect_code 403 "没勾新增：再建药品被拒" -X POST "$BASE/api/medcare/books/$BOOK_ID/medicines" -H "$MAUTH" -H 'Content-Type: application/json' -d '{"name":"钙片","category":"oral","unit":"片"}'

# ---------- 旧版粗粒度键自动展开（不让升级把人变成只读） ----------
set_perms "$MEMBER_ID" '{"permissions":["medicine","remind"]}'
MSET=$(member_perms perm_member)
expect_has "$MSET" "medicine:delete" "旧键 medicine 展开为四格"
expect_has "$MSET" "today:remind" "旧键 remind 展开为点提醒"
expect_has "$MSET" "today:view" "旧键展开时补上默认可见"

# ---------- 管理员不可削减 / 非法输入 ----------
OWNER_ID=$(member_id perm_admin)
expect_code 400 "管理员自身权限不可设置" -X PUT "$BASE/api/medcare/books/$BOOK_ID/members/$OWNER_ID/permissions" -H "$AUTH" -H 'Content-Type: application/json' -d '{"permissions":["today:view"]}'
expect_code 400 "未知能力键被拒" -X PUT "$BASE/api/medcare/books/$BOOK_ID/members/$MEMBER_ID/permissions" -H "$AUTH" -H 'Content-Type: application/json' -d '{"permissions":["medicine:fly"]}'
expect_code 400 "缺少 permissions 字段被拒" -X PUT "$BASE/api/medcare/books/$BOOK_ID/members/$MEMBER_ID/permissions" -H "$AUTH" -H 'Content-Type: application/json' -d '{}'

# ---------- 用药人：默认能记录自己，可被收回 ----------
INVR=$(curl -sS -X POST "$BASE/api/medcare/books/$BOOK_ID/invites" -H "$AUTH" -H 'Content-Type: application/json' -d '{"role":"member","expires_hours":168,"max_uses":5,"as_care_recipient":true}')
INVRT=$(printf '%s' "$INVR" | json_get "d['data']['token']")
REG3=$(curl -sS -X POST "$BASE/api/auth/register" -H 'Content-Type: application/json' -d '{"username":"perm_elder","password":"smoke-Perm1"}')
M3=$(printf '%s' "$REG3" | json_get "d['data']['token']")
curl -sS -X POST "$BASE/api/medcare/invites/$INVRT/join" -H "Authorization: Bearer $M3" >/dev/null
EAUTH="Authorization: Bearer $M3"
ESET=$(member_perms perm_elder)
expect_has "$ESET" "today:record" "用药人默认可记录自己的用药"
expect_lacks "$ESET" "image:create" "用药人默认不能传图"
expect_code 200 "用药人能给今天的自己打卡" -X POST "$BASE/api/medcare/books/$BOOK_ID/dose-events/$E4/complete" -H "$EAUTH" -H 'Content-Type: application/json' -d '{"deduct_stock":true}'
set_perms "$(member_id perm_elder)" '{"permissions":["today:view","today:remind"]}'
BOOK3=$(curl -sS "$BASE/api/medcare/books" -H "$EAUTH")
P3=$(printf '%s' "$BOOK3" | json_get "','.join(sorted(d['data'][0]['permissions']))")
expect_lacks "$P3" "today:record" "收回后用药本人不能打卡"
expect_code 403 "收回点完成后打卡被拒" -X POST "$BASE/api/medcare/books/$BOOK_ID/dose-events/$E1/complete" -H "$EAUTH" -H 'Content-Type: application/json' -d '{"deduct_stock":true}'

# ---------- 非成员一律 404 ----------
REG4=$(curl -sS -X POST "$BASE/api/auth/register" -H 'Content-Type: application/json' -d '{"username":"perm_outsider","password":"smoke-Perm1"}')
OAUTH="Authorization: Bearer $(printf '%s' "$REG4" | json_get "d['data']['token']")"
expect_code 404 "非成员看不到用药本" "$BASE/api/medcare/books/$BOOK_ID/permissions" -H "$OAUTH"

say ""
if [ "$FAILS" -eq 0 ]; then say "全部通过"; exit 0; else say "失败 $FAILS 项"; exit 1; fi
