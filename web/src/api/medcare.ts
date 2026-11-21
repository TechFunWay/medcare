import request from './request'

export interface Book {
  id: number
  name: string
  care_recipient_name: string
  relation?: string
  color?: string
  timezone: string
  status: string
  role: 'owner' | 'manager' | 'member'
  is_care_recipient?: boolean
  /** 当前用户在本用药本里生效的能力，后端 requirePermission 同源下发 */
  permissions?: string[]
  care_recipient_user_id?: number
  owner_user_id: number
  owner_name?: string
  member_count?: number
  notes?: string
  created_by_name?: string
  updated_by_name?: string
  created_at?: string
}

export interface Member {
  user_id: number
  username: string
  role: string
  is_care_recipient: boolean
  /** 该成员最终生效的能力 */
  permissions?: string[]
  /** true＝管理员单独勾选过；false＝跟随角色默认 */
  custom_permissions?: boolean
  joined_at: string
  is_self: boolean
  notifications_enabled?: boolean
}

export interface PermissionCatalog {
  /** 模块 × 动作矩阵，后端是权威清单 */
  modules: { module: string; label: string; hint: string; actions: { key: string; action: string; label: string }[] }[]
  keys: string[]
  defaults: Record<string, string[]>
  mine: string[]
  mine_custom: boolean
  can_manage_members: boolean
}

export interface Medicine {
  id: number
  book_id: number
  name: string
  generic_name?: string
  aliases?: string[]
  category: string
  form?: string
  spec?: string
  unit: string
  package_spec?: string
  manufacturer?: string
  cover_image_id?: number
  notes?: string
  stock_quantity: number
  low_stock_threshold: number
  low_stock_notify_enabled?: boolean
  expiry_warn_days: number
  unit_price?: number
  stock_status: string
  nearest_expiry?: string
  active_plan_count: number
  version: number
  created_by_name?: string
  updated_by_name?: string
  created_at?: string
  updated_at?: string
}

export interface Location {
  id: number
  book_id: number
  name: string
  type: string
  address: string
  contact_name?: string
  contact_phone?: string
  notes?: string
}

export interface Plan {
  id: number
  book_id: number
  medicine_id: number
  medicine_name: string
  location_id?: number
  location_name?: string
  address?: string
  executor_type?: string
  executor_name?: string
  drip_rate?: number
  duration_minutes?: number
  title: string
  type: string
  dose_amount: number
  dose_unit: string
  frequency: { type: string; times?: string[]; interval_days?: number; weekdays?: number[]; interval_hours?: number; start_at?: string; custom_schedule?: { date: string; times: string[] }[] }
  start_date: string
  end_date?: string
  timezone: string
  status: string
  doctor?: string
  hospital?: string
  notes?: string
  notifications_enabled?: boolean
  next_event_at?: string
  completed_count: number
  total_count: number
  today_total: number
  today_done: number
  created_by_name?: string
  updated_by_name?: string
  created_at?: string
  updated_at?: string
  version: number
}

export interface DoseEvent {
  id: number
  book_id: number
  plan_id: number
  medicine_id: number
  medicine_name: string
  plan_title?: string
  type: string
  dose_amount: number
  dose_unit: string
  location_id?: number
  location_name?: string
  address?: string
  executor_type?: string
  executor_name?: string
  drip_rate?: number
  duration_minutes?: number
  actual_start_at?: string
  actual_end_at?: string
  scheduled_at: string
  local_date: string
  local_time: string
  status: string
  overdue_minutes: number
  completed_at?: string
  skipped_at?: string
  missed_at?: string
  skip_reason?: string
  note?: string
  snooze_until?: string
  was_missed: boolean
  stock_shortage: boolean
  version: number
}

export interface Dashboard {
  date: string
  book: Book
  summary: Record<string, number>
  overdue: DoseEvent[]
  groups: { key: string; label: string; events: DoseEvent[] }[]
  next_event?: DoseEvent
  active_plan_count?: number
  next_upcoming?: DoseEvent
  server_time: string
}

