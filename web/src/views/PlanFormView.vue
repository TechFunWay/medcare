<template>
  <div class="page narrow">
    <div class="page-head">
      <div style="display:flex;gap:8px;align-items:center">
        <button class="icon-btn" aria-label="返回" @click="router.back()"><AppIcon name="arrowLeft" /></button>
        <div>
          <div class="eyebrow">{{ isNew ? '新建' : '编辑' }}</div>
          <h1>{{ isNew ? '新建用药计划' : '用药计划' }}</h1>
          <p v-if="!isNew && plan" class="tiny muted plan-editor">新增：{{ plan.created_by_name || '—' }}<template v-if="plan.updated_by_name"> ｜ 最后更新：{{ plan.updated_by_name }}</template></p>
        </div>
      </div>
    </div>
    <div v-if="!canSave" class="card readonly-tip" style="margin-bottom:12px">
      <div class="small"><b>你还没有「计划管理」的权限</b></div>
      <p class="tiny muted" style="margin-top:4px">这份计划只能查看。需要新建或改动，请让管理员在「用药本 → 成员与权限 → 设置权限」里给你开通「计划管理」。</p>
    </div>
    <div v-if="loading" class="card muted">加载中…</div>
    <form v-else class="card" @submit.prevent="save">
      <div class="field">
        <label>药品</label>
        <MedicinePicker :model-value="form.medicine_id" :medicines="medicines" placeholder="选择药品" @update:model-value="onMedicineChange" />
      </div>
      <div class="field-row">
        <div class="field"><label>类型</label><SheetSelect v-model="form.type" placeholder="类型" :options="typeOptions" /></div>
        <div v-if="isShot" class="field"><label>打针/输液地点</label><SheetSelect v-model="form.location_id" placeholder="选择医院 / 诊所 / 居家" :options="locationOptions" /></div>
      </div>
      <div v-if="isShot" class="card shot-card">
        <div class="field">
          <label>谁来打针 / 输液</label>
          <div class="chip-row">
            <button v-for="e in executorOptions" :key="e.value" type="button" class="chip" :class="{ active: form.executor_type === e.value }" @click="form.executor_type = e.value">{{ e.label }}</button>
          </div>
        </div>
        <div class="field"><label>执行人姓名（可空）</label><input v-model="form.executor_name" class="input" placeholder="例如：社区医院李医生 / 家属小张" /></div>
        <p class="tiny muted" style="line-height:1.65;margin:0">在医院输液选「护士」；带药水回家打针/输液选「自己」或「家人」；请附近诊所医生帮忙选「医生」。地点可在 <router-link to="/locations" class="link">地点库</router-link> 添加。</p>
      </div>
      <div v-if="form.type === 'infusion'" class="field-row">
        <div class="field"><label>滴速（滴/分钟）</label><input v-model.number="form.drip_rate" type="number" min="0" step="1" class="input" placeholder="如 60" /></div>
        <div class="field"><label>预计时长（分钟）</label><input v-model.number="form.duration_minutes" type="number" min="0" step="1" class="input" placeholder="如 90" /></div>
      </div>
      <div class="field-row">
        <div class="field"><label>每次剂量</label><input v-model.number="form.dose_amount" type="number" min="0.01" step="0.01" class="input" required /></div>
        <div class="field"><label>单位</label><SheetSelect v-model="form.dose_unit" placeholder="单位" :options="doseUnitOptions" searchable allow-custom custom-label="没有合适的单位？自己填" custom-placeholder="例如：滴 / IU" /></div>
      </div>
      <div class="field">
        <label>频率</label>
        <div class="chip-row">
          <button v-for="f in freqOptions" :key="f.value" type="button" class="chip" :class="{ active: form.frequency.type === f.value }" @click="setFrequencyType(f.value)">{{ f.label }}</button>
        </div>
      </div>
      <div v-if="form.frequency.type === 'every_n_days'" class="field"><label>间隔天数</label><input v-model.number="form.frequency.interval_days" type="number" min="1" class="input" /></div>
      <div v-if="form.frequency.type === 'weekly'" class="field">
        <label>星期</label>
        <div class="chip-row"><button v-for="d in [1, 2, 3, 4, 5, 6, 7]" :key="d" type="button" class="chip" :class="{ active: form.frequency.weekdays?.includes(d) }" @click="toggleWeekday(d)">{{ ['一','二','三','四','五','六','日'][d - 1] }}</button></div>
      </div>
      <template v-if="form.frequency.type === 'interval_hours'">
        <div class="field"><label>间隔小时</label><input v-model.number="form.frequency.interval_hours" type="number" min="1" max="72" class="input" /></div>
        <div class="field-row">
          <div class="field"><label>起始日期</label><DateField v-model="startDateValue" placeholder="选择日期" /></div>
          <div class="field"><label>起始时间</label><TimeField v-model="startTimeValue" placeholder="选择时间" /></div>
        </div>
      </template>
      <div v-else-if="form.frequency.type === 'custom_dates'" class="field">
        <label>指定日期与时间</label>
        <p class="tiny muted" style="margin:0 0 8px;line-height:1.6">不规律用药就一天一天排：今天一针、明天两针、后天一针，每个日期的时间点单独填。</p>
        <div class="custom-day-list">
          <div v-for="(d, index) in form.frequency.custom_schedule" :key="index" class="custom-day">
            <div class="custom-day-head"><b>{{ d.date || '未选日期' }}</b><button type="button" class="icon-btn" aria-label="删除这一天" @click="removeCustomDay(index)"><AppIcon name="close" :size="16" /></button></div>
            <div class="field"><label>日期</label><DateField v-model="d.date" placeholder="选择日期" /></div>
            <div class="field"><label>当天时间（逗号分隔）</label><input v-model="d.timesText" class="input" placeholder="例如：09:00, 18:00" /></div>
          </div>
        </div>
        <button type="button" class="btn btn-outline btn-block" @click="addCustomDay"><AppIcon name="plus" :size="16" />添加一天</button>
        <button type="button" class="btn btn-ghost btn-block batch-toggle" @click="toggleBatch"><AppIcon name="sparkles" :size="16" />批量生成日期（按周复制 / 按疗程）</button>
        <div v-if="showBatch" class="batch-card">
          <div class="view-tabs">
            <button type="button" :class="{ active: batch.mode === 'weekly' }" @click="batch.mode = 'weekly'">按周复制</button>
            <button type="button" :class="{ active: batch.mode === 'course' }" @click="batch.mode = 'course'">按疗程生成</button>
          </div>
          <div class="field"><label>起始日期</label><DateField v-model="batch.startDate" placeholder="选择日期" /></div>
          <template v-if="batch.mode === 'weekly'">
            <div class="field-row">
              <div class="field"><label>连续几周</label><input v-model.number="batch.weeks" type="number" min="1" max="52" class="input" /></div>
              <div class="field"><label>每周时间（逗号分隔）</label><input v-model="batch.timesText" class="input" placeholder="09:00, 18:00" /></div>
            </div>
            <p class="tiny muted" style="margin:0 0 8px">从起始日期开始，每 7 天复制一天，时间不变。</p>
          </template>
          <template v-else>
            <div class="field-row">
              <div class="field"><label>间隔天数</label><input v-model.number="batch.intervalDays" type="number" min="1" max="60" class="input" placeholder="1 每天，2 隔天" /></div>
              <div class="field"><label>疗程天数</label><input v-model.number="batch.durationDays" type="number" min="1" max="365" class="input" /></div>
            </div>
            <div class="field"><label>每天时间（逗号分隔）</label><input v-model="batch.timesText" class="input" placeholder="09:00, 18:00" /></div>
            <p class="tiny muted" style="margin:0 0 8px">例如间隔 2 天、疗程 14 天：从起始日期起每 2 天生成一天，共 7 天。</p>
          </template>
          <button type="button" class="btn btn-primary btn-block" @click="applyBatch">生成并追加</button>
        </div>
      </div>
      <div v-else class="field">
        <label>每天时间</label>
        <div class="time-list">
          <div v-for="(slot, index) in timeSlots" :key="index" class="time-row">
            <TimeField :model-value="slot" placeholder="选择时间" @update:model-value="updateSlot(index, $event)" />
            <button v-if="timeSlots.length > 1" type="button" class="icon-btn" aria-label="删除时间" @click="removeSlot(index)"><AppIcon name="close" :size="18" /></button>
          </div>
          <button v-if="timeSlots.length < 12" type="button" class="btn btn-outline btn-block" @click="addSlot"><AppIcon name="plus" :size="16" />添加时间</button>
        </div>
      </div>
      <div class="field-row">
        <div class="field"><label>开始日期</label><DateField v-model="form.start_date" placeholder="选择日期" /></div>
        <div class="field"><label>结束日期（可空）</label><DateField v-model="form.end_date" placeholder="长期" clearable /></div>
      </div>
      <div class="field-row">
        <div class="field"><label>医生</label><input v-model="form.doctor" class="input" /></div>
        <div class="field"><label>医院</label><input v-model="form.hospital" class="input" /></div>
      </div>
      <div class="field"><label>医嘱备注</label><textarea v-model="form.notes" class="textarea" /></div>
      <div class="field" style="display:flex;align-items:center;justify-content:space-between">
        <label style="margin:0">接收提醒</label>
        <label class="switch"><input v-model="form.notifications_enabled" type="checkbox" /><span /></label>
      </div>
      <div class="sheet-foot" style="padding-top:6px">
        <button type="button" class="btn btn-outline btn-block" @click="router.back()">取消</button>
        <button v-if="canSave" class="btn btn-primary btn-block" :disabled="saving">{{ saving ? '保存中…' : isNew ? '创建计划' : '保存修改' }}</button>
      </div>
    </form>
    <div v-if="!isNew && plan && (book.canPlanEdit || book.canPlanDelete)" class="card" style="margin-top:14px">
      <div class="section-head" style="margin-top:0"><h2>计划操作</h2><span class="badge">{{ statusLabel(plan.status) }}</span></div>
      <div class="list">
        <button v-if="plan.status === 'active'" class="btn btn-outline btn-block" @click="pause">暂停计划</button>
        <button v-if="plan.status === 'paused'" class="btn btn-outline btn-block" @click="resume">恢复计划</button>
        <button v-if="plan.status !== 'ended'" class="btn btn-outline btn-block" @click="end">结束计划</button>
        <button class="btn btn-outline btn-block" @click="openTransfer">转移到其它用药本</button>
        <button class="btn btn-danger btn-block" @click="remove">删除计划</button>
      </div>
    </div>
    <Teleport to="body">
      <div v-if="showTransfer" class="sheet-backdrop" @click.self="showTransfer=false">
        <section class="sheet"><div class="sheet-grab" /><div class="sheet-head"><h2>转移到其它用药本</h2><button class="icon-btn" @click="showTransfer=false"><AppIcon name="close" /></button></div>
          <div class="sheet-body">
            <p class="small muted" style="line-height:1.7;margin-bottom:10px">会把该用药计划及它的用药记录一起转到目标用药本；目标本需要已有该药品。</p>
            <div class="field"><label>目标用药本</label><SheetSelect v-model="transferTarget" placeholder="选择用药本" :options="transferOptions" /></div>
          </div>
          <div class="sheet-foot"><button type="button" class="btn btn-outline btn-block" @click="showTransfer=false">取消</button><button class="btn btn-primary btn-block" :disabled="saving || !transferTarget" @click="doTransfer">{{ saving ? '转移中…' : '确认转移' }}</button></div>
        </section>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppIcon from '../components/AppIcon.vue'
