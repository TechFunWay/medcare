<template>
  <div class="page">
    <div class="page-head">
      <div style="display:flex;gap:8px;align-items:flex-start">
        <button class="icon-btn" aria-label="返回" @click="router.back()"><AppIcon name="arrowLeft" /></button>
        <div><div class="eyebrow">审计</div><h1>操作日志</h1><p>谁在什么时候做了什么，都记在这里。</p></div>
      </div>
    </div>

    <FilterPanel v-model:open="filtersOpen" :count="activeFilterCount" @search="load(true)" @reset="resetFilters">
      <div class="chip-row">
        <button v-for="opt in methodFilters" :key="opt.value" class="chip" :class="{ active: method === opt.value }" @click="setMethod(opt.value)">{{ opt.label }}</button>
      </div>
      <div class="field" style="max-width:260px;margin-bottom:0">
        <label>操作人</label>
        <SheetSelect v-model.number="actorId" placeholder="全部成员" :options="actorOptions" />
      </div>
    </FilterPanel>

    <div v-if="loading && !items.length" class="card muted">加载中…</div>
    <div v-else-if="!items.length" class="empty card">
      <div class="empty-icon"><AppIcon name="clock" :size="30" /></div>
      <h3>还没有操作记录</h3>
      <p>新增、修改、入库、打卡等操作都会记录在这里。</p>
    </div>
    <div v-else class="list">
      <div v-for="log in items" :key="log.id" class="card log-card">
        <div class="log-main">
          <div class="log-title"><b>{{ log.action }}</b><span class="badge" :class="methodBadge(log.method)">{{ methodLabel(log.method) }}</span></div>
          <div class="tiny muted log-meta">
            <span>{{ log.actor_name || '未知成员' }}</span>
            <span class="dot">·</span><span>{{ targetLabel(log.target_type) }}</span>
            <span class="dot">·</span><span>{{ formatDateTime(log.created_at) }}</span>
            <template v-if="log.ip"><span class="dot">·</span><span>{{ log.ip }}</span></template>
          </div>
          <div v-if="log.detail" class="tiny muted log-detail">{{ prettyDetail(log.detail) }}</div>
        </div>
      </div>
    </div>
    <button v-if="hasMore" class="btn btn-outline btn-block" style="margin-top:10px" :disabled="loading" @click="load(false)">{{ loading ? '加载中…' : '加载更多' }}</button>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import AppIcon from '../components/AppIcon.vue'
import SheetSelect from '../components/SheetSelect.vue'
import FilterPanel from '../components/FilterPanel.vue'
import { bookApi, operationApi, type OperationLogItem } from '../api/medcare'
import { formatDateTime } from '../utils/date'
import { useBookStore } from '../stores/book'
import { useUIStore } from '../stores/ui'
import { reportError } from '../utils/errors'

const router = useRouter()
const book = useBookStore()
const ui = useUIStore()
const items = ref<OperationLogItem[]>([])
const loading = ref(false)
const page = ref(1)
const hasMore = ref(false)
const method = ref('')
const filtersOpen = ref(false)
const actorId = ref(0)
// 折叠态只显示一个「筛选」按钮；条数统计动作类型与操作人。
const activeFilterCount = computed(() => (method.value ? 1 : 0) + (actorId.value ? 1 : 0))
const members = ref<any[]>([])
const methodFilters = [
  { value: '', label: '全部' }, { value: 'POST', label: '新增' }, { value: 'PUT', label: '修改' }, { value: 'DELETE', label: '删除' },
]
const actorOptions = computed(() => [{ value: 0, label: '全部成员' }, ...members.value.map((m) => ({ value: m.user_id, label: m.username + (m.is_self ? '（我）' : '') }))])
function methodLabel(m: string) { return m === 'POST' ? '新增' : m === 'PUT' || m === 'PATCH' ? '修改' : m === 'DELETE' ? '删除' : '操作' }
function methodBadge(m: string) { return m === 'DELETE' ? 'missed' : m === 'POST' ? 'done' : 'warn' }
function targetLabel(t: string) {
  const map: Record<string, string> = { book: '用药本', medicine: '药品', plan: '用药计划', dose_event: '用药记录', location: '地点', member: '成员', invite: '邀请', prescription: '处方', stock: '库存' }
  return map[t] || t || '其他'
}
const detailLabels: Record<string, string> = {
  name: '名称', title: '标题', generic_name: '通用名', quantity: '数量', unit: '单位', spec: '规格',
  category: '分类', form: '剂型', channel: '渠道', medicine_id: '药品', medicine_name: '药品',
  dose_amount: '剂量', dose_unit: '剂量单位', frequency: '频率', interval_days: '间隔天数',
  interval_hours: '间隔小时', duration_days: '疗程天数', start_date: '开始日期', end_date: '结束日期',
  times: '时间', weekdays: '星期', version: '版本', expiry_warn_days: '临期天数', low_stock_threshold: '低库存阈值',
  role: '角色', user_id: '成员', note: '备注', notes: '备注', reason: '原因', target_quantity: '盘点数量',
  new_quantity: '调整后数量', batch_id: '批次', batch_no: '批号', expiry_date: '有效期', as_care_recipient: '设为用药人',
}
function prettyDetail(d: string) {
  if (!d) return ''
  if (d.startsWith('[含敏感字段')) return d
  try {
    const obj = JSON.parse(d)
    if (obj && typeof obj === 'object' && !Array.isArray(obj)) {
      const parts = Object.entries(obj)
        .filter(([, v]) => v !== '' && v !== null && v !== undefined)
        .map(([k, v]) => (detailLabels[k] || k) + '：' + (typeof v === 'object' ? JSON.stringify(v) : String(v)))
      if (parts.length) return parts.join(' · ').slice(0, 220)
      return ''
    }
  } catch { /* 非 JSON 直接原样显示 */ }
  return d.length > 160 ? d.slice(0, 160) + '…' : d
}
async function load(reset = true) {
  if (!book.currentId) return
  loading.value = true
  const target = reset ? 1 : page.value + 1
  try {
    const res = await operationApi.logs(book.currentId, { page: target, pageSize: 20, method: method.value, actor_user_id: actorId.value || '' })
    const list = res.data?.data?.items || []
    items.value = reset ? list : items.value.concat(list)
    page.value = target
    hasMore.value = items.value.length < (res.data?.data?.total || 0)
  } catch (e: any) { ui.showToast(e?.response?.data?.message || '读取操作日志失败', 'error') }
  finally { loading.value = false }
}
function setMethod(v: string) { method.value = v }
function resetFilters() { method.value = ''; actorId.value = 0; load(true) }
async function loadMembers() {
  // 按操作人筛选要读成员列表，没有「成员与邀请 · 查看」就不请求，日志本身照样能看。
  if (!book.currentId || !book.canMemberView) { members.value = []; return }
  try { const r = await bookApi.members(book.currentId); members.value = r.data?.data || [] } catch (e) { reportError(e, '读取成员列表失败') }
}
onMounted(async () => { await Promise.all([loadMembers(), load(true)]) })
watch(actorId, () => load(true))
watch(() => book.currentId, () => { loadMembers(); load(true) })
</script>

<style scoped>
.log-card { padding: 11px 12px; }
.log-main { min-width: 0; }
.log-title { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.log-title b { font-size: 14px; overflow-wrap: anywhere; }
.log-meta { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 3px; }
.log-meta .dot { color: var(--mc-border); }
.log-detail { margin-top: 4px; overflow-wrap: anywhere; }
@media (min-width: 900px) {
  .log-card { padding: 13px 15px; }
  .log-title b { font-size: 15px; }
}
</style>
