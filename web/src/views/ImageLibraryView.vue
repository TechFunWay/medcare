<template>
  <div class="page">
    <div class="page-head">
      <div>
        <div class="eyebrow">图片</div>
        <h1>图片管理</h1>
        <p>{{ book.current?.name }} · 药品照片、处方单、治疗单、检查报告，一处保存。</p>
      </div>
      <button v-if="book.canImageUpload && scope === 'current'" class="btn btn-primary" @click="openUpload"><AppIcon name="plus" :size="18" />上传图片</button>
    </div>

    <FilterPanel v-model:open="filtersOpen" :count="activeFilterCount" @search="reload" @reset="resetFilters">
      <template #search>
        <div class="card search-card">
          <AppIcon name="search" :size="19" class="search-icon" />
          <input v-model="search" class="input search-input" placeholder="搜索标题、备注或标签" @keyup.enter="reload" />
        </div>
      </template>
      <div class="chip-row">
        <button v-for="c in categories" :key="c.value" type="button" class="chip" :class="{ active: category === c.value }" @click="selectCategory(c.value)">{{ c.label }}</button>
      </div>
      <div v-if="book.books.length > 1" class="chip-row">
        <span class="tiny muted" style="align-self:center">范围</span>
        <button type="button" class="chip" :class="{ active: scope === 'current' }" @click="switchScope('current')">当前用药本</button>
        <button type="button" class="chip" :class="{ active: scope === 'all' }" @click="switchScope('all')">全部用药本</button>
      </div>
      <div class="chip-row">
        <span class="tiny muted" style="align-self:center">分组</span>
        <button type="button" class="chip" :class="{ active: groupBy === 'none' }" @click="groupBy = 'none'">不分组</button>
        <button type="button" class="chip" :class="{ active: groupBy === 'date' }" @click="groupBy = 'date'">按日期</button>
        <button type="button" class="chip" :class="{ active: groupBy === 'person' }" @click="groupBy = 'person'">按用药人</button>
      </div>
      <p v-if="scope === 'all'" class="tiny muted" style="margin:8px 0 0">正在看全部用药本的图片；上传请切回「当前用药本」。</p>
    </FilterPanel>

    <div v-if="loading" class="card muted">加载中…</div>
    <div v-else-if="!items.length" class="empty card">
      <div class="empty-icon"><AppIcon name="camera" :size="30" /></div>
      <h3>{{ search || category !== 'all' ? '没有匹配的图片' : '还没有图片' }}</h3>
      <p v-if="book.canImageUpload">把药品盒、处方单、治疗单、检查报告拍下来存这儿，随时能翻。</p>
      <button v-if="book.canImageUpload && scope === 'current'" class="btn btn-primary" style="margin-top:12px" @click="openUpload">上传第一张图片</button>
    </div>

    <div v-else class="grouped-list">
      <section v-for="group in groupedItems" :key="group.key" class="group-block">
        <div v-if="groupBy !== 'none'" class="group-head"><b>{{ group.title }}</b><span class="tiny muted">{{ group.items.length }} 张</span></div>
        <div class="img-grid">
          <button v-for="img in group.items" :key="img.book_id + '-' + img.id" type="button" class="img-card" @click="openImage(img)">
            <div class="thumb">
              <img v-if="thumbs[imgKey(img)]" :src="thumbs[imgKey(img)]" :alt="img.title" loading="lazy" />
              <div v-else class="thumb-loading">加载中…</div>
              <span class="cat-badge">{{ categoryLabel(img.category) }}</span>
            </div>
            <div class="img-body">
              <div class="img-title">{{ img.title }}</div>
              <div v-if="groupBy !== 'person' && (img.care_recipient_name || img.book_name)" class="tiny muted img-person">{{ img.care_recipient_name || img.book_name }}</div>
              <div v-if="img.tags?.length" class="img-tags"><span v-for="t in img.tags" :key="t">#{{ t }}</span></div>
              <div class="img-date">{{ formatDate(img.created_at) }} · {{ formatSize(img.size) }}</div>
            </div>
          </button>
        </div>
      </section>
    </div>

    <div v-if="hasMore && !loading" style="text-align:center;margin-top:14px">
      <button class="btn btn-outline" :disabled="loadingMore" @click="loadMore">{{ loadingMore ? '加载中…' : '加载更多' }}</button>
    </div>

    <!-- 上传 -->
    <Teleport to="body">
      <div v-if="showUpload" class="sheet-backdrop" @click.self="closeUpload">
        <section class="sheet" role="dialog" aria-modal="true" aria-label="上传图片">
          <div class="sheet-grab" />
          <div class="sheet-head"><div><div class="eyebrow">图片</div><h2>上传图片</h2></div><button class="icon-btn" aria-label="关闭" @click="closeUpload"><AppIcon name="close" /></button></div>
          <div class="sheet-body">
            <div class="field">
              <label>图片（可多选）</label>
              <input ref="fileInput" type="file" accept="image/jpeg,image/png,image/gif,image/webp" multiple style="display:none" @change="onPick" />
              <button type="button" class="btn btn-outline btn-block" :disabled="preparing" @click="fileInput?.click()"><AppIcon name="camera" :size="18" />{{ preparing ? '压缩中…' : '选择图片' }}</button>
              <div v-if="files.length" class="file-list">
                <div v-for="(f, i) in files" :key="i" class="file-row"><span class="truncate">{{ f.name }}</span><span class="file-size">{{ formatSize(f.size) }}</span><button type="button" class="icon-btn" aria-label="移除" @click="files.splice(i, 1)"><AppIcon name="close" :size="14" /></button></div>
              </div>
            </div>
            <div class="field"><label>标题（可选）</label><input v-model="uploadForm.title" class="input" placeholder="不填就用文件名" /></div>
            <div class="field"><label>分类</label><SheetSelect v-model="uploadForm.category" :options="uploadCategories" placeholder="选择分类" /></div>
            <div class="field"><label>标签（逗号分隔，最多 10 个）</label><input v-model="uploadForm.tags" class="input" placeholder="例如：降压药, 早饭后" /></div>
            <div class="field"><label>备注</label><textarea v-model="uploadForm.note" class="textarea" placeholder="补充说明（可选）"></textarea></div>
            <div v-if="uploading" class="upload-progress">
              <div class="upload-bar"><span :style="{ width: uploadProgress + '%' }" /></div>
              <div class="tiny muted">{{ uploadProgress >= 95 ? '上传完成，正在压缩处理图片…' : '正在上传 ' + uploadProgress + '%' }}</div>
            </div>
          </div>
          <div class="sheet-foot"><button class="btn btn-outline btn-block" :disabled="uploading" @click="closeUpload">取消</button><button class="btn btn-primary btn-block" :disabled="uploading || preparing || !files.length" @click="submitUpload">{{ uploading ? (uploadProgress >= 95 ? '处理中…' : '上传中 ' + uploadProgress + '%') : '上传' }}</button></div>
        </section>
      </div>
    </Teleport>

    <!-- 预览 / 编辑 -->
    <Teleport to="body">
      <div v-if="active" class="sheet-backdrop" @click.self="closeImage">
        <section class="sheet" role="dialog" aria-modal="true" aria-label="图片详情">
          <div class="sheet-grab" />
          <div class="sheet-head"><div><div class="eyebrow">{{ categoryLabel(active.category) }}</div><h2>{{ active.title }}</h2></div><button class="icon-btn" aria-label="关闭" @click="closeImage"><AppIcon name="close" /></button></div>
          <div class="sheet-body">
            <div class="preview-wrap" @touchstart.passive="onTouchStart" @touchend="onTouchEnd" @touchmove.prevent>
              <img v-if="previewUrl" :src="previewUrl" :alt="active.title" class="preview-img" @click="enterFullscreen" />
              <div v-else class="thumb-loading">图片加载中…</div>
              <button type="button" class="preview-fullscreen-btn" @click.stop="enterFullscreen"><AppIcon name="maximize" :size="14" />全屏预览</button>
            </div>
            <div class="preview-nav">
              <button class="btn btn-outline btn-sm" :disabled="!hasPrev" @click="showPrev"><AppIcon name="arrowLeft" :size="16" />上一张</button>
              <span class="tiny muted">{{ activeIndex + 1 }} / {{ viewerItems.length }}</span>
              <button class="btn btn-outline btn-sm" :disabled="!hasNext" @click="showNext">下一张<AppIcon name="chevron" :size="16" /></button>
            </div>
            <p class="tiny muted preview-hint">左右滑动或上下滑动翻看，点图片放大</p>
            <div class="preview-actions">
              <button class="btn btn-outline btn-sm" @click="downloadActive"><AppIcon name="download" :size="16" />下载</button>
              <button v-if="book.canImageDelete" class="btn btn-outline btn-sm danger-text" @click="deleteActive"><AppIcon name="trash" :size="16" />删除</button>
            </div>
            <template v-if="book.canImageEdit || book.canImageDelete">
              <div class="field"><label>标题</label><input v-model="editForm.title" class="input" /></div>
              <div class="field"><label>分类</label><SheetSelect v-model="editForm.category" :options="uploadCategories" placeholder="选择分类" /></div>
              <div class="field"><label>标签（逗号分隔，最多 10 个）</label><input v-model="editForm.tags" class="input" placeholder="例如：降压药, 早饭后" /></div>
              <div class="field"><label>备注</label><textarea v-model="editForm.note" class="textarea"></textarea></div>
              <div class="tiny muted" style="margin-bottom:10px">上传：{{ active.created_by_name || '—' }} · {{ formatDate(active.created_at) }}</div>
              <button class="btn btn-primary btn-block" :disabled="saving" @click="saveActive">{{ saving ? '保存中…' : '保存' }}</button>
            </template>
            <template v-else>
              <div class="tiny muted">上传：{{ active.created_by_name || '—' }} · {{ formatDate(active.created_at) }}</div>
            </template>
          </div>
        </section>
      </div>
    </Teleport>

    <!-- 全屏预览 -->
    <Teleport to="body">
      <div v-if="fullscreen && active" class="fullscreen-viewer" @click.self="fullscreen = false">
        <div class="fullscreen-top">
          <div class="fullscreen-title"><b>{{ active.title }}</b><span>{{ activeIndex + 1 }} / {{ viewerItems.length }}</span></div>
          <button class="fullscreen-close" aria-label="退出全屏" @click="fullscreen = false"><AppIcon name="close" :size="22" /></button>
        </div>
        <div class="fullscreen-stage" @touchstart.passive="onTouchStart" @touchend="onTouchEnd" @touchmove.prevent>
          <img v-if="previewUrl" :src="previewUrl" :alt="active.title" class="fullscreen-img" />
          <div v-else class="fullscreen-loading">图片加载中…</div>
        </div>
        <div class="fullscreen-nav">
          <button class="fullscreen-nav-btn" :disabled="!hasPrev" @click="showPrev"><AppIcon name="arrowLeft" :size="18" />上一张</button>
          <span>{{ activeIndex + 1 }} / {{ viewerItems.length }}</span>
          <button class="fullscreen-nav-btn" :disabled="!hasNext" @click="showNext">下一张<AppIcon name="chevron" :size="18" /></button>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import AppIcon from '../components/AppIcon.vue'
