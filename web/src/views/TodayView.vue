<template>
  <div class="page">
    <div class="page-head">
      <div>
        <div class="eyebrow">今日</div>
        <h1>{{ dateTitle }}</h1>
        <p>{{ weekdayLabel }} · {{ isToday ? '今天' : '当天' }}有 {{ dashboard?.summary?.total || 0 }} 项用药<template v-if="medSummary">：{{ medSummary }}</template></p>
      </div>
      <button class="icon-btn" aria-label="刷新" @click="load"><AppIcon name="refresh" :size="20" /></button>
    </div>

    <section class="card progress-card">
      <div class="ring" :style="{ '--pct': progress }" role="img" :aria-label="'今日完成进度 ' + progress + '%'"><div class="ring-inner">{{ progress }}%</div></div>
      <div class="progress-main">
        <div class="eyebrow">今日进度</div>
        <div class="progress-count">{{ dashboard?.summary?.done || 0 }} / {{ dashboard?.summary?.total || 0 }}</div>
        <div class="muted small">完成 {{ dashboard?.summary?.done || 0 }} 项 · 待完成 {{ dashboard?.summary?.pending || 0 }} 项</div>
        <div class="progress-track" style="margin-top:8px"><span :style="{ width: progress + '%' }" /></div>
      </div>
    </section>

    <div class="date-strip">
      <button v-for="d in dateStrip" :key="d.date" class="date-pill" :class="{ active: d.date === date, today: d.date === today }" @click="selectDate(d.date)">
        <span>{{ d.week }}</span>
        <b>{{ d.day }}</b>
      </button>
    </div>

    <div class="type-bar">
      <button v-for="t in typeFilters" :key="t.value" class="type-chip" :class="{ active: typeFilter === t.value }" @click="typeFilter = t.value">{{ t.label }}</button>
    </div>

    <div class="status-bar">
      <button v-for="s in statusFilters" :key="s.value" class="status-chip" :class="[s.value || 'all', { active: statusFilter === s.value }]" @click="statusFilter = s.value">
        <span>{{ s.label }}</span>
        <b>{{ statusCounts[s.value || 'all'] ?? 0 }}</b>
      </button>
    </div>

    <div v-if="loading" class="card muted">加载中…</div>
    <template v-else>
      <section v-if="filteredOverdue.length" class="section">
        <div class="section-head"><h2 style="color:var(--mc-danger)">超时 {{ filteredOverdue.length }} 项</h2><span class="tiny muted">完成后可撤销</span></div>
        <div class="list">
          <article v-for="ev in filteredOverdue" :key="ev.id" class="event-card overdue">
            <div class="type-dot" :class="ev.type"><AppIcon :name="typeIcon(ev.type)" :size="18" /></div>
            <div class="event-main">
              <div class="event-title-row"><span class="event-title">{{ ev.medicine_name }}</span><span class="badge missed">{{ overdueLabel(ev) }}</span></div>
              <div class="event-meta"><span>{{ ev.local_time }}</span><span>{{ ev.dose_amount }} {{ ev.dose_unit }}</span><span v-if="ev.location_name">{{ ev.location_name }}</span><span v-if="ev.executor_name">{{ executorLabel(ev.executor_type) }} {{ ev.executor_name }}</span></div>
              <div class="event-actions">
                <template v-if="canRecord">
                  <button class="btn btn-primary btn-sm" @click="complete(ev)">完成</button>
                  <button class="btn btn-outline btn-sm" @click="miss(ev)">漏服</button>
                </template>
                <button v-if="canRemind" class="btn btn-outline btn-sm" @click="remind(ev)">提醒用药</button>
                <button v-if="canRecord" class="btn btn-outline btn-sm" @click="openActions(ev)">更多</button>
              </div>
            </div>
          </article>
        </div>
      </section>

      <section v-if="dashboard?.next_event && !typeFilter" class="card next-card">
        <div class="next-main">
          <div class="eyebrow">下一项</div>
          <div class="next-title">{{ dashboard.next_event.local_time }} · {{ dashboard.next_event.medicine_name }}</div>
          <div class="muted small">{{ dashboard.next_event.dose_amount }} {{ dashboard.next_event.dose_unit }}<span v-if="dashboard.next_event.location_name"> · {{ dashboard.next_event.location_name }}</span><span v-if="dashboard.next_event.executor_name"> · {{ executorLabel(dashboard.next_event.executor_type) }} {{ dashboard.next_event.executor_name }}</span></div>
        </div>
        <div class="next-actions">
          <button v-if="canRecord" class="btn btn-outline" @click="snooze(dashboard.next_event, 10)">稍后</button>
          <button v-if="canRecord" class="btn btn-primary" @click="complete(dashboard.next_event)">完成</button>
          <button v-else-if="canRemind" class="btn btn-primary" @click="remind(dashboard.next_event)">提醒用药</button>
        </div>
      </section>

      <section v-for="group in visibleGroups" :key="group.key" class="section">
        <div class="section-head"><h2>{{ group.label }}</h2><span class="tiny muted">{{ group.events.length }} 项</span></div>
        <div class="list">
          <article v-for="ev in group.events" :key="ev.id" class="event-card" :class="{ done: eventStatus(ev) === 'done', overdue: eventStatus(ev) === 'overdue' }">
            <button class="check" :class="{ done: ev.status === 'done', locked: !canRecord }" :aria-label="ev.status === 'done' ? '撤销完成' : '标记完成'" @click="onCheck(ev)">
              <AppIcon v-if="ev.status === 'done'" name="check" :size="22" />
              <AppIcon v-else name="clock" :size="20" />
            </button>
            <div class="event-main" @click="openActions(ev)">
              <div class="event-title-row">
                <span class="event-title">{{ ev.medicine_name }}</span>
                <span class="badge" :class="statusClass(ev)">{{ statusLabel(ev) }}</span>
              </div>
              <div class="event-meta"><span>{{ ev.local_time }}</span><span>{{ ev.dose_amount }} {{ ev.dose_unit }}</span><span v-if="ev.plan_title">{{ ev.plan_title }}</span></div>
              <div v-if="ev.location_name" class="event-meta location-line"><AppIcon name="location" :size="13" />{{ ev.location_name }}<span v-if="ev.address">· {{ ev.address }}</span></div>
              <div v-if="ev.executor_name" class="event-meta location-line"><AppIcon name="user" :size="13" />{{ executorLabel(ev.executor_type) }} {{ ev.executor_name }}</div>
              <div v-if="ev.type === 'infusion' && infusionText(ev)" class="event-meta location-line"><AppIcon name="clock" :size="13" />{{ infusionText(ev) }}</div>
              <div v-if="ev.type === 'infusion' && actualRange(ev)" class="event-meta location-line"><AppIcon name="clock" :size="13" />{{ actualRange(ev) }}</div>
            </div>
            <div class="type-dot" :class="ev.type"><AppIcon :name="typeIcon(ev.type)" :size="18" /></div>
          </article>
        </div>
      </section>

      <section v-if="hasTodayEvents && statusFilter && !filteredOverdue.length && !visibleGroups.length" class="card muted">该状态下暂无用药</section>
      <section v-if="hasTodayEvents && typeFilter && !filteredOverdue.length && !visibleGroups.length" class="card muted">该分类下暂无用药</section>

      <section v-if="!hasTodayEvents && (dashboard?.active_plan_count || 0) > 0" class="empty card">
        <div class="empty-icon"><AppIcon name="calendar" :size="30" /></div>
        <h3>{{ isToday ? '今天没有用药安排' : '当天没有用药安排' }}</h3>
        <p v-if="dashboard?.next_upcoming">下一项：{{ upcomingLabel }} · {{ dashboard.next_upcoming.medicine_name }}（{{ dashboard.next_upcoming.dose_amount }} {{ dashboard.next_upcoming.dose_unit }}）</p>
        <p v-else>已有进行中的计划，但这一天没有排到用药。</p>
        <div class="empty-actions">
          <router-link to="/plans" class="btn btn-outline">查看计划</router-link>
        </div>
      </section>

      <section v-if="!hasTodayEvents && !(dashboard?.active_plan_count || 0)" class="empty card">
        <div class="empty-icon"><AppIcon name="sparkles" :size="30" /></div>
        <h3>{{ isToday ? '今天还没有安排' : '当天还没有安排' }}</h3>
        <template v-if="canMedicine || canPlan">
          <p>拍一张处方单/治疗单让 AI 生成计划，或手动新建一个用药计划（需要「处方识别 · 识别」或「用药计划 · 新建」）。</p>
          <div class="empty-actions">
            <router-link v-if="canMedicine" to="/prescriptions" class="btn btn-primary">AI 识别处方</router-link>
            <router-link v-if="canPlan" to="/plans/new" class="btn btn-outline">新建计划</router-link>
          </div>
        </template>
        <p v-else>用药计划还没建好，请联系管理员或监护人添加；建好后你会在这里看到今天的安排。</p>
      </section>

      <section v-if="alerts.length" class="section">
        <div class="section-head"><h2>库存提醒</h2><router-link to="/medicines" class="small" style="color:var(--mc-primary-strong);font-weight:700">去补货</router-link></div>
        <div class="list">
          <div v-for="m in alerts.slice(0, 3)" :key="m.id" class="card alert-row">
            <span class="badge warn">库存 {{ m.stock_quantity }} {{ m.unit }}</span>
            <div style="flex:1;min-width:0"><div style="font-weight:700">{{ m.name }}</div><div class="tiny muted">{{ m.nearest_expiry ? '最近有效期 ' + m.nearest_expiry : '未设置有效期' }}</div></div>
            <AppIcon name="chevron" :size="16" />
          </div>
        </div>
      </section>
    </template>

    <Teleport to="body">
      <div v-if="actionEvent" class="sheet-backdrop" @click.self="actionEvent = null">
        <section class="sheet" role="dialog" aria-modal="true">
          <div class="sheet-grab" />
          <div class="sheet-head"><div><div class="eyebrow">{{ actionEvent.local_time }}</div><h2>{{ actionEvent.medicine_name }}</h2></div><button class="icon-btn" aria-label="关闭" @click="actionEvent = null"><AppIcon name="close" /></button></div>
          <div class="sheet-body">
            <div class="event-meta" style="margin-bottom:14px"><span>{{ actionEvent.dose_amount }} {{ actionEvent.dose_unit }}</span><span v-if="actionEvent.location_name">{{ actionEvent.location_name }}</span><span v-if="actionEvent.executor_name">{{ executorLabel(actionEvent.executor_type) }} {{ actionEvent.executor_name }}</span><span v-if="actionEvent.type === 'infusion' && infusionText(actionEvent)">{{ infusionText(actionEvent) }}</span><span v-if="actionEvent.type === 'infusion' && actualRange(actionEvent)">{{ actualRange(actionEvent) }}</span><span v-if="actionEvent.plan_title">{{ actionEvent.plan_title }}</span></div>
            <div class="list">
              <button v-if="canRecord && actionEvent.type === 'infusion' && actionEvent.status === 'pending' && !actionEvent.actual_start_at" class="btn btn-outline btn-block" @click="startInfusion(actionEvent)">开始输液（记录开始时间）</button>
              <button v-if="canRecord && (actionEvent.status === 'pending' || actionEvent.status === 'missed')" class="btn btn-primary btn-block" @click="complete(actionEvent)">标记完成</button>
              <button v-if="canRecord && (actionEvent.status === 'pending' || actionEvent.status === 'missed')" class="btn btn-outline btn-block" @click="miss(actionEvent)">标记漏服</button>
              <button v-if="canRecord && actionEvent.status === 'done'" class="btn btn-outline btn-block" @click="undo(actionEvent)">撤销完成</button>
              <button v-if="canRecord" class="btn btn-outline btn-block" @click="openAdjust(actionEvent)">医生调整 / 自己调整</button>
              <button v-if="canRecord && actionEvent.status === 'pending'" class="btn btn-outline btn-block" @click="snooze(actionEvent, 30)">稍后 30 分钟</button>
              <button v-if="canRemind && actionEvent.status !== 'done' && actionEvent.status !== 'skipped'" class="btn btn-outline btn-block" @click="remind(actionEvent)">提醒用药</button>
              <p v-if="!canRecord && !canRemind" class="tiny muted" style="margin:2px 0 0">你在这里只能查看。需要记录或催办，请让管理员在「用药本 → 成员与权限」里勾上「用药记录 · 点完成」或「点提醒」。</p>
              <button v-if="actionEvent.location_name" class="btn btn-outline btn-block" @click="copy(actionEvent.address || actionEvent.location_name)">复制地点</button>
            </div>
          </div>
        </section>
      </div>

      <div v-if="adjustEvent" class="sheet-backdrop" @click.self="adjustEvent = null">
        <section class="sheet" role="dialog" aria-modal="true">
          <div class="sheet-grab" />
          <div class="sheet-head"><div><div class="eyebrow">调整</div><h2>记录一次调整</h2></div><button class="icon-btn" aria-label="关闭" @click="adjustEvent = null"><AppIcon name="close" /></button></div>
          <form class="sheet-body" @submit.prevent="submitAdjust">
            <div class="field"><label>调整类型</label><div class="chip-row"><button type="button" class="chip" :class="{ active: adjustForm.type === 'doctor' }" @click="adjustForm.type = 'doctor'">医生调整</button><button type="button" class="chip" :class="{ active: adjustForm.type === 'self' }" @click="adjustForm.type = 'self'">自己调整</button></div></div>
            <div class="field"><label>影响范围</label><div class="chip-row"><button type="button" class="chip" :class="{ active: adjustForm.scope === 'event' }" @click="adjustForm.scope = 'event'">仅本次</button><button type="button" class="chip" :class="{ active: adjustForm.scope === 'from_now' }" @click="adjustForm.scope = 'from_now'">从这次起</button><button type="button" class="chip" :class="{ active: adjustForm.scope === 'pause' }" @click="adjustForm.scope = 'pause'">暂停计划</button><button type="button" class="chip" :class="{ active: adjustForm.scope === 'end' }" @click="adjustForm.scope = 'end'">结束计划</button></div></div>
            <div v-if="adjustForm.scope === 'from_now'" class="card" style="margin-bottom:12px">
              <div class="field-row"><div class="field"><label>每次剂量</label><input v-model.number="adjustForm.doseAmount" type="number" min="0.01" step="0.01" class="input" /></div><div class="field"><label>单位</label><input v-model="adjustForm.doseUnit" class="input" /></div></div>
              <div class="field"><label>每天时间（逗号分隔，24 小时制）</label><input v-model="adjustForm.times" class="input" placeholder="08:00, 20:00" /></div>
              <div class="field"><label>结束日期（可空）</label><DateField v-model="adjustForm.endDate" placeholder="长期" clearable /></div>
            </div>
            <div class="field"><label>原因 / 备注</label><textarea v-model="adjustForm.note" class="textarea" placeholder="例如：复诊后医生减量" /></div>
            <div class="field-row"><div class="field"><label>医生（医生调整可填）</label><input v-model="adjustForm.doctor" class="input" /></div><div class="field"><label>医院</label><input v-model="adjustForm.hospital" class="input" /></div></div>
            <div class="sheet-foot"><button type="button" class="btn btn-outline btn-block" @click="adjustEvent = null">取消</button><button class="btn btn-primary btn-block" :disabled="adjusting">{{ adjusting ? '保存中…' : '确认调整' }}</button></div>
          </form>
        </section>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import AppIcon from '../components/AppIcon.vue'
