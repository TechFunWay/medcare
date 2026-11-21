<template>
  <div class="page narrow">
    <div v-if="fromPlans" class="back-row">
      <button class="btn btn-ghost btn-sm" @click="router.push('/plans')"><AppIcon name="arrowLeft" :size="16" />返回用药计划</button>
    </div>
    <div class="page-head">
      <div><div class="eyebrow">AI 识别</div><h1>AI 识别记录</h1><p>处方单、治疗单、缴费单与药盒的识别都在这里：上传后 AI 提取，你核对确认再入库或生成计划。</p></div>
    </div>

    <div v-if="!book.canPrescCreate" class="card readonly-tip" style="margin-bottom:14px">
      <div class="muted small" style="line-height:1.7">你在这里只能查看识别记录。拍照识别需要「处方识别 · 识别」，确认入库还要「药品与库存 · 新增」，请让管理员在「成员与权限」里勾上。</div>
    </div>
    <div v-if="book.canPrescCreate && purposeTab === 'plan'" class="card" style="background:linear-gradient(135deg,var(--mc-primary-soft),var(--mc-surface))">
      <div class="upload-head"><div class="empty-icon upload-icon"><AppIcon name="camera" :size="24" /></div><div class="upload-text"><b>上传处方单 / 治疗单图片</b><p class="muted small" style="margin-top:3px">支持 JPG / PNG / WebP，单张不超过 10MB。AI 只提取文字，不做诊断。</p></div></div>
      <button class="btn btn-primary btn-block" style="margin-top:12px" :disabled="busy" @click="pickFile"><AppIcon name="camera" :size="18" />拍照 / 选择图片</button>
      <label class="tiny muted upload-check"><input v-model="saveImage" type="checkbox" />保存原图到用药本（加密，随备份）</label>
      <div class="scan-tip">
        <span>只想建药品、补库存？</span>
        <button type="button" class="link-btn" @click="router.push('/medicines?scan=1')">去药品页拍照识别药盒 / 缴费单（只建档）</button>
        <button type="button" class="link-btn" @click="router.push('/stock')">去库存页识别缴费单并入库</button>
      </div>
      <p class="tiny muted" style="margin-top:6px;line-height:1.65">首次使用前请确认：处方图片会发送到管理员配置的 AI 服务。默认仅保存识别结果，不保存原图。</p>
      <input ref="fileInput" type="file" accept="image/jpeg,image/png,image/webp" style="display:none" @change="onFile" />
    </div>

    <div v-if="recognizing" class="card muted" style="margin-top:14px">AI 正在识别处方（通常 30–60 秒），完成后会自动显示核对结果…</div>
    <div v-if="error" class="card" style="margin-top:14px;color:var(--mc-danger);background:var(--mc-danger-soft)">{{ error }}</div>

    <template v-if="draft">
      <div class="section-head" style="margin-top:20px">
        <h2>核对后录入</h2>
        <div style="display:flex;align-items:center;gap:8px">
          <span class="badge" :class="stockOnly ? 'warn' : 'done'" style="font-size:12px">{{ docTypeLabel }}</span>
          <button class="btn btn-ghost btn-xs" style="color:var(--mc-danger)" @click="discardCurrent">删除记录</button>
        </div>
      </div>
      <div v-if="readOnly" class="card stock-tip">这条记录已导入过，以下为只读回看；补库存请走「出入库」，同一条识别记录不能重复加入。</div>
      <div v-if="!noPlan" class="card stock-tip">处方单确认后只生成用药计划，<b>不会入库</b>；需要补库存请到「库存」页做出入库。</div>
      <div v-if="noPlan" class="card stock-tip">{{ stockTip }}</div>
      <div v-else class="card">
        <div class="field-row"><div class="field"><label>医院</label><input v-model="draft.hospital" class="input" /></div><div class="field"><label>医生</label><input v-model="draft.doctor" class="input" /></div></div>
        <div class="field-row"><div class="field"><label>处方日期</label><DateField v-model="draft.prescription_date" placeholder="选择日期" /></div><div class="field"><label>开始日期</label><DateField v-model="startDate" placeholder="选择日期" /></div></div>
      </div>
      <p class="tiny muted" style="margin:8px 2px 0;line-height:1.6">{{ archiveOnly ? '同名药品会自动匹配到已有档案，不会重复建档；也可在条目里改选「匹配已有药品」。' : '同名药品会自动合并到已有档案补库存，不会重复建档；也可在条目里改选「匹配已有药品」。' }}</p>
      <div v-if="stockOnly && !archiveOnly" class="batch-bar">
        <span class="tiny muted">同批次入库？</span>
        <button type="button" class="btn btn-outline btn-xs" @click="showBatchTime = true">批量设置入库时间</button>
      </div>
      <div class="list" style="margin-top:12px">
        <article v-for="(item, index) in draft.items" :key="index" :id="'draft-item-' + index" class="card" :style="(item.errName || item.errUnit) ? 'border-color:var(--mc-danger);border-width:2px' : (item.needs_review ? 'border-color:var(--mc-warning)' : '')">
          <div class="draft-head"><b>{{ item.raw_name || item.name || '（未命名条目）' }}</b><span v-if="item.errName || item.errUnit" class="badge missed">缺必填</span><span v-else-if="item.needs_review" class="badge warn">请核对</span><button class="icon-btn draft-del" @click="removeItem(index)"><AppIcon name="trash" :size="16" /></button></div>
          <div v-if="item.errName || item.errUnit" class="tiny" style="color:var(--mc-danger);margin:2px 0 8px">第 {{ index + 1 }} 条：{{ item.errName ? '药品名称为空' : '' }}{{ item.errName && item.errUnit ? '、' : '' }}{{ item.errUnit ? '单位为空' : '' }}，请补全或删除这一条</div>
          <div class="field"><label>药品名称</label><input v-model="item.name" class="input" :style="item.errName ? 'border-color:var(--mc-danger)' : ''" /></div>
          <!-- 数量属于出入库，只在「确认后要入库」的场景出现；只建档时只维护规格与单位。 -->
          <div v-if="stockOnly && !archiveOnly" class="field-row"><div class="field"><label>入库数量</label><input v-model.number="item.quantity" type="number" min="0" step="0.01" class="input" /></div><div class="field"><label>单位</label><input v-model="item.dose_unit" class="input" :style="item.errUnit ? 'border-color:var(--mc-danger)' : ''" placeholder="盒 / 支 / 瓶" /></div></div>
          <div v-if="stockOnly && !archiveOnly" class="field-row"><div class="field"><label>批次日期</label><DateField v-model="item.receivedDate" placeholder="入库日期" /></div><div class="field"><label>批次时间（时:分:秒）</label><TimeField v-model="item.receivedTime" placeholder="时间" with-seconds /></div></div>
          <!-- 只建档：字段与「药品 → 添加」表单一致，规格/剂型/单位都是下拉（可搜索、可自填）。 -->
          <template v-else-if="archiveOnly">
            <div class="field-row">
              <div class="field"><label>分类</label><select v-model="item.category" class="select"><option value="oral">口服</option><option value="injection">注射</option><option value="infusion">输液</option><option value="topical">外用</option><option value="other">其他</option></select></div>
              <div class="field"><label>剂型</label><SheetSelect v-model="item.form" placeholder="剂型" :options="formOptions()" searchable allow-custom custom-label="没有合适的剂型？自己填" /></div>
            </div>
            <div class="field-row">
              <div class="field"><label>规格</label><SheetSelect v-model="item.spec" placeholder="规格" :options="specOptions()" searchable allow-custom custom-label="没有合适的规格？自己填" custom-placeholder="例如：0.2g×10片" /></div>
              <div class="field" :style="item.errUnit ? 'border-color:var(--mc-danger)' : ''"><label>库存单位</label><SheetSelect v-model="item.dose_unit" placeholder="库存单位" :options="unitOptions()" searchable allow-custom custom-label="没有合适的单位？自己填" /></div>
            </div>
            <div class="field-row">
              <div class="field"><label>单价（元/单位）</label><input v-model.number="item.unit_price" type="number" min="0" step="0.01" class="input" placeholder="可选，例如 3.50" /></div>
              <div class="field"><label>低库存阈值</label><input v-model.number="item.low_stock_threshold" type="number" min="0" class="input" /></div>
            </div>
            <div class="field"><label>临期预警天数</label><input v-model.number="item.expiry_warn_days" type="number" min="1" class="input" /></div>
          </template>
          <div v-else class="field-row"><div class="field"><label>剂量</label><input v-model.number="item.dose_amount" type="number" min="0.01" step="0.01" class="input" /></div><div class="field"><label>单位</label><input v-model="item.dose_unit" class="input" :style="item.errUnit ? 'border-color:var(--mc-danger)' : ''" /></div></div>
          <div v-if="!noPlan" class="field-row"><div class="field"><label>频率</label><SheetSelect v-model="item.frequency_type" placeholder="频率" :options="frequencyOptions" /></div><div class="field" v-if="item.frequency_type !== 'custom_dates'"><label>疗程天数</label><input v-model.number="item.duration_days" type="number" min="1" class="input" /></div></div>
          <!-- 指定日期：有的药只在某几天用（如 21、23、25、27 日） -->
          <div v-if="!noPlan && item.frequency_type === 'custom_dates'" class="field">
            <label>用药日期（可加多个）</label>
            <div class="cd-list">
              <div v-for="(d, di) in (item.customDates || [])" :key="di" class="cd-row">
                <DateField :model-value="item.customDates ? item.customDates[di] : ''" placeholder="选择日期" @update:model-value="(v: string) => setCustomDate(item, di, v)" />
                <button type="button" class="icon-btn" aria-label="删除日期" @click="removeCustomDate(item, di)"><AppIcon name="close" :size="15" /></button>
              </div>
              <button type="button" class="btn btn-outline btn-sm" @click="addCustomDate(item)">+ 添加日期</button>
            </div>
          </div>
          <div v-if="!noPlan && item.frequency_type !== 'interval_hours'" class="field"><label>时间（逗号分隔，24 小时制）</label><input v-model="item.timesText" class="input" placeholder="08:00, 20:00" /></div>
          <div class="field-row" v-if="!noPlan && item.frequency_type === 'every_n_days'"><div class="field"><label>间隔天数</label><input v-model.number="item.interval_days" type="number" min="1" class="input" /></div></div>
          <div class="field"><label>匹配已有药品</label><SheetSelect v-model="item.medicineMode" placeholder="新建药品" :options="medicineModeOptions(item)" searchable /></div>
          <div v-if="draft.doc_type === 'receipt' && !archiveOnly" class="tiny muted" style="margin-top:-6px;margin-bottom:8px">单价 {{ item.unit_price || 0 }} 元 · 金额 {{ item.amount || 0 }} 元（来自收费单）</div>
          <div v-if="archiveOnly" class="field"><label>备注</label><textarea v-model="item.instructions" class="textarea" /></div>
          <div v-else class="field"><label>用法备注</label><input v-model="item.instructions" class="input" /></div>
        </article>
      </div>
      <div v-if="stockOnly && !archiveOnly" class="batch-bar" style="margin-top:10px">
        <span class="tiny muted">共 {{ draft.items.length }} 条，可统一批次时间</span>
        <button type="button" class="btn btn-outline btn-xs" @click="showBatchTime = true">批量设置入库时间</button>
      </div>
      <div class="sheet-foot" style="padding-top:12px"><button class="btn btn-outline btn-block" :disabled="confirming" @click="reset">{{ readOnly ? '关闭' : '取消' }}</button><button v-if="!readOnly" class="btn btn-primary btn-block" :disabled="confirming || !draft.items.length" @click="confirm">{{ confirming ? '处理中…' : confirmLabel }}</button></div>
    </template>

    <!-- 批量设置入库时间 -->
    <Teleport to="body">
      <div v-if="showBatchTime" class="batch-sheet" @click.self="showBatchTime = false">
        <div class="batch-card">
          <h3>批量设置入库时间</h3>
          <p class="tiny muted" style="margin:0 0 10px;line-height:1.6">把本次识别到的 {{ draft?.items.length || 0 }} 条全部设为同一个入库批次时间。</p>
          <div class="field-row">
            <div class="field"><label>日期</label><DateField v-model="batchDate" placeholder="入库日期" /></div>
            <div class="field"><label>时间（时:分:秒）</label><TimeField v-model="batchTime" placeholder="时间" with-seconds /></div>
          </div>
          <div class="batch-actions">
            <button type="button" class="btn btn-outline btn-block" @click="showBatchTime = false">取消</button>
            <button type="button" class="btn btn-primary btn-block" @click="applyBatchTime">全部设置</button>
          </div>
        </div>
      </div>
    </Teleport>

    <div class="section" style="margin-top:22px">
      <div class="section-head"><h2>识别历史</h2><button class="btn btn-ghost btn-xs" @click="loadHistory">刷新</button></div>
      <div class="tabs" style="margin-bottom:10px">
        <button v-for="t in purposeTabs" :key="t.value" type="button" class="tab" :class="{ active: purposeTab === t.value }" @click="purposeTab = t.value; historyFilter = ''">{{ t.label }}<span v-if="t.count" class="tab-count">{{ t.count }}</span></button>
      </div>
      <div class="chip-row" style="margin-bottom:10px">
        <button v-for="f in historyFilters" :key="f.value" type="button" class="chip" :class="{ active: historyFilter === f.value }" @click="historyFilter = f.value">{{ f.label }}</button>
      </div>
      <div v-if="!filteredHistory.length" class="card muted">{{ history.length ? '没有符合筛选条件的记录' : '还没有识别记录' }}</div>
      <div v-else class="list">
        <div v-for="h in filteredHistory" :key="h.id" class="card history-card" @click="openHistory(h)">
          <div class="type-dot"><AppIcon name="prescription" :size="18" /></div>
          <div class="history-body"><div class="history-title">{{ historyTitle(h) }} · {{ h.item_count }} 项<span class="badge" :class="intentBadge(h)">{{ intentLabel(h) }}</span></div><div class="tiny muted">识别时间 {{ formatDateTime(h.created_at, true) }} · {{ statusLabel(h.status) }} · {{ sourceLabel(h) }}<template v-if="tokenLabel(h)"> · {{ tokenLabel(h) }}</template><template v-if="h.status === 'recognizing'"> · 已等 {{ elapsedLabel(h) }}</template></div>
            <div v-if="h.status === 'failed' && h.error_message" class="tiny" style="color:var(--mc-danger);margin-top:2px">{{ h.error_message }}</div></div>
          <span class="badge" :class="h.status === 'confirmed' ? 'done' : h.status === 'failed' ? 'missed' : h.status === 'recognized' ? 'warn' : 'pending'">{{ statusLabel(h.status) }}</span>
          <button v-if="h.status === 'failed' && h.has_image" class="btn btn-outline btn-xs" @click.stop="retry(h)">重试</button>
          <span v-else-if="h.status === 'failed'" class="tiny muted" style="flex:0 0 auto">需重新上传</span>
          <button class="icon-btn history-del" aria-label="删除识别记录" title="删除识别记录" @click.stop="discardHistory(h)"><AppIcon name="trash" :size="16" /></button>
          <AppIcon v-if="h.status === 'recognized'" name="chevron" :size="16" />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppIcon from '../components/AppIcon.vue'
