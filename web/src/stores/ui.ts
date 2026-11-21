import { ref } from 'vue'
import { defineStore } from 'pinia'

export interface ToastState { message: string; type: 'success' | 'error' | 'info' }
export interface ConfirmState {
  title: string
  message: string
  confirmText: string
  danger: boolean
  // 只提示、不给「取消」时置 true（例如「不能删除」这类纯告知弹窗）。
  hideCancel?: boolean
  resolve: (ok: boolean) => void
}

export const useUIStore = defineStore('ui', () => {
  const toast = ref<ToastState | null>(null)
  const confirmState = ref<ConfirmState | null>(null)
  let timer: number | null = null

  function showToast(message: string, type: ToastState['type'] = 'success', durationMs = 3000) {
    toast.value = { message, type }
    if (timer) window.clearTimeout(timer)
    timer = window.setTimeout(() => { toast.value = null }, durationMs)
  }

  function confirm(options: { title: string; message: string; confirmText?: string; danger?: boolean; hideCancel?: boolean }): Promise<boolean> {
    return new Promise((resolve) => {
      confirmState.value = {
        title: options.title,
        message: options.message,
        confirmText: options.confirmText || '确认',
        danger: options.danger !== false,
        hideCancel: options.hideCancel === true,
        resolve,
      }
    })
  }

  function resolveConfirm(ok: boolean) {
    confirmState.value?.resolve(ok)
    confirmState.value = null
  }

  return { toast, confirmState, showToast, confirm, resolveConfirm }
})
