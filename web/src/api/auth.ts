import request from './request'

export function login(username: string, password: string) {
  return request.post('/api/auth/login', { username, password })
}

export function register(username: string, password: string) {
  return request.post('/api/auth/register', { username, password })
}

export function logout() {
  return request.post('/api/auth/logout')
}

export function checkAuth() {
  return request.get('/api/auth/check')
}

export function checkSetupRequired() {
  return request.get('/api/auth/setup-required')
}

export function getPublicConfigs() {
  return request.get('/api/configs/public')
}

export function fnosIdentity() {
  return request.get('/api/auth/fnos/identity')
}

export function fnosLogin() {
  return request.post('/api/auth/fnos/login')
}

// 登录已有应用账号并把当前飞牛身份绑定上去（mode=bind）；mode=register 用于首次创建并绑定。
export function bindFnOSAccount(mode: 'login' | 'register' | 'bind', username: string, password: string) {
  return request.post('/api/auth/fnos/bind', { mode, username, password })
}
