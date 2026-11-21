// 统一的错误处理口径：后端消息优先，其次按网络/超时/状态码给出中文提示。
// 用户该知道的失败走 reportError（轻提示 + 控制台记录）；后台轮询这类不打扰
// 用户的失败走 logOnly（只留日志）。两者都不再裸吞异常。
import { useUIStore } from '../stores/ui'

export function errMessage(e: any, fallback: string): string {
  const msg = e?.response?.data?.message
  if (typeof msg === 'string' && msg.trim()) return msg
  if (e?.code === 'ECONNABORTED' || String(e?.message || '').includes('timeout')) return '请求超时，请检查网络后重试'
  if (!e?.response) return '网络不可用，请检查连接后重试'
  const status = Number(e.response.status)
  if (status === 403) return '没有权限执行该操作'
  if (status === 404) return '数据不存在或已被删除'
  if (status >= 500) return '服务器出错了，请稍后重试'
  return fallback
}

export function reportError(e: unknown, fallback: string): void {
  console.warn('[用药管理] ' + fallback, e)
  try {
    useUIStore().showToast(errMessage(e, fallback), 'error')
  } catch {
    // 极早期启动失败时 pinia 可能还没就绪，此时至少保留上面的控制台记录
  }
}

export function logOnly(scope: string, e: unknown): void {
  console.warn('[用药管理] ' + scope + '（后台任务，不打扰用户）', e)
}
