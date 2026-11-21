<template>
  <div class="page">
    <div class="page-head">
      <div><div class="eyebrow">库存</div><h1>库存</h1><p>入库、出库与当前库存量都在这里；药品的基础信息在「药品」页维护。</p></div>
      <div class="head-actions">
        <button v-if="book.canMedicineCreate" class="btn btn-primary btn-sm" @click="openAction('in')"><AppIcon name="download" :size="16" />入库</button>
        <button v-if="book.canMedicineCreate" class="btn btn-outline btn-sm" @click="openAction('out')"><AppIcon name="refresh" :size="16" />出库</button>
        <button v-if="book.canMedicineEdit" class="btn btn-outline btn-sm" @click="openAction('stocktake')"><AppIcon name="check" :size="16" />盘点</button>
        <button class="btn btn-outline btn-sm" @click="router.push('/prescriptions?purpose=stock')"><AppIcon name="prescription" :size="16" />识别记录</button>
        <button class="btn btn-outline btn-sm" @click="router.push('/stock/detail')"><AppIcon name="clock" :size="16" />出入库明细</button>
      </div>
    </div>

    <div class="stat-grid">
      <button type="button" class="card stat" :class="{ active: statusFilter === '' }" @click="pickStatus('')"><div class="stat-num">{{ stats.total }}</div><div class="tiny muted">在管药品</div></button>
      <button type="button" class="card stat" :class="{ active: statusFilter === 'low' }" @click="pickStatus('low')"><div class="stat-num" :class="{ warn: stats.low > 0 }">{{ stats.low }}</div><div class="tiny muted">低库存</div></button>
      <button type="button" class="card stat" :class="{ active: statusFilter === 'out' }" @click="pickStatus('out')"><div class="stat-num" :class="{ danger: stats.out > 0 }">{{ stats.out }}</div><div class="tiny muted">已缺货</div></button>
      <button type="button" class="card stat" :class="{ active: statusFilter === 'expiring' }" @click="pickStatus('expiring')"><div class="stat-num" :class="{ warn: stats.expiring > 0 }">{{ stats.expiring }}</div><div class="tiny muted">临期/过期</div></button>
    </div>

    <!-- 按分类统计（与药品页一致，点击可筛选） -->
    <div class="cat-stats" style="margin-top:10px">
      <button type="button" class="cat-stat" :class="{ active: catFilter === '' }" @click="pickCategory('')">
        <b>{{ medicines.length }}</b><span>全部</span>
      </button>
      <button v-for="c in catCounts" :key="c.value" type="button" class="cat-stat" :class="{ active: catFilter === c.value }" @click="pickCategory(c.value)">
        <b>{{ c.count }}</b><span>{{ c.label }}</span>
      </button>
    </div>

    <button v-if="book.canMedicineCreate" type="button" class="scan-banner" style="margin-top:12px" @click="showScan = true">
      <div class="scan-banner-icon"><AppIcon name="camera" :size="20" /></div>
      <div class="scan-banner-body"><b>AI 识别缴费单入库</b><div class="tiny muted">拍缴费单 / 发票，识别药品与数量，核对后直接入库（建批次并记账）</div></div>
      <AppIcon name="chevron" :size="16" />
    </button>
    <FilterPanel v-model:open="filtersOpen" :count="activeFilterCount" style="margin-top:12px" @search="load(true)" @reset="resetFilters">
      <template #search>
        <div class="card search-card">
          <AppIcon name="search" :size="19" class="search-icon" />
          <input v-model="query" class="input search-input" placeholder="搜索药品名、通用名、规格" />
        </div>
      </template>
      <div class="filter-group">
        <div class="filter-label">排序</div>
        <div class="chip-row">
          <button v-for="opt in sortOptions" :key="opt.value" class="chip" :class="{ active: sort === opt.value }" @click="sort = opt.value">{{ opt.label }}</button>
        </div>
      </div>
      <div class="chip-row">
        <button v-for="opt in typeFilters" :key="opt.value" class="chip" :class="{ active: type === opt.value }" @click="setType(opt.value)">{{ opt.label }}</button>
      </div>
      <div class="chip-row">
        <button v-for="opt in channelFilters" :key="opt.value" class="chip" :class="{ active: channel === opt.value }" @click="setChannel(opt.value)">{{ opt.label }}</button>
      </div>
    </FilterPanel>

    <!-- 排序常驻可见，不用展开筛选也能改。 -->
    <div class="sort-bar">
      <div class="sort-bar-left"><AppIcon name="filter" :size="15" /><span class="tiny muted">共 {{ filtered.length }} 种 · 排序</span></div>
      <div class="sort-select"><SheetSelect v-model="sort" placeholder="排序" :options="sortOptions" /></div>
    </div>

    <div v-if="loading && !medicines.length" class="card muted">加载中…</div>
    <div v-else-if="!filtered.length" class="empty card">
      <div class="empty-icon"><AppIcon name="filter" :size="28" /></div>
      <h3>{{ medicines.length ? '没有匹配的药品' : '还没有药品' }}</h3>
      <p>{{ medicines.length ? '换个关键词试试。' : '先去「药品」页添加药品，再回来做入库。' }}</p>
    </div>
    <div v-else class="list">
      <article v-for="m in filtered" :key="m.id" class="card stock-row">
        <div class="stock-body" @click="openDetail(m)">
          <div class="stock-title"><b>{{ m.name }}</b><span class="badge" :class="badgeClass(m)">{{ statusLabel(m) }}</span></div>
          <div class="tiny muted stock-meta">
            <span>{{ m.spec || '未填规格' }}</span><span class="dot">·</span>
            <span class="stock-qty" :class="m.stock_quantity <= 0 ? 'low' : ''">{{ m.stock_quantity }} {{ m.unit }}</span>
            <template v-if="m.nearest_expiry"><span class="dot">·</span><span>最近有效期 {{ m.nearest_expiry }}</span></template>
          </div>
        </div>
        <div class="stock-actions">
          <button v-if="book.canMedicineCreate" class="btn btn-outline btn-xs" @click.stop="openAction('in', m)">入库</button>
          <button v-if="book.canMedicineCreate" class="btn btn-outline btn-xs" @click.stop="openAction('out', m)">出库</button>
          <button v-if="book.canMedicineEdit" class="btn btn-ghost btn-xs" @click.stop="openAction('stocktake', m)">盘点</button>
        </div>
      </article>
    </div>

    <div class="section-head" style="margin-top:18px">
      <h2>全部出入库记录</h2>
      <button class="btn btn-ghost btn-sm" @click="router.push('/stock/detail')">明细查询<AppIcon name="chevron" :size="15" /></button>
    </div>
    <div v-if="loading && !items.length" class="card muted">加载中…</div>
    <div v-else-if="!items.length" class="card muted">还没有出入库记录。入库、出库、完成用药、盘点都会写在这里。</div>
    <div v-else class="list">
      <router-link v-for="m in items" :key="m.id" :to="'/medicines/' + m.medicine_id" class="card move-card">
        <div class="move-body">
          <div class="move-title"><b>{{ m.medicine_name || '药品' }}</b><span class="badge" :class="movementBadge(m.type)">{{ movementTypeLabel(m.type) }}</span></div>
          <div class="tiny muted move-meta">
            <span>{{ entrySourceLabel(m.entry_source) }}</span>
            <template v-if="m.type === 'in'"><span class="dot">·</span><span>{{ channelLabel(m.channel) }}</span></template>
            <span class="dot">·</span><span>{{ formatDate(m.created_at) }}</span>
            <template v-if="m.reason"><span class="dot">·</span><span>{{ m.reason }}</span></template>
            <template v-if="m.actor_name"><span class="dot">·</span><span>{{ m.actor_name }}</span></template>
          </div>
        </div>
        <div class="move-right">
          <b class="move-delta" :style="{ color: m.quantity_delta >= 0 ? 'var(--mc-primary-strong)' : 'var(--mc-danger)' }">{{ m.quantity_delta > 0 ? '+' : '' }}{{ m.quantity_delta }}</b>
          <span class="tiny muted">{{ m.unit }}</span>
        </div>
      </router-link>
    </div>
    <button v-if="hasMore" class="btn btn-outline btn-block" style="margin-top:10px" :disabled="loading" @click="load(false)">{{ loading ? '加载中…' : '加载更多' }}</button>

    <MedicineScanSheet :open="showScan" :book-id="book.currentId || 0" :medicines="medicines" mode="stock" @close="showScan = false" @imported="onScanImported" />

    <Teleport to="body">
      <!-- 单个药品的库存明细：什么时间入库、什么时间出库、谁操作的、余量多少 -->
      <div v-if="detailMed" class="sheet-backdrop" @click.self="closeDetail">
        <section class="sheet" role="dialog" aria-modal="true">
          <div class="sheet-grab" />
          <div class="sheet-head"><h2>{{ detailMed.name }} · 库存明细</h2><button class="icon-btn" aria-label="关闭" @click="closeDetail"><AppIcon name="close" /></button></div>
          <div class="sheet-body">
            <p class="tiny muted" style="margin:-2px 0 10px">
              当前库存 {{ detailMed.stock_quantity }} {{ detailMed.unit }}。入库、出库、盘点与
              <b>用药计划点完成后的自动出库</b>都会记在这里。
            </p>
            <div v-if="detailLoading && !detailItems.length" class="card muted">加载中…</div>
            <div v-else-if="!detailItems.length" class="card muted">这个药品还没有库存变动记录。</div>
            <div v-else class="list">
              <div v-for="x in detailItems" :key="x.id" class="card move-line">
                <div class="ml-main">
                  <div class="ml-top"><span class="badge" :class="movementBadge(x.type)">{{ movementTypeLabel(x.type) }}</span><b class="tiny muted">{{ entrySourceLabel(x.entry_source) }}</b></div>
                  <div class="tiny muted ml-meta">{{ formatFull(x.created_at) }}<template v-if="x.reason"> · {{ x.reason }}</template><template v-if="x.actor_name"> · {{ x.actor_name }}</template></div>
                </div>
                <div class="ml-right">
                  <b class="ml-delta" :style="{ color: x.quantity_delta >= 0 ? 'var(--mc-primary-strong)' : 'var(--mc-danger)' }">{{ x.quantity_delta > 0 ? '+' : '' }}{{ x.quantity_delta }}</b>
                  <span class="tiny muted">余 {{ x.quantity_after }} {{ detailMed.unit }}</span>
                </div>
              </div>
            </div>
            <button v-if="detailHasMore" class="btn btn-outline btn-block" style="margin-top:10px" :disabled="detailLoading" @click="loadDetail(false)">{{ detailLoading ? '加载中…' : '加载更多' }}</button>
          </div>
          <div class="sheet-foot">
            <button type="button" class="btn btn-outline btn-block" @click="router.push('/medicines/' + detailMed.id)">查看药品详情</button>
            <button v-if="book.canMedicineEdit" type="button" class="btn btn-outline btn-block" @click="openAction('stocktake', detailMed)">盘点修正</button>
            <button v-if="book.canMedicineCreate" type="button" class="btn btn-primary btn-block" @click="openAction('in', detailMed)">入库</button>
          </div>
        </section>
      </div>
      <div v-if="action" class="sheet-backdrop" @click.self="closeAction">
        <section class="sheet" role="dialog" aria-modal="true">
          <div class="sheet-grab" />
          <div class="sheet-head"><h2>{{ actionTitle }}</h2><button class="icon-btn" aria-label="关闭" @click="closeAction"><AppIcon name="close" /></button></div>
          <form class="sheet-body" @submit.prevent="submitAction">
            <div class="field">
              <label>药品</label>
              <MedicinePicker :model-value="form.medicineId || ''" :medicines="medicines" placeholder="选择药品" show-stock @update:model-value="pickMedicine" />
            </div>
            <div v-if="picked" class="tiny muted" style="margin:-6px 0 10px">当前库存：{{ picked.stock_quantity }} {{ picked.unit }}</div>

            <div v-if="action === 'in'">
              <div class="field-row">
                <div class="field"><label>入库数量（{{ picked?.unit || '单位' }}）</label><input v-model.number="form.quantity" type="number" min="0" step="0.01" class="input" placeholder="例如 2" /></div>
                <div class="field"><label>来源</label><SheetSelect v-model="form.channel" placeholder="选择来源" :options="channelOptions" /></div>
              </div>
              <div class="field-row">
                <div class="field"><label>批次号（可空）</label><input v-model="form.batchNo" class="input" placeholder="例如 20261101" /></div>
                <div class="field"><label>有效期（可空）</label><DateField v-model="form.expiryDate" placeholder="选择日期" clearable /></div>
              </div>
              <div class="field"><label>供应商（可空）</label><input v-model="form.supplier" class="input" /></div>
              <div class="field"><label>备注（可空）</label><input v-model="form.note" class="input" /></div>
            </div>

            <div v-else-if="action === 'out'">
              <div class="field-row">
                <div class="field"><label>出库数量（{{ picked?.unit || '单位' }}）</label><input v-model.number="form.quantity" type="number" min="0" step="0.01" class="input" placeholder="例如 1" /></div>
                <div class="field"><label>用途</label><SheetSelect v-model="form.channel" placeholder="选择用途" :options="channelOptions" /></div>
              </div>
              <div class="field"><label>原因</label><input v-model="form.reason" class="input" placeholder="例如：过期丢弃 / 借出 / 带走" /></div>
              <p class="tiny muted" style="margin:-4px 0 8px">出库按先进先出扣批次，数量不能超过当前库存。</p>
            </div>

            <div v-else>
              <div class="field"><label>实际数量（盘点后）</label><input v-model.number="form.targetQuantity" type="number" min="0" step="0.01" class="input" placeholder="盘点后剩余多少" /></div>
              <div class="field"><label>盘点原因</label><input v-model="form.reason" class="input" placeholder="例如：月度盘点 / 损耗核对" /></div>
              <p class="tiny muted" style="margin:-4px 0 8px">盘点只把库存改成实际数量，多退少补；不是出库。</p>
            </div>

            <div class="sheet-foot">
              <button type="button" class="btn btn-outline btn-block" :disabled="saving" @click="closeAction">取消</button>
              <button class="btn btn-primary btn-block" :disabled="saving || !canSubmit">{{ saving ? '提交中…' : '确认' }}</button>
            </div>
          </form>
        </section>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppIcon from '../components/AppIcon.vue'
