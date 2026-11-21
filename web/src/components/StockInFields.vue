<template>
  <div class="field"><label>入库来源</label><SheetSelect :model-value="modelValue.channel" :options="channelOptions" placeholder="选择来源" @update:model-value="patch({ channel: String($event) })" /></div>
  <div class="field-row">
    <div class="field"><label>包装数</label><input ref="pkEl" :value="modelValue.packages || ''" type="number" min="0" step="1" class="input" placeholder="可空" @input="onPackages" @change="onPackages" /></div>
    <div class="field"><label>每包装含量</label><input ref="upEl" :value="modelValue.units_per_package || ''" type="number" min="0" step="0.01" class="input" placeholder="可空" @input="onUnits" @change="onUnits" /></div>
  </div>
  <p class="tiny muted" style="margin:-4px 0 10px">按包装填会自动算数量（包装数 × 每包装含量）；也可以直接填下面的入库数量。</p>
  <div class="field"><label>入库数量（{{ unit || '单位' }}）</label><input ref="qtyEl" :value="modelValue.quantity || ''" type="number" min="0" step="0.01" class="input" placeholder="例如 2" @input="onQuantity" @change="onQuantity" /></div>
  <div class="field-row">
    <div class="field"><label>批号</label><input :value="modelValue.batch_no" class="input" placeholder="可空" @input="patch({ batch_no: ($event.target as HTMLInputElement).value })" /></div>
    <div class="field"><label>有效期</label><DateField :model-value="modelValue.expiry_date" placeholder="选择日期" clearable @update:model-value="patch({ expiry_date: String($event || '') })" /></div>
  </div>
  <div class="field"><label>供应商</label><input :value="modelValue.supplier" class="input" placeholder="可空" @input="patch({ supplier: ($event.target as HTMLInputElement).value })" /></div>
  <div class="field"><label>备注</label><input :value="modelValue.note" class="input" placeholder="可空" @input="patch({ note: ($event.target as HTMLInputElement).value })" /></div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import SheetSelect from './SheetSelect.vue'
import DateField from './DateField.vue'
import { STOCK_CHANNEL_OPTIONS, type StockInDraft } from '../utils/stock'

const props = defineProps<{ modelValue: StockInDraft; unit?: string }>()
const emit = defineEmits<{ (e: 'update:modelValue', v: StockInDraft): void }>()
const channelOptions = STOCK_CHANNEL_OPTIONS
const pkEl = ref<HTMLInputElement | null>(null)
const upEl = ref<HTMLInputElement | null>(null)
const qtyEl = ref<HTMLInputElement | null>(null)

function patch(p: Partial<StockInDraft>) {
  emit('update:modelValue', { ...props.modelValue, ...p })
}
function round3(n: number) { return Math.round(n * 1000) / 1000 }
function readPackages() { return Number(pkEl.value?.value || '') || 0 }
function readUnits() { return Number(upEl.value?.value || '') || 0 }
// 两个包装字段任一变化时，按当前两个输入框里的真实值重算数量；只有一个有值时保留手填数量。
function syncQuantity() {
  const p = readPackages()
  const u = readUnits()
  const qty = p > 0 && u > 0 ? round3(p * u) : (Number(qtyEl.value?.value || '') || 0)
  patch({ packages: p, units_per_package: u, quantity: qty })
}
function onPackages() { syncQuantity() }
function onUnits() { syncQuantity() }
function onQuantity() {
  patch({ quantity: Number(qtyEl.value?.value || '') || 0 })
}
</script>
