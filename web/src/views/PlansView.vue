<template>
  <div class="page">
    <div class="page-head">
      <div><div class="eyebrow">计划</div><h1>用药计划</h1><p>按时间看今天、明天和整月，或按药名看每种药怎么用。</p></div>
      <div class="head-actions">
        <router-link v-if="book.canPrescCreate" to="/prescriptions?from=plans&purpose=plan" class="btn btn-outline btn-sm"><AppIcon name="camera" :size="16" />AI 识别处方</router-link>
        <router-link v-if="book.canPlanCreate" to="/plans/new" class="btn btn-primary"><AppIcon name="plus" :size="18" />新建</router-link>
      </div>
    </div>

    <div class="mode-switch">
      <button class="mode-btn" :class="{ active: mode === 'time' }" @click="switchMode('time')"><AppIcon name="calendar" :size="17" />按时间</button>
      <button class="mode-btn" :class="{ active: mode === 'medicine' }" @click="switchMode('medicine')"><AppIcon name="pill" :size="17" />按药名</button>
    </div>

    <div class="chip-row type-row">
      <button v-for="t in typeFilters" :key="t.value" type="button" class="chip" :class="{ active: typeFilter === t.value }" @click="typeFilter = t.value">
        {{ t.label }}<b class="chip-count">{{ typeCounts[t.value] || 0 }}</b>
      </button>
    </div>

    <!-- 药名模糊搜索：按时间（按天/按月）与按药名两种视图共用同一个关键词 -->
    <div class="card plan-search">
      <AppIcon name="search" :size="19" class="ps-icon" />
      <input v-model="medQuery" class="input ps-input" placeholder="搜索药品名 / 计划标题" />
      <button v-if="medQuery" class="ps-clear" aria-label="清空搜索" @click="medQuery = ''"><AppIcon name="close" :size="16" /></button>
    </div>

    <!-- 药品已被删除的计划：计划还在但库存为 0，打卡会提示库存不足，这里提前提醒去改 -->
    <div v-if="missingPlans.length" class="card missing-banner">
      <div class="small" style="color:var(--mc-danger);font-weight:700">有 {{ missingPlans.length }} 个计划的药品已被删除</div>
      <p class="tiny muted" style="margin-top:4px;line-height:1.6">这些计划打卡时会因库存为 0 提示库存不足。点下面的计划，把「药品」改成现有的药品即可恢复。</p>
      <div class="missing-list">
        <button v-for="p in missingPlans" :key="p.id" type="button" class="missing-item" @click="router.push('/plans/' + p.id)">
          <span class="missing-name">{{ p.title || p.medicine_name || '未命名计划' }}</span>
          <span class="tiny muted">{{ p.medicine_name || '药品已删除' }} · 点此修改</span>
          <AppIcon name="chevron" :size="16" />
        </button>
      </div>
    </div>

    <!-- ============ 按时间 ============ -->
    <template v-if="mode === 'time'">
      <div class="card time-controls">
        <div class="view-tabs">
          <button :class="{ active: timeView === 'day' }" @click="setTimeView('day')">按天</button>
          <button :class="{ active: timeView === 'month' }" @click="setTimeView('month')">按月</button>
        </div>
        <template v-if="timeView === 'day'">
          <div class="date-nav">
            <button class="icon-btn" aria-label="前一天" @click="setDay(addDays(selectedDate, -1))"><AppIcon name="arrowLeft" :size="18" /></button>
            <div class="date-center">
              <DateField v-model="selectedDate" placeholder="选择日期" />
              <div class="date-caption">{{ humanDate(selectedDate) }}</div>
            </div>
            <button class="icon-btn" aria-label="后一天" @click="setDay(addDays(selectedDate, 1))"><AppIcon name="chevron" :size="18" /></button>
          </div>
          <div class="chip-row" style="margin-top:8px">
            <button type="button" class="chip" :class="{ active: selectedDate === today }" @click="setDay(today)">今天</button>
            <button type="button" class="chip" :class="{ active: selectedDate === tomorrow }" @click="setDay(tomorrow)">明天</button>
          </div>
        </template>
        <template v-else>
          <div class="month-nav">
            <button class="icon-btn" aria-label="上个月" @click="moveMonth(-1)"><AppIcon name="arrowLeft" :size="18" /></button>
            <b>{{ monthLabel }}</b>
            <button class="icon-btn" aria-label="下个月" @click="moveMonth(1)"><AppIcon name="chevron" :size="18" /></button>
            <button class="btn btn-ghost btn-xs" @click="goCurrentMonth">本月</button>
          </div>
        </template>
      </div>

      <div v-if="timeLoading" class="card muted">加载中…</div>

      <template v-else-if="timeView === 'day'">
        <div v-if="!filteredDayEvents.length" class="empty card">
          <div class="empty-icon"><AppIcon name="calendar" :size="28" /></div>
          <h3>{{ (typeFilter || medQuery) ? '没有符合条件的安排' : '这天没有安排' }}</h3>
          <p>{{ medQuery ? '换个药名关键词试试，或清空搜索。' : '可以前后翻一天，或到「按月」看看整月的计划。' }}</p>
        </div>
        <div v-else class="list">
          <div v-for="ev in filteredDayEvents" :key="ev.id" class="card event-card" @click="router.push('/plans/' + ev.plan_id)">
            <div class="event-time"><b>{{ ev.local_time }}</b><span>{{ eventStatus(ev.status) }}</span></div>
            <div class="event-main">
              <div class="event-title"><b>{{ ev.medicine_name || '药品已删除' }}</b><span v-if="ev.medicine_missing" class="badge missed">药品已删除</span><span class="badge" :class="eventBadge(ev.status)">{{ eventStatus(ev.status) }}</span></div>
              <div class="tiny muted">{{ ev.plan_title || '用药计划' }} · {{ ev.dose_amount }} {{ ev.dose_unit }}<span v-if="ev.location_name"> · {{ ev.location_name }}</span></div>
              <div v-if="ev.executor_name" class="tiny muted">{{ ev.executor_name }}</div>
              <div v-if="ev.medicine_missing" class="tiny" style="color:var(--mc-danger)">关联药品已被删除，点进计划改成现有药品，否则会提示库存不足</div>
            </div>
            <AppIcon name="chevron" :size="16" class="event-chevron" />
          </div>
        </div>
      </template>

      <template v-else>
        <div class="card calendar-card">
          <div class="weekdays"><span v-for="w in weekdays" :key="w">{{ w }}</span></div>
          <div class="month-grid">
            <div v-for="n in firstOffset" :key="'blank' + n" class="month-cell blank" />
            <button v-for="d in monthDays" :key="d" type="button" class="month-cell" :class="{ active: selectedDate === monthDate(d), today: monthDate(d) === today, 'has-events': (monthCounts[monthDate(d)]?.total || 0) > 0 }" @click="setDay(monthDate(d))">
              <span class="day-num">{{ d }}</span>
              <span v-if="(monthCounts[monthDate(d)]?.total || 0) > 0" class="day-count">{{ monthCounts[monthDate(d)]?.total }} 项</span>
              <span v-if="(monthCounts[monthDate(d)]?.missed || 0) > 0" class="day-dot missed" />
              <span v-else-if="(monthCounts[monthDate(d)]?.pending || 0) > 0" class="day-dot pending" />
              <span v-else-if="(monthCounts[monthDate(d)]?.done || 0) > 0" class="day-dot done" />
            </button>
          </div>
          <p class="tiny muted" style="margin-top:10px">点某天可切到按天查看；圆点表示当天有未完成/漏服/已完成安排。</p>
        </div>

        <div class="section-head" style="margin-top:16px"><h2>本月安排</h2><span class="tiny muted">共 {{ (typeFilter || medQuery) ? filteredMonthEvents.length : monthTotal }} 项</span></div>
        <div v-if="!monthGroups.length" class="empty card"><div class="empty-icon"><AppIcon name="calendar" :size="28" /></div><h3>{{ medQuery ? '没有匹配「' + medQuery + '」的安排' : '这个月还没有安排' }}</h3><p>{{ medQuery ? '换个关键词，或清空搜索看全部。' : '切换月份或先新建计划。' }}</p></div>
        <div v-else class="list">
          <div v-for="group in monthGroups" :key="group.date" class="card month-group">
            <button type="button" class="month-group-head" @click="setDay(group.date)">
              <b>{{ humanDate(group.date) }}</b><span class="tiny muted">{{ group.events.length }} 项</span><AppIcon name="chevron" :size="16" />
            </button>
            <button v-for="ev in group.events" :key="ev.id" type="button" class="month-event" @click.stop="router.push('/plans/' + ev.plan_id)">
              <span class="month-time">{{ ev.local_time }}</span>
              <span class="month-med">{{ ev.medicine_name }}</span>
              <span class="tiny muted">{{ ev.dose_amount }} {{ ev.dose_unit }}</span>
              <span class="badge" :class="eventBadge(ev.status)">{{ eventStatus(ev.status) }}</span>
            </button>
          </div>
        </div>
        <p v-if="monthTotal > monthEvents.length" class="tiny muted" style="text-align:center;margin-top:10px">本月共 {{ monthTotal }} 项，列表先展示 {{ monthEvents.length }} 项，点具体日期查看当天全部。</p>
      </template>
    </template>

    <!-- ============ 按药名 ============ -->
    <template v-else>
      <div class="tabs" style="margin-bottom:10px">
        <button v-for="t in tabs" :key="t.value" class="tab" :class="{ active: status === t.value }" @click="status = t.value; loadPlans()">
          {{ t.label }}<b class="tab-count">{{ statusCounts[t.value] || 0 }}</b>
        </button>
      </div>

      <div v-if="planLoading" class="card muted">加载中…</div>
      <div v-else-if="!plans.length" class="empty card">
        <div class="empty-icon"><AppIcon name="calendar" :size="30" /></div>
        <h3>还没有计划</h3>
        <p>手动新建，或拍处方单让 AI 识别后自动生成计划。</p>
        <div v-if="book.canPlanCreate" class="empty-actions">
          <router-link to="/prescriptions?from=plans&purpose=plan" class="btn btn-primary">AI 识别处方单</router-link>
          <router-link to="/plans/new" class="btn btn-outline">手动新建</router-link>
        </div>
      </div>
      <div v-else-if="!medicineGroups.length" class="empty card">
        <div class="empty-icon"><AppIcon name="search" :size="28" /></div>
        <h3>没有匹配「{{ medQuery }}」的药品</h3>
        <p>换个关键词试试，或清空搜索看全部计划。</p>
        <button class="btn btn-outline" @click="medQuery = ''">清空搜索</button>
      </div>
      <div v-else class="list">
        <section v-for="group in medicineGroups" :key="group.key" class="medicine-group">
          <div class="medicine-head">
            <div class="type-dot"><AppIcon name="pill" :size="18" /></div>
            <div class="medicine-head-main"><b>{{ group.name }}</b><span class="tiny muted">{{ group.plans.length }} 个计划<span v-if="group.nextText"> · 下次 {{ group.nextText }}</span></span></div>
          </div>
          <article v-for="p in group.plans" :key="p.id" class="card plan-card" @click="router.push('/plans/' + p.id)">
            <div class="plan-main">
              <div class="type-dot" :class="p.type"><AppIcon :name="p.type === 'infusion' ? 'location' : 'pill'" :size="18" /></div>
              <div class="plan-body">
                <div class="plan-title-row"><b>{{ p.title || p.medicine_name || '药品已删除' }}</b><span v-if="p.medicine_missing" class="badge missed">药品已删除</span><span class="badge" :class="p.status === 'active' || p.status === 'completed' ? 'done' : 'pending'">{{ statusLabel(p.status) }}</span>
                  <span v-if="p.status === 'active' && p.today_total > 0 && p.today_done >= p.today_total" class="badge done">今日已完成</span></div>
                <div v-if="p.medicine_missing" class="tiny" style="color:var(--mc-danger);margin-top:2px">关联药品已被删除，点进来改成现有药品，否则会提示库存不足</div>
                <div class="muted small" style="margin-top:2px">{{ p.dose_amount }} {{ p.dose_unit }} · {{ frequencyLabel(p) }}</div>
                <div class="tiny muted" style="margin-top:3px">{{ p.start_date }}{{ p.end_date ? ' 至 ' + p.end_date : ' 起' }}<span v-if="p.location_name"> · {{ p.location_name }}</span></div>
                <div class="progress-track" style="margin-top:8px"><span :style="{ width: p.total_count ? Math.round(p.completed_count / p.total_count * 100) + '%' : '0%' }" /></div>
                <div class="tiny muted" style="margin-top:3px">完成 {{ p.completed_count }} / {{ p.total_count }} 项<span v-if="p.next_event_at"> · 下次 {{ formatTime(p.next_event_at) }}</span></div>
              </div>
              <AppIcon name="chevron" :size="18" class="plan-chevron" />
            </div>
            <div v-if="(book.canPlanEdit || book.canPlanDelete) && (p.status === 'active' || p.status === 'paused')" class="plan-actions" @click.stop>
              <button class="btn btn-outline btn-xs" @click="p.status === 'active' ? pause(p) : resume(p)">{{ p.status === 'active' ? '暂停' : '恢复' }}</button>
              <button class="btn btn-outline btn-xs" @click="end(p)">结束</button>
            </div>
          </article>
        </section>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import AppIcon from '../components/AppIcon.vue'