import SheetSelect from '../components/SheetSelect.vue'
import FilterPanel from '../components/FilterPanel.vue'
import { imageApi, type BookImage } from '../api/medcare'
import { useBookStore } from '../stores/book'
import { useUIStore } from '../stores/ui'
import { formatDate, localDate, weekdayShort } from '../utils/date'
import { compressImageFile } from '../utils/image'

type LibraryItem = BookImage & { book_name?: string; care_recipient_name?: string }
const book = useBookStore()
const ui = useUIStore()
const items = ref<LibraryItem[]>([])
const scope = ref<'current' | 'all'>('current')
const groupBy = ref<'none' | 'date' | 'person'>('none')
const total = ref(0)
const page = ref(1)
const pageSize = 24
const loading = ref(true)
const loadingMore = ref(false)
const search = ref('')
const category = ref('all')
const filtersOpen = ref(false)
// 折叠态只留搜索框；条数统计分类、范围与分组三处非默认设置。
const activeFilterCount = computed(() => (category.value !== 'all' ? 1 : 0) + (scope.value !== 'current' ? 1 : 0) + (groupBy.value !== 'none' ? 1 : 0))
const thumbs = reactive<Record<string, string>>({})
const objectUrls = new Set<string>()
function imgKey(it: { book_id: number; id: number }) { return String(it.book_id) + '-' + String(it.id) }
function bookIdOf(img: { book_id: number }) { return img.book_id || book.currentId! }

