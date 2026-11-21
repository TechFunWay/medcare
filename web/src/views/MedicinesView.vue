<template>
  <div class="page">
    <div class="page-head">
      <div><div class="eyebrow">药品</div><h1>药品</h1><p>药品基础信息、库存与出入库都在这里。</p></div>
      <div class="head-actions">
        <button class="btn btn-outline btn-sm" @click="router.push('/prescriptions?from=medicines&purpose=archive')"><AppIcon name="prescription" :size="16" />识别记录</button>
      <button class="btn btn-outline btn-sm" @click="router.push('/stock')"><AppIcon name="filter" :size="16" />库存</button>
        <button v-if="book.canMedicineCreate" class="btn btn-primary btn-sm" @click="showAdd = true"><AppIcon name="plus" :size="16" />添加</button>
      </div>
    </div>
    <!-- 顶部统计：总数 + 按分类（点击可筛选） -->
    <div class="cat-stats">
      <button type="button" class="cat-stat" :class="{ active: category === '' }" @click="pickCategory('')">
        <b>{{ stats.total }}</b><span>全部</span>
      </button>
      <button v-for="c in catStats" :key="c.value" type="button" class="cat-stat" :class="{ active: category === c.value }" @click="pickCategory(c.value)">
        <b>{{ c.count }}</b><span>{{ c.label }}</span>
      </button>
    </div>
    <button v-if="book.canMedicineCreate" type="button" class="scan-banner" @click="showScan = true">
      <div class="scan-banner-icon"><AppIcon name="camera" :size="20" /></div>
      <div class="scan-banner-body"><b>AI 拍照识别药品</b><div class="tiny muted">拍药盒或缴费单，读出名称、规格、厂家，只添加药品资料；入库请到「库存」页</div></div>
      <AppIcon name="chevron" :size="16" />
    </button>
    <FilterPanel v-model:open="filtersOpen" :count="activeFilterCount" @search="load()" @reset="resetFilters">
      <template #search>
        <div class="card search-card">
          <AppIcon name="search" :size="19" class="search-icon" />
          <input v-model="query" class="input search-input" placeholder="搜索药品名、通用名、规格" @keyup.enter="load()" />
        </div>
      </template>
      <div class="chip-row">
        <button v-for="c in categories" :key="c.value" class="chip" :class="{ active: category === c.value }" @click="category = c.value">{{ c.label }}</button>
        <button class="chip" :class="{ active: lowOnly }" @click="lowOnly = !lowOnly">低库存</button>
      </div>
    </FilterPanel>
    <div class="sort-bar">
      <div class="sort-bar-left"><span class="tiny muted">共 {{ stats.total }} 种 · 排序</span></div>
      <div class="sort-select"><SheetSelect v-model="sort" placeholder="排序" :options="sortOptions" @update:model-value="load()" /></div>
    </div>
    <div v-if="loading" class="card muted">加载中…</div>
    <div v-else-if="!items.length" class="empty card">
      <div class="empty-icon"><AppIcon name="pill" :size="30" /></div>
      <h3>先把药品放进来</h3>
      <p>添加药品后可以入库、建计划，处方识别也能自动匹配。</p>
      <button v-if="book.canMedicineCreate" class="btn btn-primary" style="margin-top:12px" @click="showAdd = true">添加药品</button>
    </div>
    <div v-else class="list">
      <article v-for="m in shownItems" :key="m.id" class="card med-card" @click="router.push('/medicines/' + m.id)">
        <div v-if="m.cover_image_id && coverThumbs[m.id]" class="med-cover"><img :src="coverThumbs[m.id]" :alt="m.name" loading="lazy" /></div>
        <div v-else class="type-dot" :class="m.category"><AppIcon :name="m.category === 'injection' ? 'pill' : m.category === 'infusion' ? 'location' : 'pill'" :size="18" /></div>
        <div class="med-body">
          <!-- 药品页只看档案，不提示缺货/临期；库存状态统一在「库存」页看。 -->
          <div class="med-title-row"><b>{{ m.name }}</b><span class="med-tag">{{ categoryLabel(m.category) }}</span><span v-if="(dupNames.get(normName(m.name)) || 0) > 1" class="badge warn" :title="'列表里有 ' + dupNames.get(normName(m.name)) + ' 条同名药品'">重名 {{ dupNames.get(normName(m.name)) }}</span></div>
          <div class="muted small" style="margin-top:2px">{{ m.spec || '未填规格' }} · <span class="med-qty">{{ m.stock_quantity }} {{ m.unit }}</span><span v-if="m.unit_price"> · ¥{{ m.unit_price }}/{{ m.unit }}</span><span v-if="m.active_plan_count"> · {{ m.active_plan_count }} 个计划</span><span v-if="shortDate(m.updated_at)"> · 更新 {{ shortDate(m.updated_at) }}</span></div>
          <div v-if="m.nearest_expiry" class="tiny muted" style="margin-top:2px">最近有效期 {{ m.nearest_expiry }}</div>
        </div>
        <button v-if="book.canMedicineDelete" class="icon-btn med-del" aria-label="删除药品" title="删除药品" @click.stop="removeMed(m)"><AppIcon name="trash" :size="17" /></button>
        <AppIcon name="chevron" :size="18" class="med-chevron" />
      </article>
    </div>

    <MedicineScanSheet :open="showScan" :book-id="book.currentId || 0" :medicines="items" @close="showScan = false" @imported="onScanned" />

    <Teleport to="body">
      <div v-if="showAdd" class="sheet-backdrop" @click.self="showAdd = false">
        <section class="sheet" role="dialog" aria-modal="true">
          <div class="sheet-grab" />
          <div class="sheet-head"><h2>添加药品</h2><button class="icon-btn" aria-label="关闭" @click="showAdd=false"><AppIcon name="close" /></button></div>
          <form class="sheet-body" @submit.prevent="create">
            <button type="button" class="ai-entry" @click="showAdd = false; showScan = true">
              <div class="ai-entry-icon"><AppIcon name="sparkles" :size="20" /></div>
              <div class="ai-entry-body"><b>AI 拍照识别</b><div class="tiny muted">拍药盒或缴费单，只自动填好药品资料（名称/规格/厂家），不入库</div></div>
              <AppIcon name="chevron" :size="16" />
            </button>
            <div class="field"><label>药品名称</label><input v-model="form.name" class="input" required placeholder="例如：氨氯地平片" /></div>
            <div class="field-row"><div class="field"><label>分类</label><select v-model="form.category" class="select"><option value="oral">口服</option><option value="injection">注射</option><option value="infusion">输液</option><option value="topical">外用</option><option value="other">其他</option></select></div><div class="field"><label>剂型</label><SheetSelect v-model="form.form" placeholder="剂型" :options="formOptions()" searchable allow-custom custom-label="没有合适的剂型？自己填" /></div></div>
            <div class="field-row"><div class="field"><label>规格</label><SheetSelect v-model="form.spec" placeholder="规格" :options="specOptions()" searchable allow-custom custom-label="没有合适的规格？自己填" custom-placeholder="例如：0.2g×10片" /></div><div class="field"><label>库存单位</label><SheetSelect v-model="form.unit" placeholder="库存单位" :options="unitOptions()" searchable allow-custom custom-label="没有合适的单位？自己填" /></div></div>
            <div class="field-row"><div class="field"><label>单价（元）</label><input v-model.number="form.unit_price" type="number" min="0" step="0.01" class="input" placeholder="可选，例如 3.50" /></div><div class="field"><label>低库存阈值</label><input v-model.number="form.low_stock_threshold" type="number" min="0" class="input" /></div></div>
            <div class="field" style="display:flex;justify-content:space-between;align-items:center;gap:12px">
              <div><label style="margin:0">库存不足提醒</label><div class="tiny muted" style="margin-top:2px">关掉后这个药低于阈值或缺货都不再提醒补货</div></div>
              <label class="switch"><input v-model="form.low_stock_notify_enabled" type="checkbox" /><span /></label>
            </div>
            <div class="field"><label>临期预警天数</label><input v-model.number="form.expiry_warn_days" type="number" min="1" class="input" /></div>
            <div class="field"><label>备注</label><textarea v-model="form.notes" class="textarea" /></div>
            <div class="sheet-foot"><button type="button" class="btn btn-outline btn-block" @click="showAdd=false">取消</button><button class="btn btn-primary btn-block" :disabled="saving">{{ saving ? '保存中…' : '保存' }}</button></div>
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
import MedicineScanSheet from '../components/MedicineScanSheet.vue'
import SheetSelect from '../components/SheetSelect.vue'
import FilterPanel from '../components/FilterPanel.vue'
import { medicineApi, type Medicine } from '../api/medcare'
import { useBookStore } from '../stores/book'
import { logOnly } from '../utils/errors'
import { useUIStore } from '../stores/ui'
import { useScanStore } from '../stores/scan'
import { loadThumb } from '../utils/image'
import { tryDeleteMedicine } from '../utils/medicine'
import { formOptions, specOptions, unitOptions } from '../utils/medicineMeta'
const router = useRouter()
const route = useRoute()
const book = useBookStore()
const ui = useUIStore()
const items = ref<Medicine[]>([])
const loading = ref(true)
const query = ref('')
const category = ref('')
const lowOnly = ref(false)
const filtersOpen = ref(false)
// 折叠态只留搜索框；筛选条数统计分类与低库存两个条件。
const activeFilterCount = computed(() => (category.value ? 1 : 0) + (lowOnly.value ? 1 : 0))
// 列表排序：默认按最近更新；名称换成拼音序更符合中文习惯。
type MedSort = 'default' | 'name' | 'name_desc' | 'created' | 'stock' | 'stock_desc'
const sort = ref<MedSort>('default')
const sortOptions: { value: MedSort; label: string }[] = [
  { value: 'default', label: '默认（最近更新）' },
  { value: 'name', label: '名称 A→Z' },
  { value: 'name_desc', label: '名称 Z→A' },
  { value: 'created', label: '最近添加' },
  { value: 'stock', label: '库存少→多' },
  { value: 'stock_desc', label: '库存多→少' },
]
function resetFilters() {
  sort.value = 'default'
  query.value = ''; category.value = ''; lowOnly.value = false; load()
}
const showAdd = ref(false)
const showScan = ref(false)
// 后台识别胶囊点「去核对」：无论在不在本页都能把弹层叫起来。
const scan = useScanStore()
watch(() => scan.requestOpen, (v) => { if (v) { showScan.value = true; scan.requestOpen = false } })
const saving = ref(false)
const form = reactive({ name: '', category: 'oral', form: '', spec: '', unit: '片', low_stock_threshold: 0, low_stock_notify_enabled: true, expiry_warn_days: 30, unit_price: 0, notes: '' })
const categories = [{ value: '', label: '全部' }, { value: 'oral', label: '口服' }, { value: 'injection', label: '注射' }, { value: 'infusion', label: '输液' }, { value: 'topical', label: '外用' }, { value: 'other', label: '其他' }]
const CATEGORY_LABELS: Record<string, string> = { oral: '口服', injection: '注射', infusion: '输液', topical: '外用', other: '其他' }
function categoryLabel(c: string) { return CATEGORY_LABELS[c] || '其他' }
// 最后更新时间只显示到日，列表里够用；时间放在 meta 行尾部。
function shortDate(v?: string) {
  const d = String(v || '').slice(0, 10)
  return d ? d.replace(/-/g, '/') : ''
}
// 顶部统计：总数与按分类数量（后端按在管药品聚合，不受分页影响）。
const stats = reactive({ total: 0, oral: 0, injection: 0, infusion: 0, topical: 0, other: 0 })
const catStats = computed(() => [
  { value: 'oral', label: '口服', count: stats.oral },
  { value: 'injection', label: '注射', count: stats.injection },
  { value: 'infusion', label: '输液', count: stats.infusion },
  { value: 'topical', label: '外用', count: stats.topical },
  { value: 'other', label: '其他', count: stats.other },
])
function pickCategory(v: string) { category.value = category.value === v ? '' : v; load() }
async function loadStats() {
  if (!book.currentId) return
  try { const res = await medicineApi.stats(book.currentId); if (res.data?.code === 0) Object.assign(stats, res.data.data) } catch (e) { logOnly('读取药品统计', e) }
}
let timer: number | null = null
function stockBadge(m: Medicine) { return m.stock_status === 'out' || m.stock_status === 'low' || m.stock_status === 'expired' ? 'missed' : m.stock_status === 'expiring' ? 'warn' : 'done' }
function isLowStock(m: Medicine) { return m.stock_status === 'out' || m.stock_status === 'low' }
function stockQtyClass(m: Medicine) { return isLowStock(m) ? 'low' : 'ok' }
function stockLabel(m: Medicine) { return m.stock_status === 'out' ? '缺货' : m.stock_status === 'low' ? '库存不足' : m.stock_status === 'expired' ? '已过期' : m.stock_status === 'expiring' ? '临期' : '正常' }
// 名称排序在浏览器里按中文拼音比较（SQLite 的 ORDER BY name 是字节序，排不出拼音顺序）。
const shownItems = computed(() => {
  if (sort.value !== 'name' && sort.value !== 'name_desc') return items.value
  const dir = sort.value === 'name_desc' ? -1 : 1
  return items.value.slice().sort((a, b) => dir * String(a.name || '').localeCompare(String(b.name || ''), 'zh-Hans-CN'))
})
async function load() {
  if (!book.currentId) return
  loading.value = true
  try { const res = await medicineApi.list(book.currentId, { query: query.value, category: category.value, stock_status: lowOnly.value ? 'low' : '', sort: sort.value === 'name' || sort.value === 'name_desc' ? 'default' : sort.value, pageSize: 100 }); items.value = res.data?.data?.items || []; loadCovers(); void loadStats() }
  catch (e: any) { ui.showToast(e?.response?.data?.message || '读取药品失败', 'error') }
  finally { loading.value = false }
}
function debouncedLoad() { if (timer) window.clearTimeout(timer); timer = window.setTimeout(load, 250) }
// 封面缩略图：只给设了封面的药品取 420px 缩略图，取到缓存起来，不阻塞列表渲染。
const coverThumbs = reactive<Record<number, string>>({})
function loadCovers() {
  const bookId = book.currentId
  if (!bookId) return
  for (const m of items.value) {
    if (!m.cover_image_id || coverThumbs[m.id]) continue
    loadThumb(bookId, m.cover_image_id).then((url) => { coverThumbs[m.id] = url }).catch(() => { /* 单张失败不影响列表 */ })
  }
}
async function create() {
  saving.value = true
  try { await medicineApi.create(book.currentId!, { ...form }); showAdd.value = false; Object.assign(form, { name: '', category: 'oral', form: '', spec: '', unit: '片', low_stock_threshold: 0, low_stock_notify_enabled: true, expiry_warn_days: 30, unit_price: 0, notes: '' }); ui.showToast('药品已添加'); load() }
  catch (e: any) { ui.showToast(e?.response?.data?.message || '保存失败', 'error') }
  finally { saving.value = false }
}
function onScanned() { showScan.value = false; load() }
// 同名药品统计：AI 多次识别可能建出重复条目，标出来方便清理。
function normName(s: string) { return String(s || '').toLowerCase().replace(/\s+/g, '') }
const dupNames = computed(() => {
  const map = new Map<string, number>()
  for (const m of items.value) {
    const k = normName(m.name)
    if (!k) continue
    map.set(k, (map.get(k) || 0) + 1)
  }
  return map
})
// 删除（归档）药品：先查绑定关系（计划 / 库存 / 出入库），有绑定就不让删，只说明原因。
async function removeMed(m: Medicine) {
  const done = await tryDeleteMedicine(ui, book.currentId!, m)
  if (!done) return
  ui.showToast('已删除「' + m.name + '」')
  await load()
}
onMounted(() => {
  load()
  // 从处方页「去拍照识别」跳过来时直接打开扫码面板。
  if (route.query.scan === '1' && book.canMedicineCreate) showScan.value = true
})
watch(() => book.currentId, load)
</script>