import DateField from '../components/DateField.vue'
import { eventApi, planApi, type DoseEvent, type Plan } from '../api/medcare'
import { frequencyLabel as frequencyLabelShared, planStatusLabel } from '../utils/plan'
import { useBookStore } from '../stores/book'
import { useUIStore } from '../stores/ui'
import { addDays, beijingMonthOf, daysInMonth, firstWeekdayOffset, formatDateTime as formatTime, localDate, weekdayShort } from '../utils/date'
import { fetchAllPages } from '../utils/paging'
const book = useBookStore()
const ui = useUIStore()
const router = useRouter()

const mode = ref<'time' | 'medicine'>('time')
const timeView = ref<'day' | 'month'>('day')
const selectedDate = ref(localDate())
const monthAnchor = ref(localDate().slice(0, 7))
const today = localDate()
const tomorrow = addDays(today, 1)
const dayEvents = ref<DoseEvent[]>([])
const monthEvents = ref<DoseEvent[]>([])
const monthTotal = ref(0)
const monthCounts = ref<Record<string, { total: number; pending: number; done: number; missed: number; skipped: number }>>({})
const timeLoading = ref(false)

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
const medQuery = ref('')
function matchesType(t?: string) { return !typeFilter.value || (t || 'other') === typeFilter.value }
// 药名模糊搜索：药品名或计划标题命中即可（按时间视图也用同一关键词）。
function matchesQuery(ev: DoseEvent) {
  const q = medQuery.value.trim().toLowerCase()
  if (!q) return true
  return ((ev.medicine_name || '') + ' ' + (ev.plan_title || '')).toLowerCase().includes(q)
}
const filteredDayEvents = computed(() => dayEvents.value.filter((ev) => matchesType(ev.type) && matchesQuery(ev)))
const filteredMonthEvents = computed(() => monthEvents.value.filter((ev) => matchesType(ev.type) && matchesQuery(ev)))