const categories = [
  { value: 'all', label: '全部' },
  { value: 'medicine', label: '药品' },
  { value: 'prescription', label: '处方单' },
  { value: 'treatment', label: '治疗单' },
  { value: 'report', label: '检查报告' },
  { value: 'other', label: '其他' },
]
const uploadCategories = categories.filter((c) => c.value !== 'all')
function categoryLabel(v: string) { return (categories.find((c) => c.value === v) || categories[categories.length - 1]).label }
function formatSize(n: number) { if (!n) return ''; if (n < 1024) return '<1 KB'; if (n < 1024 * 1024) return Math.round(n / 1024) + ' KB'; return (n / 1024 / 1024).toFixed(1) + ' MB' }
const hasMore = computed(() => scope.value === 'current' && items.value.length < total.value)
const groupedItems = computed(() => {
  if (groupBy.value === 'none') return [{ key: 'all', title: '', items: items.value }]
  const map = new Map<string, { key: string; title: string; items: LibraryItem[] }>()
  for (const img of items.value) {
    let key = ''
    let title = ''
    if (groupBy.value === 'date') {
      const d = localDate(new Date(img.created_at))
      key = d
      title = d.replace(/-/g, '/') + ' ' + weekdayShort(d)
    } else {
      key = String(img.book_id)
      title = img.care_recipient_name || img.book_name || ('用药本 ' + img.book_id)
    }
    if (!map.has(key)) map.set(key, { key, title, items: [] })
    map.get(key)!.items.push(img)
  }
  const groups = [...map.values()]
  if (groupBy.value === 'date') groups.sort((a, b) => b.key.localeCompare(a.key))
  else groups.sort((a, b) => a.title.localeCompare(b.title, 'zh-Hans-CN'))
  return groups
})

