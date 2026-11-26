import { defineStore } from 'pinia'
import { ref } from 'vue'
import { checkUpdate, donateSupport, getVersionInfo, type UpdateInfo } from '../api/account'
import { preferenceApi } from '../api/medcare'

// 赞赏支持：
// - 仅管理员提示；普通用户不显示横幅。
// - 未支持过的用户，每次升级后首次打开自动弹一次；同一版本只弹一次。
// - 暂不支持、关闭或手动打开后关闭，只记当前版本已提示，升级后仍会再弹。
// - 只有点击过【已支持】的用户才永久不再自动弹。
// - 「已支持」状态按版本号记忆（服务端账号状态优先，localStorage 兜底），用于控制横幅展示。
const KEY_SUPPORTED_VERSION = 'medcare.donate_supported_version'
const KEY_DISMISSED_START = 'medcare.donate_dismissed_start'
const KEY_DISMISSED_VERSION = 'medcare.donate_dismissed_version'
const KEY_LEGACY_PROMPT_DISMISSED = 'medcare.donate_prompt_dismissed'
const KEY_PERMANENT_SUPPRESSED = 'medcare.donate_permanent_suppressed'
const KEY_AUTO_PROMPT_VERSION = 'medcare.donate_auto_prompt_version'

function lsGet(key: string): string | null { try { return localStorage.getItem(key) } catch { return null } }
function lsSet(key: string, value: string) { try { localStorage.setItem(key, value) } catch { /* ignore */ } }
function lsRemove(key: string) { try { localStorage.removeItem(key) } catch { /* ignore */ } }

export const useSupportStore = defineStore('support', () => {
  const show = ref(false)
  const sending = ref(false)
  const errorText = ref('')
  const bannerVisible = ref(false)
  const appVersion = ref('')
  const supportedVersion = ref(lsGet(KEY_SUPPORTED_VERSION) || '')
  const donateSupported = ref(false)
  const permanentSuppressed = ref(lsGet(KEY_PERMANENT_SUPPRESSED) === '1')
  const autoPromptVersion = ref(lsGet(KEY_AUTO_PROMPT_VERSION) || '')
  const updateInfo = ref<UpdateInfo | null>(null)

  /** 只有已支持意图/成功才写永久标记。 */
  function markPermanentSuppressed() {
    lsSet(KEY_PERMANENT_SUPPRESSED, '1')
    permanentSuppressed.value = true
  }

  /** 当前版本已提示过；升级后版本变化会允许再次自动提示。 */
  function markVersionPrompted() {
    if (!appVersion.value) return
    autoPromptVersion.value = appVersion.value
    lsSet(KEY_AUTO_PROMPT_VERSION, appVersion.value)
  }

  function isSupported(): boolean {
    return !!appVersion.value && supportedVersion.value === appVersion.value
  }
  function refreshSupported() { donateSupported.value = isSupported() }

  /** 每个版本最多自动弹一次；自动弹只写版本标记，绝不写永久标记。 */
  function scheduleModal() {
    if (!appVersion.value || permanentSuppressed.value || donateSupported.value) return
    if (autoPromptVersion.value === appVersion.value) return
    markVersionPrompted()
    show.value = true
  }

  const initialized = ref(false)

  async function init(isAdmin: boolean) {
    if (!isAdmin || initialized.value) return
    initialized.value = true
    try {
      const res = await getVersionInfo()
      if (res.data?.code === 0 && res.data.data) {
        appVersion.value = res.data.data.version || ''
      }
    } catch { /* ignore */ }
    // 服务端账号级状态优先：手机点过「已支持」，这里读到就不再弹（跨设备共享）。
    try {
      const pr = await preferenceApi.get()
      const serverVersion = pr.data?.data?.donate_supported_version || ''
      if (serverVersion) {
        supportedVersion.value = serverVersion
        lsSet(KEY_SUPPORTED_VERSION, serverVersion)
        markPermanentSuppressed()
      }
    } catch { /* 读取失败退回本地缓存 */ }
    migrateLegacyPromptState()
    refreshSupported()
    if (!donateSupported.value) {
      maybeShowBanner()
      // 未支持过的用户按版本自动弹；版本接口失败时不自动弹。
      if (appVersion.value && !permanentSuppressed.value) setTimeout(scheduleModal, 1500)
    }
    maybeCheckUpdate()
  }

  function maybeShowBanner() {
    if (donateSupported.value) return
    bannerVisible.value = true
  }
  /** 旧 supported_version 视为已支持；旧 dismissed 记录与含糊永久键不再永久抑制。 */
  function migrateLegacyPromptState() {
    if (supportedVersion.value) markPermanentSuppressed()
    lsRemove(KEY_LEGACY_PROMPT_DISMISSED)
    lsRemove(KEY_DISMISSED_START)
    lsRemove(KEY_DISMISSED_VERSION)
    permanentSuppressed.value = lsGet(KEY_PERMANENT_SUPPRESSED) === '1' || !!supportedVersion.value
  }

  function dismissBanner() {
    bannerVisible.value = false
    markVersionPrompted()
  }

  function dismiss() {
    show.value = false
    markVersionPrompted()
  }

  /** 横幅入口：手动打开只记当前版本已提示，不永久抑制。 */
  function open() {
    markVersionPrompted()
    show.value = true
  }

  async function markSupported(amount: number): Promise<boolean> {
    markPermanentSuppressed()
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

  return { show, sending, errorText, bannerVisible, appVersion, donateSupported, updateInfo, initialized, permanentSuppressed, autoPromptVersion, markPermanentSuppressed, init, open, dismiss, dismissBanner, markSupported }
})