import DateField from '../components/DateField.vue'
import { eventApi, medicineApi, planApi, type Dashboard, type DoseEvent, type Medicine } from '../api/medcare'
import { useBookStore } from '../stores/book'
import { addDays, formatDateTime, localDate, weekdayLong, weekdayShort } from '../utils/date'
import { useUIStore } from '../stores/ui'
import { reportError } from '../utils/errors'

const book = useBookStore()
const ui = useUIStore()
const dashboard = ref<Dashboard | null>(null)
const loading = ref(true)
const alerts = ref<Medicine[]>([])
const date = ref(localDate())
const actionEvent = ref<DoseEvent | null>(null)
const adjustEvent = ref<DoseEvent | null>(null)
const adjustPlan = ref<any>(null)
// 能力判定与后端 requirePermission 同源：管理员可以逐人指定谁能点提醒、谁能点完成、
// 谁能管药品与计划。老人不识字时，通常由家人在自己的手机上代打卡。
const canRemind = computed(() => book.canRemind)
const canRecord = computed(() => book.canRecord)
const canMedicine = computed(() => book.canMedicineCreate || book.canPrescCreate)
const canPlan = computed(() => book.canPlanCreate)
const noRecordHint = '管理员没有勾「用药记录 · 点完成」，可在「成员与权限」里勾上，或点「提醒用药」催家人'
const adjusting = ref(false)
const today = localDate()
const adjustForm = reactive({ type: 'doctor', scope: 'event', doseAmount: 1, doseUnit: '片', times: '08:00', endDate: '', note: '', doctor: '', hospital: '' })
let loadingSeq = 0
// 今日用药状态：未到时 / 进行中 / 超时 / 已完成 / 已跳过 / 漏服。
const GRACE_MS = 15 * 60 * 1000
const nowTs = ref(Date.now())
let clockTimer: number | null = null
const statusFilter = ref('')
const statusFilters = [
  { value: '', label: '全部' },
  { value: 'upcoming', label: '未到时' },
  { value: 'due', label: '进行中' },
  { value: 'overdue', label: '超时' },
  { value: 'done', label: '已完成' },
  { value: 'skipped', label: '已跳过' },
  { value: 'missed', label: '漏服' },
]
function eventTs(ev: DoseEvent): number {
  const t = Date.parse(ev.scheduled_at || '')
  if (!Number.isNaN(t)) return t
  const d = (ev as any).local_date || ''
  const tm = (ev as any).local_time || '00:00'
  return Date.parse(d + 'T' + tm + ':00+08:00')
}
function eventStatus(ev: DoseEvent): string {
  if (ev.status === 'done') return 'done'
  if (ev.status === 'skipped') return 'skipped'
  if (ev.status === 'missed') return 'missed'
  const at = eventTs(ev)
  if (!at) return 'upcoming'
  const diff = nowTs.value - at
  if (diff < 0) return 'upcoming'
  if (diff < GRACE_MS) return 'due'
  return 'overdue'
}

