// 图片上传前压缩与缩略图缓存：药品图片、图片库共用一套，避免各页面各写一份。
import { imageApi } from '../api/medcare'

// 上传前在浏览器里压缩：限制长边 2560px，JPEG 质量 0.9；小图不强行重编码，保证清晰。
// 服务端还会再压一次并生成 420px 缩略图（image_process.go），两边口径一致。
export async function compressImageFile(file: File): Promise<File> {
  if (file.type === 'image/gif') return file
  if (!file.type.startsWith('image/')) return file
  try {
    const bitmap = await createImageBitmap(file, { imageOrientation: 'from-image' })
    const maxDim = 2560
    const scale = Math.min(1, maxDim / Math.max(bitmap.width, bitmap.height))
    const targetW = Math.max(1, Math.round(bitmap.width * scale))
    const targetH = Math.max(1, Math.round(bitmap.height * scale))
    // 本来就不大、尺寸也不超限的图片保留原图，避免二次压缩掉清晰度。
    if (scale >= 1 && file.size <= 1.2 * 1024 * 1024) { bitmap.close?.(); return file }
    const canvas = document.createElement('canvas')
    canvas.width = targetW
    canvas.height = targetH
    const ctx = canvas.getContext('2d')
    if (!ctx) { bitmap.close?.(); return file }
    ctx.drawImage(bitmap, 0, 0, targetW, targetH)
    bitmap.close?.()
    const outType = file.type === 'image/png' ? 'image/png' : 'image/jpeg'
    const blob: Blob | null = await new Promise((resolve) => canvas.toBlob(resolve, outType, 0.9))
    if (!blob || blob.size >= file.size) return file
    const ext = outType === 'image/png' ? '.png' : '.jpg'
    return new File([blob], file.name.replace(/\.[^.]+$/, '') + ext, { type: outType, lastModified: file.lastModified })
  } catch {
    return file
  }
}

export function formatImageSize(bytes: number): string {
  if (bytes >= 1024 * 1024) return (bytes / 1024 / 1024).toFixed(1) + ' MB'
  if (bytes >= 1024) return Math.round(bytes / 1024) + ' KB'
  return bytes + ' B'
}

// 缩略图走接口取 blob（图片要带令牌，不能直接写 img src），取到后缓存 objectURL。
// 缓存有上限，超出按先进先出回收，避免长时间浏览后内存里堆满 blob。
const thumbCache = new Map<string, string>()
const thumbPending = new Map<string, Promise<string>>()
const THUMB_CACHE_MAX = 240

function keyOf(bookId: number, id: number) { return bookId + ':' + id }

export function cachedThumb(bookId: number, id: number): string {
  return thumbCache.get(keyOf(bookId, id)) || ''
}

export function loadThumb(bookId: number, id: number): Promise<string> {
  const key = keyOf(bookId, id)
  const hit = thumbCache.get(key)
  if (hit) return Promise.resolve(hit)
  const running = thumbPending.get(key)
  if (running) return running
  const task = imageApi.thumb(bookId, id).then((res) => {
    const url = URL.createObjectURL(res.data as Blob)
    thumbCache.set(key, url)
    while (thumbCache.size > THUMB_CACHE_MAX) {
      const oldest = thumbCache.keys().next().value as string | undefined
      if (!oldest) break
      const old = thumbCache.get(oldest)
      if (old) { try { URL.revokeObjectURL(old) } catch { /* 已回收 */ } }
      thumbCache.delete(oldest)
    }
    return url
  }).finally(() => { thumbPending.delete(key) })
  thumbPending.set(key, task)
  return task
}

// 图片被删除或替换后丢掉缓存，下次重新取。
export function dropThumb(bookId: number, id: number) {
  const key = keyOf(bookId, id)
  const url = thumbCache.get(key)
  if (url) { try { URL.revokeObjectURL(url) } catch { /* 已回收 */ } }
  thumbCache.delete(key)
}

export function clearThumbs() {
  for (const url of thumbCache.values()) { try { URL.revokeObjectURL(url) } catch { /* 已回收 */ } }
  thumbCache.clear()
}