const showUpload = ref(false)
const uploading = ref(false)
const uploadProgress = ref(0)
const preparing = ref(false)
const fileInput = ref<HTMLInputElement | null>(null)
const files = ref<File[]>([])
const uploadForm = reactive({ title: '', category: 'medicine', tags: '', note: '' })

const active = ref<LibraryItem | null>(null)
const previewUrl = ref('')
const fullUrls = reactive<Record<string, string>>({})
const viewerItems = computed(() => groupedItems.value.flatMap((g) => g.items))
const activeIndex = computed(() => {
  if (!active.value) return -1
  return viewerItems.value.findIndex((x) => x.book_id === active.value!.book_id && x.id === active.value!.id)
})
const hasPrev = computed(() => activeIndex.value > 0)
const hasNext = computed(() => activeIndex.value >= 0 && activeIndex.value < viewerItems.value.length - 1)
const saving = ref(false)
const editing = ref(false)
const editForm = reactive({ title: '', category: 'other', tags: '', note: '' })
const zoom = ref(false)
const fullscreen = ref(false)

// 释放对象 URL 失败不影响功能（浏览器可能已自行回收），这是刻意忽略的唯一一类异常。
function revoke(url?: string) { if (url) { try { URL.revokeObjectURL(url) } catch {} ; objectUrls.delete(url) } }
function revokeAll() { for (const u of objectUrls) { try { URL.revokeObjectURL(u) } catch {} } objectUrls.clear(); for (const k of Object.keys(thumbs)) delete thumbs[k]; for (const k of Object.keys(fullUrls)) delete fullUrls[k] }

