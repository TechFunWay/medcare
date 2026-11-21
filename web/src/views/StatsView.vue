<template>
  <div class="page">
    <div class="page-head">
      <div><div class="eyebrow">统计</div><h1>执行情况</h1><p>{{ book.current?.name }} · 看看哪里容易漏。</p></div>
    </div>
    <div class="tabs" style="margin-bottom:10px"><button v-for="p in periods" :key="p.days" class="tab" :class="{ active: days === p.days }" @click="days = p.days; load()">{{ p.label }}</button></div>
    <div v-if="loading" class="card muted">加载中…</div>
    <template v-else>
      <div class="grid-4">
        <div class="stat-card"><div class="stat-value">{{ Math.round((overview?.completion_rate || 0) * 100) }}%</div><div class="stat-label">完成率</div><div class="progress-track" style="margin-top:6px"><span :style="{ width: Math.round((overview?.completion_rate || 0) * 100) + '%' }" /></div></div>
        <div class="stat-card"><div class="stat-value">{{ overview?.done || 0 }}<span class="muted stat-sub">/{{ overview?.total || 0 }}</span></div><div class="stat-label">完成/应完成</div></div>
        <div class="stat-card"><div class="stat-value" style="color:var(--mc-danger)">{{ overview?.missed || 0 }}</div><div class="stat-label">当前漏服<span class="stat-extra">曾 {{ overview?.missed_ever || 0 }} 次</span></div></div>
        <div class="stat-card"><div class="stat-value">{{ overview?.clean_streak_days || 0 }}</div><div class="stat-label">连续无漏服<span class="stat-extra">天数</span></div></div>
        <div class="stat-card"><div class="stat-value" style="color:var(--mc-info)">{{ overview?.skipped_doctor || 0 }}</div><div class="stat-label">医生调整</div></div>
        <div class="stat-card"><div class="stat-value" style="color:var(--mc-warning)">{{ overview?.skipped_self || 0 }}</div><div class="stat-label">自己调整</div></div>
      </div>

      <section class="section">
        <div class="section-head"><h2>每日趋势</h2><span class="tiny muted">完成 / 漏服 / 调整</span></div>
        <div class="card">
          <div v-if="!trend.length" class="muted small">数据不足</div>
          <div v-else class="trend">
            <div v-for="r in trend" :key="r.date" class="trend-col" :title="r.date">
              <div class="trend-bar"><span class="done" :style="{ height: barHeight(r.done) }" /><span class="miss" :style="{ height: barHeight(r.missed) }" /><span class="adj" :style="{ height: barHeight(r.skipped_doctor + r.skipped_self) }" /></div>
              <div class="trend-label">{{ r.date.slice(5) }}</div>
            </div>
          </div>
          <div class="legend"><span><i class="dot done" />完成</span><span><i class="dot miss" />漏服</span><span><i class="dot adj" />调整</span></div>
        </div>
      </section>

      <section class="section">
        <div class="section-head"><h2>日历热力图</h2><span class="tiny muted">绿=完成，红=漏服</span></div>
        <div class="card"><div class="heatmap"><div v-for="d in heatmap" :key="d.date" class="heat-cell" :class="heatClass(d)" :title="d.date + ' 完成 ' + d.done + ' / 漏服 ' + d.missed">{{ Number(d.date.slice(8)) }}</div></div></div>
      </section>

      <section class="section">
        <div class="section-head"><h2>按药品</h2></div>
        <div class="card">
          <div v-if="!medicineBars.length" class="muted small">暂无数据</div>
          <div v-for="b in medicineBars" :key="b.name" class="bar-row"><span class="truncate">{{ b.name }}</span><div class="bar-track"><div class="bar-fill" :style="{ width: b.width + '%' }" /></div><span>{{ b.count }}</span></div>
        </div>
      </section>

      <section class="section">
        <div class="section-head"><h2>按时间段</h2></div>
        <div class="card">
          <div v-for="b in timeBars" :key="b.name" class="bar-row"><span>{{ b.name }}</span><div class="bar-track"><div class="bar-fill" :style="{ width: b.width + '%', background: 'var(--mc-accent)' }" /></div><span>{{ b.count }}</span></div>
        </div>
      </section>

      <section v-if="locationTypeBars.length" class="section">
        <div class="section-head"><h2>按地点类型</h2><span class="tiny muted">医院 / 诊所 / 居家</span></div>
        <div class="card">
          <div v-for="b in locationTypeBars" :key="b.name" class="bar-row"><span>{{ b.name }}</span><div class="bar-track"><div class="bar-fill" :style="{ width: b.width + '%' }" /></div><span>{{ b.count }}</span></div>
        </div>
      </section>

      <section v-if="locationBars.length" class="section">
        <div class="section-head"><h2>按地点</h2><span class="tiny muted">打针 / 输液常去的地方</span></div>
        <div class="card">
          <div v-for="b in locationBars" :key="b.name" class="bar-row"><span class="truncate">{{ b.name }}</span><div class="bar-track"><div class="bar-fill" :style="{ width: b.width + '%', background: 'var(--mc-info)' }" /></div><span>{{ b.count }}</span></div>
        </div>
      </section>

      <section v-if="executorBars.length" class="section">
        <div class="section-head"><h2>按执行人</h2><span class="tiny muted">谁打针 / 输液</span></div>
        <div class="card">
          <div v-for="b in executorBars" :key="b.name" class="bar-row"><span class="truncate">{{ b.name }}</span><div class="bar-track"><div class="bar-fill" :style="{ width: b.width + '%', background: 'var(--mc-warning)' }" /></div><span>{{ b.count }}</span></div>
        </div>
      </section>
      <p class="tiny muted" style="margin-top:14px;line-height:1.65">完成率 = 完成 /（完成 + 跳过 + 漏服）；待完成不计入分母。补记完成后当前漏服减少，曾漏服保留。连续无漏服：从最近有用药记录的一天往前数，遇到漏服或没有用药记录的一天就停；当天还没到点的不计入。地点与执行人统计只计注射 / 输液。</p>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { statsApi } from '../api/medcare'