import FilterPanel from '../components/FilterPanel.vue'
import MedicineScanSheet from '../components/MedicineScanSheet.vue'
import SheetSelect from '../components/SheetSelect.vue'
import MedicinePicker from '../components/MedicinePicker.vue'
import DateField from '../components/DateField.vue'
import { medicineApi, stockApi, type Medicine, type StockMovementItem } from '../api/medcare'
import { STOCK_CHANNEL_OPTIONS, channelLabel, entrySourceLabel, movementBadge, movementTypeLabel } from '../utils/stock'
import { fetchAllPages } from '../utils/paging'
import { formatDate } from '../utils/date'
import { useBookStore } from '../stores/book'
import { useScanStore } from '../stores/scan'
import { useUIStore } from '../stores/ui'

const router = useRouter()
const route = useRoute()
const book = useBookStore()
const scan = useScanStore()
const ui = useUIStore()

// ---- 库存总览 ----
const medicines = ref<Medicine[]>([])
const query = ref('')
const loading = ref(false)
// 分类统计与筛选（全部 / 口服 / 注射 / 输液 / 外用 / 其他）
const catFilter = ref('')
// 库存状态筛选：点顶部状态卡即刻筛列表（'' / low / out / expiring）
const statusFilter = ref('')
function pickStatus(v: string) { statusFilter.value = statusFilter.value === v ? '' : v }
function matchStatus(m: Medicine): boolean {
  switch (statusFilter.value) {
    case 'low':
      return m.stock_status === 'low' || (m.stock_quantity > 0 && m.low_stock_threshold > 0 && m.stock_quantity <= m.low_stock_threshold)
    case 'out':
      return Number(m.stock_quantity) <= 0
    case 'expiring':
      return m.stock_status === 'expiring' || m.stock_status === 'expired'
    default:
      return true
  }
}
const CATEGORY_LABELS: Record<string, string> = { oral: '口服', injection: '注射', infusion: '输液', topical: '外用', other: '其他' }
const catCounts = computed(() => ['oral', 'injection', 'infusion', 'topical', 'other'].map((value) => ({
  value, label: CATEGORY_LABELS[value],
  count: medicines.value.filter((m) => (m.category || 'other') === value).length,
})))
function pickCategory(v: string) { catFilter.value = catFilter.value === v ? '' : v }