async function fetchThumbs() {
  if (!items.value.length) return
  const todo = items.value.filter((it) => !thumbs[imgKey(it)])
  await Promise.allSettled(todo.map(async (it) => {
    try {
      const res = await imageApi.thumb(bookIdOf(it), it.id)
      const url = URL.createObjectURL(res.data as Blob)
      objectUrls.add(url)
      thumbs[imgKey(it)] = url
    } catch { /* 单张失败不阻塞列表 */ }
  }))
}
async function load(reset = true) {
  if (!book.currentId) return
  if (reset) { loading.value = true; page.value = 1; revokeAll() }
  else { loadingMore.value = true }
  try {
    const params = { page: page.value, pageSize, category: category.value, q: search.value.trim() }
    if (scope.value === 'all') {
      // 逐格权限是按用药本各自设置的：没有「图片资料 · 查看」的本直接跳过，不发无谓的请求。
      const visible = book.books.filter((b) => !Array.isArray(b.permissions) || b.permissions.includes('image:view'))
      const lists = await Promise.all(visible.map(async (b) => {
        try {
          const res = await imageApi.list(b.id, { page: 1, pageSize: 100, category: category.value, q: search.value.trim() })
          const data = res.data?.data || {}
          const rows: LibraryItem[] = (data.items || []).map((it: BookImage) => ({ ...it, book_name: b.name, care_recipient_name: (b as any).care_recipient_name || b.name }))
          return { rows, total: data.total || rows.length }
        } catch { return { rows: [] as LibraryItem[], total: 0 } }
      }))
      items.value = lists.flatMap((x) => x.rows)
      total.value = lists.reduce((sum, x) => sum + x.total, 0)
    } else {
      const res = await imageApi.list(book.currentId, params)
      const data = res.data?.data || {}
      const rows: LibraryItem[] = (data.items || []).map((it: BookImage) => ({ ...it, book_name: book.current?.name }))
      items.value = reset ? rows : items.value.concat(rows)
      total.value = data.total || rows.length
    }
    await fetchThumbs()
  } catch (e: any) { ui.showToast(e?.response?.data?.message || '读取图片失败', 'error') }
  finally { loading.value = false; loadingMore.value = false }
}
function reload() { load(true) }
function loadMore() { if (loadingMore.value || !hasMore.value) return; page.value += 1; load(false) }
function selectCategory(v: string) { if (category.value === v) return; category.value = v }
function resetFilters() { category.value = 'all'; scope.value = 'current'; groupBy.value = 'none'; search.value = ''; reload() }
function switchScope(v: 'current' | 'all') { if (scope.value === v) return; scope.value = v; groupBy.value = v === 'all' ? 'person' : 'none' }

