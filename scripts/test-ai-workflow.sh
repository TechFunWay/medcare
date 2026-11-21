#!/usr/bin/env bash
# test-ai-workflow.sh — 校验真实 AI 每日巡检工作流配置
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FILE="$ROOT/.gitea/workflows/ai-e2e.yml"
fail=0
ok() { echo "PASS $1"; }
bad() { echo "FAIL $1 :: $2"; fail=1; }

[ -f "$FILE" ] && ok "TC-147a 工作流文件存在" || bad "TC-147a 工作流文件存在" "缺少 $FILE"
grep -q "cron: '0 20 \* \* \*'" "$FILE" && ok "TC-147b 每天定时触发" || bad "TC-147b 每天定时触发" "未找到 cron"
grep -q "workflow_dispatch" "$FILE" && ok "TC-147c 支持手动触发" || bad "TC-147c 支持手动触发" "未找到 workflow_dispatch"
for key in E2E_AI_BASE_URL E2E_AI_API_KEY E2E_AI_MODEL; do
  grep -q "$key" "$FILE" && ok "TC-147d 使用 $key" || bad "TC-147d 使用 $key" "未找到"
done
grep -q "e2e-test.sh" "$FILE" && ok "TC-147e 调用端到端脚本" || bad "TC-147e 调用端到端脚本" "未找到脚本调用"
if command -v ruby >/dev/null 2>&1; then
  ruby -e "require 'yaml'; YAML.load_file('$FILE')" >/dev/null && ok "TC-147f YAML 语法正确" || bad "TC-147f YAML 语法正确" "解析失败"
else
  echo "SKIP TC-147f YAML 语法正确 :: 未安装 ruby"
fi

exit $fail
