<template>
  <AuthShell>
    <div class="eyebrow">找回密码</div>
    <h1 style="font-size:26px;margin:6px 0 6px">通过安全问题重置密码</h1>
    <p class="muted small" style="margin-bottom:20px">先输入用户名，回答设置过的 3 个安全问题后设置新密码。</p>
    <form @submit.prevent="next">
      <template v-if="step === 1">
        <div class="field"><label>用户名</label><input v-model="username" class="input" autocomplete="username" placeholder="请输入用户名" required /></div>
        <div v-if="error" class="form-error">{{ error }}</div>
        <button class="btn btn-primary btn-block" :disabled="loading" type="submit">{{ loading ? '查询中…' : '下一步' }}</button>
      </template>
      <template v-else>
        <div v-for="i in 3" :key="i" class="field">
          <label>问题 {{ i }}：{{ questions[i - 1] }}</label>
          <input v-model="answers[i - 1]" class="input" :placeholder="'答案 ' + i" />
        </div>
        <div class="field"><label>新密码</label><input v-model="newPassword" type="password" class="input" autocomplete="new-password" placeholder="至少 6 位" /></div>
        <div class="field"><label>确认新密码</label><input v-model="confirmPassword" type="password" class="input" autocomplete="new-password" placeholder="再输入一次" /></div>
        <div v-if="error" class="form-error">{{ error }}</div>
        <button class="btn btn-primary btn-block" :disabled="loading" type="submit">{{ loading ? '重置中…' : '重置密码' }}</button>
      </template>
    </form>
    <div class="auth-foot muted small"><router-link to="/login" class="link">返回登录</router-link></div>
    <div class="auth-note tiny muted">本应用只帮助记录和提醒，不提供诊断或用药建议。请遵医嘱。</div>
  </AuthShell>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import AuthShell from '../components/AuthShell.vue'
import { forgotQuestions, resetBySecurity } from '../api/account'
import { useUIStore } from '../stores/ui'

const router = useRouter()
const ui = useUIStore()
const step = ref(1)
const username = ref('')
const questions = ref(['', '', ''])
const answers = ref(['', '', ''])
const newPassword = ref('')
const confirmPassword = ref('')
const loading = ref(false)
const error = ref('')

async function next() {
  if (step.value !== 1) { await reset(); return }
  loading.value = true; error.value = ''
  try {
    const res = await forgotQuestions(username.value.trim())
    if (res.data?.code !== 0) throw new Error(res.data?.message || '查询失败')
    const d = res.data.data || {}
    if (!d.has_questions) throw new Error('该账号没有设置安全问题，请联系管理员重置密码，或使用飞牛 NAS 登录')
    questions.value = [d.question1 || '', d.question2 || '', d.question3 || '']
    answers.value = ['', '', '']
    step.value = 2
  } catch (e: any) { error.value = e?.response?.data?.message || e?.message || '查询失败' }
  finally { loading.value = false }
}

async function reset() {
  if (!newPassword.value) { error.value = '请输入新密码'; return }
  if (newPassword.value !== confirmPassword.value) { error.value = '两次新密码不一致'; return }
  if (answers.value.some((x) => !x.trim())) { error.value = '请回答全部 3 个安全问题'; return }
  loading.value = true; error.value = ''
  try {
    const res = await resetBySecurity({ username: username.value.trim(), answer1: answers.value[0], answer2: answers.value[1], answer3: answers.value[2], new_password: newPassword.value })
    if (res.data?.code !== 0) throw new Error(res.data?.message || '重置失败')
    ui.showToast('密码已重置，请用新密码登录')
    router.replace('/login')
  } catch (e: any) { error.value = e?.response?.data?.message || e?.message || '重置失败' }
  finally { loading.value = false }
}
</script>