function openUpload() { files.value = []; uploadProgress.value = 0; Object.assign(uploadForm, { title: '', category: 'medicine', tags: '', note: '' }); showUpload.value = true }
function closeUpload() { if (!uploading.value) showUpload.value = false }
// 压缩逻辑已下沉到 utils/image.ts，药品图片与图片库共用同一套口径。
async function onPick(e: Event) {
  const input = e.target as HTMLInputElement
  const picked = Array.from(input.files || [])
  const max = 20 * 1024 * 1024
  const bad = picked.find((f) => f.size > max)
  if (bad) { ui.showToast('单张图片不能超过 20MB', 'error'); input.value = ''; return }
  preparing.value = true
  try {
    const compressed: File[] = []
    for (const f of picked) compressed.push(await compressImageFile(f))
    files.value = files.value.concat(compressed).slice(0, 9)
    const saved = compressed.reduce((sum, f, i) => sum + Math.max(0, picked[i].size - f.size), 0)
    if (saved > 0) ui.showToast('已压缩图片，约节省 ' + formatSize(saved))
  } finally { preparing.value = false; input.value = '' }
}
async function submitUpload() {
  if (!book.currentId || !files.value.length) return
  uploading.value = true
  uploadProgress.value = 0
  try {
    const fd = new FormData()
    for (const f of files.value) fd.append('files', f)
    fd.append('title', uploadForm.title.trim())
    fd.append('category', uploadForm.category)
    fd.append('tags', uploadForm.tags)
    fd.append('note', uploadForm.note)
    await imageApi.upload(book.currentId, fd, (e: any) => {
      if (e && e.total) uploadProgress.value = Math.min(95, Math.round((e.loaded / e.total) * 100))
    })
    uploadProgress.value = 100
    showUpload.value = false
    ui.showToast('已上传 ' + files.value.length + ' 张图片')
    files.value = []
    await load(true)
  } catch (e: any) { ui.showToast(e?.response?.data?.message || '上传失败', 'error') }
  finally { uploading.value = false }
}

function loadFullImage(img: LibraryItem) {
  const key = imgKey(img)
  if (fullUrls[key]) return Promise.resolve(fullUrls[key])
  return imageApi.file(bookIdOf(img), img.id).then((res) => {
    const url = URL.createObjectURL(res.data as Blob)
    objectUrls.add(url)
    fullUrls[key] = url
    return url
  })
}
function openImage(img: LibraryItem) {
  active.value = img
  previewUrl.value = fullUrls[imgKey(img)] || ''
  zoom.value = false
  Object.assign(editForm, { title: img.title, category: img.category, tags: (img.tags || []).join('，'), note: img.note || '' })
  if (!previewUrl.value) {
    loadFullImage(img).then((url) => { if (active.value && active.value.id === img.id && active.value.book_id === img.book_id) previewUrl.value = url }).catch(() => {})
  }
}
function enterFullscreen() { if (active.value) fullscreen.value = true }
function closeImage() { active.value = null; previewUrl.value = ''; fullscreen.value = false }
function showAt(index: number) {
  const arr = viewerItems.value
  if (index < 0 || index >= arr.length) return
  openImage(arr[index])
}
function showPrev() { showAt(activeIndex.value - 1) }
function showNext() { showAt(activeIndex.value + 1) }
let touchStart = { x: 0, y: 0, moved: false }
function onTouchStart(e: TouchEvent) { const t = e.touches[0]; touchStart = { x: t.clientX, y: t.clientY, moved: false } }
function onTouchEnd(e: TouchEvent) {
  const t = e.changedTouches[0]
  const dx = t.clientX - touchStart.x
  const dy = t.clientY - touchStart.y
  if (Math.max(Math.abs(dx), Math.abs(dy)) < 55) return
  if (Math.abs(dx) >= Math.abs(dy)) { dx < 0 ? showNext() : showPrev() }
  else { dy < 0 ? showNext() : showPrev() }
}
async function saveActive() {
  if (!active.value) return
  const title = editForm.title.trim()
  if (!title) { ui.showToast('请填写标题', 'error'); return }
  saving.value = true
  try {
    const res = await imageApi.update(bookIdOf(active.value), active.value.id, {
      title, category: editForm.category,
      tags: editForm.tags.split(/[,，、\s]+/).map((s) => s.trim()).filter(Boolean).slice(0, 10),
      note: editForm.note,
    })
    Object.assign(active.value, res.data?.data || {})
    const idx = items.value.findIndex((x) => x.book_id === active.value!.book_id && x.id === active.value!.id)
    if (idx >= 0) items.value[idx] = { ...items.value[idx], ...(res.data?.data || {}) }
    ui.showToast('已保存')
    editing.value = false
  } catch (e: any) { ui.showToast(e?.response?.data?.message || '保存失败', 'error') }
  finally { saving.value = false }
}
async function downloadActive() {
  if (!active.value) return
  try {
    const res = await imageApi.file(bookIdOf(active.value), active.value.id, true)
    const url = URL.createObjectURL(res.data as Blob)
    const a = document.createElement('a')
    a.href = url
    const ext = active.value.mime === 'image/png' ? '.png' : active.value.mime === 'image/gif' ? '.gif' : active.value.mime === 'image/webp' ? '.webp' : '.jpg'
    a.download = active.value.title + ext
    document.body.appendChild(a); a.click(); a.remove()
    setTimeout(() => URL.revokeObjectURL(url), 5000)
  } catch (e: any) { ui.showToast(e?.response?.data?.message || '下载失败', 'error') }
}
async function deleteActive() {
  if (!active.value) return
  const ok = await ui.confirm({ title: '删除这张图片？', message: '删除后无法恢复。', confirmText: '删除', danger: true })
  if (!ok) return
  try {
    await imageApi.remove(bookIdOf(active.value), active.value.id)
    const key = imgKey(active.value)
    revoke(thumbs[key])
    delete thumbs[key]
    revoke(fullUrls[key])
    delete fullUrls[key]
    items.value = items.value.filter((x) => !(x.book_id === active.value!.book_id && x.id === active.value!.id))
    total.value = Math.max(0, total.value - 1)
    fullscreen.value = false
    closeImage()
    ui.showToast('已删除')
  } catch (e: any) { ui.showToast(e?.response?.data?.message || '删除失败', 'error') }
}