const dateTitle = computed(() => {
  const [y, m, d] = date.value.split('-')
  return `${Number(m)}月${Number(d)}日`
})
const weekdayLabel = computed(() => weekdayLong(date.value))
const isToday = computed(() => date.value === localDate())
const progress = computed(() => {
  const total = dashboard.value?.summary?.total || 0
  const done = dashboard.value?.summary?.done || 0
  return total ? Math.round((done / total) * 100) : 0
})
// 首屏摘要：今天要用的药按时间取前 3 项，一打开就知道今天吃什么。
const medSummary = computed(() => {
  const list = [...allEvents.value].sort((a, b) => String(a.local_time || '').localeCompare(String(b.local_time || '')))
  const head = list.slice(0, 3).map((ev) => (ev.local_time ? ev.local_time + ' ' : '') + ev.medicine_name)
  if (!head.length) return ''
  return head.join('、') + (list.length > 3 ? ' 等 ' + list.length + ' 项' : '')
})

const allEvents = computed<DoseEvent[]>(() => {
  const seen = new Set<number>()
  const out: DoseEvent[] = []
  const push = (ev: DoseEvent) => { if (ev && !seen.has(ev.id)) { seen.add(ev.id); out.push(ev) } }
  ;(dashboard.value?.overdue || []).forEach(push)
  ;(dashboard.value?.groups || []).forEach((g) => (g.events || []).forEach(push))
  return out
})
// 药品分类筛选：口服 / 注射 / 输液 / 外用 / 其他
const typeFilter = ref('')
const typeFilters = [
  { value: '', label: '全部' },
  { value: 'oral', label: '口服' },
  { value: 'injection', label: '注射' },
  { value: 'infusion', label: '输液' },
  { value: 'topical', label: '外用' },
  { value: 'other', label: '其他' },
]
function matchesType(ev: DoseEvent) { return !typeFilter.value || (ev.type || 'other') === typeFilter.value }
const typedEvents = computed(() => allEvents.value.filter(matchesType))
const statusCounts = computed<Record<string, number>>(() => {
  const c: Record<string, number> = { all: typedEvents.value.length, upcoming: 0, due: 0, overdue: 0, done: 0, skipped: 0, missed: 0 }
  for (const ev of typedEvents.value) { const s = eventStatus(ev); c[s] = (c[s] || 0) + 1 }
  return c
})
function matchesStatus(ev: DoseEvent) { return !statusFilter.value || eventStatus(ev) === statusFilter.value }
// 超时的单独置顶展示，时间分组里不再重复。
const overdueIds = computed(() => new Set(allEvents.value.filter((ev) => eventStatus(ev) === 'overdue').map((ev) => ev.id)))
const visibleGroups = computed(() => (dashboard.value?.groups || [])
  .map((g) => ({ ...g, events: (g.events || []).filter((ev) => !overdueIds.value.has(ev.id)).filter(matchesType).filter(matchesStatus) }))
  .filter((g) => g.events.length))