const stats = computed(() => {
  const list = medicines.value
  return {
    total: list.length,
    low: list.filter((m) => m.stock_status === 'low' || (m.stock_quantity > 0 && m.low_stock_threshold > 0 && m.stock_quantity <= m.low_stock_threshold)).length,
    out: list.filter((m) => Number(m.stock_quantity) <= 0).length,
    expiring: list.filter((m) => m.stock_status === 'expiring' || m.stock_status === 'expired').length,
  }
})
const filtered = computed(() => {
  const q = query.value.trim().toLowerCase()
  const byCat = (m: Medicine) => (!catFilter.value || (m.category || 'other') === catFilter.value) && matchStatus(m)
  const base = (q
    ? medicines.value.filter((m) => [m.name, m.generic_name, m.spec].filter(Boolean).some((v) => String(v).toLowerCase().includes(q)))
    : medicines.value.slice()).filter(byCat)
  if (sort.value === 'default') return base
  const byName = (a: Medicine, b: Medicine) => String(a.name || '').localeCompare(String(b.name || ''), 'zh-Hans-CN')
  return base.sort((a, b) => {
    switch (sort.value) {
      case 'qty_asc': return Number(a.stock_quantity) - Number(b.stock_quantity) || byName(a, b)
      case 'qty_desc': return Number(b.stock_quantity) - Number(a.stock_quantity) || byName(a, b)
      case 'name_asc': return byName(a, b)
      case 'name_desc': return byName(b, a)
      case 'expiry': {
        // 有有效期的排前面（越近越前），没有有效期的排后面。
        const ea = a.nearest_expiry || ''
        const eb = b.nearest_expiry || ''
        if (!ea && !eb) return byName(a, b)
        if (!ea) return 1
        if (!eb) return -1
        return ea.localeCompare(eb) || byName(a, b)
      }
      default: return 0
    }
  })
})
function statusLabel(m: Medicine) {
  if (Number(m.stock_quantity) <= 0) return '缺货'
  if (m.stock_status === 'expired') return '已过期'
  if (m.stock_status === 'expiring') return '临期'
  if (m.stock_status === 'low') return '低库存'
  return '正常'
}
function badgeClass(m: Medicine) {
  const s = statusLabel(m)
  return s === '缺货' || s === '已过期' ? 'missed' : s === '低库存' || s === '临期' ? 'warn' : 'done'
}

