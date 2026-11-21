<template>
  <div class="page narrow">
    <div class="page-head"><div><div class="eyebrow">我的</div><h1>{{ auth.user?.username || '我' }}</h1><p>{{ auth.isAdmin ? '管理员' : '家庭成员' }} · 用药管理</p></div></div>
    <div class="card profile-me">
      <div class="book-icon profile-avatar">{{ auth.user?.username?.slice(0,1) || '我' }}</div>
      <div class="profile-me-body"><div class="profile-me-name">{{ auth.user?.username }}</div><div class="muted small">当前用药本：{{ book.current?.name || '未选择' }}</div></div>
    </div>
    <router-link to="/more" class="card more-entry">
      <span class="more-entry-icon"><AppIcon name="grid" :size="21" /></span>
      <span class="more-entry-body">
        <b>更多功能</b>
        <span class="tiny muted">图片、处方、地点、统计、日志、设置…</span>
      </span>
      <AppIcon name="chevron" :size="17" class="more-entry-arrow" />
    </router-link>

    <div class="section-head" style="margin-top:16px"><h2>账号与安全</h2></div>
    <CollapseSection v-model:open="accountOpen" title="账户信息" icon="user" :hint="accountHint">
      <div class="info-row"><span>用户名</span><b>{{ account?.username || auth.user?.username || '—' }}</b></div>
      <div class="info-row"><span>角色</span><b>{{ roleLabel(account?.role) }}</b></div>
      <div class="info-row"><span>登录密码</span><b>{{ account?.has_password ? '已设置' : '未设置' }}</b></div>
      <!-- 只有飞牛部署才有飞牛绑定，独立部署（本机/裸二进制/Docker）不显示这一行。 -->
      <div v-if="isFnos" class="info-row"><span>飞牛 NAS</span><b>{{ account?.fnos_bound ? '已绑定：' + (account.fnos_username || '') : '未绑定' }}</b></div>
    </CollapseSection>

    <CollapseSection v-model:open="pwdOpen" title="安全信息" icon="lock" :hint="pwdHint">
      <template #tail>
        <span class="badge" :class="account?.has_password ? 'done' : 'warn'">{{ account?.has_password ? '已设密码' : '未设密码' }}</span>
      </template>
      <p class="tiny muted" style="margin:0 0 8px;line-height:1.6">当前已登录，直接设置新密码即可；改完其它设备上的登录会失效，需要用新密码重新登录。</p>
      <form @submit.prevent="savePassword">
        <div class="field-row">
          <div class="field"><label>新密码</label><input v-model="newPassword" type="password" class="input" autocomplete="new-password" /></div>
          <div class="field"><label>确认新密码</label><input v-model="confirmPassword" type="password" class="input" autocomplete="new-password" /></div>
        </div>
        <button class="btn btn-primary btn-block" :disabled="savingPwd">{{ savingPwd ? '修改中…' : '修改密码' }}</button>
      </form>
      <template v-if="isFnos">
        <div style="height:12px" />
        <button v-if="!account?.fnos_bound" class="btn btn-outline btn-block" :disabled="savingFnos" @click="bindFnos">{{ savingFnos ? '处理中…' : '绑定飞牛 NAS 账号' }}</button>
        <button v-else class="btn btn-outline btn-block" :disabled="savingFnos" @click="unbindFnos">{{ savingFnos ? '处理中…' : '解除飞牛 NAS 绑定' }}</button>
        <p class="tiny muted" style="margin-top:8px;line-height:1.6">{{ account?.fnos_bound ? '解除绑定后需要用账号密码登录，请先确保已设置密码。' : '绑定后可以用飞牛 NAS 账号一键登录本应用。' }}</p>
      </template>
    </CollapseSection>

    <CollapseSection v-model:open="sqOpen" title="安全问题" icon="lock" :hint="sqHint">
      <template #tail>
        <span class="badge" :class="sqHas ? 'done' : 'warn'">{{ sqHas ? '已设置' : '未设置' }}</span>
      </template>
      <p class="tiny muted" style="margin-bottom:10px;line-height:1.6">设置 3 个问题和答案，忘记密码时可以凭答案重置密码；修改时需重新填写答案。</p>
      <div v-for="i in 3" :key="i" class="sq-item">
        <div class="field"><label>问题 {{ i }}</label><input v-model="sqQuestions[i - 1]" class="input" placeholder="例如：我的小学班主任姓什么" /></div>
        <div class="field"><label>答案 {{ i }}</label><input v-model="sqAnswers[i - 1]" class="input" placeholder="答案（不区分大小写）" /></div>
      </div>
      <button class="btn btn-primary btn-block" :disabled="savingSq" @click="saveSecurityQuestions">{{ savingSq ? '保存中…' : '保存安全问题' }}</button>
    </CollapseSection>

    <div class="card" style="margin-top:16px;text-align:center"><p class="tiny muted" style="line-height:1.7">本应用只帮助记录和提醒，不提供诊断、用药建议或剂量推荐。<br />请遵医嘱。</p></div>
    <button class="btn btn-danger btn-block" style="margin-top:12px" @click="logout">退出登录</button>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import AppIcon from '../components/AppIcon.vue'