const hasTodayEvents = computed(() => (dashboard.value?.summary?.total || 0) > 0)
const upcomingLabel = computed(() => {
  const ev = dashboard.value?.next_upcoming
  if (!ev) return ''
  const day = ev.local_date === localDate() ? '今天' : ev.local_date.slice(5).replace('-', '月') + '日'
  return day + ' ' + ev.local_time
})
const filteredOverdue = computed(() => allEvents.value.filter((ev) => eventStatus(ev) === 'overdue').filter(matchesType).filter(matchesStatus))
const dateStrip = computed(() => {
  const out = []
  for (let i = -3; i <= 3; i++) {
    const day = addDays(date.value, i)
    out.push({ date: day, week: weekdayShort(day), day: Number(day.slice(8)) })
  }
  return out
})

function typeIcon(type: string) { return type === 'injection' ? 'pill' : type === 'infusion' ? 'location' : 'pill' }
function executorLabel(type?: string) { return type === 'self' ? '自己' : type === 'family' ? '家人' : type === 'nurse' ? '护士' : type === 'doctor' ? '医生' : type === 'other' ? '他人' : '执行人' }
function hm(iso?: string) { if (!iso) return ''; try { return new Intl.DateTimeFormat('zh-CN', { hour: '2-digit', minute: '2-digit', hour12: false, timeZone: 'Asia/Shanghai' }).format(new Date(iso)) } catch { return '' } }
function actualRange(ev: any) {
  const s = hm(ev?.actual_start_at); const e = hm(ev?.actual_end_at)
  if (!s && !e) return ''
  let text = '实际 '
  if (s) text += s + ' 开始'
  if (e) text += (s ? ' · ' : '') + e + ' 结束'
  if (s && e) { const mins = Math.round((new Date(ev.actual_end_at).getTime() - new Date(ev.actual_start_at).getTime()) / 60000); if (mins > 0) text += '（' + mins + ' 分钟）' }
  return text
}
async function startInfusion(ev: DoseEvent) {
  if (!book.currentId) return
  try { await eventApi.start(book.currentId, ev.id); ui.showToast('已记录开始时间'); actionEvent.value = null; await load() } catch (e: any) { ui.showToast(e?.response?.data?.message || '记录失败', 'error') }
}
function infusionText(ev: any) { const parts: string[] = []; if (ev?.drip_rate > 0) parts.push('滴速 ' + ev.drip_rate + ' 滴/分'); if (ev?.duration_minutes > 0) parts.push('预计 ' + ev.duration_minutes + ' 分钟'); return parts.join(' · ') }
function statusClass(ev: DoseEvent) {
  const s = eventStatus(ev)
  if (s === 'done') return 'done'
  if (s === 'missed' || s === 'overdue') return 'missed'
  if (s === 'due') return 'doctor'
  if (s === 'skipped') return ev.skip_reason === 'doctor_adjusted' ? 'doctor' : 'self'
  return 'pending'
}
function overdueLabel(ev: DoseEvent) {
  if (ev.overdue_minutes < 1) return '刚到时间'
  if (ev.overdue_minutes < 60) return '逾期 ' + ev.overdue_minutes + ' 分钟'
  const hours = Math.floor(ev.overdue_minutes / 60)
  const mins = ev.overdue_minutes % 60
  return '逾期 ' + hours + ' 小时' + (mins ? ' ' + mins + ' 分钟' : '')
}
function statusLabel(ev: DoseEvent) {
  const s = eventStatus(ev)
  if (s === 'done') return ev.was_missed ? '补记完成' : '已完成'
  if (s === 'missed') return '漏服'
  if (s === 'skipped') return ev.skip_reason === 'doctor_adjusted' ? '医生调整' : ev.skip_reason === 'self_adjusted' ? '自己调整' : '已跳过'
  if (s === 'upcoming') return '未到时'
  if (s === 'due') return '进行中'
  return '超时'
}
function selectDate(d: string) { date.value = d; load() }
async function load() {
  if (!book.currentId) return
  const seq = ++loadingSeq
  loading.value = true
  try {
    // 库存提醒属于「药品与库存 · 查看」，没有这一格时不请求，今日页照常能看。
    const [dash, alertRes] = await Promise.all([
      eventApi.dashboard(book.currentId, date.value),
      book.canMedicineView ? medicineApi.alerts(book.currentId) : Promise.resolve(null),
    ])
    if (seq !== loadingSeq) return
    dashboard.value = dash.data?.data || null
    const a = alertRes?.data?.data || {}
    alerts.value = [...(a.low_stock || []), ...(a.expiring || []), ...(a.expired || [])]
  } catch (e: any) {
    ui.showToast(e?.response?.data?.message || '读取今日安排失败', 'error')
  } finally { if (seq === loadingSeq) loading.value = false }
}
/** 列表里的圆形勾选：没有「点击完成」权限时不静默失败，直接说明该找谁。 */
function onCheck(ev: DoseEvent) {
  if (!canRecord.value) {
    ui.showToast(noRecordHint, 'error')
    return
  }
  if (ev.status === 'done') undo(ev)
  else complete(ev)
}
async function complete(ev: DoseEvent) {
  if (!book.currentId) return
  actionEvent.value = null
  try {
    const res = await eventApi.complete(book.currentId, ev.id, { version: ev.version, deduct_stock: true })
    ui.showToast('已完成')
    await load()
  } catch (e: any) {
    const code = e?.response?.data?.code
    if (code === 2002) {
      const ok = await ui.confirm({ title: '库存不足', message: '库存不足，是否仍标记完成？会按现有库存扣减并记录缺口。', confirmText: '仍完成' })
      if (ok) {
        try { await eventApi.complete(book.currentId, ev.id, { version: ev.version, deduct_stock: true, allow_shortage: true }); ui.showToast('已完成'); await load() } catch (err: any) { ui.showToast(err?.response?.data?.message || '操作失败', 'error') }
      }
      return
    }
    if (code === 2003) {
      const ok = await ui.confirm({ title: '药品已过期', message: '当前只剩过期批次。建议不要使用。是否仅标记完成、不扣库存？', confirmText: '仍标记' })
      if (ok) {
        try { await eventApi.complete(book.currentId, ev.id, { version: ev.version, deduct_stock: false }); ui.showToast('已标记完成'); await load() } catch (err: any) { ui.showToast(err?.response?.data?.message || '操作失败', 'error') }
      }
      return
    }
    ui.showToast(e?.response?.data?.message || '完成失败', 'error')
  }
}
async function undo(ev: DoseEvent) {
  if (!book.currentId) return
  try { await eventApi.undo(book.currentId, ev.id); ui.showToast('已撤销'); await load() } catch (e: any) { ui.showToast(e?.response?.data?.message || '撤销失败', 'error') }
}
async function miss(ev: DoseEvent) {
  if (!book.currentId) return
  actionEvent.value = null
  const ok = await ui.confirm({ title: '标记漏服？', message: '标记后会取消该事件的提醒，之后仍可补记完成。', confirmText: '标记漏服' })
  if (!ok) return
  try { await eventApi.miss(book.currentId, ev.id, {}); ui.showToast('已标记漏服'); await load() } catch (e: any) { ui.showToast(e?.response?.data?.message || '操作失败', 'error') }
}
async function remind(ev: DoseEvent) {
  if (!book.currentId) return
  try {
    const res = await eventApi.remind(book.currentId, ev.id)
    const count = res.data?.data?.reminded || 0
    ui.showToast('已提醒 ' + count + ' 位用药人/监护人')
  } catch (e: any) { ui.showToast(e?.response?.data?.message || '提醒失败', 'error') }
}
async function snooze(ev: DoseEvent, minutes: number) {
  if (!book.currentId) return
  actionEvent.value = null
  try { await eventApi.snooze(book.currentId, ev.id, minutes); ui.showToast(`已稍后 ${minutes} 分钟`); await load() } catch (e: any) { ui.showToast(e?.response?.data?.message || '稍后提醒失败', 'error') }
}
function openActions(ev: DoseEvent) { actionEvent.value = ev }
async function openAdjust(ev: DoseEvent) {
  actionEvent.value = null
  adjustEvent.value = ev
  adjustForm.note = ''
  adjustForm.doctor = ''
  adjustForm.hospital = ''
  adjustForm.scope = 'event'
  adjustForm.type = 'doctor'
  try {
    const res = await planApi.get(book.currentId!, ev.plan_id)
    adjustPlan.value = res.data?.data?.plan
    adjustForm.doseAmount = adjustPlan.value?.dose_amount || ev.dose_amount
    adjustForm.doseUnit = adjustPlan.value?.dose_unit || ev.dose_unit
    adjustForm.times = (adjustPlan.value?.frequency?.times || [ev.local_time]).join(', ')
    adjustForm.endDate = adjustPlan.value?.end_date || ''
  } catch (e) { reportError(e, '读取计划详情失败') }
}
async function submitAdjust() {
  if (!book.currentId || !adjustEvent.value) return
  if (adjustForm.type === 'doctor' && !adjustForm.note.trim()) { ui.showToast('请填写医生调整原因', 'error'); return }
  const changes: any = adjustForm.scope === 'from_now' ? {
    medicine_id: adjustPlan.value?.medicine_id || adjustEvent.value.medicine_id,
    title: adjustPlan.value?.title || adjustEvent.value.plan_title,
    type: adjustPlan.value?.type || adjustEvent.value.type,
    dose_amount: Number(adjustForm.doseAmount),
    dose_unit: adjustForm.doseUnit,
    frequency: { type: 'daily', times: adjustForm.times.split(',').map((s) => s.trim()).filter(Boolean) },
    start_date: adjustPlan.value?.start_date || today,
    end_date: adjustForm.endDate || '',
    timezone: adjustPlan.value?.timezone || book.current?.timezone || 'Asia/Shanghai',
    doctor: adjustForm.doctor,
    hospital: adjustForm.hospital,
    notes: adjustForm.note,
  } : undefined
  if (adjustForm.scope === 'from_now' && (!changes.dose_amount || !changes.dose_unit || !changes.frequency.times.length)) {
    ui.showToast('请填写剂量和至少一个时间', 'error'); return
  }
  adjusting.value = true
  try {
    await eventApi.adjust(book.currentId, adjustEvent.value.id, {
      version: adjustEvent.value.version,
      adjustment_type: adjustForm.type,
      scope: adjustForm.scope,
      changes,
      note: adjustForm.note,
      doctor_name: adjustForm.doctor,
      hospital: adjustForm.hospital,
    })
    ui.showToast('调整已记录')
    adjustEvent.value = null
    await load()
  } catch (e: any) { ui.showToast(e?.response?.data?.message || '调整失败', 'error') }
  finally { adjusting.value = false }
}
async function copy(text: string) { try { await navigator.clipboard.writeText(text); ui.showToast('已复制') } catch { ui.showToast('复制失败', 'error') } }
onMounted(() => {
  load()
  // 每分钟刷新一次「未到时/进行中/超时」的判定。
  clockTimer = window.setInterval(() => { nowTs.value = Date.now() }, 30000)
})
onUnmounted(() => { if (clockTimer) window.clearInterval(clockTimer) })
watch(() => book.currentId, load)
</script>

