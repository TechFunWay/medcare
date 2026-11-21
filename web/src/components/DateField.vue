<template>
  <button type="button" class="select-like" :class="{ placeholder: !modelValue }" @click="openSheet">
    <span>{{ display || placeholder || '选择日期' }}</span>
    <AppIcon name="calendar" :size="16" />
  </button>
  <Teleport to="body">
    <div v-if="open" class="sheet-backdrop" @click.self="open = false">
      <section class="sheet" role="dialog" aria-modal="true" aria-label="选择日期">
        <div class="sheet-grab" />
        <div class="sheet-head">
          <h2>{{ viewYear }} 年 {{ viewMonth }} 月</h2>
          <button class="icon-btn" aria-label="关闭" @click="open = false"><AppIcon name="close" /></button>
        </div>
        <div class="sheet-body">
          <div class="month-nav">
            <button class="icon-btn" aria-label="上个月" @click="shiftMonth(-1)"><AppIcon name="arrowLeft" :size="18" /></button>
            <div class="chip-row">
              <button class="chip" type="button" @click="pickToday">今天</button>
              <button v-if="clearable" class="chip" type="button" @click="clear">清除</button>
            </div>
            <button class="icon-btn" aria-label="下个月" @click="shiftMonth(1)"><AppIcon name="chevron" :size="18" /></button>
          </div>
          <div class="week-head">
            <span v-for="w in ['一', '二', '三', '四', '五', '六', '日']" :key="w">{{ w }}</span>
          </div>
          <div class="day-grid">
            <button
              v-for="cell in cells"
              :key="cell.key"
              type="button"
              class="day-cell"
              :class="{ muted: !cell.inMonth, active: cell.date === modelValue, today: cell.date === today }"
              @click="choose(cell.date)"
            >{{ cell.day }}</button>
          </div>
        </div>
      </section>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import AppIcon from './AppIcon.vue'
import { addDays, daysInMonth, firstWeekdayOffset, localDate } from '../utils/date'

const props = defineProps<{ modelValue?: string; placeholder?: string; clearable?: boolean }>()
const emit = defineEmits<{ (e: 'update:modelValue', value: string): void }>()
const open = ref(false)
const today = localDate()
const viewYear = ref(Number((props.modelValue || today).slice(0, 4)))
const viewMonth = ref(Number((props.modelValue || today).slice(5, 7)))
const display = computed(() => (props.modelValue ? props.modelValue.replace(/-/g, '/') : ''))

const cells = computed(() => {
  const offset = firstWeekdayOffset(viewYear.value, viewMonth.value)
  const total = daysInMonth(viewYear.value, viewMonth.value)
  const first = `${viewYear.value}-${String(viewMonth.value).padStart(2, '0')}-01`
  const out: { key: string; day: number; date: string; inMonth: boolean }[] = []
  for (let i = offset; i > 0; i--) {
    const date = addDays(first, -i)
    out.push({ key: 'p' + i, day: Number(date.slice(8)), date, inMonth: false })
  }
  for (let d = 1; d <= total; d++) {
    const date = `${viewYear.value}-${String(viewMonth.value).padStart(2, '0')}-${String(d).padStart(2, '0')}`
    out.push({ key: 'c' + d, day: d, date, inMonth: true })
  }
  while (out.length % 7 !== 0) {
    const date = addDays(first, out.length - offset + 1)
    out.push({ key: 'n' + out.length, day: Number(date.slice(8)), date, inMonth: false })
  }
  return out
})

function openSheet() {
  const base = props.modelValue || today
  viewYear.value = Number(base.slice(0, 4))
  viewMonth.value = Number(base.slice(5, 7))
  open.value = true
}
function shiftMonth(delta: number) {
  let m = viewMonth.value + delta
  let y = viewYear.value
  if (m < 1) { m = 12; y -= 1 }
  if (m > 12) { m = 1; y += 1 }
  viewMonth.value = m
  viewYear.value = y
}
function choose(date: string) { emit('update:modelValue', date); open.value = false }
function pickToday() { emit('update:modelValue', today); open.value = false }
function clear() { emit('update:modelValue', ''); open.value = false }
</script>

<style scoped>
.select-like { width: 100%; max-width: 100%; min-width: 0; min-height: 46px; padding: 10px 12px; border-radius: 12px; border: 1px solid var(--mc-border); background: var(--mc-surface-2); display: flex; align-items: center; justify-content: space-between; gap: 6px; font-size: 16px; text-align: left; }
.select-like > span { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.select-like.placeholder { color: var(--mc-text-muted); }
.month-nav { display: flex; align-items: center; justify-content: space-between; margin-bottom: 10px; }
.week-head { display: grid; grid-template-columns: repeat(7, 1fr); text-align: center; font-size: 11px; color: var(--mc-text-muted); margin-bottom: 6px; }
.day-grid { display: grid; grid-template-columns: repeat(7, 1fr); gap: 4px; }
.day-cell { aspect-ratio: 1; border: 0; border-radius: 12px; background: transparent; font-size: 14px; font-weight: 600; color: var(--mc-text); }
.day-cell.muted { color: var(--mc-text-muted); opacity: .45; }
.day-cell.today { box-shadow: inset 0 0 0 1px var(--mc-primary); }
.day-cell.active { background: var(--mc-primary); color: #fff; box-shadow: none; }
</style>
