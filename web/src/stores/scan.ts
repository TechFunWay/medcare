import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { prescriptionApi } from '../api/medcare'
import { useUIStore } from './ui'

// AI 拍照识别的后台任务台（2026-11-16 用户要求：识别可在后台跑，关窗 / 切页不丢）：
// 服务端本来就是入队异步识别（prescriptions/recognize 立即返回 import_id），
// 但原先轮询与核对结果都放在弹层组件里，关窗重开会被重置、切页组件卸载就丢。
// 这里把任务状态提升到全局 store：弹层只是它的一个视图，关掉不影响任务继续。
export interface ScanJob {
  id: number
  bookId: number
  kind: 'box' | 'receipt'
  // 从哪个页面发起：药品页只建档（medicine）／库存页入库（stock）。
  mode: 'medicine' | 'stock'
  status: 'running' | 'done' | 'error'
  draft: any | null
  error: string
  startedAt: number
}

const POLL_INTERVAL = 3000
const POLL_TIMEOUT = 240000

export const useScanStore = defineStore('scan', () => {
  const jobs = ref<ScanJob[]>([])
  // 弹层是否开着：开着时不显示左下角的后台任务胶囊，避免重复入口。
  const sheetOpen = ref(false)
  // 胶囊 / 通知点「去核对」时置真，药品页监听到就打开弹层。
  const requestOpen = ref(false)
  const polling = new Set<number>()

  const runningJobs = computed(() => jobs.value.filter((j) => j.status === 'running'))
  // 当前用药本里「还在跑」或「跑完还没核对」的任务，供胶囊与弹层共用。
  function jobsFor(bookId: number) {
    return jobs.value.filter((j) => j.bookId === bookId && j.status !== 'error')
  }
  function latestFor(bookId: number): ScanJob | null {
    const list = jobsFor(bookId)
    return list.length ? list[list.length - 1] : null
  }
  function pillJob(bookId: number): ScanJob | null {
    if (sheetOpen.value) return null
    return latestFor(bookId)
  }

  async function poll(job: ScanJob) {
    if (polling.has(job.id)) return
    polling.add(job.id)
    const ui = useUIStore()
    const started = Date.now()
    try {
      while (Date.now() - started < POLL_TIMEOUT) {
        await new Promise((r) => setTimeout(r, POLL_INTERVAL))
        // 任务可能被用户清掉（导入完成 / 手动放弃），清掉就停轮询。
        if (!jobs.value.some((j) => j.id === job.id)) return
        try {
          const res = await prescriptionApi.get(job.bookId, job.id)
          const data = res.data?.data
          const status = data?.status || data?.import?.status
          if (status === 'recognized') {
            job.draft = data.draft
            job.status = 'done'
            ui.showToast('AI 识别完成，去核对导入')
            return
          }
          if (status === 'failed') {
            job.status = 'error'
            job.error = data?.import?.error_message || '识别失败，请重试或手动录入'
            ui.showToast(job.error, 'error')
            return
          }
        } catch (e: any) {
          // 网络抖動不立刻判死：记录还在服务端跑，下一轮再问。
          if (e?.response?.status === 404 || e?.response?.status === 403) {
            job.status = 'error'
            job.error = '识别记录不可访问（可能已删除或无权限）'
            return
          }
        }
      }
      job.status = 'error'
      job.error = '识别超时：AI 服务响应较慢，可在处方识别历史里重试'
      ui.showToast(job.error, 'error')
    } finally {
      polling.delete(job.id)
    }
  }

  // 上传并挂后台任务：recognize 立即返回 import_id，轮询由 store 负责。
  async function start(bookId: number, kind: 'box' | 'receipt', fd: FormData, mode: 'medicine' | 'stock' = 'medicine'): Promise<ScanJob> {
    const res = await prescriptionApi.recognize(bookId, fd)
    if (res.data?.code !== 0) throw new Error(res.data?.message || '识别失败')
    const id = Number(res.data.data?.import_id)
    if (!id) throw new Error('服务端未返回识别任务号')
    const job: ScanJob = { id, bookId, kind, mode, status: 'running', draft: null, error: '', startedAt: Date.now() }
    jobs.value.push(job)
    void poll(job)
    return job
  }

  // 进入用药本 / 刷新页面后，把服务端还在「识别中」的任务接回来继续轮询，
  // 关 App 再开也不丢（服务端重启还会收尾中断的记录）。
  async function resume(bookId: number) {
    if (!bookId) return
    try {
      const res = await prescriptionApi.list(bookId)
      const rows = res.data?.data || []
      for (const row of rows) {
        if (row.status !== 'recognizing') continue
        // 只接拍照识别（药盒 / 缴费单）的任务；处方笺上传走处方历史页自己的流程。
        if (row.kind !== 'box' && row.kind !== 'receipt') continue
        if (jobs.value.some((j) => j.id === row.id)) continue
        // 恢复的任务不知道原来从哪个页面发起，默认按药品页（只建档）处理，用户仍可在弹层里改。
        const job: ScanJob = { id: row.id, bookId, kind: row.kind === 'receipt' ? 'receipt' : 'box', mode: 'medicine', status: 'running', draft: null, error: '', startedAt: Date.now() }
        jobs.value.push(job)
        void poll(job)
      }
    } catch { /* 读历史失败不打扰，下次进本再试 */ }
  }

  function clear(id: number) {
    jobs.value = jobs.value.filter((j) => j.id !== id)
  }

  return { jobs, sheetOpen, requestOpen, runningJobs, jobsFor, latestFor, pillJob, start, resume, clear }
})