// ---- 操作弹层 ----
type Action = '' | 'in' | 'out' | 'stocktake'
const action = ref<Action>('')
const saving = ref(false)
const form = reactive({ medicineId: 0, quantity: 0, targetQuantity: 0, reason: '', channel: 'pharmacy', batchNo: '', expiryDate: '', supplier: '', note: '' })
const channelOptions = STOCK_CHANNEL_OPTIONS
const medicineOptions = computed(() => medicines.value.map((m) => ({ value: m.id, label: m.name + (m.spec ? ' · ' + m.spec : '') + `（${m.stock_quantity} ${m.unit}）` })))
const picked = computed(() => medicines.value.find((m) => m.id === form.medicineId) || null)
const actionTitle = computed(() => (action.value === 'in' ? '入库' : action.value === 'out' ? '出库' : '盘点'))
const canSubmit = computed(() => {
  if (!form.medicineId) return false
  if (action.value === 'in') return Number(form.quantity) > 0
  if (action.value === 'out') return Number(form.quantity) > 0
  return form.targetQuantity !== null && form.targetQuantity !== undefined && Number(form.targetQuantity) >= 0
})
function openAction(kind: Action, m?: Medicine) {
  action.value = kind
  form.medicineId = m ? m.id : medicines.value.length === 1 ? medicines.value[0].id : 0
  form.quantity = 0
  form.targetQuantity = m ? Number(m.stock_quantity) : 0
  form.reason = ''
  form.channel = 'pharmacy'
  form.batchNo = ''
  form.expiryDate = ''
  form.supplier = ''
  form.note = ''
}
function closeAction() { if (!saving.value) action.value = '' }
function pickMedicine(v: any) {
  const id = Number(v) || 0
  form.medicineId = id
  const m = medicines.value.find((x) => x.id === id)
  if (m && action.value === 'stocktake') form.targetQuantity = Number(m.stock_quantity)
}
async function submitAction() {
  if (!canSubmit.value || !book.currentId) return
  const id = form.medicineId
  saving.value = true
  try {
    if (action.value === 'in') {
      await medicineApi.stockIn(book.currentId, id, {
        quantity: Number(form.quantity), channel: form.channel, batch_no: form.batchNo, expiry_date: form.expiryDate,
        supplier: form.supplier, note: form.note,
      })
      ui.showToast(`已入库 ${form.quantity} ${picked.value?.unit || ''}`)
    } else if (action.value === 'out') {
      await medicineApi.stockOut(book.currentId, id, { quantity: Number(form.quantity), channel: form.channel, reason: form.reason })
      ui.showToast(`已出库 ${form.quantity} ${picked.value?.unit || ''}`)
    } else {
      await medicineApi.stocktake(book.currentId, id, { target_quantity: Number(form.targetQuantity), reason: form.reason || '盘点' })
      ui.showToast('盘点完成，库存已更新')
    }
    action.value = ''
    // 从「库存明细」弹层里发起的操作：刷新列表之外，还要刷新明细弹层本身，
    // 否则弹层里的「当前库存」与新记录都停在旧数据上，只能手动刷新页面。
    const keepDetail = detailMed.value?.id === id
    await loadMedicines()
    if (keepDetail) {
      const fresh = medicines.value.find((x) => x.id === id)
      if (fresh) detailMed.value = fresh
      await loadDetail(true)
    }
    await load(true)
  } catch (e: any) {
    ui.showToast(e?.response?.data?.message || e?.message || '操作失败', 'error')
  } finally { saving.value = false }
}

