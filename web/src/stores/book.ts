import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { bookApi, preferenceApi, type Book } from '../api/medcare'
import { fallbackKeysFor, normalizeKeys } from '../utils/permissions'

export const useBookStore = defineStore('book', () => {
  const books = ref<Book[]>([])
  const currentId = ref<number | null>(Number(localStorage.getItem('medcare.current_book') || 0) || null)
  const defaultId = ref<number | null>(Number(localStorage.getItem('medcare.default_book') || 0) || null)
  const loading = ref(false)

  const current = computed(() => books.value.find((b) => b.id === currentId.value) || null)
  const isOwner = computed(() => current.value?.role === 'owner')
  const isManager = computed(() => current.value?.role === 'owner' || current.value?.role === 'manager')
  // 用药人本人即使角色是「成员」，默认也能记录自己的用药执行；管理员可以单独收回。
  const isCareRecipient = computed(() => !!current.value?.is_care_recipient)

  /**
   * 当前用户在这个用药本里最终生效的权限格（模块:动作）。
   * 后端按「管理员逐格勾选优先，否则角色默认」算好后下发（与 requirePermission 同源），
   * 所以这里只做界面判定，真正的拦截永远在后端。
   */
  const permissionKeys = computed<string[]>(() => {
    const book = current.value
    if (!book) return []
    if (Array.isArray(book.permissions)) return normalizeKeys(book.permissions)
    return fallbackKeysFor(book.role, book.is_care_recipient)
  })

  /** 勾了才有：一格一个键，例如 medicine:create、today:record */
  function can(key: string): boolean {
    return permissionKeys.value.includes(key)
  }

  /** 某个模块能不能打开来看（查看也是一格权限） */
  function canView(module: string): boolean {
    return can(module + ':view')
  }

  /** 某个具体用药本里有没有某一格（管理弹层可能在看不一样的本） */
  function canIn(id: number | null | undefined, key: string): boolean {
    const target = books.value.find((b) => b.id === id)
    if (!target) return false
    const keys = Array.isArray(target.permissions) ? normalizeKeys(target.permissions) : fallbackKeysFor(target.role, target.is_care_recipient)
    return keys.includes(key)
  }

  const canRemind = computed(() => can('today:remind'))
  const canRecord = computed(() => can('today:record'))
  const canMedicineView = computed(() => canView('medicine'))
  const canMedicineCreate = computed(() => can('medicine:create'))
  const canMedicineEdit = computed(() => can('medicine:edit'))
  const canMedicineDelete = computed(() => can('medicine:delete'))
  const canPlanView = computed(() => canView('plan'))
  const canPlanCreate = computed(() => can('plan:create'))
  const canPlanEdit = computed(() => can('plan:edit'))
  const canPlanDelete = computed(() => can('plan:delete'))
  const canPrescView = computed(() => canView('prescription'))
  const canPrescCreate = computed(() => can('prescription:create'))
  const canPrescConfirm = computed(() => can('prescription:edit'))
  const canPrescDelete = computed(() => can('prescription:delete'))
  const canImageView = computed(() => canView('image'))
  const canImageUpload = computed(() => can('image:create'))
  const canImageEdit = computed(() => can('image:edit'))
  const canImageDelete = computed(() => can('image:delete'))
  const canStatsView = computed(() => canView('stats'))
  const canMemberView = computed(() => canView('member'))
  const canInvite = computed(() => can('member:invite'))
  const canWriteAnything = computed(() => permissionKeys.value.some((key) => key.endsWith(':create') || key.endsWith(':edit') || key.endsWith(':record') || key === 'today:remind'))

  function select(id: number | null) {
    currentId.value = id
    if (id) localStorage.setItem('medcare.current_book', String(id))
    else localStorage.removeItem('medcare.current_book')
  }

  async function fetchBooks() {
    loading.value = true
    try {
      const res = await bookApi.list()
      if (res.data?.code === 0) {
        books.value = res.data.data || []
        if (!books.value.find((b) => b.id === currentId.value)) {
          // 优先用设置里的默认药本，其次第一个。
          const preferred = books.value.find((b) => b.id === defaultId.value)
          select(preferred?.id ?? books.value[0]?.id ?? null)
        }
        if (currentId.value) localStorage.setItem('medcare.current_book', String(currentId.value))
      }
    } finally { loading.value = false }
  }

  async function create(data: any) {
    const res = await bookApi.create(data)
    if (res.data?.code !== 0) throw new Error(res.data?.message || '创建失败')
    await fetchBooks()
    select(res.data.data.id)
    return res.data.data as Book
  }

  // 设定「我的默认药本」，同时写本地与后端偏好，换设备也能记住。
  async function setDefault(id: number | null) {
    defaultId.value = id
    if (id) localStorage.setItem('medcare.default_book', String(id))
    else localStorage.removeItem('medcare.default_book')
    try { await preferenceApi.update({ last_book_id: id }) } catch { /* 本地已保存，忽略远端失败 */ }
  }

  // 登录后按后端偏好恢复默认药本（本地没有显式当前选择时）。
  function applyDefault(prefLastBookId?: number | null) {
    if (prefLastBookId) {
      defaultId.value = prefLastBookId
      localStorage.setItem('medcare.default_book', String(prefLastBookId))
    }
    if (!currentId.value && defaultId.value && books.value.find((b) => b.id === defaultId.value)) select(defaultId.value)
  }

  function clear() { books.value = []; select(null) }

  return { books, currentId, defaultId, current, loading, isOwner, isManager, isCareRecipient, permissionKeys, can, canIn, canView, canRemind, canRecord, canMedicineView, canMedicineCreate, canMedicineEdit, canMedicineDelete, canPlanView, canPlanCreate, canPlanEdit, canPlanDelete, canPrescView, canPrescCreate, canPrescConfirm, canPrescDelete, canImageView, canImageUpload, canImageEdit, canImageDelete, canStatsView, canMemberView, canInvite, select, setDefault, applyDefault, fetchBooks, create, clear }
})
