<template>
  <div v-if="open" class="sheet-backdrop" @click.self="close">
    <section class="sheet" role="dialog" aria-modal="true">
      <div class="sheet-grab" />
      <div class="sheet-head"><h2>修改入库记录</h2><button class="icon-btn" aria-label="关闭" @click="close"><AppIcon name="close" /></button></div>
      <form class="sheet-body" @submit.prevent="save">
        <p class="tiny muted" style="margin:-2px 0 10px">
          入库后 7 天内可以修改。改数量只调整这个批次的入库总量，已用掉的量保持不变。
          <template v-if="editUntil">（可改到 {{ editUntil }}）</template>
        </p>
        <div class="field">
          <label>所属药品</label>
          <MedicinePicker :model-value="form.medicine_id" :medicines="medicines" placeholder="选择药品" show-stock @update:model-value="(v) => form.medicine_id = Number(v)" />
          <p v-if="batch?.medicine_missing" class="tiny" style="color:var(--mc-danger);margin:4px 0 0">
            原药品「{{ batch?.medicine_name || '未知' }}」已被删除，请重新选择所属药品后再保存。
          </p>
          <p v-else-if="batch?.medicine_name" class="tiny muted" style="margin:4px 0 0">当前归属：{{ batch.medicine_name }}</p>
        </div>
        <div class="field"><label>入库数量（{{ unit || '单位' }}）</label><input v-model.number="form.quantity" type="number" min="0" step="0.01" class="input" /></div>
        <div class="field"><label>入库来源</label><SheetSelect v-model="form.channel" :options="channelOptions" placeholder="选择来源" /></div>
        <div class="field-row">
          <div class="field"><label>批号</label><input v-model="form.batch_no" class="input" placeholder="可空" /></div>
          <div class="field"><label>有效期</label><DateField v-model="form.expiry_date" placeholder="选择日期" clearable /></div>
        </div>
        <div class="field"><label>供应商</label><input v-model="form.supplier" class="input" placeholder="可空" /></div>
        <div class="field"><label>备注</label><input v-model="form.note" class="input" placeholder="可空" /></div>
        <div class="sheet-foot">
          <button type="button" class="btn btn-outline btn-block" @click="close">取消</button>
          <button class="btn btn-primary btn-block" :disabled="saving">{{ saving ? '保存中…' : '保存修改' }}</button>
        </div>
      </form>
    </section>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import AppIcon from './AppIcon.vue'
import MedicinePicker from './MedicinePicker.vue'
import type { Medicine } from '../api/medcare'
import SheetSelect from './SheetSelect.vue'
import DateField from './DateField.vue'
import { medicineApi } from '../api/medcare'
import { STOCK_CHANNEL_OPTIONS } from '../utils/stock'
import { useBookStore } from '../stores/book'
import { useUIStore } from '../stores/ui'

const props = defineProps<{ open: boolean; batch: any; unit?: string; medicines: Medicine[] }>()
const emit = defineEmits<{ (e: 'close'): void; (e: 'saved'): void }>()
const book = useBookStore()
const ui = useUIStore()
const channelOptions = STOCK_CHANNEL_OPTIONS
const saving = ref(false)
const form = reactive({ medicine_id: 0, quantity: 0, channel: 'pharmacy', batch_no: '', expiry_date: '', supplier: '', note: '' })
const editUntil = ref('')

watch(() => props.batch, (b) => {
  if (!b) return
  form.medicine_id = Number(b.medicine_id || 0)
  form.quantity = Number(b.quantity_initial || 0)
  form.channel = b.channel || 'pharmacy'
  form.batch_no = b.batch_no || ''
  form.expiry_date = b.expiry_date || ''
  form.supplier = b.supplier || ''
  form.note = b.note || ''
  editUntil.value = b.edit_until || ''
}, { immediate: true })

function close() { if (!saving.value) emit('close') }

async function save() {
  if (!props.batch || !book.currentId) return
  if (!(Number(form.quantity) > 0)) { ui.showToast('入库数量必须大于 0', 'error'); return }
  if (!form.medicine_id) { ui.showToast('请选择所属药品', 'error'); return }
  saving.value = true
  try {
    await medicineApi.updateBatch(book.currentId, props.batch.id, {
      medicine_id: Number(form.medicine_id),
      quantity: Number(form.quantity), channel: form.channel, batch_no: form.batch_no,
      expiry_date: form.expiry_date, supplier: form.supplier, note: form.note,
    })
    ui.showToast('入库记录已更新')
    emit('saved')
  } catch (e: any) {
    ui.showToast(e?.response?.data?.message || '保存失败', 'error')
  } finally { saving.value = false }
}
</script>
