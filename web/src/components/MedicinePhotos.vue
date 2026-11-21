<template>
  <div class="section">
    <div class="section-head">
      <h2>药品图片</h2>
      <span class="tiny muted">{{ images.length }} 张</span>
    </div>
    <div class="card photos-card">
      <div class="cover-row">
        <div class="cover-box" @click="coverImage ? preview(coverImage) : pick()">
          <img v-if="coverThumb" :src="coverThumb" alt="药品封面" />
          <div v-else class="cover-empty"><AppIcon name="camera" :size="22" /><span class="tiny">封面</span></div>
        </div>
        <div class="cover-text">
          <b>封面图片</b>
          <p class="tiny muted">封面会作为缩略图显示在药品列表里。上传后点图片上的「设为封面」即可更换。</p>
          <div class="cover-btns">
            <button v-if="canUpload" class="btn btn-primary btn-sm" :disabled="uploading" @click="pick">
              <AppIcon name="camera" :size="16" />{{ uploading ? '上传中 ' + progress + '%' : '上传图片' }}
            </button>
            <button v-if="canEdit && coverImageId" class="btn btn-ghost btn-sm" :disabled="uploading" @click="setCover(0)">清除封面</button>
          </div>
          <p class="tiny muted">上传前会自动压缩（长边 2560px、质量 0.9），服务端再存一份 420px 缩略图，清晰且不占空间。</p>
        </div>
      </div>

      <div v-if="uploading" class="upload-progress">
        <div class="upload-bar"><span :style="{ width: progress + '%' }" /></div>
        <div class="tiny muted">{{ progress >= 95 ? '上传完成，正在压缩处理图片…' : '正在上传 ' + progress + '%' }}</div>
      </div>

      <div v-if="loading" class="tiny muted" style="margin-top:10px">图片加载中…</div>
      <div v-else-if="images.length" class="photo-grid">
        <div v-for="img in images" :key="img.id" class="photo-tile" :class="{ 'is-cover': img.id === coverImageId }">
          <img v-if="thumbs[img.id]" :src="thumbs[img.id]" :alt="img.title" loading="lazy" @click="preview(img)" />
          <div v-else class="tile-loading" @click="preview(img)">加载中…</div>
          <span v-if="img.id === coverImageId" class="badge done cover-badge">封面</span>
          <div class="tile-actions">
            <button v-if="canEdit && img.id !== coverImageId" class="tile-btn" @click.stop="setCover(img.id)">设为封面</button>
            <button v-if="canDelete" class="tile-btn danger" :aria-label="'删除' + img.title" @click.stop="remove(img)"><AppIcon name="trash" :size="14" /></button>
          </div>
        </div>
      </div>
      <p v-else class="tiny muted" style="margin-top:10px">还没有图片。可以拍药盒正面、说明书或药片外观，方便家人核对。</p>

      <input ref="fileInput" type="file" accept="image/*" multiple class="hidden-input" @change="onPick" />
    </div>

    <Teleport to="body">
      <div v-if="previewUrl" class="sheet-backdrop" @click.self="closePreview">
        <section class="sheet photo-sheet" role="dialog" aria-modal="true">
          <div class="sheet-grab" />
          <div class="sheet-head"><h2>{{ previewTitle }}</h2><button class="icon-btn" aria-label="关闭" @click="closePreview"><AppIcon name="close" /></button></div>
          <div class="sheet-body photo-body"><img :src="previewUrl" :alt="previewTitle" /></div>
        </section>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import AppIcon from './AppIcon.vue'
import { imageApi, medicineApi, type BookImage } from '../api/medcare'
import { compressImageFile, dropThumb, formatImageSize, loadThumb } from '../utils/image'
import { useBookStore } from '../stores/book'
import { useUIStore } from '../stores/ui'

const props = defineProps<{ bookId: number; medicineId: number; coverImageId: number }>()
const emit = defineEmits<{ (e: 'update:cover', id: number): void }>()

const book = useBookStore()
const ui = useUIStore()
const canUpload = computed(() => book.canImageUpload)
const canEdit = computed(() => book.canMedicineEdit)
const canDelete = computed(() => book.canImageDelete)

const images = ref<BookImage[]>([])
const thumbs = reactive<Record<number, string>>({})
const loading = ref(false)
const uploading = ref(false)
const progress = ref(0)
const fileInput = ref<HTMLInputElement | null>(null)
const coverImage = computed(() => images.value.find((i) => i.id === props.coverImageId) || null)
const coverThumb = computed(() => (props.coverImageId ? thumbs[props.coverImageId] || '' : ''))

async function load() {
  if (!props.bookId || !props.medicineId || !book.canImageView) return
  loading.value = true
  try {
    const res = await imageApi.list(props.bookId, { medicine_id: props.medicineId, pageSize: 100 })
    images.value = (res.data?.data?.items || []) as BookImage[]
    await Promise.all(images.value.map(async (img) => {
      try { thumbs[img.id] = await loadThumb(props.bookId, img.id) } catch { /* 单张失败不影响其它 */ }
    }))
  } catch (e: any) {
    ui.showToast(e?.response?.data?.message || '读取药品图片失败', 'error')
  } finally { loading.value = false }
}

function pick() { if (!uploading.value) fileInput.value?.click() }