import SheetSelect from '../components/SheetSelect.vue'
import { fixUnit, formOptions, specOptions, unitOptions } from '../utils/medicineMeta'
import DateField from '../components/DateField.vue'
import TimeField from '../components/TimeField.vue'
import { medicineApi, prescriptionApi, type Medicine, type PrescriptionDraft } from '../api/medcare'
import { useBookStore } from '../stores/book'
import { localDate } from '../utils/date'
import { useUIStore } from '../stores/ui'
import { reportError } from '../utils/errors'
import { fetchAllPages } from '../utils/paging'
const router = useRouter()
const route = useRoute()
// 从「用药计划」页进来时，确认录入后回到计划列表，形成闭环。
const fromPlans = computed(() => route.query.from === 'plans')
// ?archive=1：只建档（药品页入口）；核对表单不出「入库数量」，改为「规格」。
const archiveMode = computed(() => route.query.archive === '1' || route.query.from === 'medicines')
const book = useBookStore()
const ui = useUIStore()
const fileInput = ref<HTMLInputElement | null>(null)
const saveImage = ref(false)
const recognizing = ref(false)
const confirming = ref(false)
const error = ref('')
const draft = ref<PrescriptionDraft | null>(null)
const importId = ref(0)
const startDate = ref(localDate())
// 入库批次时间：AI 识别结果默认同一批次，可一键全部设置，也可逐条改。
const batchDate = ref(localDate())
const batchTime = ref(nowClock())
const showBatchTime = ref(false)
function nowClock() {
  const d = new Date()
  const p = (n: number) => String(n).padStart(2, '0')
  return p(d.getHours()) + ':' + p(d.getMinutes()) + ':' + p(d.getSeconds())
}
function applyBatchTime() {
  for (const it of draft.value?.items || []) {
    ;(it as any).receivedDate = batchDate.value
    ;(it as any).receivedTime = batchTime.value
  }
  showBatchTime.value = false
  ui.showToast('已把 ' + (draft.value?.items?.length || 0) + ' 条设为 ' + batchDate.value + ' ' + batchTime.value)
}
const medicines = ref<Medicine[]>([])
const history = ref<any[]>([])
// 识别历史筛选：按用途（只建档/入库/生成计划）或单据类型（药盒/缴费单/处方单）区分。
// 识别历史按用途分成三个独立清单：药品建档 / 库存入库 / 用药计划，互不混在一起。
type HistoryPurpose = 'archive' | 'stock' | 'plan'
function purposeOf(h: any): HistoryPurpose {
  const i = String(h.intent || '')
  if (i === 'archive' || i === 'stock' || i === 'plan') return i
  // 老记录没有 intent：处方单按「用药计划」，其余按「药品建档」处理。
  return (h.kind === 'prescription' || !h.kind) ? 'plan' : 'archive'
}
const purposeTab = ref<HistoryPurpose>(
  (() => {
    const q = String(route.query.purpose || '')
    if (q === 'archive' || q === 'stock' || q === 'plan') return q as HistoryPurpose
    if (route.query.from === 'medicines') return 'archive' as HistoryPurpose
    if (route.query.from === 'plans') return 'plan' as HistoryPurpose
    return 'plan' as HistoryPurpose
  })(),
)
const purposeTabs = computed(() => [
  { value: 'archive' as HistoryPurpose, label: '药品建档', count: history.value.filter((h) => purposeOf(h) === 'archive').length },
  { value: 'stock' as HistoryPurpose, label: '库存入库', count: history.value.filter((h) => purposeOf(h) === 'stock').length },
  { value: 'plan' as HistoryPurpose, label: '用药计划', count: history.value.filter((h) => purposeOf(h) === 'plan').length },
])
// 次级筛选：按单据类型（药盒 / 缴费单 / 处方单）。
const historyFilter = ref('')
const historyFilters = [
  { value: '', label: '全部类型' },
  { value: 'box', label: '药盒' },
  { value: 'receipt', label: '缴费单' },
  { value: 'prescription', label: '处方单' },
]
const filteredHistory = computed(() => {
  const list = history.value.filter((h) => purposeOf(h) === purposeTab.value)
  if (!historyFilter.value) return list
  return list.filter((h) => (h.kind || 'prescription') === historyFilter.value)
})
// 识别记录页开着时，有「识别中」的记录就每 5 秒刷一次状态，
// 用户不用手动刷新也能看到它变成待确认 / 失败（2026-11-17 用户要求状态可见）。
let histTimer: number | null = null
function syncHistTimer() {
  const running = history.value.some((h) => h.status === 'recognizing')
  if (running && histTimer === null) histTimer = window.setInterval(() => { void loadHistory() }, 5000)
  else if (!running && histTimer !== null) { window.clearInterval(histTimer); histTimer = null }
}
function elapsedLabel(h: any) {
  const s = Math.max(0, Math.round((Date.now() - new Date(h.created_at).getTime()) / 1000))
  return s < 60 ? s + ' 秒' : Math.floor(s / 60) + ' 分 ' + (s % 60) + ' 秒'
}
// 已录入的记录只读回看：后端禁止重复确认，前端也不再给「确认录入」按钮。
const readOnly = ref(false)
const docTypeLabel = computed(() => {
  const t = draft.value?.doc_type
  return t === 'receipt' ? '门诊收费凭据' : t === 'box' ? '药品包装盒' : t === 'prescription' ? '处方笺' : '其他单据'
})
// 药盒与缴费单只入库、不排计划，核对表单只问数量与单位。
const stockOnly = computed(() => draft.value?.doc_type === 'receipt' || draft.value?.doc_type === 'box')
// 只建档（不入库）：药品页的识别只加药品资料，核对时不该出现「数量」。
// 直接由 URL 派生，避免只初始化在 reset() 里导致打开历史记录时又冒出数量字段。
const archiveOnly = computed(() => archiveMode.value)
// 没有「计划管理」能力时，处方确认只做建档与入库，不排计划（后端同口径拒绝，避免绕过）。
const noPlan = computed(() => stockOnly.value || !book.canPlanCreate)
const stockTip = computed(() => archiveOnly.value
  ? '这里只添加药品资料（名称、规格、单位），不入库；要入库请到「库存」页做出入库。'
  : stockOnly.value
  ? '这些药品只建档案并入库，不排用药计划；需要吃药提醒的话，请到计划页新建计划。'
  : '你还没有「计划管理」的权限，这次只建档案并入库，不排用药计划；需要提醒请让管理员在「成员与权限」里开通。')