import SheetSelect from '../components/SheetSelect.vue'
import MedicinePicker from '../components/MedicinePicker.vue'
import { unitOptions } from '../utils/medicineMeta'
import DateField from '../components/DateField.vue'
import TimeField from '../components/TimeField.vue'
import { locationApi, medicineApi, planApi, type Location, type Medicine, type Plan } from '../api/medcare'
import { useBookStore } from '../stores/book'
import { useUIStore } from '../stores/ui'
import { addDays, beijingParts, beijingToISO, localDate } from '../utils/date'
import { fetchAllPages } from '../utils/paging'

const route = useRoute()
const router = useRouter()
const book = useBookStore()
// 新建要「用药计划 · 新建」，改期与暂停要「用药计划 · 修改」；两格都没有就整页只读。
const canSave = computed(() => (isNew.value ? book.canPlanCreate : book.canPlanEdit))
const ui = useUIStore()
const isNew = computed(() => !route.params.id || route.params.id === 'new')
const medicines = ref<Medicine[]>([])
const locations = ref<Location[]>([])
const loading = ref(true)
const saving = ref(false)
const plan = ref<Plan | null>(null)
const showTransfer = ref(false)
const transferTarget = ref(0)
const transferOptions = computed(() => book.books.filter((b) => b.id !== book.currentId && (b.role === 'owner' || b.role === 'manager')).map((b) => ({ value: b.id, label: b.name + '（' + b.care_recipient_name + '）' })))
const timeSlots = ref<string[]>(['08:00'])
const startDateValue = ref(localDate())
const startTimeValue = ref('08:00')
const freqOptions = [
  { value: 'daily', label: '每天' },
  { value: 'every_n_days', label: '每 N 天' },
  { value: 'weekly', label: '每周' },
  { value: 'interval_hours', label: '每 N 小时' },
  { value: 'custom_dates', label: '指定日期' },
]
const typeOptions = [
  { value: 'oral', label: '口服' },
  { value: 'injection', label: '注射' },
  { value: 'infusion', label: '输液' },
  { value: 'other', label: '其他' },
]
const executorOptions = [
  { value: 'self', label: '自己' },
  { value: 'family', label: '家人' },
  { value: 'nurse', label: '护士' },
  { value: 'doctor', label: '诊所/医院医生' },
  { value: 'other', label: '其他' },
]
const isShot = computed(() => form.type === 'injection' || form.type === 'infusion')
const medicineOptions = computed(() => medicines.value.map((m) => ({ value: m.id, label: m.name + ' · ' + (m.spec || m.unit) })))
// 单位下拉：常用单位 + 当前药品的库存单位 + 已保存的值（编辑旧计划时不会丢）。
const doseUnitOptions = computed(() => {
  const set = new Set(unitOptions().map((o) => String(o.value)))
  const med = medicines.value.find((m) => m.id === form.medicine_id)
  if (med?.unit) set.add(med.unit)
  if (form.dose_unit) set.add(String(form.dose_unit))
  return [...set].map((v) => ({ value: v, label: v }))
})
// 选药品时把单位同步成该药的库存单位；用户仍可再改（比如库存按盒、每次按片）。
function onMedicineChange(id: string | number) {
  form.medicine_id = Number(id)
  const med = medicines.value.find((m) => m.id === form.medicine_id)
  if (med?.unit) form.dose_unit = med.unit
}
const locationOptions = computed(() => [{ value: 0, label: '不指定' }, ...locations.value.map((l) => ({ value: l.id, label: l.name }))])
const form = reactive<any>({
  medicine_id: 0, location_id: 0, title: '', type: 'oral', dose_amount: 1, dose_unit: '片',
  frequency: { type: 'daily', times: ['08:00'], interval_days: 2, weekdays: [1, 3, 5], interval_hours: 8, start_at: '', custom_schedule: [] as { date: string; timesText: string }[] },
  start_date: localDate(), end_date: '', timezone: 'Asia/Shanghai', doctor: '', hospital: '', executor_type: 'self', executor_name: '', drip_rate: 0, duration_minutes: 0, notes: '',
  notifications_enabled: true, version: 0,
})
function statusLabel(s: string) { return s === 'active' ? '进行中' : s === 'paused' ? '已暂停' : s === 'ended' ? '已结束' : '已归档' }
function toggleWeekday(d: number) {
  const list = form.frequency.weekdays || []
  form.frequency.weekdays = list.includes(d) ? list.filter((x: number) => x !== d) : [...list, d].sort((a: number, b: number) => a - b)
}
function updateSlot(index: number, value: string) { timeSlots.value[index] = value }
function addSlot() { if (timeSlots.value.length < 12) timeSlots.value.push('12:00') }
function removeSlot(index: number) { timeSlots.value.splice(index, 1) }