const allPlans = ref<Plan[]>([])
// 当前状态筛选后的计划（分类筛选在 medicineGroups 里再筛一次）
const plans = computed(() => allPlans.value.filter((p) => !status.value || p.status === status.value))
// 药品已被删除的计划：顶部提醒 + 「按时间」事件卡徽标都用它
const missingPlans = computed(() => allPlans.value.filter((p) => p.medicine_missing))
// 分类数量：按「当前状态」统计各类有多少个计划
// 计划是否命中当前药名搜索（药品名或计划标题）
function matchesPlanQuery(p: Plan) {
  const q = medQuery.value.trim().toLowerCase()
  if (!q) return true
  return ((p.medicine_name || '') + ' ' + (p.title || '')).toLowerCase().includes(q)
}
const typeCounts = computed<Record<string, number>>(() => {
  const c: Record<string, number> = { '': 0, oral: 0, injection: 0, infusion: 0, topical: 0, other: 0 }
  for (const p of plans.value) {
    if (!matchesPlanQuery(p)) continue
    c[''] = (c[''] || 0) + 1
    const t = p.type || 'other'
    c[t] = (c[t] || 0) + 1
  }
  return c
})
// 状态数量：按「当前分类」统计各状态有多少个计划
const statusCounts = computed<Record<string, number>>(() => {
  const c: Record<string, number> = { '': 0, active: 0, paused: 0, completed: 0, ended: 0 }
  for (const p of allPlans.value) {
    if (typeFilter.value && (p.type || 'other') !== typeFilter.value) continue
    if (!matchesPlanQuery(p)) continue
    c[''] = (c[''] || 0) + 1
    c[p.status] = (c[p.status] || 0) + 1
  }
  return c
})
const planLoading = ref(true)
const status = ref('active')
const tabs = [{ value: '', label: '全部' }, { value: 'active', label: '进行中' }, { value: 'paused', label: '已暂停' }, { value: 'completed', label: '已完成' }, { value: 'ended', label: '已结束' }]