const busy = ref(false)
const frequencyOptions = [
  { value: 'daily', label: '每天' },
  { value: 'every_n_days', label: '每 N 天' },
  { value: 'weekly', label: '每周' },
  { value: 'interval_hours', label: '每 N 小时' },
  { value: 'custom_dates', label: '指定日期' },
]
// 指定日期的增删改（customDates 可能为空数组，统一在这里兜底）。
function setCustomDate(item: any, i: number, v: string) {
  if (!item.customDates) item.customDates = []
  item.customDates[i] = v
}
function removeCustomDate(item: any, i: number) {
  if (item.customDates) item.customDates.splice(i, 1)
}
function addCustomDate(item: any) {
  if (!item.customDates) item.customDates = []
  item.customDates.push('')
}
function medicineModeOptions(item: any) {
  return [
    { value: 'new', label: '新建药品「' + (item.name || item.raw_name || '') + '」' },
    ...medicines.value.map((m) => ({ value: 'id:' + m.id, label: '使用已有：' + m.name + ' · ' + (m.spec || m.unit) })),
  ]
}
function pickFile() { fileInput.value?.click() }
import { formatDate, formatDateTime } from '../utils/date'
function statusLabel(s: string) { return s === 'confirmed' ? '已录入' : s === 'recognized' ? '待确认' : s === 'failed' ? '失败' : s === 'discarded' ? '已删除' : '识别中' }
// 这条记录是怎么读出来的：走了 OCR 标出服务商，没走就是直读图片（视觉模型）。
const OCR_LABELS: Record<string, string> = { baidu: '百度', tencent: '腾讯云', aliyun: '阿里云', youdao: '有道', deepseek: 'DeepSeek', custom: '自定义模型' }
function sourceLabel(h: any) {
  const p = String(h.ocr_provider || '')
  return p ? 'OCR · ' + (OCR_LABELS[p] || p) : '直读图片（视觉模型）'
}
function tokenLabel(h: any) {
  const ai = (Number(h.ai_prompt_tokens) || 0) + (Number(h.ai_completion_tokens) || 0)
  const ocr = Number(h.ocr_tokens) || 0
  const parts: string[] = []
  if (ai > 0) parts.push('模型 ' + ai + ' token')
  if (ocr > 0) parts.push('OCR ' + ocr + ' token')
  return parts.join(' + ')
}
// 药盒与缴费单在药品页导入，历史里按单据类型区分标题，不再一律叫「处方」。
// 用途：只建档 / 入库 / 生成计划（老记录没有 intent，按单据类型给个默认说明）。
function intentLabel(h: any) {
  const i = String(h.intent || '')
  if (i === 'archive') return '只建档'
  if (i === 'stock') return '入库'
  if (i === 'plan') return '生成计划'
  return h.kind === 'prescription' || !h.kind ? '生成计划' : '未标注'
}
function intentBadge(h: any) {
  const i = String(h.intent || '')
  if (i === 'stock') return 'done'
  if (i === 'archive') return 'warn'
  return 'pending'
}
function historyTitle(h: any) {
  // 标题先点名单据类型，再带医院/药店，历史列表一眼能分清是药盒、缴费单还是处方单。
  if (h.kind === 'box') return '药盒识别'
  if (h.kind === 'receipt') return '缴费单' + (h.hospital ? ' · ' + h.hospital : '')
  return '处方单' + (h.hospital ? ' · ' + h.hospital : '')
}
function reset() { draft.value = null; importId.value = 0; error.value = ''; readOnly.value = false }
async function onFile(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (!file) return
  error.value = ''
  recognizing.value = true
  try {
    const fd = new FormData()
    fd.append('file', file)
    fd.append('save_image', saveImage.value ? 'true' : 'false')
    // 这里的处方单上传默认目标是生成用药计划；没有计划权限时只是建档。
    fd.append('intent', book.canPlanCreate ? 'plan' : 'archive')
    const res = await prescriptionApi.recognize(book.currentId!, fd)
    if (res.data?.code !== 0) throw new Error(res.data?.message || '识别失败')
    importId.value = res.data.data.import_id
    // 识别在后台跑：轮询直到 recognized / failed
    const polled = await pollImport(importId.value)
    if (!polled) throw new Error('识别超时：AI 服务响应较慢，请稍后重试，或手动录入')
    draft.value = polled
    startDate.value = draft.value?.prescription_date || localDate()
    for (const item of draft.value?.items || []) {
      ;(item as any).timesText = (item.times && item.times.length ? item.times : ['08:00']).join(', ')
      ;(item as any).medicineMode = 'new'
      ;(item as any).createPlan = draft.value?.doc_type !== 'receipt' && draft.value?.doc_type !== 'box'
      ;(item as any).interval_days = item.interval_days || 2
      ;(item as any).customDates = ((item as any).custom_schedule || []).map((d: any) => d.date).filter(Boolean)
      ;(item as any).duration_days = item.duration_days || 7
    }
    await loadHistory()
    ui.showToast('识别完成，请核对')
  } catch (e: any) {
    const status = e?.response?.status
    const msg = e?.response?.data?.message
    error.value = msg || (status ? '识别失败（' + status + '），可重试或手动录入' : (e?.code === 'ECONNABORTED' ? '识别超时：AI 服务响应较慢，请稍后重试，或手动录入' : (e?.message && !String(e.message).includes('fetch') ? e.message : '网络异常，识别失败，可重试或手动录入')))
  }
  finally { recognizing.value = false; if (fileInput.value) fileInput.value.value = '' }
}
async function pollImport(id: number, maxMs = 240000): Promise<PrescriptionDraft | null> {
  const started = Date.now()
  while (Date.now() - started < maxMs) {
    await new Promise((r) => setTimeout(r, 3000))
    try {
      const res = await prescriptionApi.get(book.currentId!, id)
      const data = res.data?.data
      const status = data?.status || data?.import?.status
      if (status === 'recognized') return data.draft as PrescriptionDraft
      if (status === 'failed') {
        throw new Error(data?.import?.error_message || '识别失败，可重试或手动录入')
      }
    } catch (e: any) {
      if (e?.message && !e?.response) throw e
    }
  }
  return null
}

