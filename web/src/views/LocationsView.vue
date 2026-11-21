<template>
  <div class="page narrow">
    <div class="page-head">
      <div><div class="eyebrow">地点</div><h1>打针 / 输液地点</h1><p>医院、诊所、家里，地址都记在这里。</p></div>
      <button v-if="book.canPlanCreate" class="btn btn-primary" @click="openAdd"><AppIcon name="plus" :size="18" />添加</button>
    </div>
    <div v-if="loading" class="card muted">加载中…</div>
    <div v-else-if="!items.length" class="empty card"><div class="empty-icon"><AppIcon name="location" :size="30" /></div><h3>还没有地点</h3><p>添加常去的医院、诊所，或居家地址。</p><button v-if="book.canPlanCreate" class="btn btn-primary" style="margin-top:12px" @click="openAdd">添加地点</button></div>
    <div v-else class="list">
      <article v-for="l in items" :key="l.id" class="card loc-card">
        <div class="loc-main">
          <div class="type-dot" :class="l.type === 'hospital' ? 'infusion' : l.type === 'clinic' ? 'injection' : ''"><AppIcon name="location" :size="18" /></div>
          <div class="loc-body"><div class="loc-title-row"><b>{{ l.name }}</b><span class="badge">{{ typeLabel(l.type) }}</span></div><div class="muted small" style="margin-top:2px">{{ l.address || '未填地址' }}</div><div v-if="l.contact_name || l.contact_phone" class="tiny muted" style="margin-top:2px">{{ l.contact_name }} {{ l.contact_phone }}</div></div>
        </div>
        <div class="loc-actions">
          <button class="btn btn-outline btn-xs" @click="copy(l.address)">复制地址</button>
          <a class="btn btn-outline btn-xs" :href="mapUrl(l.address)" target="_blank" rel="noopener">打开地图</a>
          <button v-if="book.canPlanEdit" class="btn btn-outline btn-xs" @click="edit(l)">编辑</button>
        </div>
      </article>
    </div>
    <Teleport to="body">
      <div v-if="show" class="sheet-backdrop" @click.self="show=false">
        <section class="sheet"><div class="sheet-grab" /><div class="sheet-head"><h2>{{ editingId ? '编辑地点' : '添加地点' }}</h2><button class="icon-btn" @click="show=false"><AppIcon name="close" /></button></div>
          <form class="sheet-body" @submit.prevent="save">
            <div class="field"><label>名称</label><input v-model="form.name" class="input" required placeholder="例如：社区诊所" /></div>
            <div class="field"><label>类型</label><div class="chip-row"><button v-for="t in types" :key="t.value" type="button" class="chip" :class="{ active: form.type === t.value }" @click="form.type = t.value">{{ t.label }}</button></div></div>
            <div class="field"><label>地址</label><textarea v-model="form.address" class="textarea" placeholder="详细地址" /></div>
            <div class="field-row"><div class="field"><label>联系人</label><input v-model="form.contact_name" class="input" /></div><div class="field"><label>电话</label><input v-model="form.contact_phone" class="input" /></div></div>
            <div class="field"><label>备注</label><input v-model="form.notes" class="input" /></div>
            <div class="sheet-foot"><button type="button" class="btn btn-outline btn-block" @click="show=false">取消</button><button class="btn btn-primary btn-block" :disabled="saving">{{ saving ? '保存中…' : '保存' }}</button></div>
          </form>
        </section>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref, watch } from 'vue'
import AppIcon from '../components/AppIcon.vue'
import { locationApi, type Location } from '../api/medcare'
import { useBookStore } from '../stores/book'
import { useUIStore } from '../stores/ui'
const book = useBookStore()
const ui = useUIStore()
const items = ref<Location[]>([])
const loading = ref(true)
const show = ref(false)
const saving = ref(false)
const editingId = ref(0)
const form = reactive({ name: '', type: 'clinic', address: '', contact_name: '', contact_phone: '', notes: '' })
const types = [{ value: 'home', label: '居家' }, { value: 'clinic', label: '诊所' }, { value: 'hospital', label: '医院' }, { value: 'pharmacy', label: '药房' }, { value: 'other', label: '其他' }]
function typeLabel(t: string) { return types.find((x) => x.value === t)?.label || '其他' }
function mapUrl(address: string) { return 'https://uri.amap.com/search?keyword=' + encodeURIComponent(address || '') }
async function load() { if (!book.currentId) return; loading.value = true; try { const res = await locationApi.list(book.currentId); items.value = res.data?.data || [] } catch (e: any) { ui.showToast(e?.response?.data?.message || '读取地点失败', 'error') } finally { loading.value = false } }
function openAdd() { editingId.value = 0; Object.assign(form, { name: '', type: 'clinic', address: '', contact_name: '', contact_phone: '', notes: '' }); show.value = true }
function edit(l: Location) { editingId.value = l.id; Object.assign(form, { name: l.name, type: l.type, address: l.address, contact_name: l.contact_name || '', contact_phone: l.contact_phone || '', notes: l.notes || '' }); show.value = true }
async function save() { saving.value = true; try { if (editingId.value) await locationApi.update(book.currentId!, editingId.value, { ...form }); else await locationApi.create(book.currentId!, { ...form }); show.value = false; ui.showToast('地点已保存'); load() } catch (e: any) { ui.showToast(e?.response?.data?.message || '保存失败', 'error') } finally { saving.value = false } }
async function copy(text: string) { if (!text) return; try { await navigator.clipboard.writeText(text); ui.showToast('已复制') } catch { ui.showToast('复制失败', 'error') } }
onMounted(load)
watch(() => book.currentId, load)
</script>

<style scoped>
.loc-card { display: grid; gap: 9px; }
.loc-main { display: flex; gap: 10px; align-items: flex-start; min-width: 0; }
.loc-body { flex: 1; min-width: 0; }
.loc-title-row { display: flex; gap: 6px; align-items: center; flex-wrap: wrap; min-width: 0; }
.loc-title-row b { font-size: 14.5px; overflow-wrap: anywhere; }
.loc-actions { display: flex; gap: 6px; flex-wrap: wrap; }
@media (min-width: 900px) {
  .loc-card { gap: 10px; }
  .loc-title-row { gap: 8px; }
  .loc-title-row b { font-size: 15px; }
  .loc-actions { gap: 8px; }
}
</style>