const weekdays = ['一', '二', '三', '四', '五', '六', '日']
const monthInfo = computed(() => beijingMonthOf(monthAnchor.value + '-01'))
const monthDays = computed(() => daysInMonth(monthInfo.value.year, monthInfo.value.month))
const firstOffset = computed(() => firstWeekdayOffset(monthInfo.value.year, monthInfo.value.month))
const monthLabel = computed(() => monthInfo.value.year + ' 年 ' + monthInfo.value.month + ' 月')
function monthDate(day: number) { return monthAnchor.value + '-' + String(day).padStart(2, '0') }
function humanDate(date: string) { return date.slice(5).replace('-', '月') + '日 ' + weekdayShort(date) }
function eventStatus(s: string) { return s === 'done' ? '已完成' : s === 'skipped' ? '已跳过' : s === 'missed' ? '漏服' : '待用药' }
function eventBadge(s: string) { return s === 'done' ? 'done' : s === 'skipped' ? 'pending' : s === 'missed' ? 'missed' : 'pending' }
// 状态与频率文案统一放在 utils/plan.ts，药品详情的「关联计划」也用同一套。
const statusLabel = planStatusLabel
const frequencyLabel = frequencyLabelShared

const monthGroups = computed(() => {
  const map = new Map<string, DoseEvent[]>()
  for (const ev of filteredMonthEvents.value) {
    if (!map.has(ev.local_date)) map.set(ev.local_date, [])
    map.get(ev.local_date)!.push(ev)
  }
  return [...map.entries()].sort((a, b) => a[0].localeCompare(b[0])).map(([date, events]) => ({ date, events }))
})