function removeItem(index: number) { if (draft.value) draft.value.items.splice(index, 1) }

// 放弃当前识别草稿并删除记录：用于识别结果太差、不想留在历史里的场景。
async function discardCurrent() {
  if (!importId.value) { reset(); return }
  const ok = await ui.confirm({
    title: '删除这条识别记录？',
    message: '删除后这条识别结果不再出现在「识别历史」里；已经确认录入的药品、计划不会被删除。',
    confirmText: '删除',
    danger: true,
  })
  if (!ok) return
  try {
    await prescriptionApi.remove(book.currentId!, importId.value)
    reset()
    await loadHistory()
    ui.showToast('已删除')
  } catch (e: any) {
    ui.showToast(e?.response?.data?.message || '删除失败', 'error')
  }
}

async function discardHistory(h: any) {
  const ok = await ui.confirm({
    title: '删除这条识别记录？',
    message: '删除后它不再出现在「识别历史」里；已经确认录入的药品、计划不会被删除。',
    confirmText: '删除',
    danger: true,
  })
  if (!ok) return
  try {
    await prescriptionApi.remove(book.currentId!, h.id)
    if (importId.value === h.id) reset()
    await loadHistory()
    ui.showToast('已删除')
  } catch (e: any) {
    ui.showToast(e?.response?.data?.message || '删除失败', 'error')
  }
}
// 提交前逐条校验「药品名称 / 单位」，把缺的那一条标红、滚动到眼前并说明是第几条，
// 避免只弹一句「药品名称、单位不能为空」让人满屏找（2026-11-17 用户反馈）。
function validateDraftItems(): boolean {
  if (!draft.value) return false
  let firstEl: HTMLElement | null = null
  for (let i = 0; i < draft.value.items.length; i++) {
    const it: any = draft.value.items[i]
    it.errName = !String(it.name || '').trim()
    it.errUnit = !String(it.dose_unit || '').trim()
    if ((it.errName || it.errUnit) && !firstEl) {
      const who = it.raw_name && String(it.raw_name).trim() ? `「${it.raw_name}」` : ''
      const lack = it.errName && it.errUnit ? '药品名称和单位都为空' : it.errName ? '药品名称为空' : '单位为空'
      ui.showToast(`第 ${i + 1} 条${who}${lack}，请补全或删除这一条`, 'error')
      firstEl = document.getElementById('draft-item-' + i)
    }
  }
  if (firstEl) {
    firstEl.scrollIntoView({ block: 'center', behavior: 'smooth' })
    return false
  }
  return true
}

