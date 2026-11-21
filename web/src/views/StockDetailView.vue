<template>
  <div class="page">
    <div class="page-head">
      <div style="display:flex;gap:8px;align-items:flex-start">
        <button class="icon-btn" aria-label="返回" @click="router.back()"><AppIcon name="arrowLeft" /></button>
        <div><div class="eyebrow">库存</div><h1>出入库明细</h1><p>按药品、类型、来源、时间与关键词筛选；完成后自动出库的记录会带计划时间与实际完成时间。</p></div>
      </div>
    </div>

    <FilterPanel v-model:open="filtersOpen" :count="activeFilterCount" @search="load(true)" @reset="resetFilters">
      <template #search>
        <div class="card search-card">
          <AppIcon name="search" :size="19" class="search-icon" />
          <input v-model="keyword" class="input search-input" placeholder="搜索药品名，如：氯化钠" @keyup.enter="load(true)" />
          <button v-if="keyword" class="search-clear" aria-label="清空" @click="keyword = ''"><AppIcon name="close" :size="16" /></button>
        </div>
      </template>
      <div class="field-row">
        <div class="field"><label>开始日期</label><DateField v-model="from" placeholder="不限" clearable /></div>
        <div class="field"><label>结束日期</label><DateField v-model="to" placeholder="不限" clearable /></div>
      </div>
      <div class="field-row">
        <div class="field"><label>来源</label><SheetSelect v-model="channel" placeholder="全部来源" :options="channelFilterOptions" /></div>
        <div class="field"><label>排序</label><SheetSelect v-model="sort" placeholder="排序" :options="sortOptions" /></div>
      </div>
      <div class="chip-row">
        <button v-for="opt in typeFilters" :key="opt.value" class="chip" :class="{ active: type === opt.value }" @click="setType(opt.value)">{{ opt.label }}</button>
      </div>
    </FilterPanel>

    <!-- 出入库条数与数量合计（点击按类型筛选） -->
    <div class="mv-stats">
      <button type="button" class="mv-stat" :class="{ active: type === '' }" @click="pickType('')">
        <div class="mv-num">{{ stats.total.count }}</div>
        <div class="mv-label">全部条数</div>
        <div class="mv-sum">合计 {{ fmtNum(stats.total.sum) }}</div>
      </button>
      <button type="button" class="mv-stat in" :class="{ active: type === 'in' }" @click="pickType('in')">
        <div class="mv-num">{{ stats.in.count }}</div>
        <div class="mv-label">入库条数</div>
        <div class="mv-sum">入库 {{ fmtNum(stats.in.sum) }}</div>
      </button>
      <button type="button" class="mv-stat out" :class="{ active: type === 'out' }" @click="pickType('out')">
        <div class="mv-num">{{ stats.out.count }}</div>
        <div class="mv-label">出库条数</div>
        <div class="mv-sum">出库 {{ fmtNum(stats.out.sum) }}</div>
      </button>
      <button v-if="stats.adjust.count" type="button" class="mv-stat" :class="{ active: type === 'adjust' }" @click="pickType('adjust')">
        <div class="mv-num">{{ stats.adjust.count }}</div>
        <div class="mv-label">调整条数</div>
        <div class="mv-sum">调整 {{ fmtNum(stats.adjust.sum) }}</div>
      </button>
    </div>

    <div class="list" style="margin-top:10px">
      <div class="list-head">
        <span class="tiny muted">共 {{ total }} 条</span>
        <span v-if="loading" class="tiny muted">加载中…</span>
      </div>
      <div v-if="!loading && !items.length" class="card muted">没有符合条件的出入库记录。</div>
      <article v-for="x in items" :key="x.id" class="card detail-row">
        <div class="dr-main">
          <div class="dr-title">
            <b>{{ x.medicine_name }}</b>
            <span class="badge" :class="movementBadge(x.type)">{{ movementTypeLabel(x.type) }}</span>
            <span class="badge pending">{{ entrySourceLabel(x.entry_source) }}</span>
          </div>
          <div class="tiny muted dr-meta">
            <span>操作时间 {{ formatFull(x.created_at) }}</span>
            <template v-if="x.event_local_time"><span class="dot">·</span><span>计划 {{ x.event_local_date }} {{ x.event_local_time }}</span></template>
            <template v-if="x.event_completed_at"><span class="dot">·</span><span>实际完成 {{ formatFull(x.event_completed_at) }}</span></template>
          </div>
          <div class="tiny muted dr-meta">
            <template v-if="x.reason"><span>{{ x.reason }}</span></template>
            <template v-if="x.channel"><span class="dot">·</span><span>{{ channelLabel(x.channel) }}</span></template>
            <template v-if="x.actor_name"><span class="dot">·</span><span>{{ x.actor_name }}</span></template>
          </div>
        </div>
        <div class="dr-right">
          <b class="dr-delta" :style="{ color: x.quantity_delta >= 0 ? 'var(--mc-primary-strong)' : 'var(--mc-danger)' }">{{ x.quantity_delta > 0 ? '+' : '' }}{{ x.quantity_delta }}</b>
          <span class="tiny muted">余 {{ x.quantity_after }} {{ x.unit }}</span>
          <button v-if="canEditMove(x)" type="button" class="btn btn-ghost btn-xs" @click="openEditBatch((x as any).batch_id)"><AppIcon name="edit" :size="14" />修改</button>
        </div>
      </article>
    </div>
    <button v-if="hasMore" class="btn btn-outline btn-block" style="margin-top:10px" :disabled="loading" @click="load(false)">{{ loading ? '加载中…' : '加载更多' }}</button>

    <StockInEditSheet :open="showEditBatch" :batch="editBatch" :unit="editUnit" @close="showEditBatch=false" @saved="onBatchSaved" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import AppIcon from '../components/AppIcon.vue'