import CollapseSection from '../components/CollapseSection.vue'
import { bindFnOS, changePassword, getAccount, getSecurityQuestions, setSecurityQuestions, unbindFnOS, type AccountInfo } from '../api/account'
import { useAuthStore } from '../stores/auth'
import { useBookStore } from '../stores/book'
import { useUIStore } from '../stores/ui'
import { reportError } from '../utils/errors'
import { isFnOSGatewayOrigin } from '../utils/fnos'
const auth = useAuthStore()
const book = useBookStore()
const ui = useUIStore()
const router = useRouter()
const account = ref<AccountInfo | null>(null)
// 安全类表单属于低频操作：手机端默认收起，避免六七个输入框把常用入口挤出首屏；
// 桌面（>=900px）空间充裕，密码表单保持展开。是否展开由 CollapseSection 的开合状态决定。
const isWide = typeof window !== 'undefined' && window.matchMedia('(min-width: 900px)').matches
const pwdOpen = ref(isWide)
const sqOpen = ref(false)
// 账户信息默认收起：用户名/角色在页头与下面的安全区块里都能看到，不必长期占一块。
const accountOpen = ref(false)
const accountHint = computed(() => (account.value?.username || auth.user?.username || '') + ' · ' + roleLabel(account.value?.role))
// 是否飞牛部署：飞牛网关会注入 __FNOS_GATEWAY__，网关路径固定挂在 /app/techfunway- 下。
// 其它形态（本机 99xx、Docker、裸二进制）一律不显示飞牛绑定入口。
const isFnos = isFnOSGatewayOrigin() || window.location.pathname.startsWith('/app/techfunway-')
const sqQuestions = ref(['', '', ''])
const sqAnswers = ref(['', '', ''])
const sqHas = ref(false)
const savingSq = ref(false)
const oldPassword = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const savingPwd = ref(false)
const savingFnos = ref(false)
const pwdHint = computed(() => {
  if (!isFnos) return '修改登录密码'
  return account.value?.fnos_bound ? '修改登录密码 · 已绑定飞牛 NAS' : '修改登录密码 · 可绑定飞牛 NAS'
})
const sqHint = computed(() => (sqHas.value ? '点按修改这 3 组问答' : '点按设置，忘记密码时可凭答案重置'))
function roleLabel(role?: string) {
  if (role === 'admin' || role === 'owner') return '管理员'
  if (role === 'manager') return '监护人'
  if (role === 'member' || role === 'user') return '普通用户'
  return '—'
}
async function loadAccount() {
  try { const res = await getAccount(); if (res.data?.code === 0) account.value = res.data.data } catch (e) { reportError(e, '读取账户信息失败') }
  try {
    const r = await getSecurityQuestions()
    if (r.data?.code === 0 && r.data.data) {
      sqHas.value = !!r.data.data.has_questions
      sqQuestions.value = [r.data.data.question1 || '', r.data.data.question2 || '', r.data.data.question3 || '']
      sqAnswers.value = ['', '', '']
    }
  } catch (e) { reportError(e, '读取安全问题失败') }
}
async function saveSecurityQuestions() {
  const qs = sqQuestions.value.map((x) => x.trim())
  const as = sqAnswers.value.map((x) => x.trim())
  if (qs.some((x) => !x) || as.some((x) => !x)) { ui.showToast('请填写全部 3 个问题和答案', 'error'); return }
  savingSq.value = true
  try {
    await setSecurityQuestions({ question1: qs[0], answer1: as[0], question2: qs[1], answer2: as[1], question3: qs[2], answer3: as[2] })
    ui.showToast('安全问题已保存')
    await loadAccount()
  } catch (e: any) { ui.showToast(e?.response?.data?.message || '保存安全问题失败', 'error') }
  finally { savingSq.value = false }
}
async function savePassword() {
  if (!newPassword.value) { ui.showToast('请输入新密码', 'error'); return }
  if (newPassword.value !== confirmPassword.value) { ui.showToast('两次新密码不一致', 'error'); return }
  savingPwd.value = true
  try {
    // 已登录会话里改密码不再传旧密码（后端对已登录请求也不强制校验）。
    await changePassword('', newPassword.value)
    ui.showToast('密码已修改，其它设备需要重新登录')
    newPassword.value = ''; confirmPassword.value = ''
    await loadAccount()
  } catch (e: any) { ui.showToast(e?.response?.data?.message || '修改密码失败', 'error') }
  finally { savingPwd.value = false }
}
async function bindFnos() {
  savingFnos.value = true
  try { await bindFnOS(); ui.showToast('已绑定飞牛 NAS 账号'); await loadAccount() }
  catch (e: any) { ui.showToast(e?.response?.data?.message || '绑定失败，请在飞牛 NAS 入口内操作', 'error') }
  finally { savingFnos.value = false }
}
async function unbindFnos() {
  if (!(await ui.confirm({ title: '解除飞牛绑定？', message: '解除后需用账号密码登录；请先确保已设置密码。', confirmText: '解除绑定' }))) return
  savingFnos.value = true
  try { await unbindFnOS(); ui.showToast('已解除飞牛绑定'); await loadAccount() }
  catch (e: any) { ui.showToast(e?.response?.data?.message || '解除绑定失败', 'error') }
  finally { savingFnos.value = false }
}
onMounted(loadAccount)
async function logout() { if (!(await ui.confirm({ title: '退出登录？', message: '退出后需要重新登录才能查看用药本。', confirmText: '退出' }))) return; await auth.logout(); book.clear(); router.push('/login') }
</script>