async function onPick(e: Event) {
  const input = e.target as HTMLInputElement
  const picked = Array.from(input.files || [])
  input.value = ''
  if (!picked.length || !props.bookId || !props.medicineId) return
  const max = 20 * 1024 * 1024
  if (picked.some((f) => f.size > max)) { ui.showToast('单张图片不能超过 20MB', 'error'); return }
  const files: File[] = []
  for (const f of picked.slice(0, 9)) files.push(await compressImageFile(f))
  const saved = files.reduce((sum, f, i) => sum + Math.max(0, picked[i].size - f.size), 0)
  uploading.value = true
  progress.value = 0
  try {
    const fd = new FormData()
    for (const f of files) fd.append('files', f)
    fd.append('medicine_id', String(props.medicineId))
    fd.append('category', 'medicine')
    const res = await imageApi.upload(props.bookId, fd, (ev: any) => {
      if (ev && ev.total) progress.value = Math.min(95, Math.round((ev.loaded / ev.total) * 100))
    })
    progress.value = 100
    const added = (res.data?.data?.items || []) as BookImage[]
    ui.showToast('已上传 ' + files.length + ' 张图片' + (saved > 0 ? '，压缩节省 ' + formatImageSize(saved) : ''))
    await load()
    // 第一张上传的图片自动当封面，省一步操作。
    if (!props.coverImageId && added.length) await setCover(added[0].id, true)
  } catch (e: any) {
    ui.showToast(e?.response?.data?.message || '上传失败', 'error')
  } finally { uploading.value = false; progress.value = 0 }
}

async function setCover(id: number, silent = false) {
  if (!props.bookId || !props.medicineId) return
  try {
    await medicineApi.setCover(props.bookId, props.medicineId, id)
    emit('update:cover', id)
    if (!silent) ui.showToast(id ? '已设为封面' : '已清除封面')
  } catch (e: any) {
    ui.showToast(e?.response?.data?.message || '设置封面失败', 'error')
  }
}

async function remove(img: BookImage) {
  const ok = await ui.confirm({ title: '删除这张图片？', message: '「' + img.title + '」将从药品图片里移除，删除后无法恢复。', confirmText: '删除' })
  if (!ok) return
  try {
    await imageApi.remove(props.bookId, img.id)
    dropThumb(props.bookId, img.id)
    delete thumbs[img.id]
    if (img.id === props.coverImageId) emit('update:cover', 0)
    ui.showToast('已删除图片')
    await load()
  } catch (e: any) {
    ui.showToast(e?.response?.data?.message || '删除失败', 'error')
  }
}

const previewUrl = ref('')
const previewTitle = ref('')
let previewObjectUrl = ''
async function preview(img: BookImage) {
  previewTitle.value = img.title
  try {
    const res = await imageApi.file(props.bookId, img.id)
    previewObjectUrl = URL.createObjectURL(res.data as Blob)
    previewUrl.value = previewObjectUrl
  } catch (e: any) {
    ui.showToast(e?.response?.data?.message || '读取图片失败', 'error')
  }
}
function closePreview() {
  previewUrl.value = ''
  if (previewObjectUrl) { try { URL.revokeObjectURL(previewObjectUrl) } catch { /* 已回收 */ } ; previewObjectUrl = '' }
}

onMounted(load)
watch(() => [props.bookId, props.medicineId], load)
</script>

<style scoped>
.photos-card { padding: 12px; }
.cover-row { display: flex; gap: 12px; align-items: flex-start; }
.cover-box { flex: 0 0 auto; width: 84px; height: 84px; border-radius: 12px; overflow: hidden; border: 1px solid var(--mc-border); background: var(--mc-surface-2); display: grid; place-items: center; cursor: pointer; }
.cover-box img { width: 100%; height: 100%; object-fit: cover; display: block; }
.cover-empty { display: grid; place-items: center; gap: 2px; color: var(--mc-text-muted); }
.cover-text { min-width: 0; flex: 1 1 auto; display: flex; flex-direction: column; gap: 5px; }
.cover-text b { font-size: 14px; }
.cover-btns { display: flex; gap: 8px; flex-wrap: wrap; }
.upload-progress { margin-top: 10px; }
.upload-bar { height: 6px; border-radius: 999px; background: var(--mc-surface-2); overflow: hidden; }
.upload-bar span { display: block; height: 100%; background: var(--mc-primary); transition: width .2s; }
.photo-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 8px; margin-top: 12px; }
.photo-tile { position: relative; border-radius: 12px; overflow: hidden; border: 1px solid var(--mc-border); background: var(--mc-surface-2); aspect-ratio: 1 / 1; }
.photo-tile.is-cover { border-color: var(--mc-primary); box-shadow: 0 0 0 1px var(--mc-primary); }
.photo-tile img { width: 100%; height: 100%; object-fit: cover; display: block; cursor: zoom-in; }
.tile-loading { width: 100%; height: 100%; display: grid; place-items: center; font-size: 11px; color: var(--mc-text-muted); }
.cover-badge { position: absolute; top: 5px; left: 5px; }
.tile-actions { position: absolute; left: 0; right: 0; bottom: 0; display: flex; gap: 4px; justify-content: space-between; padding: 4px; background: linear-gradient(to top, rgba(0,0,0,.55), rgba(0,0,0,0)); }
.tile-btn { border: 0; border-radius: 8px; padding: 4px 7px; font-size: 11px; font-weight: 700; background: var(--mc-surface); color: var(--mc-text); display: inline-flex; align-items: center; gap: 3px; }
.tile-btn.danger { color: var(--mc-danger); margin-left: auto; }
.hidden-input { display: none; }
.photo-sheet { max-width: 900px; }
.photo-body { display: grid; place-items: center; }
.photo-body img { max-width: 100%; max-height: 70dvh; object-fit: contain; border-radius: 12px; }
@media (min-width: 900px) {
  .photos-card { padding: 15px; }
  .cover-box { width: 104px; height: 104px; }
  .photo-grid { grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: 10px; }
  .tile-btn { font-size: 12px; padding: 5px 9px; }
}
</style>