import DateField from '../components/DateField.vue'
import FilterPanel from '../components/FilterPanel.vue'
import SheetSelect from '../components/SheetSelect.vue'
import { medicineApi, stockApi, type StockMovementItem } from '../api/medcare'
import StockInEditSheet from '../components/StockInEditSheet.vue'
import { STOCK_CHANNEL_OPTIONS, channelLabel, entrySourceLabel, movementBadge, movementTypeLabel } from '../utils/stock'
import { useBookStore } from '../stores/book'
import { useUIStore } from '../stores/ui'

const router = useRouter()
const book = useBookStore()
const ui = useUIStore()

const showEditBatch = ref(false)
const editBatch = ref<any>(null)
const editUnit = ref('')
// 入库记录 7 天内可改（后端也会按入库流水创建时间再校验一次）。
function within7Days(s?: string) { return !!s && Date.now() - new Date(s).getTime() <= 7 * 24 * 3600 * 1000 }
function canEditMove(x: any) { return !!book.canMedicineEdit && x.type === 'in' && !!x.batch_id && within7Days(x.created_at) }
async function openEditBatch(batchId: number) {
  if (!batchId || !book.currentId) return
  try {
    const res = await medicineApi.getBatch(book.currentId, batchId)
    editBatch.value = res.data?.data
    editUnit.value = (items.value.find((it: any) => it.batch_id === batchId) as any)?.unit || ''
    showEditBatch.value = true
  } catch (e: any) { ui.showToast(e?.response?.data?.message || '读取入库记录失败', 'error') }
}
async function onBatchSaved() { showEditBatch.value = false; await load(true) }

const keyword = ref('')
const from = ref('')
const to = ref('')
const type = ref('')
const channel = ref('')
const sort = ref('time_desc')

const items = ref<StockMovementItem[]>([])
const total = ref(0)
// 顶部统计：按类型统计条数与数量合计（条件与列表一致）
const stats = reactive({ total: { count: 0, sum: 0 }, in: { count: 0, sum: 0 }, out: { count: 0, sum: 0 }, adjust: { count: 0, sum: 0 } })
const page = ref(1)
const hasMore = ref(false)
const loading = ref(false)

const typeFilters = [
  { value: '', label: '全部' }, { value: 'in', label: '入库' }, { value: 'out', label: '出库' },
  { value: 'return', label: '退回' }, { value: 'adjust', label: '调整' }, { value: 'waste', label: '丢弃' },
]
const channelFilterOptions = [{ value: '', label: '全部来源' }, ...STOCK_CHANNEL_OPTIONS]
const sortOptions = [
  { value: 'time_desc', label: '时间：新 → 旧' },
  { value: 'time_asc', label: '时间：旧 → 新' },
  { value: 'medicine', label: '按药品排序' },
]
const hasFilter = computed(() => !!(keyword.value || from.value || to.value || type.value || channel.value))
const filtersOpen = ref(false)
// 折叠态只显示搜索；这里的条数只统计「点筛选才展开」的条件，不含关键词。
const activeFilterCount = computed(() => [from.value, to.value, type.value, channel.value].filter(Boolean).length)

// 日期按北京时间换算成 UTC 区间，与后端存储的 UTC 时间对齐。
function isoStart(d: string) { return new Date(d + 'T00:00:00+08:00').toISOString() }
function isoEnd(d: string) { return new Date(d + 'T23:59:59+08:00').toISOString() }

