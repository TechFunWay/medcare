import { defineStore } from 'pinia'
import { ref } from 'vue'
import { checkUpdate, donateSupport, getVersionInfo, type UpdateInfo } from '../api/account'
import { preferenceApi } from '../api/medcare'

// 赞赏支持（与家族应用一致）：
// - 仅管理员提示；普通用户不弹窗、不显示横幅。
// - 每次应用启动后管理员首次进入，1.5 秒后弹一次；点「暂不支持」或关闭后，
//   同版本、同一次运行内不再弹，应用重启后再次提示。
// - 「已支持」状态记在服务端账号上（user_preferences.donate_supported_version，
//   2026-11-16 由本地存储改到后端）：同账号手机点过，电脑 / 飞牛入口不再提示；
//   应用升级到新版本后会重新弹窗并显示横幅。localStorage 只作离线缓存兜底。
// - 横幅常驻，关闭不持久化，直到支持过当前版本。
const KEY_SUPPORTED_VERSION = 'medcare.donate_supported_version'
const KEY_DISMISSED_START = 'medcare.donate_dismissed_start'
const KEY_DISMISSED_VERSION = 'medcare.donate_dismissed_version'

function lsGet(key: string): string | null { try { return localStorage.getItem(key) } catch { return null } }
function lsSet(key: string, value: string) { try { localStorage.setItem(key, value) } catch { /* ignore */ } }

export const useSupportStore = defineStore('support', () => {
  const show = ref(false)
  const sending = ref(false)
  const errorText = ref('')
  const bannerVisible = ref(false)
  const appVersion = ref('')
  const serverStartedAt = ref('')
  const supportedVersion = ref(lsGet(KEY_SUPPORTED_VERSION) || '')
  const donateSupported = ref(false)
  const updateInfo = ref<UpdateInfo | null>(null)

  function isSupported(): boolean {
    return !!appVersion.value && supportedVersion.value === appVersion.value
  }
  function refreshSupported() { donateSupported.value = isSupported() }

  const initialized = ref(false)

  async function init(isAdmin: boolean) {
    if (!isAdmin || initialized.value) return
    initialized.value = true
    try {
      const res = await getVersionInfo()
      if (res.data?.code === 0 && res.data.data) {
        appVersion.value = res.data.data.version || ''
        serverStartedAt.value = res.data.data.startedAt || ''
      }
    } catch { /* ignore */ }
    // 服务端账号级状态优先：手机点过「已支持」，这里读到就不再弹（跨设备共享）。
    try {
      const pr = await preferenceApi.get()
      const serverVersion = pr.data?.data?.donate_supported_version || ''
      if (serverVersion) {
        supportedVersion.value = serverVersion
        lsSet(KEY_SUPPORTED_VERSION, serverVersion)
      }
    } catch { /* 读取失败退回本地缓存 */ }
    refreshSupported()
    if (!donateSupported.value) {
      setTimeout(scheduleModal, 1500)
      maybeShowBanner()
    }
    maybeCheckUpdate()
  }

  function scheduleModal() {
    if (donateSupported.value) return
    const dismissedStart = lsGet(KEY_DISMISSED_START)
    const dismissedVersion = lsGet(KEY_DISMISSED_VERSION)
    const sameRun = !!serverStartedAt.value && dismissedStart === serverStartedAt.value
    const sameVersion = !!appVersion.value && dismissedVersion === appVersion.value
    if (sameRun && sameVersion) return
    show.value = true
  }

  function maybeShowBanner() {
    if (donateSupported.value) return
    bannerVisible.value = true
  }
  function dismissBanner() { bannerVisible.value = false }

  function dismiss() {
    show.value = false
    if (serverStartedAt.value) lsSet(KEY_DISMISSED_START, serverStartedAt.value)
    if (appVersion.value) lsSet(KEY_DISMISSED_VERSION, appVersion.value)
  }

  async function markSupported(amount: number): Promise<boolean> {
    sending.value = true
    errorText.value = ''
    try {
      const res = await donateSupport(amount)
      if (res.data?.code !== 0) throw new Error(res.data?.message || '上报失败')
      const ver = res.data?.data?.supported_version || appVersion.value
      supportedVersion.value = ver
      lsSet(KEY_SUPPORTED_VERSION, ver)
      refreshSupported()
      show.value = false
      bannerVisible.value = false
      return true
    } catch (e: any) {
      errorText.value = e?.response?.data?.message || e?.message || '上报失败，请稍后再试'
      return false
    } finally {
      sending.value = false
    }
  }

  async function maybeCheckUpdate() {
    try {
      const res = await checkUpdate()
      if (res.data?.code === 0 && res.data.data?.has_update) updateInfo.value = res.data.data
    } catch { /* ignore */ }
  }

  return { show, sending, errorText, bannerVisible, appVersion, donateSupported, updateInfo, initialized, init, dismiss, dismissBanner, markSupported }
})
