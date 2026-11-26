<template>
  <div class="page narrow">
    <div class="page-head">
      <div style="display:flex;gap:8px;align-items:center"><button class="icon-btn" aria-label="返回" @click="router.back()"><AppIcon name="arrowLeft" /></button><div><div class="eyebrow">药品</div><h1>{{ medicine?.name || '药品' }}</h1></div></div>
      <button v-if="book.canMedicineEdit || book.canMedicineDelete" class="icon-btn" aria-label="更多" @click="menu = !menu"><AppIcon name="edit" /></button>
    </div>
    <div v-if="loading" class="card muted">加载中…</div>
    <template v-else-if="medicine">
      <div class="tabs tab-row">
        <button class="tab" :class="{ active: tab === 'info' }" @click="tab = 'info'">基本信息</button>
        <button class="tab" :class="{ active: tab === 'stock' }" @click="tab = 'stock'">库存</button>
        <button class="tab" :class="{ active: tab === 'moves' }" @click="openMoves">出入库明细</button>
      </div>

      <!-- 基本信息 -->
      <template v-if="tab === 'info'">
        <form class="card" @submit.prevent="saveInfo">
          <div class="field"><label>药品名称</label><input v-model="infoForm.name" class="input" required /></div>
          <div class="field"><label>通用名</label><input v-model="infoForm.generic_name" class="input" placeholder="例如：苯磺酸氨氯地平" /></div>
          <div class="field-row">
            <div class="field"><label>分类</label><SheetSelect v-model="infoForm.category" placeholder="分类" :options="categoryOptions" /></div>
            <div class="field"><label>剂型</label><SheetSelect v-model="infoForm.form" placeholder="剂型" :options="formOptions()" searchable allow-custom custom-label="没有合适的剂型？自己填" custom-placeholder="例如：滴剂" /></div>
          </div>
          <div class="field-row">
            <div class="field"><label>规格</label><SheetSelect v-model="infoForm.spec" placeholder="规格" :options="specOptions()" searchable allow-custom custom-label="没有合适的规格？自己填" custom-placeholder="例如：0.2g×10片" /></div>
            <div class="field"><label>库存单位</label><SheetSelect v-model="infoForm.unit" placeholder="库存单位" :options="unitOptions()" searchable allow-custom custom-label="没有合适的单位？自己填" custom-placeholder="例如：支/瓶/盒" /></div>
          </div>
          <div class="field-row">
            <div class="field"><label>包装规格</label><SheetSelect v-model="infoForm.package_spec" placeholder="包装规格" :options="packageSpecOptions()" searchable allow-custom custom-label="没有合适的包装？自己填" custom-placeholder="例如：20片/盒" /></div>
            <div class="field"><label>生产企业</label><input v-model="infoForm.manufacturer" class="input" /></div>
          </div>
          <div class="field-row">
            <div class="field"><label>低库存阈值</label><input v-model.number="infoForm.low_stock_threshold" type="number" min="0" class="input" /></div>
            <div class="field"><label>临期预警天数</label><input v-model.number="infoForm.expiry_warn_days" type="number" min="0" class="input" /></div>
          </div>
          <div class="field" style="display:flex;justify-content:space-between;align-items:center;gap:12px">
            <div><label style="margin:0">库存不足提醒</label><div class="tiny muted" style="margin-top:2px">关掉后这个药低于阈值或缺货都不再提醒补货；不再使用的药可以关掉</div></div>
            <label class="switch"><input v-model="infoForm.low_stock_notify_enabled" type="checkbox" /><span /></label>
          </div>
          <div class="field"><label>单价（元/单位）</label><input v-model.number="infoForm.unit_price" type="number" min="0" step="0.01" class="input" placeholder="可选，例如 3.50" /></div>
          <div class="field"><label>备注</label><textarea v-model="infoForm.notes" class="textarea" /></div>
          <p class="tiny muted editor-line">新增：{{ medicine.created_by_name || '—' }} · {{ formatDate(medicine.created_at || '') }}<template v-if="medicine.updated_by_name"> ｜ 最后更新：{{ medicine.updated_by_name }} · {{ formatDate(medicine.updated_at || '') }}</template></p>
          <button v-if="book.canMedicineEdit" class="btn btn-primary btn-block" :disabled="saving">{{ saving ? '保存中…' : '保存基本信息' }}</button>
          <p v-else class="tiny muted">当前身份只能查看药品信息。</p>
        </form>
        <MedicinePlans v-if="book.canPlanView" :medicine-id="medicine.id" />
        <MedicinePhotos v-if="book.canImageView" :book-id="book.currentId || 0" :medicine-id="medicine.id" :cover-image-id="medicine.cover_image_id || 0" @update:cover="onCoverChange" />
      </template>

      <!-- 库存 -->
      <template v-else-if="tab === 'stock'">
        <div class="card stock-hero">
          <div class="eyebrow">当前库存</div>
          <div class="stock-line"><span class="stock-num" :class="stockQtyClass">{{ medicine.stock_quantity }}</span><span class="muted stock-unit">{{ medicine.unit }}</span><span class="badge" :class="[stockBadge, { blink: isLowStock }]" style="margin-left:auto">{{ stockLabel }}</span></div>
          <div class="muted small" style="margin-top:3px">{{ medicine.spec || '未填规格' }}<span v-if="medicine.nearest_expiry"> · 最近有效期 {{ medicine.nearest_expiry }}</span></div>
          <div class="progress-track" style="margin-top:10px"><span :style="{ width: stockPercent + '%' }" /></div>
          <div class="tiny muted" style="margin-top:3px">低库存阈值 {{ medicine.low_stock_threshold ? medicine.low_stock_threshold + ' ' + medicine.unit : '未设置' }}</div>
        </div>
        <div v-if="book.canMedicineCreate || book.canMedicineEdit" class="stock-btn-row" style="margin-top:10px">
          <button v-if="book.canMedicineCreate" class="btn btn-primary" @click="openStockIn"><AppIcon name="plus" :size="18" />入库</button>
          <button v-if="book.canMedicineCreate" class="btn btn-outline" @click="openStockOut"><AppIcon name="refresh" :size="18" />出库</button>
          <button v-if="book.canMedicineEdit" class="btn btn-outline" @click="openStocktake"><AppIcon name="edit" :size="18" />盘点</button>
        </div>
        <div class="section">
          <div class="section-head"><h2>批次</h2><span class="tiny muted">{{ batches.length }} 个</span></div>
          <div v-if="!batches.length" class="card muted">暂无批次</div>
          <div v-else class="list">
            <div v-for="b in batches" :key="b.id" class="card batch-card">
              <div class="batch-head"><b>{{ b.batch_no || '无批号' }}</b><span class="badge" :class="b.quantity_remaining <= 0 ? 'pending' : 'done'">{{ b.quantity_remaining }} / {{ b.quantity_initial }} {{ medicine.unit }}</span></div>
              <div class="tiny muted" style="margin-top:3px">有效期 {{ b.expiry_date || '未设置' }} · 入库 {{ formatDate(b.received_at) }}</div>
              <button v-if="b.editable && book.canMedicineEdit" type="button" class="btn btn-ghost btn-xs" style="margin-top:6px" @click="openEditBatch(b.id)"><AppIcon name="edit" :size="14" />修改入库</button>
            </div>
          </div>
        </div>
      </template>

      <!-- 出入库明细 -->
      <template v-else>
        <div v-if="book.canMedicineCreate || book.canMedicineEdit" class="moves-actions">
          <button v-if="book.canMedicineCreate" class="btn btn-primary btn-sm" @click="openStockIn"><AppIcon name="plus" :size="16" />入库</button>
          <button v-if="book.canMedicineCreate" class="btn btn-outline btn-sm" @click="openStockOut"><AppIcon name="refresh" :size="16" />出库</button>
          <button v-if="book.canMedicineEdit" class="btn btn-outline btn-sm" @click="openStocktake"><AppIcon name="edit" :size="16" />盘点</button>
          <button class="btn btn-ghost btn-sm" @click="router.push('/stock?medicine_id=' + medicine.id)">去库存页<AppIcon name="chevron" :size="16" /></button>
        </div>
        <div class="chip-row" style="margin-bottom:10px">
          <button v-for="opt in moveFilters" :key="opt.value" class="chip" :class="{ active: moveType === opt.value }" @click="setMoveType(opt.value)">{{ opt.label }}</button>
        </div>
        <div v-if="!movements.length" class="card muted">暂无出入库记录</div>
        <div v-else class="list">
          <div v-for="m in movements" :key="m.id" class="card move-card">
            <div class="move-body">
              <div class="move-title"><span class="badge" :class="movementBadge(m.type)">{{ movementTypeLabel(m.type) }}</span><b v-if="m.entry_source || m.channel" class="tiny muted">{{ entrySourceLabel(m.entry_source) }}<template v-if="m.type === 'in'"> · {{ channelLabel(m.channel) }}</template></b></div>
              <div class="tiny muted" style="margin-top:2px">{{ formatDate(m.created_at) }}<template v-if="m.reason"> · {{ m.reason }}</template></div>
            </div>
            <div style="display:flex;align-items:center;gap:8px">
              <button v-if="canEditMove(m)" type="button" class="btn btn-ghost btn-xs" @click="openEditBatch(m.batch_id)"><AppIcon name="edit" :size="14" />修改</button>
              <b class="move-delta" :style="{ color: m.quantity_delta >= 0 ? 'var(--mc-primary-strong)' : 'var(--mc-danger)' }">{{ m.quantity_delta > 0 ? '+' : '' }}{{ m.quantity_delta }}</b>
            </div>
          </div>
        </div>
        <button v-if="hasMore" class="btn btn-outline btn-block" style="margin-top:10px" :disabled="loadingMoves" @click="loadMoves(false)">{{ loadingMoves ? '加载中…' : '加载更多' }}</button>
      </template>

      <div v-if="menu && (book.canMedicineEdit || book.canMedicineDelete)" class="card" style="margin-top:12px">
        <button class="btn btn-outline btn-block" style="margin-bottom:8px" @click="openTransfer">转移到其它用药本</button>
        <button class="btn btn-danger btn-block" @click="archive">删除药品</button>
      </div>
    </template>

    <Teleport to="body">
      <div v-if="showStockIn" class="sheet-backdrop" @click.self="showStockIn=false">
        <section class="sheet"><div class="sheet-grab" /><div class="sheet-head"><h2>药品入库</h2><button class="icon-btn" @click="showStockIn=false"><AppIcon name="close" /></button></div>
          <form class="sheet-body" @submit.prevent="stockIn">
            <StockInFields :model-value="stockInForm" :unit="medicine?.unit || ''" @update:model-value="(v) => Object.assign(stockInForm, v)" />
            <div class="sheet-foot"><button type="button" class="btn btn-outline btn-block" @click="showStockIn=false">取消</button><button class="btn btn-primary btn-block" :disabled="saving">{{ saving ? '入库中…' : '确认入库' }}</button></div>
          </form>
        </section>
      </div>
      <div v-if="showUnitConvert" class="sheet-backdrop" @click.self="showUnitConvert=false">
        <section class="sheet"><div class="sheet-grab" /><div class="sheet-head"><h2>修改库存单位</h2><button class="icon-btn" @click="showUnitConvert=false"><AppIcon name="close" /></button></div>
          <div class="sheet-body">
            <p class="small" style="line-height:1.7;margin-bottom:10px">
              库存单位要从 <b>{{ medicine?.unit }}</b> 改成 <b>{{ infoForm.unit }}</b>。
              已有批次、出入库流水与当前库存（{{ medicine?.stock_quantity }} {{ medicine?.unit }}）都按旧单位记着，
              需要确认怎么换算。
            </p>
            <div class="field"><label>1 {{ medicine?.unit }} 等于多少 {{ infoForm.unit }}？</label><input v-model.number="unitFactor" type="number" min="0" step="0.01" class="input" placeholder="例如 20" /></div>
            <p class="tiny muted" style="margin:-4px 0 10px">按此换算：库存会变成 {{ (Number(medicine?.stock_quantity || 0) * Number(unitFactor || 0)).toFixed(2).replace(/\.00$/, '') }} {{ infoForm.unit }}</p>
            <p class="tiny muted" style="margin:0">如果之前只是单位选错、数量本身是对的（比如 2 片被记成 2 盒），选下面的「只换单位」即可。</p>
          </div>
          <div class="sheet-foot" style="display:grid;gap:8px">
            <button type="button" class="btn btn-outline btn-block" :disabled="saving" @click="applyUnitConvert(1)">只换单位，数量不变</button>
            <button type="button" class="btn btn-primary btn-block" :disabled="saving || !(Number(unitFactor) > 0)" @click="applyUnitConvert(Number(unitFactor))">{{ saving ? '保存中…' : '按此换算并保存' }}</button>
          </div>
        </section>
      </div>
      <div v-if="showStockOut" class="sheet-backdrop" @click.self="showStockOut=false">
        <section class="sheet"><div class="sheet-grab" /><div class="sheet-head"><h2>药品出库</h2><button class="icon-btn" @click="showStockOut=false"><AppIcon name="close" /></button></div>
          <form class="sheet-body" @submit.prevent="stockOut">
            <p class="tiny muted" style="margin:-2px 0 10px">当前库存 {{ medicine?.stock_quantity }} {{ medicine?.unit }}；出库按先进先出扣批次，数量不能超过库存。</p>
            <StockOutFields :model-value="stockOutForm" :unit="medicine?.unit || ''" @update:model-value="(v) => Object.assign(stockOutForm, v)" />
            <div class="sheet-foot"><button type="button" class="btn btn-outline btn-block" @click="showStockOut=false">取消</button><button class="btn btn-primary btn-block" :disabled="saving">{{ saving ? '出库中…' : '确认出库' }}</button></div>
          </form>
        </section>
      </div>
      <div v-if="showTransfer" class="sheet-backdrop" @click.self="showTransfer=false">
        <section class="sheet"><div class="sheet-grab" /><div class="sheet-head"><h2>转移到其它用药本</h2><button class="icon-btn" @click="showTransfer=false"><AppIcon name="close" /></button></div>
          <div class="sheet-body">
            <p class="small muted" style="line-height:1.7;margin-bottom:10px">会把该药品连同它的批次、库存流水、用药计划和用药记录一起转到目标用药本；原用药本里不再保留。</p>
            <div class="field"><label>目标用药本</label><SheetSelect v-model="transferTarget" placeholder="选择用药本" :options="transferOptions" /></div>
          </div>
          <div class="sheet-foot"><button type="button" class="btn btn-outline btn-block" @click="showTransfer=false">取消</button><button class="btn btn-primary btn-block" :disabled="saving || !transferTarget" @click="doTransfer">{{ saving ? '转移中…' : '确认转移' }}</button></div>
        </section>
      </div>
      <StockInEditSheet :open="showEditBatch" :batch="editBatch" :unit="medicine?.unit || ''" :medicines="editMedicines" @close="showEditBatch=false" @saved="onBatchSaved" />
      <div v-if="showStocktake" class="sheet-backdrop" @click.self="showStocktake=false">
        <section class="sheet"><div class="sheet-grab" /><div class="sheet-head"><h2>盘点库存</h2><button class="icon-btn" @click="showStocktake=false"><AppIcon name="close" /></button></div>
          <form class="sheet-body" @submit.prevent="stocktake">
            <StocktakeFields :model-value="stocktakeForm" :unit="medicine?.unit || ''" @update:model-value="(v) => Object.assign(stocktakeForm, v)" />
            <div class="sheet-foot"><button type="button" class="btn btn-outline btn-block" @click="showStocktake=false">取消</button><button class="btn btn-primary btn-block" :disabled="saving">确认盘点</button></div>
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
import MedicinePhotos from '../components/MedicinePhotos.vue'
import MedicinePlans from '../components/MedicinePlans.vue'
import { tryDeleteMedicine } from '../utils/medicine'
import { formOptions, packageSpecOptions, specOptions, unitOptions } from '../utils/medicineMeta'
import DateField from '../components/DateField.vue'
import SheetSelect from '../components/SheetSelect.vue'
import { medicineApi, type Medicine } from '../api/medcare'
import { STOCK_CHANNEL_OPTIONS, channelLabel, entrySourceLabel, movementBadge, movementTypeLabel, emptyStockInDraft, stockInQuantity, emptyStockOutDraft, emptyStocktakeDraft, type StockInDraft, type StockOutDraft, type StocktakeDraft } from '../utils/stock'
import StockInFields from '../components/StockInFields.vue'
import StockOutFields from '../components/StockOutFields.vue'
import StocktakeFields from '../components/StocktakeFields.vue'
import StockInEditSheet from '../components/StockInEditSheet.vue'
import { useBookStore } from '../stores/book'
import { useUIStore } from '../stores/ui'
const route = useRoute()
const router = useRouter()
const book = useBookStore()
const ui = useUIStore()
const medicine = ref<Medicine | null>(null)
// 封面变更后同步到本地对象，列表返回时缩略图立即生效。
function onCoverChange(id: number) { if (medicine.value) medicine.value.cover_image_id = id }
const batches = ref<any[]>([])
const movements = ref<any[]>([])
const loading = ref(true)
const saving = ref(false)
const loadingMoves = ref(false)
const menu = ref(false)
const tab = ref<'info' | 'stock' | 'moves'>('info')
const moveType = ref('')
const movePage = ref(1)
const hasMore = ref(false)
const showStockIn = ref(false)
const showStockOut = ref(false)
const showStocktake = ref(false)
const showEditBatch = ref(false)
const editBatch = ref<any>(null)
// 修改入库记录时可改归属药品：列出本用药本里未删除的药品。
const editMedicines = ref<Medicine[]>([])
const showTransfer = ref(false)
const transferTarget = ref(0)
const transferOptions = computed(() => book.books.filter((b) => b.id !== book.currentId && (b.role === 'owner' || b.role === 'manager')).map((b) => ({ value: b.id, label: b.name + '（' + b.care_recipient_name + '）' })))
const stocktakeForm = reactive<StocktakeDraft>(emptyStocktakeDraft())
const channelOptions = STOCK_CHANNEL_OPTIONS
const categoryOptions = [
  { value: 'oral', label: '口服' }, { value: 'injection', label: '注射' },
  { value: 'infusion', label: '输液' }, { value: 'topical', label: '外用' }, { value: 'other', label: '其他' },
]
const moveFilters = [
  { value: '', label: '全部' }, { value: 'in', label: '入库' }, { value: 'out', label: '用药扣减' },
  { value: 'adjust', label: '调整' }, { value: 'waste', label: '丢弃' },
]
const infoForm = reactive({ name: '', generic_name: '', category: 'oral', form: '', spec: '', unit: '', package_spec: '', manufacturer: '', notes: '', low_stock_threshold: 0, low_stock_notify_enabled: true, expiry_warn_days: 30, unit_price: 0 })
const stockInForm = reactive<StockInDraft>(emptyStockInDraft())
const stockOutForm = reactive<StockOutDraft>(emptyStockOutDraft())
const showUnitConvert = ref(false)
const unitFactor = ref(1)
const isLowStock = computed(() => medicine.value?.stock_status === 'low' || medicine.value?.stock_status === 'out')
const stockQtyClass = computed(() => isLowStock.value ? 'stock-qty-low' : 'stock-qty-ok')
const stockBadge = computed(() => isLowStock.value || medicine.value?.stock_status === 'expired' ? 'missed' : medicine.value?.stock_status === 'expiring' ? 'warn' : 'done')
const stockLabel = computed(() => medicine.value?.stock_status === 'out' ? '缺货' : medicine.value?.stock_status === 'low' ? '库存不足' : medicine.value?.stock_status === 'expired' ? '已过期' : medicine.value?.stock_status === 'expiring' ? '临期' : '正常')
const stockPercent = computed(() => medicine.value && medicine.value.low_stock_threshold > 0 ? Math.min(100, Math.round(medicine.value.stock_quantity / medicine.value.low_stock_threshold * 100)) : (medicine.value?.stock_quantity ? 100 : 0))
import { formatDate } from '../utils/date'
async function load() {
  if (!book.currentId) return
  loading.value = true
  try {
    const [m, b] = await Promise.all([medicineApi.get(book.currentId, Number(route.params.id)), medicineApi.batches(book.currentId, Number(route.params.id))])
    medicine.value = m.data?.data
    batches.value = b.data?.data || []
    Object.assign(stocktakeForm, emptyStocktakeDraft(Number(medicine.value?.stock_quantity || 0)))
    if (medicine.value) {
      Object.assign(infoForm, {
        name: medicine.value.name, generic_name: medicine.value.generic_name || '', category: medicine.value.category,
        form: medicine.value.form || '', spec: medicine.value.spec || '', unit: medicine.value.unit,
        package_spec: medicine.value.package_spec || '', manufacturer: medicine.value.manufacturer || '',
        notes: medicine.value.notes || '', low_stock_threshold: medicine.value.low_stock_threshold, low_stock_notify_enabled: medicine.value.low_stock_notify_enabled !== false, unit_price: medicine.value.unit_price || 0,
        expiry_warn_days: medicine.value.expiry_warn_days,
      })
    }
  } catch (e: any) { ui.showToast(e?.response?.data?.message || '读取药品失败', 'error') }
  finally { loading.value = false }
}
async function loadMoves(reset = true) {
  if (!book.currentId) return
  loadingMoves.value = true
  const page = reset ? 1 : movePage.value + 1
  try {
    const res = await medicineApi.movements(book.currentId, Number(route.params.id), { page, pageSize: 20, type: moveType.value })
    const items = res.data?.data?.items || []
    movements.value = reset ? items : movements.value.concat(items)
    movePage.value = page
    const total = res.data?.data?.total || 0
    hasMore.value = movements.value.length < total
  } catch (e: any) { ui.showToast(e?.response?.data?.message || '读取流水失败', 'error') }
  finally { loadingMoves.value = false }
}
function openMoves() { tab.value = 'moves'; loadMoves(true) }
function setMoveType(v: string) { moveType.value = v; loadMoves(true) }
function openStockIn() { Object.assign(stockInForm, emptyStockInDraft()); showStockIn.value = true }
async function saveInfo() {
  if (!medicine.value) return
  if (!infoForm.name.trim()) { ui.showToast('药品名称不能为空', 'error'); return }
  // 改了库存单位就要先把单位换算问清楚：老数据是按旧单位记的，直接改会让数量对不上。
  if (infoForm.unit !== medicine.value.unit) {
    unitFactor.value = 1
    showUnitConvert.value = true
    return
  }
  await doSaveInfo()
}
async function doSaveInfo() {
  if (!medicine.value) return
  saving.value = true
  try {
    await medicineApi.update(book.currentId!, medicine.value.id, { ...infoForm, version: medicine.value.version })
    ui.showToast('已保存'); await load()
  } catch (e: any) { ui.showToast(e?.response?.data?.message || '保存失败', 'error') }
  finally { saving.value = false }
}
// 单位换算：factor=1 表示只换单位标签、数量不动（之前只是单位选错，数量本身是对的）。
async function applyUnitConvert(factor: number) {
  if (!medicine.value) return
  saving.value = true
  try {
    await medicineApi.convertUnit(book.currentId!, medicine.value.id, { unit: infoForm.unit, factor })
    showUnitConvert.value = false
    ui.showToast('库存单位已改为 ' + infoForm.unit)
    await doSaveInfo()
  } catch (e: any) {
    ui.showToast(e?.response?.data?.message || '修改库存单位失败', 'error')
  } finally { saving.value = false }
}
async function stockIn() {
  if (!medicine.value) return
  const qty = stockInQuantity(stockInForm)
  if (!(qty > 0)) { ui.showToast('请填写入库数量，或填包装数与每包装含量', 'error'); return }
  saving.value = true
  try { await medicineApi.stockIn(book.currentId!, medicine.value.id, { ...stockInForm, quantity: qty, batch_no: stockInForm.batch_no, expiry_date: stockInForm.expiry_date }); showStockIn.value = false; Object.assign(stockInForm, emptyStockInDraft()); ui.showToast('已入库'); await load(); if (tab.value === 'moves') await loadMoves(true) }
  catch (e: any) { ui.showToast(e?.response?.data?.message || '入库失败', 'error') }
  finally { saving.value = false }
}
function openStockOut() {
  Object.assign(stockOutForm, emptyStockOutDraft())
  showStockOut.value = true
}
// 入库记录 7 天内可改：流水行按入库时间判断，批次卡片由后端给的 editable 决定。
function within7Days(s?: string) {
  if (!s) return false
  return Date.now() - new Date(s).getTime() <= 7 * 24 * 3600 * 1000
}
function canEditMove(m: any) { return !!book.canMedicineEdit && m.type === 'in' && !!m.batch_id && within7Days(m.created_at) }
async function openEditBatch(batchId: number) {
  if (!batchId || !book.currentId) return
  try {
    const [res, list] = await Promise.all([
      medicineApi.getBatch(book.currentId, batchId),
      medicineApi.list(book.currentId, { pageSize: 200 }),
    ])
    editBatch.value = res.data?.data
    editMedicines.value = (list.data?.data?.items || []) as Medicine[]
    showEditBatch.value = true
  } catch (e: any) { ui.showToast(e?.response?.data?.message || '读取入库记录失败', 'error') }
}
async function onBatchSaved() {
  showEditBatch.value = false
  await load()
  await loadMoves(true)
}
function openStocktake() {
  Object.assign(stocktakeForm, emptyStocktakeDraft(Number(medicine.value?.stock_quantity || 0)))
  showStocktake.value = true
}
async function stockOut() {
  if (!medicine.value) return
  if (!(Number(stockOutForm.quantity) > 0)) { ui.showToast('出库数量必须大于 0', 'error'); return }
  saving.value = true
  try {
    await medicineApi.stockOut(book.currentId!, medicine.value.id, { ...stockOutForm })
    showStockOut.value = false
    ui.showToast('已出库 ' + stockOutForm.quantity + ' ' + medicine.value.unit)
    await load()
    if (tab.value === 'moves') await loadMoves(true)
  } catch (e: any) {
    ui.showToast(e?.response?.data?.message || '出库失败', 'error')
  } finally { saving.value = false }
}
async function stocktake() {
  if (!medicine.value) return
  saving.value = true
  try { await medicineApi.stocktake(book.currentId!, medicine.value.id, { target_quantity: stocktakeForm.target_quantity, reason: stocktakeForm.reason || '盘点' }); showStocktake.value = false; ui.showToast('盘点完成'); await load() }
  catch (e: any) { ui.showToast(e?.response?.data?.message || '盘点失败', 'error') }
  finally { saving.value = false }
}
function openTransfer() {
  if (!transferOptions.value.length) { ui.showToast('没有其它可转移的用药本（需你对目标本有管理权限）', 'error'); return }
  transferTarget.value = transferOptions.value[0].value
  showTransfer.value = true
}
async function doTransfer() {
  if (!medicine.value || !transferTarget.value) return
  saving.value = true
  try {
    await medicineApi.transfer(book.currentId!, medicine.value.id, { target_book_id: transferTarget.value })
    showTransfer.value = false
    ui.showToast('已转移到目标用药本')
    router.push('/medicines')
  } catch (e: any) { ui.showToast(e?.response?.data?.message || '转移失败', 'error') }
  finally { saving.value = false }
}
async function archive() {
  if (!medicine.value) return
  const m = medicine.value
  const done = await tryDeleteMedicine(ui, book.currentId!, m)
  if (!done) return
  ui.showToast('已删除')
  router.push('/medicines')
}
onMounted(load)
// 切换用药本后当前药品不在新本里，回到药品列表。
watch(() => book.currentId, () => { router.replace('/medicines') })
</script>