// ---- 流水 ----
const items = ref<StockMovementItem[]>([])
const page = ref(1)
const hasMore = ref(false)
const type = ref('')
const channel = ref('')
const typeFilters = [
  { value: '', label: '全部' }, { value: 'in', label: '入库' }, { value: 'out', label: '出库' },
  { value: 'adjust', label: '调整' }, { value: 'waste', label: '丢弃' },
]
const channelFilters = [
  { value: '', label: '全部来源' }, { value: 'pharmacy', label: '药店购买' },
  { value: 'hospital', label: '医院开具' }, { value: 'other', label: '其他' },
]
const filtersOpen = ref(false)
const showScan = ref(false)
// 顶栏「识别完成 · 去核对」点进来时，自动打开本页的识别弹层。
watch(() => scan.requestOpen, (v) => { if (v) { showScan.value = true; scan.requestOpen = false } })
// AI 识别入库完成后刷新库存与流水。
async function onScanImported() {
  showScan.value = false
  await Promise.all([loadMedicines(), load(true)])
}
// 折叠态只有搜索框；筛选条数统计类型与来源两个条件。
// 药品排序：库存量 / 名称 / 最近有效期；默认按最近更新（服务端返回顺序）。
type StockSort = 'default' | 'qty_asc' | 'qty_desc' | 'name_asc' | 'name_desc' | 'expiry'
const sort = ref<StockSort>('default')
const sortOptions: { value: StockSort; label: string }[] = [
  { value: 'default', label: '默认' },
  { value: 'qty_asc', label: '库存少→多' },
  { value: 'qty_desc', label: '库存多→少' },
  { value: 'name_asc', label: '名称 A→Z' },
  { value: 'name_desc', label: '名称 Z→A' },
  { value: 'expiry', label: '最近有效期' },
]
const activeFilterCount = computed(() => [type.value, channel.value, catFilter.value, statusFilter.value].filter(Boolean).length + (sort.value !== 'default' ? 1 : 0))
async function load(reset = true) {
  if (!book.currentId) return
  loading.value = true
  const target = reset ? 1 : page.value + 1
  try {
    const res = await stockApi.movements(book.currentId, { page: target, pageSize: 20, type: type.value, channel: channel.value })
    const list = res.data?.data?.items || []
    items.value = reset ? list : items.value.concat(list)
    page.value = target
    hasMore.value = items.value.length < (res.data?.data?.total || 0)
  } catch (e: any) { ui.showToast(e?.response?.data?.message || '读取库存流水失败', 'error') }
  finally { loading.value = false }
}
// 条件只改状态，点「查询」才真正拉数据，避免勾一下刷一次。
function setType(v: string) { type.value = v }
function setChannel(v: string) { channel.value = v }
function resetFilters() { type.value = ''; channel.value = ''; query.value = ''; sort.value = 'default'; catFilter.value = ''; statusFilter.value = ''; load(true) }

