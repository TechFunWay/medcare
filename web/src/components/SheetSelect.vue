<template>
  <button type="button" class="select-like" :class="{ placeholder: !selectedLabel }" @click="openSheet">
    <span>{{ selectedLabel || placeholder }}</span>
    <AppIcon name="down" :size="16" />
  </button>
  <Teleport to="body">
    <div v-if="open" class="sheet-backdrop" @click.self="close">
      <section class="sheet" role="dialog" aria-modal="true" :aria-label="placeholder">
        <div class="sheet-grab" />
        <div class="sheet-head">
          <h2>{{ placeholder }}</h2>
          <button class="icon-btn" aria-label="关闭" @click="close"><AppIcon name="close" /></button>
        </div>
        <div class="sheet-body">
          <div v-if="searchable" class="card ss-search">
            <AppIcon name="search" :size="17" class="ss-icon" />
            <input v-model="kw" class="input ss-input" :placeholder="'搜索' + (placeholder || '选项')" />
          </div>
          <div class="list ss-list">
            <button v-for="opt in filtered" :key="String(opt.value)" type="button" class="option-row" :class="{ active: opt.value === modelValue }" @click="choose(opt.value)">
              <span>{{ opt.label }}</span>
              <AppIcon v-if="opt.value === modelValue" name="check" :size="18" />
            </button>
            <p v-if="!filtered.length" class="tiny muted" style="padding:8px 4px">没有匹配的选项{{ allowCustom ? '，可在下面直接填写' : '' }}。</p>
          </div>
          <div v-if="allowCustom" class="ss-custom">
            <div class="field" style="margin:0">
              <label>{{ customLabel || '没有合适的？自己填' }}</label>
              <input v-model="customValue" class="input" :placeholder="customPlaceholder || '输入自定义值'" @keyup.enter="useCustom" />
            </div>
            <button type="button" class="btn btn-primary btn-sm" :disabled="!String(customValue).trim()" @click="useCustom">使用</button>
          </div>
        </div>
      </section>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import AppIcon from './AppIcon.vue'

const props = defineProps<{
  modelValue: string | number | null | undefined
  options: { value: string | number; label: string }[]
  placeholder?: string
  // 选项多时在弹层顶部给搜索框（剂型、单位、规格这类常用几十项的场景）。
  searchable?: boolean
  // 允许填选项以外的自定义值，并用它作为选中值（老数据里的自由文本也能正常显示）。
  allowCustom?: boolean
  customLabel?: string
  customPlaceholder?: string
}>()
const emit = defineEmits<{ (e: 'update:modelValue', value: string | number): void }>()
const open = ref(false)
const kw = ref('')
const customValue = ref('')

const selectedLabel = computed(() => {
  const hit = props.options.find((o) => o.value === props.modelValue)
  if (hit) return hit.label
  // 自定义值（不在预设里）直接展示原值，避免显示成空占位。
  if (props.allowCustom && props.modelValue !== null && props.modelValue !== undefined && String(props.modelValue) !== '') return String(props.modelValue)
  return ''
})
const filtered = computed(() => {
  const q = kw.value.trim().toLowerCase()
  if (!q) return props.options
  return props.options.filter((o) => String(o.label).toLowerCase().includes(q))
})

function openSheet() {
  kw.value = ''
  customValue.value = typeof props.modelValue === 'string' ? props.modelValue : ''
  open.value = true
}
function close() { open.value = false }
function choose(value: string | number) { emit('update:modelValue', value); open.value = false }
function useCustom() {
  const v = String(customValue.value).trim()
  if (!v) return
  emit('update:modelValue', v)
  open.value = false
}
</script>

<style scoped>
.select-like { width: 100%; max-width: 100%; min-width: 0; min-height: 46px; padding: 10px 12px; border-radius: 12px; border: 1px solid var(--mc-border); background: var(--mc-surface-2); display: flex; align-items: center; justify-content: space-between; gap: 6px; font-size: 16px; text-align: left; }
.select-like > span { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.select-like.placeholder { color: var(--mc-text-muted); }
.option-row { width: 100%; min-height: 46px; padding: 0 12px; border: 1px solid var(--mc-border); border-radius: 12px; background: var(--mc-surface); display: flex; align-items: center; justify-content: space-between; gap: 8px; font-size: 15px; font-weight: 600; text-align: left; }
.option-row > span { min-width: 0; overflow-wrap: anywhere; }
.option-row.active { border-color: var(--mc-primary); background: var(--mc-primary-soft); color: var(--mc-primary-strong); }
.ss-search { display: flex; align-items: center; gap: 8px; padding: 0 10px; margin-bottom: 8px; }
.ss-search:focus-within { border-color: var(--mc-primary); box-shadow: 0 0 0 3px var(--mc-primary-soft); }
.ss-icon { flex: 0 0 auto; color: var(--mc-text-muted); }
.ss-input, .ss-input:focus { border: 0 !important; background: transparent !important; box-shadow: none !important; border-radius: 0; padding-left: 0; }
.ss-list { max-height: 46dvh; overflow-y: auto; display: grid; gap: 6px; }
.ss-custom { display: flex; align-items: flex-end; gap: 8px; margin-top: 10px; padding-top: 10px; border-top: 1px dashed var(--mc-border); }
.ss-custom .field { flex: 1 1 auto; }
.ss-custom .btn { flex: 0 0 auto; white-space: nowrap; }
</style>
