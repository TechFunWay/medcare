<template>
  <Teleport to="body">
    <div v-if="open" class="sheet-backdrop" @click.self="close">
      <section class="sheet" role="dialog" aria-modal="true" :aria-label="isStock ? 'AI 识别入库' : 'AI 识别添加药品'">
        <div class="sheet-grab" />
        <div class="sheet-head">
          <h2>{{ step === 'pick' ? (isStock ? 'AI 识别入库' : 'AI 识别添加药品') : step === 'review' ? (isStock ? '核对后入库' : '核对后添加') : '识别中' }}</h2>
          <button class="icon-btn" aria-label="关闭" @click="close"><AppIcon name="close" /></button>
        </div>

        <div class="sheet-body">
          <template v-if="step === 'pick'">
            <div class="scan-hero">
              <div class="scan-icon"><AppIcon name="sparkles" :size="24" /></div>
              <p v-if="isStock" class="muted small">对着缴费单 / 发票拍照，AI 读出药品与数量，核对后直接入库（建批次并记出入库流水）。</p>
              <p v-else class="muted small">拍药盒或缴费单都行：AI 读出名称、规格、厂家、有效期等基础信息。这里只添加药品资料，入库请到「库存」页。</p>
            </div>
            <button type="button" class="scan-kind" :class="{ active: kind === 'box' }" @click="start('box')">
              <div class="scan-kind-icon"><AppIcon name="pill" :size="20" /></div>
              <div class="scan-kind-body"><b>{{ isStock ? '扫描药品盒子（也可入库）' : '扫描药品盒子' }}</b><div class="tiny muted">{{ isStock ? '拍药盒读出名称、规格、数量，可直接入库' : '拍药盒读出名称、规格、厂家、有效期，只建档不入库' }}</div></div>
              <AppIcon name="chevron" :size="16" />
            </button>
            <button type="button" class="scan-kind" :class="{ active: kind === 'receipt' }" @click="start('receipt')">
              <div class="scan-kind-icon"><AppIcon name="prescription" :size="20" /></div>
              <div class="scan-kind-body"><b>{{ isStock ? '扫描缴费单' : '扫描缴费单（只建档）' }}</b><div class="tiny muted">{{ isStock ? '一张单子多种药品，逐条列出后可勾选批量入库' : '一张单子多种药品，逐条列出后勾选添加药品资料，不入库' }}</div></div>
              <AppIcon name="chevron" :size="16" />
            </button>
            <label class="scan-save"><input v-model="saveImage" type="checkbox" />保存原图到用药本（加密，随备份）</label>
            <p class="tiny muted" style="margin-top:8px"><button type="button" class="link-btn" @click="router.push('/prescriptions'); close()">查看识别记录与状态</button>（识别中 / 待确认 / 已录入 / 失败，可编辑、删除、重试）</p>
            <p class="tiny muted" style="line-height:1.65;margin-top:6px">图片会发送到管理员配置的 AI 服务；默认只保存识别结果，不保存原图。</p>
          </template>

          <template v-else-if="step === 'loading'">
            <div class="scan-loading">
              <div class="scan-spinner" />
              <b>{{ loadingTitle }}</b>
              <p class="tiny muted">通常 20–60 秒。可以先关掉本窗：识别在后台继续，完成后左下角会提醒；也可在「{{ isStock ? '库存' : '药品' }}」页的「识别记录」看进度。</p>
            </div>
          </template>

          <template v-else-if="step === 'review'">
            <div class="review-bar">
              <span class="badge done">已识别 {{ items.length }} 种</span>
              <button type="button" class="link-btn" @click="toggleAll">{{ allChecked ? '全不选' : '全选' }}</button>
            </div>
            <div v-if="isStock" class="field channel-field"><label>入库来源</label><SheetSelect v-model="channel" placeholder="选择来源" :options="channelOptions" /></div>
            <div v-if="isStock" class="field-row">
              <div class="field"><label>批次日期</label><DateField v-model="batchDate" placeholder="入库日期" /></div>
              <div class="field"><label>批次时间（时:分:秒）</label><TimeField v-model="batchTime" placeholder="时间" with-seconds /></div>
            </div>
            <p v-if="isStock" class="tiny muted" style="margin:-2px 0 8px">本次识别到的药品默认同一入库批次；单条可在「修改」里改自己的批次时间。</p>
            <div class="list">
              <article v-for="(item, index) in items" :key="index" class="card scan-item" :class="{ off: !item.checked }">
                <div class="scan-item-head">
                  <button type="button" class="scan-check" :class="{ on: item.checked }" :aria-label="item.checked ? '取消选择' : '选择'" @click="item.checked = !item.checked">
                    <AppIcon v-if="item.checked" name="check" :size="16" />
                  </button>
                  <b>{{ item.name || item.raw_name || '未命名药品' }}</b>
                  <button type="button" class="link-btn" @click="item.expanded = !item.expanded">{{ item.expanded ? '收起' : '修改' }}</button>
                  <button type="button" class="icon-btn scan-del" aria-label="移除" @click="items.splice(index, 1)"><AppIcon name="trash" :size="16" /></button>
                </div>
                <div class="scan-item-meta">
                  <span v-if="item.needs_review" class="badge warn">请核对</span>
                  <span v-if="item.existing" class="badge done">同名合并</span>
                  <span>{{ item.spec || '未填规格' }}</span>
                  <span class="dot">·</span>
                  <span v-if="isStock" class="qty">{{ fmtQty(item.quantity) }} {{ item.unit || '盒' }}</span>
                  <span v-else class="qty">只建档 · 不入库</span>
                  <template v-if="item.manufacturer"><span class="dot">·</span><span>{{ item.manufacturer }}</span></template>
                  <template v-if="item.expiry_date"><span class="dot">·</span><span>有效期 {{ item.expiry_date.replace(/-/g, '/') }}</span></template>
                </div>
                <div v-if="item.expanded" class="scan-item-edit">
                  <div class="field"><label>药品名称</label><input v-model="item.name" class="input" placeholder="例如：氨氯地平片" /></div>
                  <div class="field-row">
                    <div class="field"><label>分类</label><SheetSelect v-model="item.category" placeholder="分类" :options="categoryOptions" /></div>
                    <div class="field"><label>剂型</label><SheetSelect v-model="item.form" placeholder="剂型" :options="formOptions()" searchable allow-custom custom-label="没有合适的剂型？自己填" /></div>
                  </div>
                  <div class="field-row">
                    <div class="field"><label>规格</label><SheetSelect v-model="item.spec" placeholder="规格" :options="specOptions()" searchable allow-custom custom-label="没有合适的规格？自己填" custom-placeholder="例如：0.2g×10片" /></div>
                    <div class="field"><label>库存单位</label><SheetSelect v-model="item.unit" placeholder="库存单位" :options="unitOptions()" searchable allow-custom custom-label="没有合适的单位？自己填" /></div>
                  </div>
                  <div class="field-row">
                    <div v-if="isStock" class="field"><label>入库数量</label><input v-model.number="item.quantity" type="number" min="0" step="0.01" class="input" /></div>
                    <div v-else class="field"><label>低库存阈值</label><input v-model.number="item.low_stock_threshold" type="number" min="0" class="input" /></div>
                    <div v-if="!isStock" class="field"><label>临期预警天数</label><input v-model.number="item.expiry_warn_days" type="number" min="1" class="input" /></div>
                    <div v-else class="field"><label>有效期（可空）</label><DateField v-model="item.expiry_date" placeholder="选择日期" clearable /></div>
                  </div>
                  <div v-if="isStock" class="field-row">
                    <div class="field"><label>批次日期</label><DateField v-model="item.receivedDate" placeholder="同整批" clearable /></div>
                    <div class="field"><label>批次时间</label><TimeField v-model="item.receivedTime" placeholder="同整批" with-seconds /></div>
                  </div>
                  <div v-if="!isStock" class="field-row">
                    <div class="field"><label>包装规格（可空）</label><input v-model="item.package_spec" class="input" placeholder="例如：10片/盒" /></div>
                    <div class="field"><label>生产企业（可空）</label><input v-model="item.manufacturer" class="input" /></div>
                  </div>
                  <div v-else class="field"><label>生产企业（可空）</label><input v-model="item.manufacturer" class="input" /></div>
                  <div v-if="medicines.length" class="field"><label>合并到已有药品（可空）</label><SheetSelect v-model="item.existing" placeholder="新建药品" :options="mergeOptions(item)" searchable /></div>
                </div>
              </article>
            </div>
            <p v-if="error" class="tiny" style="color:var(--mc-danger);margin-top:10px">{{ error }}</p>
          </template>

          <p v-if="step !== 'review' && error" class="scan-error">{{ error }}</p>
        </div>

        <div class="sheet-foot">
          <template v-if="step === 'review'">
            <button type="button" class="btn btn-outline" :disabled="importing" @click="close">取消</button>
            <button type="button" class="btn btn-primary" :disabled="importing || !checkedItems.length" @click="submit">{{ importing ? '处理中…' : (isStock ? '入库 ' + checkedItems.length + ' 种' : '添加 ' + checkedItems.length + ' 种药品') }}</button>
          </template>
          <template v-else-if="step === 'loading'">
            <button type="button" class="btn btn-outline btn-block" @click="close">后台运行（关窗继续识别）</button>
          </template>
          <template v-else>
            <button type="button" class="btn btn-outline btn-block" @click="close">取消</button>
          </template>
        </div>
      </section>
    </div>
  </Teleport>
  <input ref="fileInput" type="file" accept="image/jpeg,image/png,image/webp" style="display:none" @change="onFile" />
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import AppIcon from './AppIcon.vue'
import SheetSelect from './SheetSelect.vue'
import DateField from './DateField.vue'
import TimeField from './TimeField.vue'
import { medicineApi, type Medicine } from '../api/medcare'
import { fixUnit, formOptions, specOptions, unitOptions } from '../utils/medicineMeta'
import { STOCK_CHANNEL_OPTIONS } from '../utils/stock'
import { useUIStore } from '../stores/ui'
import { useScanStore } from '../stores/scan'