// 按钮文案跟着用途走：入库 / 只建档 / 录入（生成计划）。
const confirmLabel = computed(() => (stockOnly.value ? '确认入库' : archiveOnly.value ? '确认添加' : '确认录入'))
async function confirm() {
  if (!draft.value || !importId.value) return
  if (!validateDraftItems()) return
  confirming.value = true
  try {
    const items = draft.value.items.map((item: any) => {
      const mode = String(item.medicineMode || 'new')
      const existing = mode.startsWith('id:')
      return {
        raw_name: item.raw_name || item.name,
        name: item.name,
        generic_name: item.generic_name,
        spec: item.spec,
        form: item.form,
        route: item.route || 'other',
        medicine_mode: existing ? 'existing' : 'new',
        medicine_id: existing ? Number(mode.slice(3)) : 0,
        dose_amount: Number(item.dose_amount),
        dose_unit: item.dose_unit,
        frequency_type: item.frequency_type || 'daily',
        times: (item.timesText || '').split(',').map((s: string) => s.trim()).filter(Boolean),
        interval_days: item.interval_days,
        weekdays: item.weekdays || [],
        interval_hours: item.interval_hours,
        // 指定日期：每条日期都用同一组时间。
        custom_schedule: item.frequency_type === 'custom_dates'
          ? (item.customDates || []).filter((d: string) => !!d).map((d: string) => ({ date: d, times: (item.timesText || '').split(',').map((s: string) => s.trim()).filter(Boolean) }))
          : [],
        duration_days: item.duration_days,
        instructions: item.instructions || '',
        create_plan: noPlan.value ? false : item.createPlan !== false,
        // 只建档模式不入库：数量固定 0，后端据此不建批次、不写流水。
        quantity: archiveOnly.value ? 0 : (Number(item.quantity) || 0),
        // 入库批次时间（北京时间，精确到秒）；只建档不需要。
        received_at: (!archiveOnly.value && item.receivedDate && item.receivedTime) ? item.receivedDate + 'T' + item.receivedTime + '+08:00' : '',
        low_stock_threshold: Number((item as any).low_stock_threshold) || 0,
        expiry_warn_days: Number((item as any).expiry_warn_days) || 30,
        unit_price: Number(item.unit_price) || 0,
        amount: Number(item.amount) || 0,
      }
    })
    const res = await prescriptionApi.confirm(book.currentId!, importId.value, { start_date: noPlan.value ? '' : startDate.value, hospital: draft.value.hospital, doctor: draft.value.doctor, channel: 'hospital', items })
    if (res.data?.code !== 0) throw new Error(res.data?.message || '录入失败')
    // 提示语按单据类型走：缴费单 / 药盒只建药品并入库（不排计划），说「药品已录入」；
    // 处方笺才说「处方已录入」，有生成计划时再补一句（2026-11-17 用户反馈文案错位）。
    const created = res.data?.data?.created || []
    const merged = Number(res.data?.data?.merged) || 0
    const mergedTip = merged > 0 ? `（${merged} 种同名合并补库存）` : ''
    const countTip = created.length ? ' ' + created.length + ' 种' : ''
    if (archiveOnly.value) {
      ui.showToast(`已添加${countTip}药品资料${mergedTip}`)
    } else if (stockOnly.value) {
      ui.showToast(`已入库${countTip}${mergedTip}`)
    } else if (noPlan.value) {
      ui.showToast('处方已录入（未生成用药计划）')
    } else {
      ui.showToast('处方已录入，已生成用药计划')
    }
    reset()
    await loadHistory()
    if (fromPlans.value) { router.push('/plans'); return }
  } catch (e: any) { ui.showToast(e?.response?.data?.message || e?.message || '录入失败', 'error') }
  finally { confirming.value = false }
}
async function openHistory(h: any) {
  if (!book.currentId) return
  if (h.status === 'recognizing') { ui.showToast('这条还在识别中，完成后会自动变成待确认'); return }
  if (h.status === 'failed') { if (h.has_image) { await retry(h) } else { ui.showToast('这条没保存原图，请重新拍照上传', 'error') } ; return }
  if (h.status !== 'recognized' && h.status !== 'confirmed') return
  readOnly.value = h.status === 'confirmed'
  try {
    const res = await prescriptionApi.get(book.currentId, h.id)
    const data = res.data?.data
    const draftData = data?.draft
    if (!draftData || !draftData.items?.length) { ui.showToast('这条记录没有可核对的内容', 'error'); return }
    for (const item of draftData.items) {
      ;(item as any).timesText = (item.times && item.times.length ? item.times : ['08:00']).join(', ')
      ;(item as any).medicineMode = 'new'
      ;(item as any).interval_days = item.interval_days || 2
      ;(item as any).customDates = ((item as any).custom_schedule || []).map((d: any) => d.date).filter(Boolean)
      ;(item as any).duration_days = item.duration_days || 7
      ;(item as any).createPlan = draftData.doc_type !== 'receipt' && draftData.doc_type !== 'box'
      ;(item as any).category = item.category || 'oral'
      ;(item as any).form = item.form || ''
      ;(item as any).low_stock_threshold = item.low_stock_threshold ?? 0
      ;(item as any).receivedDate = item.receivedDate || (draftData.prescription_date || localDate())
      ;(item as any).receivedTime = item.receivedTime || nowClock()
      ;(item as any).expiry_warn_days = item.expiry_warn_days || 30
      ;(item as any).dose_unit = fixUnit(item.dose_unit, item.form, item.category, item.spec)
    }
    draft.value = draftData
    importId.value = h.id
    startDate.value = draftData.prescription_date || localDate()
    error.value = ''
    window.scrollTo({ top: 0, behavior: 'smooth' })
  } catch (e: any) {
    ui.showToast(e?.response?.data?.message || '读取记录失败', 'error')
  }
}

