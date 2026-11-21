<template>
  <div class="field"><label>出库数量（{{ unit || '单位' }}）</label><input :value="modelValue.quantity || ''" type="number" min="0" step="0.01" class="input" placeholder="例如 1" @input="patch({ quantity: Number(($event.target as HTMLInputElement).value) || 0 })" /></div>
  <div class="field"><label>用途</label><SheetSelect :model-value="modelValue.channel" :options="channelOptions" placeholder="选择用途" @update:model-value="patch({ channel: String($event) })" /></div>
  <div class="field"><label>原因</label><input :value="modelValue.reason" class="input" placeholder="例如：过期丢弃 / 借出 / 带走" @input="patch({ reason: ($event.target as HTMLInputElement).value })" /></div>
  <p class="tiny muted" style="margin:-4px 0 8px">出库按先进先出扣批次，数量不能超过当前库存。</p>
</template>

<script setup lang="ts">
import SheetSelect from './SheetSelect.vue'
import { STOCK_CHANNEL_OPTIONS, type StockOutDraft } from '../utils/stock'
const props = defineProps<{ modelValue: StockOutDraft; unit?: string }>()
const emit = defineEmits<{ (e: 'update:modelValue', v: StockOutDraft): void }>()
const channelOptions = STOCK_CHANNEL_OPTIONS
function patch(p: Partial<StockOutDraft>) { emit('update:modelValue', { ...props.modelValue, ...p }) }
</script>
