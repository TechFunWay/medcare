import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { checkAuth, checkSetupRequired, getPublicConfigs, login as apiLogin, logout as apiLogout, register as apiRegister } from '../api/auth'
import { logOnly } from '../utils/errors'

export const useAuthStore = defineStore('auth', () => {
  const token = ref(localStorage.getItem('medcare.token') || '')
  const user = ref<any>(null)
  const setupRequired = ref(false)
  const requireLogin = ref(true)
  const allowRegister = ref(true)
  const initialized = ref(false)
  let initPromise: Promise<void> | null = null

  const isAuthenticated = computed(() => !!token.value && !!user.value)
  const isAdmin = computed(() => user.value?.role === 'admin')

  function clear() {
    token.value = ''
    user.value = null
    localStorage.removeItem('medcare.token')
  }

  async function init() {
    if (initialized.value) return
    if (initPromise) return initPromise
    initPromise = (async () => {
      try {
        const [cfg, setup] = await Promise.all([getPublicConfigs(), checkSetupRequired()])
        if (cfg.data?.code === 0) {
          requireLogin.value = cfg.data.data?.require_login !== 'false'
          allowRegister.value = cfg.data.data?.allow_register !== 'false'
        }
        if (setup.data?.code === 0) setupRequired.value = !!setup.data.data?.setup_required
      } catch (e) { logOnly('读取公开配置', e) }
      if (token.value) {
        try {
          const res = await checkAuth()
          if (res.data?.code === 0 && res.data.data?.authenticated) user.value = res.data.data.user
          else clear()
        } catch { clear() }
      }
      initialized.value = true
    })()
    return initPromise
  }

  async function login(username: string, password: string) {
    const res = await apiLogin(username, password)
    if (res.data?.code !== 0) throw new Error(res.data?.message || '登录失败')
    token.value = res.data.data.token
    localStorage.setItem('medcare.token', token.value)
    user.value = res.data.data.user
    // 已有账号了，首次初始化标记必须立刻清掉，否则路由守卫会把登录后的跳转再拉回注册页。
    setupRequired.value = false
    initialized.value = true
    return user.value
  }

  async function register(username: string, password: string) {
    const res = await apiRegister(username, password)
    if (res.data?.code !== 0) throw new Error(res.data?.message || '注册失败')
    token.value = res.data.data.token
    localStorage.setItem('medcare.token', token.value)
    user.value = res.data.data.user
    // 注册成功即意味着库里已经有账号，清掉首次初始化标记，路由才会放行到 /today。
    setupRequired.value = false
    initialized.value = true
    return user.value
  }

  async function logout() {
    // 退出接口失败也要清本地会话，失败只记日志
    try { await apiLogout() } catch (e) { logOnly('退出登录接口', e) }
    clear()
  }

  function setUser(value: any) { user.value = value }

  function setToken(value: string) {
    token.value = value
    if (value) localStorage.setItem('medcare.token', value)
    else localStorage.removeItem('medcare.token')
  }

  return { token, user, setupRequired, requireLogin, allowRegister, initialized, isAuthenticated, isAdmin, init, login, register, logout, clear, setUser, setToken }
})
