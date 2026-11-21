// 系统级用户管理（仅管理员）：列表、改角色、启用/禁用、重置密码、删除。
// 后端在 smallgo 框架 user 包，路由前缀 /api/users。
import request from './request'

export interface AdminUser {
  id: number
  username: string
  role: string
  status: number
  // 首次注册的管理员是超级管理员，任何账号都不能修改它的设置。
  is_super_admin?: boolean
  created_at: string
}

export const userApi = {
  list: (params: any = {}) => request.get('/api/users', { params }),
  create: (data: { username: string; password: string; role?: string; status?: number }) => request.post('/api/users', data),
  update: (id: number, data: { role?: string; status?: number }) => request.put(`/api/users/${id}`, data),
  // 改名单独一个端点：管理员接口不做批量字段赋值，避免 mass-assignment。
  rename: (id: number, username: string) => request.put(`/api/users/${id}/username`, { username }),
  toggleStatus: (id: number) => request.put(`/api/users/${id}/status`),
  resetPassword: (id: number, newPassword: string) => request.put(`/api/users/${id}/password`, { new_password: newPassword }),
  remove: (id: number) => request.delete(`/api/users/${id}`),
}
