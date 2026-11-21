<template>
  <button type="button" class="select-like" :class="{ placeholder: !selectedLabel }" @click="openSheet">
    <span>{{ selectedLabel || placeholder }}</span>
    <AppIcon name="down" :size="16" />
  </button>
  <Teleport to="body">
    <div v-if="open" class="sheet-backdrop" @click.self="close">
      <section class="sheet" role="dialog" aria-modal="true" :aria-label="placeholder || '选择药品'">
        <div class="sheet-grab" />
        <div class="sheet-head">
          <h2>{{ placeholder || '选择药品' }}</h2>
          <button class="icon-btn" aria-label="关闭" @click="close"><AppIcon name="close" :size="18" /></button>
        </div>
        <div class="sheet-body">
          <div class="card mp-search">
            <AppIcon name="search" :size="17" class="mp-icon" />
            <input v-model="kw" class="input mp-input" placeholder="搜索药品名、通用名、规格" />
          </div>
          <!-- 分类筛选：药品多时先按口服/注射等缩小范围，再搜索 -->
          <div class="chip-row mp-chips">
            <button v-for="c in categoryChips" :key="c.value" type="button" class="chip" :class="{ active: cat === c.value }" @click="cat = c.value">{{ c.label }}</button>
          </div>
          <div class="list ss-list">
            <button v-for="opt in filtered" :key="String(opt.value)" type="button" class="option-row" :class="{ active: opt.value === modelValue }" @click="choose(opt.value)">
              <span>{{ opt.label }}</span>
              <AppIcon v-if="opt.value === modelValue" name="check" :size="18" />
            </button>
            <p v-if="!filtered.length" class="tiny muted" style="padding:8px 4px">没有匹配的药品，换个关键词或分类试试。</p>
          </div>
        </div>
      </section>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
// 统一的「选择药品」选择器：搜索 + 分类筛选，所有需要挑药品的表单都用它，
// 免得各页面各写一套、有的能搜有的不能搜（2026-11-21 用户要求）。
import { computed, ref } from 'vue'
import AppIcon from './AppIcon.vue'
import type { Medicine } from '../api/medcare'

const props = defineProps<{
  modelValue: string | number | null | undefined
  medicines: Medicine[]
  placeholder?: string
  // 是否在标签后带上库存（入库/出库场景更有用）
  showStock?: boolean
}>()
const emit = defineEmits<{ (e: 'update:modelValue', value: string | number): void }>()

const open = ref(false)
const kw = ref('')
const cat = ref('')

const CATEGORY_LABELS: Record<string, string> = { oral: '口服', injection: '注射', infusion: '输液', topical: '外用', other: '其他' }
const categoryChips = [
  { value: '', label: '全部' },
  { value: 'oral', label: '口服' },
  { value: 'injection', label: '注射' },
  { value: 'infusion', label: '输液' },
  { value: 'topical', label: '外用' },
  { value: 'other', label: '其他' },
]

function labelOf(m: Medicine) {
  const spec = m.spec || m.unit || ''
  const stock = props.showStock ? '（' + m.stock_quantity + ' ' + m.unit + '）' : ''
  return m.name + (spec ? ' · ' + spec : '') + stock
}
const options = computed(() => props.medicines.map((m) => ({ value: m.id, label: labelOf(m), category: m.category || 'other' })))

const selectedLabel = computed(() => {
  const hit = options.value.find((o) => String(o.value) === String(props.modelValue ?? ''))
  return hit ? hit.label : ''
})

const filtered = computed(() => {
  const q = kw.value.trim().toLowerCase()
  return options.value.filter((o) => {
    if (cat.value && o.category !== cat.value) return false
    if (!q) return true
    return o.label.toLowerCase().includes(q)
  })
})

function openSheet() { kw.value = ''; cat.value = ''; open.value = true }
function close() { open.value = false }
function choose(value: string | number) { emit('update:modelValue', value); open.value = false }
</script>

<style scoped>
.select-like { width: 100%; max-width: 100%; min-width: 0; min-height: 46px; padding: 10px 12px; border-radius: 12px; border: 1px solid var(--mc-border); background: var(--mc-surface-2); display: flex; align-items: center; justify-content: space-between; gap: 6px; font-size: 16px; text-align: left; }
.select-like > span { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.select-like.placeholder { color: var(--mc-text-muted); }
.mp-search { display: flex; align-items: center; gap: 8px; padding: 0 12px; margin-bottom: 8px; }
.mp-icon { color: var(--mc-text-muted); flex: 0 0 auto; }
.mp-input { flex: 1 1 auto; min-width: 0; border: 0; background: transparent; padding: 0; min-height: 44px; }
.mp-chips { margin-bottom: 8px; }
</style>