function openTransfer() {
  if (!transferOptions.value.length) { ui.showToast('没有其它可转移的用药本（需你对目标本有管理权限）', 'error'); return }
  transferTarget.value = transferOptions.value[0].value
  showTransfer.value = true
}
async function doTransfer() {
  if (!plan.value || !transferTarget.value) return
  saving.value = true
  try {
    await planApi.transfer(book.currentId!, plan.value.id, { target_book_id: transferTarget.value })
    showTransfer.value = false
    ui.showToast('已转移到目标用药本')
    router.push('/plans')
  } catch (e: any) { ui.showToast(e?.response?.data?.message || '转移失败', 'error') }
  finally { saving.value = false }
}
async function load() {
  if (!book.currentId) return
  loading.value = true
  try {
    // 药品与地点都可能超过一页，逐页取全，避免下拉里选不到已有药品
    const [m, l] = await Promise.all([
      fetchAllPages<Medicine>(async (page, pageSize) => {
        const res = await medicineApi.list(book.currentId!, { page, pageSize })
        return { items: (res.data?.data?.items || []) as Medicine[], total: Number(res.data?.data?.total) || 0 }
      }),
      locationApi.list(book.currentId),
    ])
    medicines.value = m
    locations.value = l.data?.data || []
    // 从药品详情「新建计划」进来时带 ?medicine_id=，预选该药品并带上单位，少点两下。
    if (isNew.value) {
      const mid = Number(route.query.medicine_id || 0)
      const picked = mid ? m.find((x) => x.id === mid) : null
      if (picked) {
        form.medicine_id = picked.id
        if (picked.unit) form.dose_unit = picked.unit
      }
    }
    if (!isNew.value) {
      const res = await planApi.get(book.currentId, Number(route.params.id))
      plan.value = res.data?.data?.plan
      if (plan.value) {
        Object.assign(form, {
          medicine_id: plan.value.medicine_id,
          location_id: plan.value.location_id || 0,
          title: plan.value.title,
          type: plan.value.type,
          dose_amount: plan.value.dose_amount,
          dose_unit: plan.value.dose_unit,
          frequency: {
            type: plan.value.frequency.type,
            times: plan.value.frequency.times || [],
            interval_days: plan.value.frequency.interval_days || 2,
            weekdays: plan.value.frequency.weekdays || [],
            interval_hours: plan.value.frequency.interval_hours || 8,
            start_at: plan.value.frequency.start_at || '',
            custom_schedule: (plan.value.frequency.custom_schedule || []).map((d) => ({ date: d.date, timesText: (d.times || []).join(', ') })),
          },
          start_date: plan.value.start_date,
          end_date: plan.value.end_date || '',
          timezone: plan.value.timezone,
          doctor: plan.value.doctor || '',
          hospital: plan.value.hospital || '',
          executor_type: plan.value.executor_type || 'self',
          executor_name: plan.value.executor_name || '',
          drip_rate: plan.value.drip_rate || 0,
          duration_minutes: plan.value.duration_minutes || 0,
          notes: plan.value.notes || '',
          notifications_enabled: plan.value.notifications_enabled,
          version: plan.value.version,
        })
        if (plan.value.frequency.times?.length) timeSlots.value = [...plan.value.frequency.times]
        if (plan.value.frequency.start_at) {
          const parts = beijingParts(plan.value.frequency.start_at)
          startDateValue.value = parts.date
          startTimeValue.value = parts.time
        }
      }
    }
  } catch (e: any) {
    ui.showToast(e?.response?.data?.message || '读取计划失败', 'error')
  } finally {
    loading.value = false
  }
}