<style scoped>
.info-row { display: flex; justify-content: space-between; gap: 10px; padding: 7px 0; border-bottom: 1px dashed var(--mc-border); font-size: 13px; }
.info-row:last-child { border-bottom: 0; }
.info-row span { color: var(--mc-text-muted); }
.info-row b { font-weight: 700; text-align: right; overflow-wrap: anywhere; }
.profile-me { display: flex; gap: 10px; align-items: center; }
.profile-avatar { width: 46px; height: 46px; font-size: 18px; }
.profile-me-body { min-width: 0; }
.profile-me-name { font-weight: 750; font-size: 16px; overflow-wrap: anywhere; }
.more-entry { display: flex; align-items: center; gap: 11px; padding: 12px; margin-top: 10px; border-color: var(--mc-primary); background: linear-gradient(135deg, var(--mc-primary-soft), var(--mc-surface)); }
.more-entry-icon { flex: 0 0 auto; width: 40px; height: 40px; border-radius: 12px; background: var(--mc-surface); color: var(--mc-primary-strong); display: grid; place-items: center; }
.more-entry-body { flex: 1 1 auto; min-width: 0; display: grid; gap: 2px; }
.more-entry-body b { font-size: 15px; }
.more-entry-arrow { flex: 0 0 auto; color: var(--mc-primary-strong); }
.profile-link { display: flex; align-items: center; gap: 10px; padding: 12px; }
.profile-link-body { flex: 1; min-width: 0; }
.profile-link-body b { font-size: 14.5px; }
.profile-link-arrow { flex: 0 0 auto; }
/* 安全问题：手机端每组问答上下排、组间虚线分隔；620px 以上问题与答案并排，展开后少占一屏的一半。 */
.sq-item + .sq-item { margin-top: 10px; padding-top: 10px; border-top: 1px dashed var(--mc-border); }
.sq-item .field:last-child { margin-bottom: 0; }
@media (min-width: 620px) {
  .sq-item { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: 8px; }
  .sq-item .field { margin-bottom: 0; }
}
@media (min-width: 900px) {
  .profile-me { gap: 12px; }
  .profile-avatar { width: 52px; height: 52px; font-size: 20px; }
  .profile-me-name { font-size: 17px; }
  .profile-link { gap: 12px; padding: 14px; }
  .profile-link-body b { font-size: 15px; }
}
</style>