export interface PrescriptionDraftItem {
  raw_name?: string
  name: string
  generic_name?: string
  spec?: string
  form?: string
  route?: string
  dose_amount: number
  dose_unit: string
  frequency_text?: string
  frequency_type?: string
  times?: string[]
  interval_days?: number
  weekdays?: number[]
  interval_hours?: number
  duration_days?: number
  quantity?: number
  instructions?: string
  confidence?: number
  needs_review?: boolean
  unit_price?: number
  amount?: number
  createPlan?: boolean
  timesText?: string
  medicineMode?: string
  category?: string
  unit?: string
  package_spec?: string
  manufacturer?: string
  // 入库批次时间（仅入库核对时使用）：日期 YYYY-MM-DD + 时间 HH:MM:SS。
  receivedDate?: string
  receivedTime?: string
  // 指定日期用药（frequency_type=custom_dates）
  customDates?: string[]
  custom_schedule?: { date: string; times: string[] }[]
  approval_no?: string
  expiry_date?: string
  low_stock_threshold?: number
  expiry_warn_days?: number
  // 前端本地校验标记：缺药品名称 / 缺单位时标红定位（不参与提交）。
  errName?: boolean
  errUnit?: boolean
}

export interface PrescriptionDraft {
  doc_type?: 'prescription' | 'receipt' | 'other' | 'box'
  kind?: string
  patient_name?: string
  hospital?: string
  department?: string
  doctor?: string
  prescription_date?: string
  diagnosis?: string
  notes?: string
  items: PrescriptionDraftItem[]
}

export const bookApi = {
  list: () => request.get('/api/medcare/books'),
  create: (data: any) => request.post('/api/medcare/books', data),
  get: (id: number) => request.get(`/api/medcare/books/${id}`),
  update: (id: number, data: any) => request.put(`/api/medcare/books/${id}`, data),
  archive: (id: number) => request.post(`/api/medcare/books/${id}/archive`),
  members: (id: number) => request.get(`/api/medcare/books/${id}/members`),
  transfer: (id: number, userId: number) => request.post(`/api/medcare/books/${id}/transfer`, { user_id: userId }),
  updateMember: (id: number, userId: number, role: string) => request.put(`/api/medcare/books/${id}/members/${userId}`, { role }),
  /** 能力清单与各角色默认（管理界面渲染用） */
  permissions: (id: number) => request.get<never, { data?: { code: number; data: PermissionCatalog } }>(`/api/medcare/books/${id}/permissions`),
  /** 逐人指定能力：数组＝单独勾选，null＝恢复跟随角色 */
  updatePermissions: (id: number, userId: number, permissions: string[] | null) => request.put(`/api/medcare/books/${id}/members/${userId}/permissions`, { permissions }),
  setCareRecipient: (id: number, userId: number) => request.put(`/api/medcare/books/${id}/members/care-recipient`, { user_id: userId }),
  removeMember: (id: number, userId: number) => request.delete(`/api/medcare/books/${id}/members/${userId}`),
  myNotifications: (id: number, enabled: boolean) => request.put(`/api/medcare/books/${id}/members/me/notifications`, { enabled }),
  invites: (id: number) => request.get(`/api/medcare/books/${id}/invites`),
  createInvite: (id: number, data: any) => request.post(`/api/medcare/books/${id}/invites`, data),
  revokeInvite: (id: number, inviteId: number) => request.delete(`/api/medcare/books/${id}/invites/${inviteId}`),
}

export const inviteApi = {
  preview: (token: string) => request.get(`/api/medcare/invites/${token}`),
  join: (token: string) => request.post(`/api/medcare/invites/${token}/join`),
  joinByCode: (code: string) => request.post('/api/medcare/invites/join-by-code', { code }),
}

