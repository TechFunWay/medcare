import { defineStore } from 'pinia'
import { ref } from 'vue'
import { notificationApi } from '../api/medcare'
import { logOnly } from '../utils/errors'

// 站内通知未读数：顶栏红点与通知中心共用同一份状态。
// 标记已读 / 全部已读后立刻本地扣减（再异步校准），不用等 30 秒轮询或手动刷新。
export const useNotifyStore = defineStore('notify', () => {
  const unread = ref(0)

  async function refresh() {
    try {
      const res = await notificationApi.unread()
      if (res.data?.code === 0) unread.value = Number(res.data.data?.count) || 0
    } catch (e) {
      logOnly('读取未读数', e)
    }
  }

  // markRead：单条已读后本地减一，并回服务端校准，避免并发/重复点击导致偏差。
  function markRead() {
    unread.value = Math.max(0, unread.value - 1)
    void refresh()
  }

  // markAllRead：全部已读后直接归零，再校准。
  function markAllRead() {
    unread.value = 0
    void refresh()
  }

  return { unread, refresh, markRead, markAllRead }
})
