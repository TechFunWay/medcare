import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { preferenceApi } from '../api/medcare'
import { reportError } from '../utils/errors'

export type ThemeMode = 'system' | 'light' | 'dark'
export type ThemeBrand = 'a' | 'b' | 'c' | 'd' | 'e' | 'f' | 'g' | 'h' | 'i' | 'j'

const BRANDS: ThemeBrand[] = ['a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j']

const MODE_KEY = 'medcare.theme_mode'
const BRAND_KEY = 'medcare.theme_brand'

export const THEME_BRANDS: { value: ThemeBrand; name: string; color: string }[] = [
  { value: 'a', name: '温润药本', color: '#5b7f6b' },
  { value: 'b', name: '清风医疗', color: '#2f7fd8' },
  { value: 'c', name: '深夜护航', color: '#3f9a74' },
  { value: 'd', name: '柔光珊瑚', color: '#e2705f' },
  { value: 'e', name: '素简黑白', color: '#2b2f36' },
  { value: 'f', name: '海盐青柠', color: '#14a58c' },
  { value: 'g', name: '紫丁护理', color: '#7b5cd6' },
  { value: 'h', name: '暖咖玫瑰', color: '#c07a5a' },
  { value: 'i', name: '琥珀晨光', color: '#d98b12' },
  { value: 'j', name: '樱粉心意', color: '#d6336c' },
]

function readValid<T extends string>(key: string, allowed: T[], fallback: T): T {
  const raw = (localStorage.getItem(key) || '') as T
  return allowed.includes(raw) ? raw : fallback
}

export const useThemeStore = defineStore('theme', () => {
  const mode = ref<ThemeMode>(readValid(MODE_KEY, ['system', 'light', 'dark'], 'system'))
  const brand = ref<ThemeBrand>(readValid(BRAND_KEY, BRANDS, 'a'))
  const systemDark = ref(false)
  let media: MediaQueryList | null = null

  const isDark = computed(() => (mode.value === 'dark' ? true : mode.value === 'light' ? false : systemDark.value))
  const modeLabel = computed(() => (mode.value === 'system' ? '跟随系统' : mode.value === 'dark' ? '暗色' : '亮色'))

  function apply() {
    const root = document.documentElement
    root.classList.toggle('dark', isDark.value)
    for (const b of BRANDS) root.classList.remove('brand-' + b)
    root.classList.add('brand-' + brand.value)
    root.style.colorScheme = isDark.value ? 'dark' : 'light'
    const meta = document.querySelector('meta[name="theme-color"]')
    // 浏览器地址栏 / 状态栏颜色跟随当前配色的背景令牌，不再写死配色 A 的两个色值。
    if (meta) meta.setAttribute('content', getComputedStyle(root).getPropertyValue('--mc-bg').trim() || (isDark.value ? '#121714' : '#f7f4ee'))
  }

  function setMode(value: ThemeMode) {
    mode.value = value
    localStorage.setItem(MODE_KEY, value)
    apply()
  }

  function setBrand(value: ThemeBrand) {
    brand.value = value
    localStorage.setItem(BRAND_KEY, value)
    apply()
  }

  function cycleMode() {
    const order: ThemeMode[] = ['system', 'light', 'dark']
    setMode(order[(order.indexOf(mode.value) + 1) % order.length])
  }

  function init() {
    if (media) return
    media = window.matchMedia('(prefers-color-scheme: dark)')
    systemDark.value = media.matches
    media.addEventListener('change', (e) => {
      systemDark.value = e.matches
      if (mode.value === 'system') apply()
    })
    apply()
  }

  function hydrate(nextMode?: string, nextBrand?: string) {
    let changed = false
    if (nextMode && ['system', 'light', 'dark'].includes(nextMode) && nextMode !== mode.value && !localStorage.getItem(MODE_KEY)) {
      mode.value = nextMode as ThemeMode
      changed = true
    }
    if (nextBrand && ['a', 'b', 'c', 'd', 'e'].includes(nextBrand) && nextBrand !== brand.value && !localStorage.getItem(BRAND_KEY)) {
      brand.value = nextBrand as ThemeBrand
      changed = true
    }
    if (changed) apply()
  }

  async function persist() {
    try {
      const res = await preferenceApi.get()
      const pref = res.data?.data
      if (!pref) return
      pref.theme_mode = mode.value
      pref.theme_brand = brand.value
      await preferenceApi.update(pref)
    } catch (e) { reportError(e, '主题偏好保存失败') }
  }

  return { mode, brand, isDark, modeLabel, init, apply, setMode, setBrand, cycleMode, hydrate, persist, THEME_BRANDS }
})