<style scoped>
.switch { position: relative; display: inline-flex; width: 44px; height: 26px; flex: 0 0 auto; }
.switch input { opacity: 0; width: 0; height: 0; }
.switch span { position: absolute; inset: 0; border-radius: 999px; background: var(--mc-surface-2); border: 1px solid var(--mc-border); transition: .15s; }
.switch span::after { content: ''; position: absolute; width: 20px; height: 20px; left: 2px; top: 2px; border-radius: 50%; background: #fff; box-shadow: 0 1px 4px rgba(0,0,0,.15); transition: .15s; }
.switch input:checked + span { background: var(--mc-primary); border-color: var(--mc-primary); }
.switch input:checked + span::after { transform: translateX(18px); }
.head-actions { display: flex; gap: 8px; flex: 0 0 auto; align-items: center; }
.sort-bar { display: flex; align-items: center; justify-content: space-between; gap: 10px; margin: 0 0 8px; }
.sort-bar-left { display: inline-flex; align-items: center; gap: 6px; color: var(--mc-text-muted); flex: 1 1 auto; min-width: 0; }
.sort-select { flex: 0 0 150px; max-width: 150px; }
.sort-select :deep(.select-like) { min-height: 36px; height: 36px; font-size: 13px; }
/* 分类统计做成一行小卡：宽度与底部导航一致、高度收到 44 左右，不抢页面视觉重心。 */
.cat-stats { display: grid; grid-template-columns: repeat(6, minmax(0, 1fr)); gap: 6px; margin-bottom: 10px; }
.cat-stat { display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 0; min-height: 44px; padding: 5px 2px; border-radius: 10px; border: 1px solid var(--mc-border); background: var(--mc-surface); }
.cat-stat b { font-size: 14.5px; line-height: 1.15; color: var(--mc-text); }
.cat-stat span { font-size: 10.5px; line-height: 1.15; color: var(--mc-text-muted); font-weight: 650; }
.cat-stat.active { border-color: var(--mc-primary); background: var(--mc-primary-soft); }
.cat-stat.active b { color: var(--mc-primary-strong); }
.scan-banner { display: flex; align-items: center; gap: 10px; width: 100%; min-height: 58px; margin-bottom: 10px; padding: 10px 12px; border-radius: 14px; border: 1px solid var(--mc-border); background: linear-gradient(135deg, var(--mc-primary-soft), var(--mc-surface)); text-align: left; }
.scan-banner-icon { flex: 0 0 auto; width: 36px; height: 36px; border-radius: 12px; display: inline-flex; align-items: center; justify-content: center; background: var(--mc-surface); color: var(--mc-primary); }
.scan-banner-body { flex: 1; min-width: 0; }
.scan-banner-body b { font-size: 14px; }
.ai-entry { display: flex; align-items: center; gap: 10px; width: 100%; min-height: 56px; margin-bottom: 12px; padding: 10px 12px; border-radius: 14px; border: 1px solid var(--mc-primary); background: var(--mc-primary-soft); text-align: left; }
.ai-entry-icon { flex: 0 0 auto; width: 34px; height: 34px; border-radius: 12px; display: inline-flex; align-items: center; justify-content: center; background: var(--mc-surface); color: var(--mc-primary); }
.ai-entry-body { flex: 1; min-width: 0; }
.ai-entry-body b { font-size: 14px; }
.search-card { display: flex; gap: 8px; align-items: center; margin-bottom: 10px; padding: 4px 12px; }
.search-icon { color: var(--mc-text-muted); flex: 0 0 auto; }
.search-input { flex: 1 1 auto; min-width: 0; border: 0; background: transparent; padding: 0; min-height: 44px; }
.med-card { display: flex; gap: 10px; align-items: center; cursor: pointer; }
.med-body { flex: 1; min-width: 0; }
.dot-sep { margin: 0 5px; color: var(--mc-text-muted); }
.med-tag { display: inline-block; padding: 1px 6px; margin-right: 6px; border-radius: 999px; background: var(--mc-surface-2); color: var(--mc-text-muted); font-size: 11px; font-weight: 650; }
.med-qty { color: var(--mc-text-muted); font-weight: 600; }
.med-title-row { display: flex; gap: 6px; align-items: center; flex-wrap: wrap; min-width: 0; }
.med-title-row b { font-size: 14.5px; overflow-wrap: anywhere; }
.med-chevron { flex: 0 0 auto; }
.med-cover { flex: 0 0 auto; width: 42px; height: 42px; border-radius: 12px; overflow: hidden; border: 1px solid var(--mc-border); background: var(--mc-surface-2); }
.med-cover img { width: 100%; height: 100%; object-fit: cover; display: block; }
@media (min-width: 900px) { .med-cover { width: 48px; height: 48px; } }
@media (min-width: 900px) {
  .ai-entry { min-height: 60px; padding: 12px 14px; }
  .ai-entry-icon { width: 38px; height: 38px; }
  .ai-entry-body b { font-size: 14.5px; }
  .scan-banner { min-height: 64px; padding: 12px 14px; gap: 12px; margin-bottom: 12px; }
  .scan-banner-icon { width: 40px; height: 40px; border-radius: 13px; }
  .scan-banner-body b { font-size: 14.5px; }
  .search-card { margin-bottom: 12px; padding: 0 14px; }
  .med-card { gap: 12px; }
  .med-title-row { gap: 8px; }
  .med-title-row b { font-size: 15px; }
}
</style>