function setFrequencyType(type: string) {
  form.frequency.type = type
  if (type === 'custom_dates' && !(form.frequency.custom_schedule || []).length) {
    form.frequency.custom_schedule = [{ date: localDate(), timesText: '08:00' }]
  }
}
function addCustomDay() { (form.frequency.custom_schedule || (form.frequency.custom_schedule = [])).push({ date: localDate(), timesText: '08:00' }) }
function removeCustomDay(index: number) { form.frequency.custom_schedule.splice(index, 1) }
const showBatch = ref(false)
const batch = reactive({ mode: 'course', startDate: localDate(), intervalDays: 1, durationDays: 7, weeks: 4, timesText: '08:00' })
function toggleBatch() {
  showBatch.value = !showBatch.value
  if (showBatch.value) batch.startDate = form.start_date || localDate()
}
function parseTimeList(raw: string) { return String(raw || '').split(/[,，、\s]+/).map((s) => s.trim()).filter(Boolean) }
function applyBatch() {
  const times = parseTimeList(batch.timesText)
  if (!times.length) { ui.showToast('请至少填写一个时间', 'error'); return }
  if (!batch.startDate) { ui.showToast('请选择起始日期', 'error'); return }
  const dates: string[] = []
  if (batch.mode === 'weekly') {
    const weeks = Math.max(1, Math.min(52, Number(batch.weeks) || 1))
    for (let i = 0; i < weeks; i++) dates.push(addDays(batch.startDate, i * 7))
  } else {
    const interval = Math.max(1, Math.min(60, Number(batch.intervalDays) || 1))
    const duration = Math.max(1, Math.min(365, Number(batch.durationDays) || 1))
    for (let offset = 0; offset < duration; offset += interval) dates.push(addDays(batch.startDate, offset))
  }
  const map = new Map<string, { date: string; timesText: string }>()
  for (const d of form.frequency.custom_schedule || []) map.set(d.date, { ...d })
  for (const date of dates) {
    const old = map.get(date)
    const merged = old ? [...new Set([...parseTimeList(old.timesText), ...times])] : times
    map.set(date, { date, timesText: merged.join(', ') })
  }
  form.frequency.custom_schedule = [...map.values()].sort((a, b) => a.date.localeCompare(b.date))
  showBatch.value = false
  ui.showToast('已生成 ' + dates.length + ' 天')
}
function customSchedulePayload() {
  return (form.frequency.custom_schedule || [])
    .map((d: { date: string; timesText: string }) => ({ date: d.date, times: String(d.timesText || '').split(/[,，、\s]+/).map((x: string) => x.trim()).filter(Boolean) }))
    .filter((d: { date: string; times: string[] }) => d.date && d.times.length)
}