// ---- 单药库存明细 ----
const detailMed = ref<Medicine | null>(null)
const detailItems = ref<StockMovementItem[]>([])
const detailLoading = ref(false)
const detailPage = ref(1)
const detailHasMore = ref(false)

function openDetail(m: Medicine) {
  detailMed.value = m
  detailItems.value = []
  detailPage.value = 1
  detailHasMore.value = false
  loadDetail(true)
}
function closeDetail() { detailMed.value = null }

async function loadDetail(reset = true) {
  const med = detailMed.value
  if (!med || !book.currentId) return
  detailLoading.value = true
  const target = reset ? 1 : detailPage.value + 1
  try {
    const res = await medicineApi.movements(book.currentId, med.id, { page: target, pageSize: 20 })
    const list = (res.data?.data?.items || []) as StockMovementItem[]
    detailItems.value = reset ? list : detailItems.value.concat(list)
    detailPage.value = target
    detailHasMore.value = detailItems.value.length < (Number(res.data?.data?.total) || 0)
  } catch (e: any) {
    ui.showToast(e?.response?.data?.message || '读取库存明细失败', 'error')
  } finally { detailLoading.value = false }
}

// 明细行要精确到时分：入库/出库时间点对不上会以为记错了。
function formatFull(v: string) {
  if (!v) return ''
  const d = new Date(v)
  if (Number.isNaN(d.getTime())) return v
  return d.toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
}

