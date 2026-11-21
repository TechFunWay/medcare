<template>
  <AuthShell :points="firstRun ? firstRunPoints : defaultPoints">
    <div class="eyebrow">{{ firstRun ? '首次使用' : '加入用药本' }}</div>
    <h1 style="font-size:26px;margin:6px 0">{{ firstRun ? '创建管理员账号' : '创建账号' }}</h1>
    <p v-if="firstRun" class="muted small" style="margin-bottom:16px">
      这是本机第一次使用，注册的第一个账号就是<b>管理员</b>，可以管理成员、系统设置与备份。
    </p>
    <p v-else class="muted small" style="margin-bottom:16px">注册后即可创建或加入用药本。</p>
    <div v-if="firstRun" class="first-run-hint">
      <b>管理员能做什么</b>
      <span>创建用药本、指定用药人与监护人、邀请家人、配置提醒渠道、备份与恢复。</span>
    </div>
    <form @submit.prevent="submit">
      <div class="field"><label>用户名</label><input v-model="username" class="input" autocomplete="username" placeholder="3–40 个字符" required /></div>
      <div class="field"><label>密码</label><input v-model="password" class="input" type="password" autocomplete="new-password" placeholder="至少 6 位" required /></div>
      <div class="field"><label>确认密码</label><input v-model="confirm" class="input" type="password" autocomplete="new-password" placeholder="再输入一次" required /></div>
      <div v-if="error" class="form-error">{{ error }}</div>
      <button class="btn btn-primary btn-block" :disabled="loading" type="submit">{{ loading ? '创建中…' : (firstRun ? '创建管理员账号' : '创建账号') }}</button>
      <button v-if="fnosGateway" class="btn btn-outline btn-block" style="margin-top:10px" :disabled="loading" type="button" @click="fnosLoginHandler">
        {{ loading ? '正在获取飞牛账号…' : '使用飞牛 NAS 登录' }}
      </button>
    </form>
    <div class="auth-foot muted small">已有账号？<router-link to="/login" class="link">去登录</router-link></div>
  </AuthShell>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AuthShell from '../components/AuthShell.vue'
import { useAuthStore } from '../stores/auth'
import { fnosIdentity, fnosLogin } from '../api/auth'
import { isFnOSGatewayOrigin } from '../utils/fnos'
import { useUIStore } from '../stores/ui'
const auth = useAuthStore()
const ui = useUIStore()
const router = useRouter()
const route = useRoute()
const username = ref('')
const password = ref('')
const confirm = ref('')
const loading = ref(false)
const error = ref('')
const fnosGateway = isFnOSGatewayOrigin()
// 首个用户即管理员：只要库里还没有账号，注册页就按「创建管理员」呈现。
const firstRun = computed(() => auth.setupRequired)
const firstRunPoints = [
  '第一个账号就是管理员：管成员、定权限、配提醒渠道',
  '先建一本用药本，再把家人邀请进来一起照护',
  '支持邮件、钉钉、飞书提醒，还能拍照识别处方单',
]
const defaultPoints = [
  '注册后就能加入家人的用药本，一起打卡与提醒',
  '成员只能查看与提醒用药，用药人本人可记录自己的执行',
  '支持邮件、钉钉、飞书提醒，随时随地收到通知',
]
onMounted(() => { auth.init() })
async function fnosLoginHandler() {
  loading.value = true
  error.value = ''
  try {
    const identity = await fnosIdentity()
    if (identity.data?.code !== 0) throw new Error(identity.data?.message || '未获取到飞牛 NAS 登录信息')
    const res = await fnosLogin()
    if (res.data?.code !== 0 || res.data?.data?.user == null) {
      throw new Error(res.data?.message || '飞牛登录失败')
    }
    auth.setUser(res.data.data.user)
    if (res.data.data.token) auth.setToken(res.data.data.token)
    ui.showToast('已使用飞牛 NAS 登录')
    router.push('/today')
  } catch (e: any) {
    error.value = e?.response?.data?.message || e?.message || '飞牛登录失败'
  } finally {
    loading.value = false
  }
}
async function submit() {
  error.value = ''
  if (password.value !== confirm.value) { error.value = '两次密码不一致'; return }
  if (password.value.length < 6) { error.value = '密码至少 6 位'; return }
  loading.value = true
  try {
    await auth.register(username.value.trim(), password.value)
    ui.showToast(auth.setupRequired ? '管理员账号已创建' : '账号已创建')
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/today'
    router.push(redirect)
  }
  catch (e: any) { error.value = e?.response?.data?.message || e?.message || '注册失败' }
  finally { loading.value = false }
}
</script>