async function save() {
  if (!book.currentId || !form.medicine_id) { ui.showToast('请选择药品', 'error'); return }
  if (form.dose_amount <= 0) { ui.showToast('剂量必须大于 0', 'error'); return }
  const times = timeSlots.value.filter(Boolean)
  if (form.frequency.type === 'custom_dates') {
    if (!customSchedulePayload().length) { ui.showToast('请至少添加一个有时间的用药日期', 'error'); return }
  } else if (form.frequency.type !== 'interval_hours' && !times.length) { ui.showToast('请至少添加一个时间', 'error'); return }
  saving.value = true
  try {
    const payload: any = {
      ...form,
      location_id: form.location_id || null,
      title: form.title || ((medicines.value.find((m) => m.id === form.medicine_id)?.name || '') + '用药'),
      frequency: {
        ...form.frequency,
        times: form.frequency.type === 'interval_hours' || form.frequency.type === 'custom_dates' ? [] : times,
        start_at: form.frequency.type === 'interval_hours' ? beijingToISO(startDateValue.value, startTimeValue.value) : '',
        ...(form.frequency.type === 'custom_dates' ? { custom_schedule: customSchedulePayload() } : {}),
      },
    }
    if (!payload.end_date) delete payload.end_date
    let skippedTimes: string[] = []
    if (isNew.value) {
      const res = await planApi.create(book.currentId, payload)
      skippedTimes = res.data?.data?.skipped_today_times || []
    } else {
      await planApi.update(book.currentId, Number(route.params.id), payload)
    }
    if (skippedTimes.length) {
      ui.showToast('计划已保存。今天的 ' + skippedTimes.join('、') + ' 已过，将从明天开始提醒', 'info', 6000)
    } else {
      ui.showToast('计划已保存')
    }
    router.push('/plans')
  } catch (e: any) {
    ui.showToast(e?.response?.data?.message || '保存失败', 'error')
  } finally {
    saving.value = false
  }
}
async function pause() { await planApi.pause(book.currentId!, Number(route.params.id)); ui.showToast('已暂停'); load() }
async function resume() { await planApi.resume(book.currentId!, Number(route.params.id)); ui.showToast('已恢复'); load() }
async function end() {
  if (await ui.confirm({ title: '结束计划？', message: '结束后不再生成未来事件，历史记录保留。', confirmText: '结束' })) {
    await planApi.end(book.currentId!, Number(route.params.id)); ui.showToast('已结束'); load()
  }
}
async function remove() {
  if (await ui.confirm({ title: '删除计划？', message: '历史事件会保留，之后不再生成新事件。', confirmText: '删除' })) {
    await planApi.remove(book.currentId!, Number(route.params.id)); ui.showToast('已删除'); router.push('/plans')
  }
}
onMounted(load)
// 切换用药本后编辑中的计划已不属于当前本，回到计划列表。
watch(() => book.currentId, () => { if (!isNew.value) router.replace('/plans') })
</script>