const medicineGroups = computed(() => {
  const map = new Map<string, { key: string; name: string; plans: Plan[]; nextText: string }>()
  for (const p of plans.value) {
    if (!matchesType(p.type)) continue
    // 按药名视图支持搜索：药品名或计划标题命中即保留该计划。
    if (!matchesPlanQuery(p)) continue
    const key = String(p.medicine_id || p.medicine_name)
    if (!map.has(key)) map.set(key, { key, name: p.medicine_name, plans: [], nextText: '' })
    map.get(key)!.plans.push(p)
  }
  const groups = [...map.values()]
  for (const g of groups) {
    g.plans.sort((a, b) => (a.status === 'active' ? -1 : 1) - (b.status === 'active' ? -1 : 1))
    const next = g.plans.map((p) => p.next_event_at).filter(Boolean).sort()[0]
    g.nextText = next ? formatTime(next) : ''
  }
  return groups.sort((a, b) => a.name.localeCompare(b.name, 'zh-Hans-CN'))
})

function switchMode(m: 'time' | 'medicine') {
  mode.value = m
  if (m === 'time') { timeView.value === 'day' ? loadDay() : loadMonth() }
  else loadPlans()
}
function setTimeView(v: 'day' | 'month') { timeView.value = v; v === 'day' ? loadDay() : loadMonth() }
function setDay(d: string) { selectedDate.value = d; timeView.value = 'day'; loadDay() }
function moveMonth(delta: number) {
  const d = new Date(Date.UTC(monthInfo.value.year, monthInfo.value.month - 1 + delta, 1))
  monthAnchor.value = d.getUTCFullYear() + '-' + String(d.getUTCMonth() + 1).padStart(2, '0')
  loadMonth()
}
function goCurrentMonth() { monthAnchor.value = localDate().slice(0, 7); loadMonth() }