async function loadHistory() { if (!book.currentId) return; try { const res = await prescriptionApi.list(book.currentId); history.value = res.data?.data || [] } catch (e) { reportError(e, '读取识别历史失败') } syncHistTimer() }
async function loadMedicines() {
  if (!book.currentId) return
  // 「使用已有药品」要读药品列表，没有「药品与库存 · 查看」就跳过，只走新建药品。
  if (!book.canMedicineView) { medicines.value = []; return }
  // 药品多时一页取不全，逐页取全，避免「使用已有药品」列表缺项
  try {
    medicines.value = await fetchAllPages<Medicine>(async (page, pageSize) => {
      const res = await medicineApi.list(book.currentId!, { page, pageSize })
      return { items: (res.data?.data?.items || []) as Medicine[], total: Number(res.data?.data?.total) || 0 }
    })
  } catch (e) { reportError(e, '读取药品列表失败') }
}
async function retry(h: any) {
  if (!book.currentId) return
  try {
    await prescriptionApi.retry(book.currentId, h.id)
    ui.showToast('已重新识别')
    recognizing.value = true
    const draftData = await pollImport(h.id)
    recognizing.value = false
    if (draftData) { draft.value = draftData; importId.value = h.id; startDate.value = draftData.prescription_date || localDate(); ui.showToast('识别完成，请核对') }
    await loadHistory()
  } catch (e: any) {
    recognizing.value = false
    // 后端会给出中文原因（例如「原图未保存，请重新上传」），优先展示它而不是英文 axios 错误。
    ui.showToast(e?.response?.data?.message || e?.message || '重试失败', 'error')
  }
}
onMounted(async () => { await Promise.all([loadMedicines(), loadHistory()]) })
onUnmounted(() => { if (histTimer !== null) window.clearInterval(histTimer) })
// 切换用药本后重置当前识别草稿并刷新历史，避免看到上一个本的数据。
watch(() => book.currentId, async () => { reset(); await Promise.all([loadMedicines(), loadHistory()]) })
</script>