export const medicineApi = {
  list: (bookId: number, params: any = {}) => request.get(`/api/medcare/books/${bookId}/medicines`, { params }),
  stats: (bookId: number) => request.get(`/api/medcare/books/${bookId}/medicines-stats`),
  create: (bookId: number, data: any) => request.post(`/api/medcare/books/${bookId}/medicines`, data),
  get: (bookId: number, id: number) => request.get(`/api/medcare/books/${bookId}/medicines/${id}`),
  update: (bookId: number, id: number, data: any) => request.put(`/api/medcare/books/${bookId}/medicines/${id}`, data),
  archive: (bookId: number, id: number) => request.delete(`/api/medcare/books/${bookId}/medicines/${id}`),
  batches: (bookId: number, id: number) => request.get(`/api/medcare/books/${bookId}/medicines/${id}/batches`),
  stockIn: (bookId: number, id: number, data: any) => request.post(`/api/medcare/books/${bookId}/medicines/${id}/stock-in`, data),
  stockOut: (bookId: number, id: number, data: any) => request.post(`/api/medcare/books/${bookId}/medicines/${id}/stock-out`, data),
  setCover: (bookId: number, id: number, imageId: number) => request.put(`/api/medcare/books/${bookId}/medicines/${id}/cover`, { image_id: imageId }),
  relations: (bookId: number, id: number) => request.get(`/api/medcare/books/${bookId}/medicines/${id}/relations`),
  convertUnit: (bookId: number, id: number, data: { unit: string; factor: number }) => request.put(`/api/medcare/books/${bookId}/medicines/${id}/unit`, data),
  stockAdjust: (bookId: number, id: number, data: any) => request.post(`/api/medcare/books/${bookId}/medicines/${id}/stock-adjust`, data),
  stocktake: (bookId: number, id: number, data: any) => request.post(`/api/medcare/books/${bookId}/medicines/${id}/stocktake`, data),
  movements: (bookId: number, id: number, params: any = {}) => request.get(`/api/medcare/books/${bookId}/medicines/${id}/stock-movements`, { params }),
  alerts: (bookId: number) => request.get(`/api/medcare/books/${bookId}/stock-alerts`),
  importBatch: (bookId: number, data: any) => request.post(`/api/medcare/books/${bookId}/medicines/import`, data),
  transfer: (bookId: number, id: number, data: any) => request.post(`/api/medcare/books/${bookId}/medicines/${id}/transfer`, data),
  getBatch: (bookId: number, batchId: number) => request.get(`/api/medcare/books/${bookId}/batches/${batchId}`),
  updateBatch: (bookId: number, batchId: number, data: any) => request.put(`/api/medcare/books/${bookId}/batches/${batchId}`, data),
}

export interface StockMovementItem {
  id: number
  medicine_id: number
  batch_id?: number
  medicine_name: string
  unit: string
  type: string
  quantity_delta: number
  quantity_after: number
  reason: string
  channel: string
  entry_source: string
  actor_user_id?: number
  actor_name?: string
  event_id?: number
  // 关联用药事件：计划时间与实际点完成的时间。
  event_local_date?: string
  event_local_time?: string
  event_status?: string
  event_completed_at?: string
  created_at: string
}

export interface OperationLogItem {
  id: number
  actor_user_id: number
  actor_name: string
  action: string
  method: string
  target_type: string
  target_id: number
  detail: string
  ip: string
  created_at: string
}

export const operationApi = {
  logs: (bookId: number, params: any = {}) => request.get(`/api/medcare/books/${bookId}/operation-logs`, { params }),
}

export const stockApi = {
  movements: (bookId: number, params: any = {}) => request.get(`/api/medcare/books/${bookId}/stock-movements`, { params }),
  movementsStats: (bookId: number, params: any = {}) => request.get(`/api/medcare/books/${bookId}/stock-movements/stats`, { params }),
}

export const locationApi = {
  list: (bookId: number) => request.get(`/api/medcare/books/${bookId}/locations`),
  create: (bookId: number, data: any) => request.post(`/api/medcare/books/${bookId}/locations`, data),
  update: (bookId: number, id: number, data: any) => request.put(`/api/medcare/books/${bookId}/locations/${id}`, data),
  archive: (bookId: number, id: number) => request.delete(`/api/medcare/books/${bookId}/locations/${id}`),
}