import { useBookStore } from '../stores/book'
import { addDays, localDate } from '../utils/date'
import { useUIStore } from '../stores/ui'
const book = useBookStore()
const ui = useUIStore()
const periods = [{ days: 7, label: '7 天' }, { days: 30, label: '30 天' }, { days: 90, label: '90 天' }]
const days = ref(30)
const overview = ref<any>(null)
const trend = ref<any[]>([])
const heatmap = ref<any[]>([])
const breakdown = ref<any>({})
const loading = ref(true)
const range = computed(() => { const to = localDate(); return { from: addDays(to, -(days.value - 1)), to } })
const medicineBars = computed(() => { const obj = breakdown.value?.by_medicine || {}; const arr = Object.entries(obj).map(([name, count]: any) => ({ name, count })).sort((a: any, b: any) => b.count - a.count).slice(0, 8); const max = Math.max(1, ...arr.map((a: any) => a.count)); return arr.map((a: any) => ({ ...a, width: Math.round(a.count / max * 100) })) })
const locationTypeLabels: any = { hospital: '医院', clinic: '诊所', home: '居家', pharmacy: '药房', other: '其他', unspecified: '未指定地点' }
const locationTypeBars = computed(() => { const obj = breakdown.value?.by_location_type || {}; const arr = Object.entries(obj).map(([k, count]: any) => ({ name: locationTypeLabels[k] || k, count })).sort((a: any, b: any) => b.count - a.count); const max = Math.max(1, ...arr.map((a: any) => a.count)); return arr.map((a: any) => ({ ...a, width: Math.round(a.count / max * 100) })) })
const locationBars = computed(() => { const obj = breakdown.value?.by_location || {}; const arr = Object.entries(obj).map(([name, count]: any) => ({ name, count })).sort((a: any, b: any) => b.count - a.count).slice(0, 8); const max = Math.max(1, ...arr.map((a: any) => a.count)); return arr.map((a: any) => ({ ...a, width: Math.round(a.count / max * 100) })) })
const executorBars = computed(() => { const obj = breakdown.value?.by_executor || {}; const arr = Object.entries(obj).map(([name, count]: any) => ({ name, count })).sort((a: any, b: any) => b.count - a.count).slice(0, 8); const max = Math.max(1, ...arr.map((a: any) => a.count)); return arr.map((a: any) => ({ ...a, width: Math.round(a.count / max * 100) })) })
const timeBars = computed(() => { const labels: any = { morning: '早上', noon: '中午', afternoon: '下午', evening: '晚上', night: '夜间' }; const obj = breakdown.value?.by_time || {}; const arr = Object.entries(obj).map(([k, count]: any) => ({ name: labels[k] || k, count })); const max = Math.max(1, ...arr.map((a: any) => a.count)); return arr.map((a: any) => ({ ...a, width: Math.round(a.count / max * 100) })) })
function barHeight(v: number) { const max = Math.max(1, ...trend.value.map((r) => r.done + r.missed + r.skipped_doctor + r.skipped_self + r.skipped_other)); return Math.round(v / max * 100) + '%' }
function heatClass(d: any) { if (d.missed > 0 && d.done > 0) return 'mixed'; if (d.missed > 0) return 'missed'; if (d.done > 0) return 'done'; return '' }
async function load() {
  if (!book.currentId) return
  loading.value = true
  try {
    const params = { ...range.value }
    const [o, t, h, b] = await Promise.all([statsApi.overview(book.currentId, params), statsApi.trend(book.currentId, params), statsApi.heatmap(book.currentId, params), statsApi.breakdown(book.currentId, params)])
    overview.value = o.data?.data; trend.value = t.data?.data || []; heatmap.value = h.data?.data || []; breakdown.value = b.data?.data || {}
  } catch (e: any) { ui.showToast(e?.response?.data?.message || '读取统计失败', 'error') }
  finally { loading.value = false }
}
onMounted(load)
watch(() => book.currentId, load)
</script>