<style scoped>
.cd-list { display: grid; gap: 8px; }
.cd-row { display: flex; align-items: center; gap: 8px; }
.cd-row :deep(.select-like) { flex: 1 1 auto; }
.batch-bar { display: flex; align-items: center; justify-content: space-between; gap: 10px; margin-top: 10px; }
/* 批量设置入库时间 */
.batch-sheet { position: fixed; inset: 0; z-index: 60; display: grid; place-items: center; padding: 16px; background: rgba(0,0,0,.35); }
.batch-card { width: 100%; max-width: 420px; background: var(--mc-surface); border-radius: 18px; padding: 16px; box-shadow: var(--mc-shadow); }
.batch-card h3 { font-size: 16px; margin: 0 0 4px; }
.batch-actions { display: flex; gap: 10px; margin-top: 12px; }
.tab-count { margin-left: 4px; font-size: 12px; opacity: .75; }
.upload-head { display: flex; gap: 10px; align-items: flex-start; }
.upload-icon { margin: 0; width: 46px; height: 46px; border-radius: 16px; flex: 0 0 auto; }
.upload-text { min-width: 0; }
.upload-check { display: flex; align-items: center; gap: 8px; min-height: 44px; margin-top: 8px; }
.upload-check input { width: 20px; height: 20px; flex: 0 0 auto; }
.back-row { margin-bottom: 8px; }
.scan-tip { display: flex; flex-wrap: wrap; align-items: center; gap: 4px 6px; margin-top: 8px; font-size: 11.5px; color: var(--mc-text-muted); }
.stock-tip { font-size: 12.5px; line-height: 1.65; color: var(--mc-text-muted); }
.link-btn { display: inline-flex; align-items: center; min-height: 32px; padding: 0; border: 0; background: transparent; color: var(--mc-primary); font-size: 11.5px; font-weight: 700; text-decoration: underline; text-underline-offset: 2px; }
.draft-head { display: flex; align-items: center; gap: 6px; margin-bottom: 8px; min-width: 0; }
.draft-head b { min-width: 0; overflow-wrap: anywhere; }
.draft-del { margin-left: auto; width: 40px; height: 40px; }
.history-card { padding: 10px 12px; display: flex; gap: 8px; align-items: center; cursor: pointer; }
.history-body { flex: 1; min-width: 0; }
.history-title { font-weight: 650; font-size: 13.5px; overflow-wrap: anywhere; }
@media (min-width: 900px) {
  .upload-head { gap: 12px; }
  .upload-icon { width: 52px; height: 52px; border-radius: 18px; }
  .draft-head { gap: 8px; margin-bottom: 10px; }
  .history-card { padding: 12px 14px; gap: 10px; }
  .history-title { font-size: 14px; }
}
</style>