export const planApi = {
  list: (bookId: number, params: any = {}) => request.get(`/api/medcare/books/${bookId}/plans`, { params }),
  create: (bookId: number, data: any) => request.post(`/api/medcare/books/${bookId}/plans`, data),
  get: (bookId: number, id: number) => request.get(`/api/medcare/books/${bookId}/plans/${id}`),
  update: (bookId: number, id: number, data: any) => request.put(`/api/medcare/books/${bookId}/plans/${id}`, data),
  remove: (bookId: number, id: number) => request.delete(`/api/medcare/books/${bookId}/plans/${id}`),
  pause: (bookId: number, id: number) => request.post(`/api/medcare/books/${bookId}/plans/${id}/pause`),
  resume: (bookId: number, id: number) => request.post(`/api/medcare/books/${bookId}/plans/${id}/resume`),
  end: (bookId: number, id: number) => request.post(`/api/medcare/books/${bookId}/plans/${id}/end`),
  transfer: (bookId: number, id: number, data: any) => request.post(`/api/medcare/books/${bookId}/plans/${id}/transfer`, data),
  preview: (bookId: number, id: number, days = 7) => request.get(`/api/medcare/books/${bookId}/plans/${id}/preview`, { params: { days } }),
}

export const eventApi = {
  dashboard: (bookId: number, date?: string) => request.get(`/api/medcare/books/${bookId}/dashboard`, { params: { date } }),
  list: (bookId: number, params: any = {}) => request.get(`/api/medcare/books/${bookId}/dose-events`, { params }),
  get: (bookId: number, id: number) => request.get(`/api/medcare/books/${bookId}/dose-events/${id}`),
  start: (bookId: number, id: number) => request.post(`/api/medcare/books/${bookId}/dose-events/${id}/start`),
  complete: (bookId: number, id: number, data: any) => request.post(`/api/medcare/books/${bookId}/dose-events/${id}/complete`, data),
  undo: (bookId: number, id: number) => request.post(`/api/medcare/books/${bookId}/dose-events/${id}/undo`),
  miss: (bookId: number, id: number, data: any = {}) => request.post(`/api/medcare/books/${bookId}/dose-events/${id}/miss`, data),
  adjust: (bookId: number, id: number, data: any) => request.post(`/api/medcare/books/${bookId}/dose-events/${id}/adjust`, data),
  snooze: (bookId: number, id: number, minutes: number) => request.post(`/api/medcare/books/${bookId}/dose-events/${id}/snooze`, { minutes }),
  remind: (bookId: number, id: number) => request.post(`/api/medcare/books/${bookId}/dose-events/${id}/remind`),
  location: (bookId: number, id: number, data: any) => request.post(`/api/medcare/books/${bookId}/dose-events/${id}/location`, data),
  note: (bookId: number, id: number, note: string) => request.post(`/api/medcare/books/${bookId}/dose-events/${id}/note`, { note }),
  calendar: (bookId: number, from: string, to: string) => request.get(`/api/medcare/books/${bookId}/calendar`, { params: { from, to } }),
}

export interface BookImage {
  id: number
  book_id: number
  title: string
  category: string
  medicine_id?: number
  tags: string[]
  note: string
  mime: string
  size: number
  created_by: number
  created_by_name?: string
  created_at: string
  updated_at: string
}

export const imageApi = {
  list: (bookId: number, params: any = {}) => request.get(`/api/medcare/books/${bookId}/images`, { params }),
  upload: (bookId: number, form: FormData, onUploadProgress?: (e: any) => void) => request.post(`/api/medcare/books/${bookId}/images`, form, { headers: { 'Content-Type': 'multipart/form-data' }, timeout: 180000, onUploadProgress }),
  update: (bookId: number, id: number, data: any) => request.put(`/api/medcare/books/${bookId}/images/${id}`, data),
  remove: (bookId: number, id: number) => request.delete(`/api/medcare/books/${bookId}/images/${id}`),
  file: (bookId: number, id: number, download = false) => request.get(`/api/medcare/books/${bookId}/images/${id}/file`, { params: download ? { download: 1 } : {}, responseType: 'blob', timeout: 120000 }),
  thumb: (bookId: number, id: number) => request.get(`/api/medcare/books/${bookId}/images/${id}/file`, { params: { thumb: 1 }, responseType: 'blob', timeout: 120000 }),
}