<style scoped>
.stat-sub { font-size: 12px; margin-left: 1px; }
.stat-extra { display: block; font-size: 10px; opacity: .85; line-height: 1.3; }
.trend { display: flex; gap: 3px; align-items: flex-end; height: 118px; overflow-x: auto; padding-bottom: 16px; scrollbar-width: none; }
.trend::-webkit-scrollbar { display: none; }
.trend-col { flex: 0 0 13px; height: 100%; display: flex; flex-direction: column; justify-content: flex-end; align-items: center; position: relative; }
.trend-bar { width: 100%; flex: 1; display: flex; flex-direction: column; justify-content: flex-end; gap: 1px; }
.trend-bar span { display: block; width: 100%; border-radius: 3px 3px 0 0; min-height: 0; }
.trend-bar .done { background: var(--mc-primary); }
.trend-bar .miss { background: var(--mc-danger); }
.trend-bar .adj { background: var(--mc-info); }
.trend-label { position: absolute; bottom: -15px; font-size: 8.5px; color: var(--mc-text-muted); white-space: nowrap; transform: rotate(-45deg); }
.legend { display: flex; gap: 12px; font-size: 11px; color: var(--mc-text-muted); margin-top: 6px; flex-wrap: wrap; }
.legend span { display: inline-flex; align-items: center; gap: 4px; }
.dot { width: 7px; height: 7px; border-radius: 50%; display: inline-block; }
.dot.done { background: var(--mc-primary); }
.dot.miss { background: var(--mc-danger); }
.dot.adj { background: var(--mc-info); }
.truncate { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
@media (min-width: 900px) {
  .stat-sub { font-size: 13px; }
  .stat-extra { display: inline; font-size: 11px; margin-left: 2px; }
  .trend { gap: 4px; height: 150px; padding-bottom: 18px; }
  .trend-col { flex: 0 0 16px; }
  .trend-label { bottom: -17px; font-size: 9px; }
  .legend { gap: 14px; margin-top: 8px; }
}
</style>
