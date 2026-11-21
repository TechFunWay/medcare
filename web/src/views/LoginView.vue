<template>
  <AuthShell>
    <div class="eyebrow">欢迎回来</div>
    <h1 style="font-size:26px;margin:6px 0 6px">登录用药管理</h1>
    <p class="muted small" style="margin-bottom:20px">家人可以一起管，提醒不再靠记忆。</p>
    <form @submit.prevent="submit">
      <div v-if="fnosBindingRequired" class="bind-tip">
        当前飞牛账号 <b>{{ fnosUsername || '当前用户' }}</b> 还没绑定应用账号。请登录已有应用账号，登录成功后会自动完成绑定，并继续使用原账号的数据。
      </div>
      <div class="field"><label>用户名</label><input v-model="username" class="input" autocomplete="username" placeholder="请输入用户名" required /></div>
      <div class="field"><label>密码 <router-link to="/forgot-password" class="link tiny" style="float:right;font-weight:600">忘记密码？</router-link></label><input v-model="password" class="input" type="password" autocomplete="current-password" placeholder="请输入密码" required /></div>
      <div v-if="error" class="form-error">{{ error }}</div>
      <button class="btn btn-primary btn-block" :disabled="loading" type="submit">{{ loading ? (fnosBindingRequired ? '登录并绑定中…' : '登录中…') : (fnosBindingRequired ? '登录并绑定' : '登录') }}</button>
      <button v-if="fnosGateway" class="btn btn-outline btn-block" style="margin-top:10px" :disabled="loading" type="button" @click="fnosLoginHandler">
        {{ loading ? '正在获取飞牛账号…' : '使用飞牛 NAS 登录' }}
      </button>
    </form>
    <div class="auth-foot muted small">还没有账号？<router-link to="/register" class="link">立即注册</router-link></div>
    <div class="auth-note tiny muted">本应用只帮助记录和提醒，不提供诊断或用药建议。请遵医嘱。</div>
  </AuthShell>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AuthShell from '../components/AuthShell.vue'
import { useAuthStore } from '../stores/auth'
import { bindFnOSAccount, fnosIdentity, fnosLogin } from '../api/auth'
import { isFnOSGatewayOrigin } from '../utils/fnos'
import { useUIStore } from '../stores/ui'
const auth = useAuthStore()
const ui = useUIStore()
const router = useRouter()
const route = useRoute()
const username = ref('')
const password = ref('')
const loading = ref(false)
const error = ref('')
const fnosGateway = isFnOSGatewayOrigin()
// 飞牛账号尚未绑定任何应用账号时，转为「登录已有账号并绑定」流程。
const fnosBindingRequired = ref(false)
const fnosUsername = ref('')
async function fnosLoginHandler() {
  loading.value = true
  error.value = ''
  try {
    const identity = await fnosIdentity()
    if (identity.data?.code !== 0) throw new Error(identity.data?.message || '未获取到飞牛 NAS 登录信息')
    const fnosName = identity.data?.data?.fnos_username || identity.data?.data?.username || ''
    // 必须先在页内确认要使用哪个飞牛账号，避免点一下就静默登录。
    const ok = await ui.confirm({
      title: '使用飞牛 NAS 登录',
      message: fnosName ? ('将使用飞牛账号「' + fnosName + '」登录用药管理，是否继续？') : '将使用当前飞牛 NAS 账号登录用药管理，是否继续？',
      confirmText: '继续登录',
    })
    if (!ok) { loading.value = false; return }
    const res = await fnosLogin()
    if (res.data?.code !== 0) throw new Error(res.data?.message || '飞牛登录失败')
    if (res.data.data?.binding_required) {
      // 已有应用账号但尚未绑定该飞牛账号：引导用应用账号密码登录并绑定。
      fnosBindingRequired.value = true
      fnosUsername.value = res.data.data.fnos_username || ''
      username.value = res.data.data.suggested_username || ''
      password.value = ''
      ui.showToast('该飞牛账号尚未绑定应用账号，请登录已有账号完成绑定')
      return
    }
    if (res.data.data?.user == null) throw new Error('飞牛登录失败')
    auth.setUser(res.data.data.user)
    if (res.data.data.token) auth.setToken(res.data.data.token)
    ui.showToast('已使用飞牛 NAS 登录')
    router.replace('/today')
  } catch (e: any) {
    error.value = e?.response?.data?.message || e?.message || '飞牛登录失败'
  } finally {
    loading.value = false
  }
}
async function submit() {
  loading.value = true; error.value = ''
  try {
    if (fnosBindingRequired.value) {
      const res = await bindFnOSAccount('bind', username.value.trim(), password.value)
      if (res.data?.code !== 0) throw new Error(res.data?.message || '登录并绑定失败')
      auth.setUser(res.data.data.user)
      if (res.data.data.token) auth.setToken(res.data.data.token)
      fnosBindingRequired.value = false
      ui.showToast('已登录并绑定飞牛账号')
      router.replace('/today')
      return
    }
    await auth.login(username.value.trim(), password.value)
    ui.showToast('登录成功')
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/today'
    router.replace(redirect)
  } catch (e: any) { error.value = e?.response?.data?.message || e?.message || '登录失败' }
  finally { loading.value = false }
}
</script>

<style scoped>
.bind-tip { margin-bottom: 14px; padding: 10px 12px; border-radius: 12px; background: var(--mc-primary-soft); color: var(--mc-primary-strong); font-size: 12.5px; line-height: 1.65; }
</style>