const props = withDefaults(defineProps<{ open: boolean; bookId: number; medicines: Medicine[]; mode?: 'medicine' | 'stock' }>(), { mode: 'medicine' })
// mode=medicine：药品页用，只识别并添加药品基础信息，不建批次、不入库。
// mode=stock：库存页用，识别缴费单后直接入库（建批次 + 流水）。
const isStock = computed(() => props.mode === 'stock')
const emit = defineEmits<{ (e: 'close'): void; (e: 'imported', count: number): void }>()

const ui = useUIStore()
const scan = useScanStore()
const router = useRouter()
// 归一化药名用于同名合并提示（与服务端口径一致：去空白、小写）。
function normName(s: string) { return String(s || '').toLowerCase().replace(/\s+/g, '') }
// 当前用药本最近的识别任务：弹层只是它的视图，关窗 / 切页任务仍在 store 里跑。
const job = computed(() => scan.latestFor(props.bookId))
const fileInput = ref<HTMLInputElement | null>(null)
const kind = ref<'box' | 'receipt'>('box')
const step = ref<'pick' | 'loading' | 'review'>('pick')
const saveImage = ref(false)
// 入库批次时间：整批默认同一个，逐条可在编辑区覆盖。
function localDateStr() {
  const d = new Date()
  const p = (n: number) => String(n).padStart(2, '0')
  return d.getFullYear() + '-' + p(d.getMonth() + 1) + '-' + p(d.getDate())
}
const batchDate = ref(localDateStr())
const batchTime = ref(new Date().toTimeString().slice(0, 8))
const channel = ref('pharmacy')
const channelOptions = STOCK_CHANNEL_OPTIONS
const error = ref('')
const importing = ref(false)
const items = ref<any[]>([])

