<template>
  <button type="button" class="select-like" :class="{ placeholder: !modelValue }" @click="openSheet">
    <span>{{ modelValue || placeholder || '选择时间' }}</span>
    <AppIcon name="clock" :size="16" />
  </button>
  <Teleport to="body">
    <div v-if="open" class="sheet-backdrop" @click.self="open = false">
      <section class="sheet" role="dialog" aria-modal="true" aria-label="选择时间">
        <div class="sheet-grab" />
        <div class="sheet-head">
          <h2>选择时间</h2>
          <button class="icon-btn" aria-label="关闭" @click="open = false"><AppIcon name="close" /></button>
        </div>
        <div class="sheet-body">
          <div class="quick-row">
            <button v-for="t in quick" :key="t" type="button" class="chip" @click="choose(t)">{{ t }}</button>
          </div>
          <div class="wheel-row" :class="{ 'with-seconds': withSeconds }">
            <div class="wheel">
              <button v-for="h in hours" :key="h" type="button" class="wheel-item" :class="{ active: h === hour }" @click="hour = h">{{ h }}</button>
            </div>
            <div class="wheel">
              <button v-for="m in minutes" :key="m" type="button" class="wheel-item" :class="{ active: m === minute }" @click="minute = m">{{ m }}</button>
            </div>
            <div v-if="withSeconds" class="wheel">
              <button v-for="sec in seconds" :key="sec" type="button" class="wheel-item" :class="{ active: sec === second }" @click="second = sec">{{ sec }}</button>
            </div>
          </div>
        </div>
        <div class="sheet-foot">
          <button type="button" class="btn btn-outline btn-block" @click="open = false">取消</button>
          <button type="button" class="btn btn-primary btn-block" @click="confirm">确定</button>
        </div>
      </section>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { nextTick, ref } from 'vue'
import AppIcon from './AppIcon.vue'

const props = defineProps<{ modelValue?: string; placeholder?: string; withSeconds?: boolean }>()
const emit = defineEmits<{ (e: 'update:modelValue', value: string): void }>()
const open = ref(false)
const quick = ['08:00', '09:00', '12:00', '18:00', '20:00', '21:00']
const hours = Array.from({ length: 24 }, (_, i) => String(i).padStart(2, '0'))
const minutes = Array.from({ length: 12 }, (_, i) => String(i * 5).padStart(2, '0'))
const seconds = Array.from({ length: 60 }, (_, i) => String(i).padStart(2, '0'))
const hour = ref('08')
const minute = ref('00')
const second = ref('00')

async function openSheet() {
  const parts = (props.modelValue || '08:00').split(':')
  const [h, m, sec] = parts
  hour.value = h || '08'
  const mm = (m || '00').padStart(2, '0')
  minute.value = minutes.includes(mm) ? mm : '00'
  second.value = (sec || '00').padStart(2, '0')
  open.value = true
  await nextTick()
  document.querySelectorAll('.wheel').forEach((wheel) => {
    const active = wheel.querySelector('.wheel-item.active')
    if (active) (active as HTMLElement).scrollIntoView({ block: 'center' })
  })
}
function choose(t: string) { emit('update:modelValue', t); open.value = false }
function confirm() {
  const v = hour.value + ':' + minute.value + (props.withSeconds ? ':' + second.value : '')
  emit('update:modelValue', v)
  open.value = false
}
</script>

<style scoped>
.select-like { width: 100%; max-width: 100%; min-width: 0; min-height: 46px; padding: 10px 12px; border-radius: 12px; border: 1px solid var(--mc-border); background: var(--mc-surface-2); display: flex; align-items: center; justify-content: space-between; gap: 6px; font-size: 16px; text-align: left; }
.select-like > span { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.select-like.placeholder { color: var(--mc-text-muted); }
.quick-row { display: flex; flex-wrap: wrap; gap: 8px; margin-bottom: 12px; }
.wheel-row { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
.wheel-row.with-seconds { grid-template-columns: 1fr 1fr 1fr; }
.wheel { max-height: 200px; overflow-y: auto; border-radius: 14px; background: var(--mc-surface-2); padding: 6px; display: grid; gap: 4px; }
.wheel-item { min-height: 40px; border: 0; border-radius: 10px; background: transparent; font-size: 15px; font-weight: 600; color: var(--mc-text); }
.wheel-item.active { background: var(--mc-primary); color: #fff; }
</style>