<style scoped>
.date-strip { display: grid; grid-template-columns: repeat(7, minmax(0, 1fr)); gap: 6px; padding: 2px 0 8px; }
.date-strip::-webkit-scrollbar { display: none; }
.date-pill { width: 100%; aspect-ratio: 1 / 1; min-height: 0; border-radius: 12px; border: 1px solid var(--mc-border); background: var(--mc-surface); color: var(--mc-text-muted); display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 1px; }
.date-pill span { font-size: 10px; font-weight: 650; }
.date-pill b { font-size: 16px; color: var(--mc-text); }
.date-pill.active { background: var(--mc-primary); border-color: var(--mc-primary); }
.date-pill.active span, .date-pill.active b { color: #fff; }
.date-pill.today:not(.active) { border-color: var(--mc-primary); }
.progress-card { margin-top: 12px; display: flex; gap: 12px; align-items: center; }
.progress-main { flex: 1; min-width: 0; }
.progress-count { font-size: 19px; font-weight: 800; margin-top: 1px; }
.next-card { margin-top: 10px; display: flex; align-items: center; justify-content: space-between; gap: 10px; flex-wrap: wrap; background: linear-gradient(135deg, var(--mc-primary-soft), var(--mc-surface)); }
.next-main { min-width: 0; flex: 1 1 140px; }
.next-title { font-size: 16px; font-weight: 800; margin-top: 2px; line-height: 1.3; overflow-wrap: anywhere; }
.next-actions { display: flex; gap: 6px; flex: 1 1 auto; justify-content: flex-end; flex-wrap: wrap; }
.next-actions .btn { white-space: nowrap; }
.empty-actions { display: flex; gap: 8px; justify-content: center; margin-top: 12px; flex-wrap: wrap; }
.alert-row { padding: 10px 12px; display: flex; align-items: center; gap: 8px; }
.location-line { color: var(--mc-primary-strong); align-items: center; }
@media (min-width: 900px) {
  .date-strip { display: flex; gap: 8px; padding-bottom: 10px; }
  /* 桌面也保持正方形：固定 48×48，别让旧的 min-height 把它拉高。 */
  .date-pill { flex: 0 0 48px; width: 48px; aspect-ratio: 1 / 1; min-height: 0; border-radius: 13px; }
  .date-pill span { font-size: 10px; }
  .date-pill b { font-size: 17px; }
  .progress-card { margin-top: 18px; gap: 16px; }
  .progress-count { font-size: 22px; }
  .next-card { margin-top: 12px; }
  .next-title { font-size: 18px; }
  .next-actions { gap: 8px; }
  .empty-actions { margin-top: 16px; }
  .alert-row { padding: 12px 14px; gap: 10px; }
}
.type-bar { display: flex; flex-wrap: wrap; gap: 6px; padding: 2px 0 4px; }
.type-chip { flex: 0 0 auto; min-height: 30px; padding: 4px 12px; border-radius: 999px; border: 1px solid var(--mc-border); background: transparent; color: var(--mc-text-muted); font-size: 12px; font-weight: 650; white-space: nowrap; }
.type-chip.active { border-color: var(--mc-primary); background: var(--mc-primary-soft); color: var(--mc-primary-strong); }
.status-bar { display: flex; flex-wrap: wrap; gap: 6px; padding: 2px 0 8px; }
.status-chip { flex: 0 0 auto; display: inline-flex; align-items: center; gap: 5px; min-height: 34px; padding: 5px 10px; border-radius: 999px; border: 1px solid var(--mc-border); background: var(--mc-surface); color: var(--mc-text-muted); font-size: 12px; font-weight: 650; white-space: nowrap; }
.status-chip b { font-size: 11px; min-width: 16px; padding: 0 4px; border-radius: 999px; background: var(--mc-surface-2); color: var(--mc-text); text-align: center; }
.status-chip.active { border-color: var(--mc-primary); background: var(--mc-primary-soft); color: var(--mc-primary-strong); }
.status-chip.overdue.active, .status-chip.missed.active { border-color: var(--mc-danger); background: var(--mc-danger-soft); color: var(--mc-danger); }
.status-chip.due.active { border-color: var(--mc-info); background: var(--mc-info-soft); color: var(--mc-info); }
</style>
