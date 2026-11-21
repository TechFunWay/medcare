import axios from 'axios'
import { useAuthStore } from '../stores/auth'
import router from '../router'

const request = axios.create({
  baseURL: import.meta.env.BASE_URL,
  timeout: 30000,
})

request.interceptors.request.use((config) => {
  if (import.meta.env.BASE_URL !== '/' && config.url?.startsWith('/')) {
    config.url = config.url.slice(1)
  }
  const auth = useAuthStore()
  if (auth.token && !(window as any).__FNOS_GATEWAY__) {
    config.headers.Authorization = `Bearer ${auth.token}`
  }
  return config
})

request.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      const auth = useAuthStore()
      auth.clear()
      if (router.currentRoute.value.name !== 'Login') router.push('/login')
    }
    return Promise.reject(error)
  },
)

export default request
