import request from './request'

// 账号信息 / 安全设置 / 赞赏与版本检查，均为 smallgo 框架提供的通用接口。

export interface AccountInfo {
  id: number
  username: string
  role: string
  api_key?: string
  has_password: boolean
  has_security_questions: boolean
  fnos_bound: boolean
  fnos_username?: string
}

export function getAccount() {
  return request.get('/api/auth/me')
}

export function changePassword(oldPassword: string, newPassword: string) {
  return request.put('/api/auth/password', { old_password: oldPassword, new_password: newPassword })
}

// 绑定当前飞牛身份到已登录账号（需要网关/票据身份，直连端口不可用）。
export function bindFnOS() {
  return request.post('/api/auth/fnos/bind-self')
}

export interface SecurityQuestionInfo {
  question1: string
  question2: string
  question3: string
  has_questions: boolean
}

export function getSecurityQuestions() {
  return request.get('/api/security/questions')
}

export function setSecurityQuestions(data: { question1: string; answer1: string; question2: string; answer2: string; question3: string; answer3: string }) {
  return request.post('/api/security/questions', data)
}

export function forgotQuestions(username: string) {
  return request.post('/api/security/forgot/username', { username })
}

export function resetBySecurity(data: { username: string; answer1: string; answer2: string; answer3: string; new_password: string }) {
  return request.post('/api/security/forgot/reset', data)
}

export function unbindFnOS() {
  return request.post('/api/medcare/account/fnos-unbind')
}

export interface VersionInfo {
  version: string
  buildTime?: string
  startedAt?: string
  appName?: string
  gitCommit?: string
}

export function getVersionInfo() {
  return request.get('/api/version')
}

export interface UpdateInfo {
  current: string
  latest: string
  download_url: string
  has_update: boolean
}

export function checkUpdate() {
  return request.get('/api/version/check')
}

// 账号级「已支持」：服务端落库（user_preferences.donate_supported_version），
// 同账号跨设备共享；匿名计数由服务端后台尽力上报，失败不影响本接口成功。
export function donateSupport(amount?: number) {
  return request.post('/api/medcare/donate/support', { amount: amount && amount > 0 ? amount : 0 })
}