function fmtNum(v: number) {
  const n = Number(v) || 0
  return Number.isInteger(n) ? String(n) : String(Math.round(n * 100) / 100)
}

async function loadStats() {
  if (!book.currentId) return
  const params: any = { type: type.value, channel: channel.value, keyword: keyword.value.trim() }
  if (from.value) params.from = isoStart(from.value)
  if (to.value) params.to = isoEnd(to.value)
  try {
    const res = await stockApi.movementsStats(book.currentId, params)
    if (res.data?.code === 0) Object.assign(stats, res.data.data)
  } catch (e) { /* 统计失败不影响列表 */ }
}

async function load(reset = true) {
  if (!book.currentId) return
  loading.value = true
  const target = reset ? 1 : page.value + 1
  try {
    const params: any = { page: target, pageSize: 20, type: type.value, channel: channel.value, sort: sort.value, keyword: keyword.value.trim() }
    if (from.value) params.from = isoStart(from.value)
    if (to.value) params.to = isoEnd(to.value)
    const res = await stockApi.movements(book.currentId, params)
    const list = (res.data?.data?.items || []) as StockMovementItem[]
    items.value = reset ? list : items.value.concat(list)
    page.value = target
    total.value = Number(res.data?.data?.total) || 0
    hasMore.value = items.value.length < total.value
    void loadStats()
  } catch (e: any) {
    ui.showToast(e?.response?.data?.message || '读取出入库明细失败', 'error')
  } finally { loading.value = false }
}

// 条件改动不自动查询，统一由「查询」按钮或回车触发。
function setType(v: string) { type.value = v }
// 顶部统计卡点击即筛选（不用再点「查询」）。
function pickType(v: string) { type.value = type.value === v ? '' : v; load(true) }
function resetFilters() { keyword.value = ''; from.value = ''; to.value = ''; type.value = ''; channel.value = ''; sort.value = 'time_desc'; load(true) }

function formatFull(v?: string) {
  if (!v) return ''
  const d = new Date(v)
  if (Number.isNaN(d.getTime())) return v
  return d.toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
}

onMounted(() => load(true))
watch(() => book.currentId, () => load(true))
</script>

<style scoped>
.mv-stats { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 8px; margin-top: 10px; }
@media (max-width: 560px) { .mv-stats { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
.mv-stat { text-align: left; padding: 10px 12px; border-radius: 12px; border: 1px solid var(--mc-border); background: var(--mc-surface); }
.mv-stat.active { border-color: var(--mc-primary); background: var(--mc-primary-soft); }
.mv-num { font-size: 19px; font-weight: 800; line-height: 1.15; color: var(--mc-text); }
.mv-label { font-size: 11.5px; color: var(--mc-text-muted); font-weight: 650; margin-top: 1px; }
.mv-sum { font-size: 11.5px; color: var(--mc-text-muted); margin-top: 3px; }
.mv-stat.in .mv-num { color: var(--mc-primary-strong); }
.mv-stat.out .mv-num { color: var(--mc-danger); }
.search-card { display: flex; align-items: center; gap: 8px; padding: 0 12px; }
.search-card:focus-within { border-color: var(--mc-primary); box-shadow: 0 0 0 3px var(--mc-primary-soft); }
.search-icon { flex: 0 0 auto; color: var(--mc-text-muted); }
.search-input, .search-input:focus { border: 0 !important; background: transparent !important; box-shadow: none !important; border-radius: 0; padding-left: 0; }
.search-clear { flex: 0 0 auto; border: 0; background: var(--mc-surface-2); color: var(--mc-text-muted); width: 28px; height: 28px; border-radius: 999px; display: grid; place-items: center; }
.list-head { display: flex; justify-content: space-between; align-items: center; padding: 0 2px 6px; }
.detail-row { display: flex; justify-content: space-between; gap: 10px; align-items: flex-start; padding: 10px 12px; }
.dr-main { min-width: 0; flex: 1 1 auto; }
.dr-title { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.dr-title b { font-size: 14px; overflow-wrap: anywhere; }
.dr-meta { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 3px; }
.dr-meta .dot { color: var(--mc-border); }
.dr-right { flex: 0 0 auto; text-align: right; display: flex; flex-direction: column; gap: 2px; }
.dr-delta { font-size: 15px; }
@media (min-width: 900px) {
  .detail-row { padding: 12px 15px; }
  .dr-title b { font-size: 15px; }
}
</style>