const categoryOptions = [
  { value: 'oral', label: '口服' },
  { value: 'injection', label: '注射' },
  { value: 'infusion', label: '输液' },
  { value: 'topical', label: '外用' },
  { value: 'other', label: '其他' },
]
// 加载中的标题跟着「模式 + 单据类型」走：库存页拍缴费单说缴费单，药品页拍药盒说药盒。
const loadingTitle = computed(() => {
  if (kind.value === 'receipt') return isStock.value ? '正在读取缴费单，准备入库' : '正在读取缴费单上的药品'
  return isStock.value ? '正在读取药盒信息，准备入库' : '正在读取药盒信息'
})
const checkedItems = computed(() => items.value.filter((i) => i.checked && String(i.name || '').trim()))
const allChecked = computed(() => items.value.length > 0 && items.value.every((i) => i.checked))

function mergeOptions(item: any) {
  return [
    { value: 0, label: '新建药品' },
    ...props.medicines.map((m) => ({ value: m.id, label: '合并到：' + m.name + ' · ' + (m.spec || m.unit) })),
  ]
}
function toggleAll() { const next = !allChecked.value; items.value.forEach((i) => { i.checked = next }) }
function fmtQty(v: any) { const n = Number(v) || 0; return Number.isInteger(n) ? String(n) : String(Math.round(n * 100) / 100) }
function close() { if (importing.value) return; emit('close') }
function reset() { step.value = 'pick'; error.value = ''; items.value = []; importing.value = false }