async function loadDay() {
  if (!book.currentId) return
  // 「按时间」看的是用药事件，属于「用药记录 · 查看」这一格。
  if (!book.canView('today')) { dayEvents.value = []; return }
  timeLoading.value = true
  try {
    const res = await eventApi.list(book.currentId, { from: selectedDate.value, to: selectedDate.value, pageSize: 200 })
    dayEvents.value = res.data?.data?.items || []
  } catch (e: any) { ui.showToast(e?.response?.data?.message || '读取当日安排失败', 'error') }
  finally { timeLoading.value = false }
}
async function loadMonth() {
  if (!book.currentId) return
  if (!book.canView('today')) { monthEvents.value = []; return }
  timeLoading.value = true
  try {
    const from = monthAnchor.value + '-01'
    const to = monthAnchor.value + '-' + String(monthDays.value).padStart(2, '0')
    const [evRes, calRes] = await Promise.all([
      eventApi.list(book.currentId, { from, to, pageSize: 200 }),
      eventApi.calendar(book.currentId, from, to),
    ])
    monthEvents.value = evRes.data?.data?.items || []
    monthTotal.value = evRes.data?.data?.total || monthEvents.value.length
    const counts: Record<string, { total: number; pending: number; done: number; missed: number; skipped: number }> = {}
    for (const row of (calRes.data?.data || [])) {
      const key = row.local_date
      if (!counts[key]) counts[key] = { total: 0, pending: 0, done: 0, missed: 0, skipped: 0 }
      counts[key].total += row.count || 0
      if (row.status === 'done') counts[key].done += row.count || 0
      else if (row.status === 'missed') counts[key].missed += row.count || 0
      else if (row.status === 'skipped') counts[key].skipped += row.count || 0
      else counts[key].pending += row.count || 0
    }
    monthCounts.value = counts
  } catch (e: any) { ui.showToast(e?.response?.data?.message || '读取本月安排失败', 'error') }
  finally { timeLoading.value = false }
}
// 计划列表一次取全（不按状态过滤），状态与分类的统计都从这份全量数据算，
// 切换 chip 只是前端筛选，不用再请求接口，也不会出现「统计数与列表对不上」。
async function loadPlans() {
  if (!book.currentId) return
  if (!book.canPlanView) { allPlans.value = []; return }
  planLoading.value = true
  try {
    allPlans.value = await fetchAllPages<Plan>(async (page, pageSize) => {
      const res = await planApi.list(book.currentId!, { page, pageSize })
      return { items: (res.data?.data?.items || []) as Plan[], total: Number(res.data?.data?.total) || 0 }
    })
  }
  catch (e: any) { ui.showToast(e?.response?.data?.message || '读取计划失败', 'error') }
  finally { planLoading.value = false }
}
async function pause(p: Plan) { await planApi.pause(book.currentId!, p.id); ui.showToast('已暂停'); loadPlans() }
async function resume(p: Plan) { await planApi.resume(book.currentId!, p.id); ui.showToast('已恢复'); loadPlans() }
async function end(p: Plan) { if (await ui.confirm({ title: '结束计划？', message: '结束后不再生成未来事件，历史记录保留。', confirmText: '结束' })) { await planApi.end(book.currentId!, p.id); ui.showToast('已结束'); loadPlans() } }

