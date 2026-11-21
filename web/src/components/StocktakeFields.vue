<template>
  <div class="field"><label>盘点后总量（{{ unit || '单位' }}）</label><input :value="modelValue.target_quantity" type="number" min="0" step="0.01" class="input" placeholder="盘点后剩余多少" @input="patch({ target_quantity: Number(($event.target as HTMLInputElement).value) || 0 })" /></div>
  <div class="field"><label>原因</label><input :value="modelValue.reason" class="input" placeholder="例如：月末盘点 / 损耗核对" @input="patch({ reason: ($event.target as HTMLInputElement).value })" /></div>
  <p class="tiny muted" style="margin:-4px 0 8px">盘点只把库存改成实际数量，多退少补；不是出库。</p>
</template>

<script setup lang="ts">
import type { StocktakeDraft } from '../utils/stock'
const props = defineProps<{ modelValue: StocktakeDraft; unit?: string }>()
const emit = defineEmits<{ (e: 'update:modelValue', v: StocktakeDraft): void }>()
function patch(p: Partial<StocktakeDraft>) { emit('update:modelValue', { ...props.modelValue, ...p }) }
</script>