function mapDraft(draft: any) {
  items.value = (draft?.items || []).map((it: any) => {
    const name = it.name || it.raw_name || ''
    // 库里已有同名药品：默认合并补库存，避免重复建档（服务端也会强制查重）。
    const hit = props.medicines.find((m) => normName(m.name) === normName(name))
    return {
      checked: true,
      raw_name: it.raw_name || '',
      name,
      generic_name: it.generic_name || '',
      category: it.category || it.route || 'oral',
      form: it.form || '',
      spec: it.spec || '',
      unit: fixUnit(it.unit || it.dose_unit, it.form, it.category || it.route, it.spec),
      package_spec: it.package_spec || '',
      manufacturer: it.manufacturer || '',
      notes: it.instructions || '',
      quantity: Number(it.quantity) || 0,
      low_stock_threshold: Number(it.low_stock_threshold) || 0,
      expiry_warn_days: Number(it.expiry_warn_days) || 30,
      expiry_date: it.expiry_date || '',
      existing: hit ? hit.id : 0,
      expanded: false,
      needs_review: !!it.needs_review,
    }
  })
}
// 弹层状态完全跟随 store 里的任务：开着时任务跑完自动进核对；
// 重开时按任务当前状态恢复（识别中 / 已识别 / 失败），不再一律重置丢结果。
function syncFromJob() {
  const j = job.value
  if (!j) { reset(); return }
  if (j.status === 'running') {
    // 后台任务可能来自缴费单：先把类型同步过来，加载文案才不会一律说「读取药盒」。
    kind.value = j.kind
    step.value = 'loading'
    error.value = ''
    return
  }
  if (j.status === 'done') {
    kind.value = j.kind
    channel.value = j.kind === 'receipt' ? 'hospital' : 'pharmacy'
    mapDraft(j.draft)
    if (items.value.length) { step.value = 'review'; error.value = '' }
    else { step.value = 'pick'; error.value = '没有识别到药品，请换一张更清晰的图片' }
    return
  }
  step.value = 'pick'
  error.value = j.error
}
watch(() => props.open, (v) => {
  scan.sheetOpen = v
  if (v) syncFromJob()
})
watch([() => job.value?.id, () => job.value?.status], () => { if (props.open) syncFromJob() })
// 切页把弹层组件卸载时也要把「弹层开着」标记放下，否则后台胶囊会被一直压住。
onUnmounted(() => { scan.sheetOpen = false })

function start(which: 'box' | 'receipt') {
  kind.value = which
  // 药盒多为药店购买，缴费单多为医院开具，默认值可再改。
  channel.value = which === 'receipt' ? 'hospital' : 'pharmacy'
  error.value = ''
  fileInput.value?.click()
}

async function onFile(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (fileInput.value) fileInput.value.value = ''
  if (!file) return
  step.value = 'loading'
  error.value = ''
  try {
    const fd = new FormData()
    fd.append('file', file)
    fd.append('kind', kind.value)
    fd.append('save_image', saveImage.value ? 'true' : 'false')
    // 用途：药品页只建档（archive）／库存页入库（stock），历史里据此区分。
    fd.append('intent', isStock.value ? 'stock' : 'archive')
    // 上传后立即返回任务号；轮询与结果都在 store 里，关窗 / 切页不丢。
    await scan.start(props.bookId, kind.value, fd, props.mode)
  } catch (err: any) {
    step.value = 'pick'
    error.value = err?.response?.data?.message || err?.message || '识别失败，请重试或手动录入'
  }
}

async function submit() {
  if (importing.value) return
  const list = checkedItems.value
  if (!list.length) return
  for (const item of list) {
    if (!String(item.name || '').trim()) { error.value = '药品名称不能为空'; return }
  }
  importing.value = true
  error.value = ''
  try {
    const payload = {
      source: kind.value,
      channel: channel.value,
      import_id: job.value?.id || 0,
      items: list.map((i) => ({
        name: String(i.name).trim(),
        generic_name: i.generic_name || '',
        category: i.category || 'other',
        form: i.form || '',
        spec: i.spec || '',
        unit: i.unit || '盒',
        package_spec: i.package_spec || '',
        manufacturer: i.manufacturer || '',
        notes: i.notes || '',
        // 药品页只建档：数量一律 0，服务端据此不建批次、不写库存。
        quantity: isStock.value ? (Number(i.quantity) || 0) : 0,
        expiry_date: i.expiry_date || '',
        low_stock_threshold: Number(i.low_stock_threshold) || 0,
        expiry_warn_days: Number(i.expiry_warn_days) || 30,
        // 入库批次时间：单条优先，否则用整批设置。
        received_at: isStock.value
          ? (i.receivedDate || batchDate.value) + 'T' + (i.receivedTime || batchTime.value) + '+08:00'
          : '',
        existing_medicine_id: Number(i.existing) || 0,
      })),
    }
    const res = await medicineApi.importBatch(props.bookId, payload)
    if (res.data?.code !== 0) throw new Error(res.data?.message || '导入失败')
    const count = res.data?.data?.count || list.length
    const merged = Number(res.data?.data?.merged) || 0
    if (isStock.value) {
      ui.showToast(merged > 0 ? '已入库 ' + count + ' 种（' + merged + ' 种同名合并补库存）' : '已入库 ' + count + ' 种')
    } else {
      ui.showToast(merged > 0 ? '已添加 ' + count + ' 种药品资料（' + merged + ' 种同名已存在）' : '已添加 ' + count + ' 种药品资料')
    }
    if (job.value) scan.clear(job.value.id)
    emit('imported', count)
    reset()
  } catch (err: any) {
    error.value = err?.response?.data?.message || err?.message || '导入失败'
  } finally {
    importing.value = false
  }
}
</script>