async function loadMedicines() {
  if (!book.currentId || !book.canMedicineView) return
  try {
    medicines.value = await fetchAllPages<Medicine>(async (p, pageSize) => {
      const res = await medicineApi.list(book.currentId!, { page: p, pageSize })
      return { items: (res.data?.data?.items || []) as Medicine[], total: Number(res.data?.data?.total) || 0 }
    })
  } catch (e: any) { ui.showToast(e?.response?.data?.message || '读取库存失败', 'error') }
}

onMounted(async () => {
  await Promise.all([loadMedicines(), load(true)])
  // 从药品详情「去库存页」进来时定位到该药品，并可带 action 直接打开入库/出库/盘点。
  const mid = Number(route.query.medicine_id || 0)
  if (!mid) return
  const target = medicines.value.find((m) => m.id === mid)
  if (!target) return
  query.value = target.name
  const act = String(route.query.action || '')
  if (act === 'in' || act === 'out' || act === 'stocktake') openAction(act, target)
})
watch(() => book.currentId, async () => { await Promise.all([loadMedicines(), load(true)]) })
</script>

<style scoped>
.scan-banner { display: flex; align-items: center; gap: 10px; width: 100%; min-height: 58px; margin-bottom: 10px; padding: 10px 12px; border-radius: 14px; border: 1px solid var(--mc-border); background: linear-gradient(135deg, var(--mc-primary-soft), var(--mc-surface)); text-align: left; }
.scan-banner-icon { flex: 0 0 auto; width: 36px; height: 36px; border-radius: 12px; display: inline-flex; align-items: center; justify-content: center; background: var(--mc-surface); color: var(--mc-primary); }
.scan-banner-body { flex: 1; min-width: 0; }
.scan-banner-body b { font-size: 14px; }
.head-actions { display: flex; gap: 8px; flex-wrap: wrap; flex: 0 0 auto; }
/* 手机窄屏：标题和说明占整行，按钮组另起一行三等分，避免说明被挤成一字一行。 */
@media (max-width: 899px) {
  /* 手机端：库存说明压成一行，避免占两行高度 */
  .page-head p { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .page-head { flex-wrap: wrap; }
  .page-head > div:first-child { flex: 1 1 100%; }
  /* 5 个按钮按 3 列两行排，比 4 列留一个孤零零的更好看。 */
  .head-actions { flex: 1 1 100%; display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 6px; margin-top: 2px; }
  .head-actions .btn { padding: 0 6px; font-size: 12px; }
}
.stat-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 8px; }
.stat.active { border-color: var(--mc-primary); background: var(--mc-primary-soft); }
.stat.active .tiny.muted { color: var(--mc-primary-strong); }
.cat-stats { display: grid; grid-template-columns: repeat(6, minmax(0, 1fr)); gap: 6px; }
.cat-stat { display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 0; min-height: 44px; padding: 5px 2px; border-radius: 10px; border: 1px solid var(--mc-border); background: var(--mc-surface); }
.cat-stat b { font-size: 14.5px; line-height: 1.15; color: var(--mc-text); }
.cat-stat span { font-size: 10.5px; line-height: 1.15; color: var(--mc-text-muted); font-weight: 650; }
.cat-stat.active { border-color: var(--mc-primary); background: var(--mc-primary-soft); }
.cat-stat.active b { color: var(--mc-primary-strong); }
.stat { padding: 7px 10px; }
.stat .tiny { font-size: 10.5px; line-height: 1.2; }
.stat-num { font-size: 16px; font-weight: 800; }
.stat-num.warn { color: var(--mc-warning); }
.stat-num.danger { color: var(--mc-danger); }
.sort-bar { display: flex; align-items: center; justify-content: space-between; gap: 10px; margin: 2px 0 8px; }
.sort-bar-left { display: inline-flex; align-items: center; gap: 6px; color: var(--mc-text-muted); flex: 1 1 auto; min-width: 0; }
.sort-select { flex: 0 0 128px; max-width: 128px; }
.sort-select :deep(.select-like) { min-height: 36px; height: 36px; font-size: 13px; }
.filter-group { margin-bottom: 6px; }
.filter-label { font-size: 12px; color: var(--mc-text-muted); margin: 0 2px 6px; font-weight: 650; }
.search-card { display: flex; align-items: center; gap: 8px; padding: 0 12px; }
.search-card:focus-within { border-color: var(--mc-primary); box-shadow: 0 0 0 3px var(--mc-primary-soft); }
.search-icon { flex: 0 0 auto; color: var(--mc-text-muted); }
.search-input, .search-input:focus { border: 0 !important; background: transparent !important; box-shadow: none !important; border-radius: 0; padding-left: 0; }
.stock-row { display: flex; align-items: center; justify-content: space-between; gap: 10px; padding: 11px 12px; }
.stock-body { min-width: 0; flex: 1 1 auto; cursor: pointer; }
.stock-title { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.stock-title b { font-size: 14px; overflow-wrap: anywhere; }
.stock-meta { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 3px; }
.stock-meta .dot { color: var(--mc-border); }
.stock-actions { display: flex; gap: 6px; flex: 0 0 auto; flex-wrap: wrap; justify-content: flex-end; }
.move-card { display: flex; justify-content: space-between; gap: 10px; align-items: flex-start; padding: 11px 12px; }
.move-body { min-width: 0; }
.move-title { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.move-title b { font-size: 14px; overflow-wrap: anywhere; }
.move-meta { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 3px; }
.move-meta .dot { color: var(--mc-border); }
.move-right { flex: 0 0 auto; text-align: right; display: flex; flex-direction: column; gap: 2px; }
.move-delta { font-size: 15px; }
.move-line { display: flex; justify-content: space-between; gap: 10px; align-items: flex-start; padding: 10px 12px; }
.ml-main { min-width: 0; flex: 1 1 auto; }
.ml-top { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.ml-top b { font-weight: 500; }
.ml-meta { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 3px; }
.ml-right { flex: 0 0 auto; text-align: right; display: flex; flex-direction: column; gap: 2px; }
.ml-delta { font-size: 15px; }
@media (min-width: 900px) {
  .stat { padding: 10px 12px; }
  .stat-num { font-size: 24px; }
  .stock-row { padding: 13px 15px; }
  .stock-title b { font-size: 15px; }
  .move-card { padding: 13px 15px; gap: 14px; }
  .move-title b { font-size: 15px; }
}
</style>