<style scoped>
.shot-card { margin-bottom: 12px; background: var(--mc-surface-2); }
.custom-day-list { display: grid; gap: 10px; margin-bottom: 10px; }
.batch-toggle { margin-top: 8px; }
.batch-card { margin-top: 10px; padding: 12px; border: 1px dashed var(--mc-border); border-radius: 12px; background: var(--mc-surface); }
.view-tabs { display: flex; gap: 6px; margin-bottom: 10px; }
.view-tabs button { flex: 1; border: 1px solid var(--mc-border); background: var(--mc-surface); color: var(--mc-text-muted); padding: 7px; border-radius: 8px; font-weight: 700; }
.view-tabs button.active { background: var(--mc-primary-soft); color: var(--mc-primary-strong); border-color: var(--mc-primary); }
.custom-day { padding: 10px 12px 4px; border: 1px solid var(--mc-border); border-radius: 12px; background: var(--mc-surface-2); }
.custom-day-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 6px; font-size: 13px; }

.plan-editor { margin-top: 4px; }
.switch { position: relative; display: inline-flex; width: 42px; height: 26px; }
.switch input { opacity: 0; width: 0; height: 0; }
.switch span { position: absolute; inset: 0; border-radius: 999px; background: var(--mc-surface-2); border: 1px solid var(--mc-border); transition: .15s; }
.switch span::after { content: ''; position: absolute; width: 20px; height: 20px; left: 2px; top: 2px; border-radius: 50%; background: #fff; box-shadow: 0 1px 4px rgba(0,0,0,.15); transition: .15s; }
.switch input:checked + span { background: var(--mc-primary); border-color: var(--mc-primary); }
.switch input:checked + span::after { transform: translateX(16px); }
.time-list { display: grid; gap: 6px; }
.time-row { display: flex; gap: 6px; align-items: center; }
.time-row > :first-child { flex: 1 1 auto; min-width: 0; }
.time-row > .icon-btn { width: 40px; height: 40px; }
</style>