<style scoped>
.scan-hero { display: flex; gap: 10px; align-items: flex-start; margin-bottom: 12px; }
.scan-icon { flex: 0 0 auto; width: 44px; height: 44px; border-radius: 15px; display: inline-flex; align-items: center; justify-content: center; background: var(--mc-primary-soft); color: var(--mc-primary); }
.scan-kind { display: flex; align-items: center; gap: 10px; width: 100%; min-height: 64px; padding: 10px 12px; margin-bottom: 8px; border-radius: 14px; border: 1px solid var(--mc-border); background: var(--mc-surface-2); text-align: left; }
.scan-kind.active { border-color: var(--mc-primary); background: var(--mc-primary-soft); }
.scan-kind-icon { flex: 0 0 auto; width: 38px; height: 38px; border-radius: 13px; display: inline-flex; align-items: center; justify-content: center; background: var(--mc-surface); color: var(--mc-primary); }
.scan-kind-body { flex: 1; min-width: 0; }
.scan-kind-body b { font-size: 14px; }
.scan-save { display: flex; align-items: center; gap: 8px; margin-top: 12px; font-size: 12px; color: var(--mc-text-muted); }
.scan-error { margin-top: 10px; padding: 10px 12px; border-radius: 12px; font-size: 12.5px; color: var(--mc-danger); background: var(--mc-danger-soft); }
.scan-loading { display: grid; justify-items: center; gap: 8px; padding: 34px 0; text-align: center; }
.scan-spinner { width: 34px; height: 34px; border-radius: 50%; border: 3px solid var(--mc-primary-soft); border-top-color: var(--mc-primary); animation: scan-spin .9s linear infinite; }
@keyframes scan-spin { to { transform: rotate(360deg); } }
.channel-field { margin-bottom: 8px; }
.review-bar { display: flex; align-items: center; justify-content: space-between; gap: 8px; margin-bottom: 8px; }
.link-btn { padding: 0; border: 0; background: transparent; color: var(--mc-primary); font-size: 11.5px; font-weight: 700; text-decoration: underline; text-underline-offset: 2px; white-space: nowrap; }
.scan-item { padding: 12px; }
.scan-item.off { opacity: .55; }
.scan-item-head { display: flex; align-items: center; gap: 8px; margin-bottom: 4px; min-width: 0; }
.scan-item-head b { flex: 1; min-width: 0; font-size: 14px; overflow-wrap: anywhere; }
.scan-check { flex: 0 0 auto; width: 26px; height: 26px; border-radius: 9px; border: 1.5px solid var(--mc-border); background: var(--mc-surface); display: inline-flex; align-items: center; justify-content: center; color: var(--mc-primary); }
.scan-check.on { border-color: var(--mc-primary); background: var(--mc-primary-soft); }
.scan-del { flex: 0 0 auto; width: 30px; height: 30px; }
.scan-item-meta { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; padding-left: 34px; font-size: 12px; color: var(--mc-text-muted); }
.scan-item-meta .dot { color: var(--mc-border); }
.scan-item-meta .qty { color: var(--mc-text); font-weight: 700; }
.scan-item-edit { margin-top: 10px; padding-top: 10px; border-top: 1px dashed var(--mc-border); }
@media (min-width: 900px) {
  .scan-kind { min-height: 70px; padding: 12px 14px; gap: 12px; }
  .scan-kind-icon { width: 42px; height: 42px; }
  .scan-kind-body b { font-size: 14.5px; }
  .scan-item { padding: 14px; }
  .link-btn { font-size: 12px; }
}
</style>