onMounted(() => { loadDay(); loadPlans() })
watch(() => book.currentId, () => { loadDay(); loadPlans() })
</script>

<style scoped>
.head-actions { display: flex; gap: 8px; flex: 0 0 auto; align-items: center; flex-wrap: wrap; justify-content: flex-end; }
.empty-actions { display: flex; gap: 8px; margin-top: 12px; flex-wrap: wrap; justify-content: center; }
.type-row { margin-bottom: 10px; }
.missing-banner { border-color: var(--mc-danger); margin-bottom: 10px; }
.missing-list { display: grid; gap: 6px; margin-top: 8px; }
.missing-item { display: flex; align-items: center; gap: 8px; width: 100%; text-align: left; padding: 9px 11px; border-radius: 10px; border: 1px solid var(--mc-border); background: var(--mc-surface-2); }
.missing-item .missing-name { font-weight: 700; font-size: 13.5px; }
.missing-item .tiny { flex: 1; min-width: 0; overflow-wrap: anywhere; }
/* chip / tab 上的数量徽标 */
.chip-count, .tab-count { display: inline-block; min-width: 16px; margin-left: 4px; padding: 0 5px; border-radius: 999px; background: var(--mc-surface-2); color: var(--mc-text); font-size: 11px; font-weight: 700; line-height: 16px; text-align: center; }
.chip.active .chip-count, .tab.active .tab-count { background: var(--mc-primary); color: #fff; }
.mode-switch { display: grid; grid-template-columns: 1fr 1fr; gap: 6px; padding: 4px; background: var(--mc-surface-2); border: 1px solid var(--mc-border); border-radius: 12px; margin-bottom: 12px; }
.mode-btn { display: inline-flex; align-items: center; justify-content: center; gap: 6px; min-height: 38px; border: 0; background: transparent; color: var(--mc-text-muted); padding: 6px 10px; border-radius: 9px; font-weight: 700; cursor: pointer; }
.mode-btn.active { background: var(--mc-surface); color: var(--mc-primary-strong); box-shadow: 0 1px 4px rgba(20,40,30,.08); }
.time-controls { padding: 12px; margin-bottom: 12px; }
.view-tabs { display: flex; gap: 6px; margin-bottom: 10px; }
.view-tabs button { flex: 1; min-height: 36px; border: 1px solid var(--mc-border); background: var(--mc-surface); color: var(--mc-text-muted); padding: 7px; border-radius: 8px; font-weight: 700; }
.view-tabs button.active { background: var(--mc-primary-soft); color: var(--mc-primary-strong); border-color: var(--mc-primary); }
.date-nav, .month-nav { display: flex; align-items: center; gap: 8px; }
.date-center { flex: 1; min-width: 0; text-align: center; }
.date-center .input { text-align: center; }
.date-caption { font-size: 12px; color: var(--mc-text-muted); margin-top: 4px; }
.month-nav { justify-content: center; }
.month-nav b { font-size: 16px; min-width: 120px; text-align: center; }
.month-nav .btn { margin-left: 4px; }
.event-card { display: flex; gap: 10px; align-items: flex-start; cursor: pointer; }
.event-time { flex: 0 0 58px; text-align: center; padding-top: 2px; }
.event-time b { display: block; font-size: 18px; color: var(--mc-primary-strong); }
.event-time span { display: block; font-size: 10.5px; color: var(--mc-text-muted); margin-top: 2px; }
.event-main { flex: 1; min-width: 0; }
.event-title { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.event-title b { font-size: 14px; }
.event-chevron { flex: 0 0 auto; margin-top: 3px; }
.calendar-card { padding: 12px; }
.weekdays { display: grid; grid-template-columns: repeat(7, 1fr); text-align: center; font-size: 11px; color: var(--mc-text-muted); margin-bottom: 6px; }
.month-grid { display: grid; grid-template-columns: repeat(7, 1fr); gap: 4px; }
.month-cell { position: relative; min-height: 46px; border: 1px solid transparent; background: var(--mc-surface-2); border-radius: 8px; padding: 5px 4px 3px; text-align: center; cursor: pointer; color: var(--mc-text); }
.month-cell.blank { background: transparent; cursor: default; }
.month-cell.today { border-color: var(--mc-primary); }
.month-cell.active { background: var(--mc-primary-soft); border-color: var(--mc-primary); }
.month-cell.has-events { background: var(--mc-surface); }
.day-num { display: block; font-size: 13px; font-weight: 700; }
.day-count { display: block; font-size: 9.5px; color: var(--mc-primary-strong); margin-top: 1px; }
.day-dot { position: absolute; left: 50%; bottom: 4px; transform: translateX(-50%); width: 4px; height: 4px; border-radius: 50%; }
.day-dot.pending { background: var(--mc-warning); }
.day-dot.missed { background: var(--mc-danger); }
.day-dot.done { background: var(--mc-success, #2f855a); }
.month-group { padding: 10px 12px; }
.month-group-head { width: 100%; display: flex; align-items: center; gap: 8px; border: 0; background: transparent; padding: 0 0 8px; color: var(--mc-text); cursor: pointer; text-align: left; }
.month-group-head b { flex: 1; font-size: 13.5px; }
.month-event { width: 100%; display: flex; align-items: center; gap: 8px; border: 0; border-top: 1px solid var(--mc-border); background: transparent; padding: 8px 0; text-align: left; color: var(--mc-text); cursor: pointer; }
.month-time { flex: 0 0 42px; font-weight: 700; font-size: 12px; color: var(--mc-primary-strong); }
.month-med { flex: 1; min-width: 0; font-size: 12.5px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.plan-search { display: flex; align-items: center; gap: 8px; padding: 0 12px; margin-bottom: 10px; }
/* 聚焦高亮放在整张搜索卡上；内部输入框彻底去掉自身边框与圆角，避免出现“胶囊套方框”。 */
.plan-search:focus-within { border-color: var(--mc-primary); box-shadow: 0 0 0 3px var(--mc-primary-soft); }
.ps-icon { flex: 0 0 auto; color: var(--mc-text-muted); }
.ps-input, .ps-input:focus { border: 0 !important; background: transparent !important; box-shadow: none !important; border-radius: 0; padding-left: 0; min-height: 44px; }
.ps-clear { flex: 0 0 auto; border: 0; background: var(--mc-surface-2); color: var(--mc-text-muted); width: 28px; height: 28px; border-radius: 999px; display: grid; place-items: center; }
.medicine-group { display: grid; gap: 8px; }
.medicine-head { display: flex; gap: 9px; align-items: center; padding: 2px 2px 0; }
.medicine-head-main { display: flex; flex-direction: column; min-width: 0; }
.medicine-head-main b { font-size: 15px; }
.plan-card { display: grid; gap: 10px; cursor: pointer; }
.plan-main { display: flex; gap: 10px; align-items: flex-start; min-width: 0; }
.plan-body { flex: 1; min-width: 0; }
.plan-title-row { display: flex; gap: 6px; align-items: center; flex-wrap: wrap; min-width: 0; }
.plan-title-row b { font-size: 14.5px; overflow-wrap: anywhere; }
.plan-chevron { flex: 0 0 auto; }
.plan-actions { display: flex; gap: 6px; flex-wrap: wrap; }
@media (min-width: 900px) {
  .plan-card { gap: 12px; }
  .plan-main { gap: 10px; }
  .plan-title-row { gap: 8px; }
  .plan-title-row b { font-size: 15px; }
  .plan-actions { gap: 8px; }
}
</style>