export const prescriptionApi = {
  recognize: (bookId: number, form: FormData) => request.post(`/api/medcare/books/${bookId}/prescriptions/recognize`, form, { headers: { 'Content-Type': 'multipart/form-data' }, timeout: 180000 }),
  list: (bookId: number) => request.get(`/api/medcare/books/${bookId}/prescriptions`),
  get: (bookId: number, id: number) => request.get(`/api/medcare/books/${bookId}/prescriptions/${id}`),
  confirm: (bookId: number, id: number, data: any) => request.post(`/api/medcare/books/${bookId}/prescriptions/${id}/confirm`, data),
  retry: (bookId: number, id: number) => request.post(`/api/medcare/books/${bookId}/prescriptions/${id}/retry`, undefined, { timeout: 180000 }),
  remove: (bookId: number, id: number) => request.delete(`/api/medcare/books/${bookId}/prescriptions/${id}`),
  imageUrl: (bookId: number, id: number) => `/api/medcare/books/${bookId}/prescriptions/${id}/image`,
}

export const statsApi = {
  overview: (bookId: number, params: any) => request.get(`/api/medcare/books/${bookId}/stats/overview`, { params }),
  trend: (bookId: number, params: any) => request.get(`/api/medcare/books/${bookId}/stats/trend`, { params }),
  heatmap: (bookId: number, params: any) => request.get(`/api/medcare/books/${bookId}/stats/heatmap`, { params }),
  breakdown: (bookId: number, params: any) => request.get(`/api/medcare/books/${bookId}/stats/breakdown`, { params }),
  stock: (bookId: number) => request.get(`/api/medcare/books/${bookId}/stats/stock`),
}

export const channelApi = {
  list: () => request.get('/api/medcare/channels'),
  create: (data: any) => request.post('/api/medcare/channels', data),
  update: (id: number, data: any) => request.put(`/api/medcare/channels/${id}`, data),
  remove: (id: number) => request.delete(`/api/medcare/channels/${id}`),
  test: (id: number) => request.post(`/api/medcare/channels/${id}/test`),
}

export const notificationApi = {
  list: (params: any = {}) => request.get('/api/medcare/notifications', { params }),
  unread: () => request.get('/api/medcare/notifications/unread-count'),
  read: (id: number) => request.post(`/api/medcare/notifications/${id}/read`),
  readAll: () => request.post('/api/medcare/notifications/read-all'),
  deliveries: (params: any = {}) => request.get('/api/medcare/deliveries', { params }),
  retry: (id: number) => request.post(`/api/medcare/deliveries/${id}/retry`),
}

export const preferenceApi = {
  get: () => request.get('/api/medcare/preferences'),
  update: (data: any) => request.put('/api/medcare/preferences', data),
}

export const adminApi = {
  getSystemConfigs: () => request.get('/api/configs/system'),
  saveConfig: (key: string, value: string) => request.put('/api/configs', { key, value }),
  getSMTP: () => request.get('/api/medcare/admin/smtp'),
  saveSMTP: (data: any) => request.put('/api/medcare/admin/smtp', data),
  testSMTP: (to: string) => request.post('/api/medcare/admin/smtp/test', { to }),
  getAI: () => request.get('/api/medcare/admin/ai'),
  saveAI: (data: any) => request.put('/api/medcare/admin/ai', data),
  getOCR: () => request.get('/api/medcare/admin/ocr'),
  saveOCR: (data: any) => request.put('/api/medcare/admin/ocr', data),
  testOCR: (data: any = {}) => request.post('/api/medcare/admin/ocr/test', data),
  testAI: (data: any = {}) => request.post('/api/medcare/admin/ai/test', data),
  listAIModels: (data: any = {}) => request.post('/api/medcare/admin/ai/models', data),
}

// 备份管理走框架的 /api/backups（仅管理员）；上传恢复是实例层新增的
// POST /api/backups/upload，上传后仍走列表里既有的「恢复」流程。
export const backupApi = {
  list: () => request.get('/api/backups'),
  create: () => request.post('/api/backups', undefined, { timeout: 180000 }),
  upload: (form: FormData) => request.post('/api/backups/upload', form, { headers: { 'Content-Type': 'multipart/form-data' }, timeout: 300000 }),
  restore: (name: string) => request.post(`/api/backups/${encodeURIComponent(name)}/restore`, undefined, { timeout: 300000 }),
  download: (name: string) => request.get(`/api/backups/${encodeURIComponent(name)}/download`, { responseType: 'blob', timeout: 300000 }),
  remove: (name: string) => request.delete(`/api/backups/${encodeURIComponent(name)}`),
}