function onKeydown(e: KeyboardEvent) {
  if (!fullscreen.value) return
  if (e.key === 'Escape') fullscreen.value = false
  else if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') { e.preventDefault(); showPrev() }
  else if (e.key === 'ArrowRight' || e.key === 'ArrowDown') { e.preventDefault(); showNext() }
}
onMounted(() => { load(true); window.addEventListener('keydown', onKeydown) })
watch(() => book.currentId, () => load(true))
onUnmounted(() => { revokeAll(); window.removeEventListener('keydown', onKeydown) })
</script>

<style scoped>
.search-card { display: flex; align-items: center; gap: 8px; padding: 0 12px; }
.search-card:focus-within { border-color: var(--mc-primary); box-shadow: 0 0 0 3px var(--mc-primary-soft); }
.search-icon { flex: 0 0 auto; color: var(--mc-text-muted); }
.search-input, .search-input:focus { border: 0 !important; background: transparent !important; box-shadow: none !important; border-radius: 0; padding-left: 0; }
.search-row { display: flex; gap: 8px; }
.search-row .input { flex: 1; min-width: 0; }
.grouped-list { display: grid; gap: 16px; }
.group-head { display: flex; align-items: baseline; gap: 8px; padding: 2px 2px 8px; }
.group-head b { font-size: 14px; }
.img-person { margin-top: 3px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.img-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
.img-card { padding: 0; border: 1px solid var(--mc-border); background: var(--mc-surface); border-radius: 14px; overflow: hidden; text-align: left; cursor: pointer; }
.thumb { position: relative; aspect-ratio: 4 / 3; background: var(--mc-surface-2); display: grid; place-items: center; overflow: hidden; }
.thumb img { width: 100%; height: 100%; object-fit: cover; display: block; }
.thumb-loading { font-size: 12px; color: var(--mc-text-muted); }
.cat-badge { position: absolute; left: 8px; top: 8px; padding: 3px 8px; border-radius: 999px; background: rgba(20,48,42,.72); color: #fff; font-size: 11px; font-weight: 700; backdrop-filter: blur(4px); }
.img-body { padding: 9px 10px 10px; }
.img-title { font-size: 13px; font-weight: 700; line-height: 1.4; overflow: hidden; text-overflow: ellipsis; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; }
.img-tags { margin-top: 5px; display: flex; gap: 4px; flex-wrap: wrap; }
.img-tags span { font-size: 10.5px; color: var(--mc-primary-strong); background: var(--mc-primary-soft); padding: 1px 6px; border-radius: 6px; }
.img-date { margin-top: 5px; font-size: 11px; color: var(--mc-text-muted); }
.file-list { margin-top: 8px; display: grid; gap: 6px; }
.file-row { display: flex; align-items: center; gap: 8px; font-size: 12px; padding: 6px 8px; background: var(--mc-surface-2); border-radius: 8px; }
.file-row .truncate { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.file-size { flex: 0 0 auto; font-size: 11px; color: var(--mc-text-muted); }
.upload-progress { margin: 4px 0 10px; }
.upload-bar { height: 8px; border-radius: 999px; background: var(--mc-surface-2); overflow: hidden; margin-bottom: 5px; }
.upload-bar span { display: block; height: 100%; border-radius: 999px; background: var(--mc-primary); transition: width .18s ease; }
.preview-wrap { position: relative; background: var(--mc-surface-2); border-radius: 12px; min-height: 160px; display: grid; place-items: center; overflow: hidden; margin-bottom: 10px; }
.preview-fullscreen-btn { position: absolute; right: 8px; top: 8px; display: inline-flex; align-items: center; gap: 4px; border: 0; border-radius: 999px; padding: 5px 9px; background: rgba(0,0,0,.55); color: #fff; font-size: 11.5px; font-weight: 700; cursor: pointer; }
.preview-img { width: 100%; max-height: 52vh; object-fit: contain; display: block; }
.preview-img:active { transform: scale(1.02); }
.preview-nav { display: flex; align-items: center; justify-content: space-between; gap: 8px; margin-bottom: 4px; }
.preview-nav .btn { flex: 0 0 auto; }
.preview-hint { text-align: center; margin: 0 0 12px; }
.preview-actions { display: flex; gap: 8px; margin-bottom: 12px; }
.preview-actions .btn { flex: 1; }
.danger-text { color: var(--mc-danger); }
@media (min-width: 720px) { .img-grid { grid-template-columns: repeat(4, minmax(0, 1fr)); } }
.fullscreen-viewer { position: fixed; inset: 0; z-index: 2000; background: #000; color: #fff; display: flex; flex-direction: column; }
.fullscreen-top { display: flex; align-items: center; gap: 10px; padding: calc(10px + env(safe-area-inset-top)) 14px 8px; }
.fullscreen-title { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
.fullscreen-title b { font-size: 14px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.fullscreen-title span { font-size: 11px; color: rgba(255,255,255,.65); }
.fullscreen-close { border: 0; background: rgba(255,255,255,.12); color: #fff; width: 38px; height: 38px; border-radius: 50%; display: grid; place-items: center; cursor: pointer; }
.fullscreen-stage { flex: 1; min-height: 0; display: flex; align-items: center; justify-content: center; overflow: hidden; touch-action: none; }
.fullscreen-img { max-width: 100vw; max-height: 100%; object-fit: contain; display: block; }
.fullscreen-loading { color: rgba(255,255,255,.7); font-size: 13px; }
.fullscreen-nav { display: flex; align-items: center; justify-content: space-between; gap: 10px; padding: 10px 16px calc(12px + env(safe-area-inset-bottom)); font-size: 12px; color: rgba(255,255,255,.8); }
.fullscreen-nav-btn { display: inline-flex; align-items: center; gap: 4px; border: 1px solid rgba(255,255,255,.28); background: rgba(255,255,255,.08); color: #fff; padding: 8px 12px; border-radius: 999px; font-size: 12px; cursor: pointer; }
.fullscreen-nav-btn:disabled { opacity: .35; }
</style>