<style scoped>
.switch { position: relative; display: inline-flex; width: 44px; height: 26px; flex: 0 0 auto; }
.switch input { opacity: 0; width: 0; height: 0; }
.switch span { position: absolute; inset: 0; border-radius: 999px; background: var(--mc-surface-2); border: 1px solid var(--mc-border); transition: .15s; }
.switch span::after { content: ''; position: absolute; width: 20px; height: 20px; left: 2px; top: 2px; border-radius: 50%; background: #fff; box-shadow: 0 1px 4px rgba(0,0,0,.15); transition: .15s; }
.switch input:checked + span { background: var(--mc-primary); border-color: var(--mc-primary); }
.switch input:checked + span::after { transform: translateX(18px); }
.tab-row { margin-bottom: 12px; }
.editor-line { margin: 0 0 10px; line-height: 1.6; }
.stock-hero { background: linear-gradient(135deg, var(--mc-primary-soft), var(--mc-surface)); }
.stock-btn-row { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; }
.moves-actions { display: flex; flex-wrap: wrap; gap: 8px; margin-bottom: 10px; }
.stock-line { display: flex; align-items: flex-end; gap: 5px; margin-top: 2px; }
.stock-num { font-size: 28px; font-weight: 850; line-height: 1.1; }
.stock-num.stock-qty-ok { color: var(--mc-primary-strong); }
.stock-num.stock-qty-low { color: var(--mc-danger); animation: mc-blink 1.1s steps(1, end) infinite; }
.stock-unit { padding-bottom: 2px; }
.batch-card { padding: 10px 12px; }
.batch-head { display: flex; justify-content: space-between; gap: 8px; align-items: center; }
.batch-head b { min-width: 0; overflow-wrap: anywhere; }
.move-card { padding: 10px 12px; display: flex; justify-content: space-between; gap: 8px; align-items: flex-start; }
.move-body { min-width: 0; }
.move-title { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.move-title b { font-weight: 500; }
.move-delta { flex: 0 0 auto; }
@media (min-width: 900px) {
  .stock-num { font-size: 34px; }
  .batch-card { padding: 12px 14px; }
  .move-card { padding: 12px 14px; gap: 10px; }
}
</style>
