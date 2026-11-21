<template>
  <div class="page">
    <div class="page-head">
      <div style="display:flex;gap:8px;align-items:flex-start">
        <button class="icon-btn" aria-label="返回" @click="router.back()"><AppIcon name="arrowLeft" /></button>
        <div><div class="eyebrow">管理员</div><h1>用户管理</h1><p>共 {{ total }} 个账号。可新增、改角色、启用/禁用、重置密码或删除。</p></div>
      </div>
      <button class="btn btn-primary btn-sm" @click="openCreate"><AppIcon name="plus" :size="16" />添加用户</button>
    </div>

    <div class="card search-card">
      <AppIcon name="search" :size="19" class="search-icon" />
      <input v-model="search" class="input search-input" placeholder="搜索用户名" />
      <button v-if="search" class="search-clear" aria-label="清空" @click="search = ''"><AppIcon name="close" :size="16" /></button>
    </div>

    <div v-if="loading && !users.length" class="card muted">加载中…</div>
    <div v-else-if="!users.length" class="empty card">
      <div class="empty-icon"><AppIcon name="user" :size="28" /></div>
      <h3>{{ search ? '没有匹配的用户' : '还没有用户' }}</h3>
      <p>{{ search ? '换个用户名试试。' : '第一个注册的账号会自动成为管理员。' }}</p>
    </div>
    <div v-else class="list">
      <article v-for="u in users" :key="u.id" class="card user-row" @click="openSheet(u)">
        <div class="avatar">{{ u.username.slice(0, 1).toUpperCase() }}</div>
        <div class="user-body">
          <div class="user-title">
            <b>{{ u.username }}</b>
            <span class="badge" :class="u.is_super_admin ? 'super' : u.role === 'admin' ? 'done' : 'pending'">
              {{ u.is_super_admin ? '超级管理员' : u.role === 'admin' ? '管理员' : '普通用户' }}
            </span>
            <span v-if="u.status === 2" class="badge warn">待审核</span>
            <span v-else-if="u.status !== 1" class="badge missed">已禁用</span>
            <span v-if="u.id === auth.user?.id" class="badge warn">当前账号</span>
          </div>
          <div class="tiny muted" style="margin-top:3px">注册于 {{ u.created_at }}</div>
        </div>
        <AppIcon name="chevron" :size="18" class="user-arrow" />
      </article>
    </div>
    <button v-if="hasMore" class="btn btn-outline btn-block" style="margin-top:10px" :disabled="loading" @click="load(false)">{{ loading ? '加载中…' : '加载更多' }}</button>

    <Teleport to="body">
      <div v-if="showCreate" class="sheet-backdrop" @click.self="closeCreate">
        <section class="sheet" role="dialog" aria-modal="true">
          <div class="sheet-grab" />
          <div class="sheet-head"><h2>添加用户</h2><button class="icon-btn" aria-label="关闭" @click="closeCreate"><AppIcon name="close" /></button></div>
          <form class="sheet-body" @submit.prevent="createUser">
            <div class="field"><label>用户名</label><input v-model="createForm.username" class="input" placeholder="登录名，1–32 个字符" /></div>
            <div class="field"><label>初始密码</label><input v-model="createForm.password" type="password" class="input" placeholder="至少 6 位" autocomplete="new-password" /></div>
            <div class="field"><label>角色</label>
              <SheetSelect v-model="createForm.role" placeholder="角色" :options="roleOptions" />
            </div>
            <p class="tiny muted" style="margin:-2px 0 0;line-height:1.6">新建的账号默认可直接登录；如需对方自行注册再审核，请到「偏好设置」打开「普通用户注册需审核」。</p>
            <div class="sheet-foot">
              <button type="button" class="btn btn-outline btn-block" :disabled="saving" @click="closeCreate">取消</button>
              <button class="btn btn-primary btn-block" :disabled="saving || !createForm.username.trim() || createForm.password.length < 6">{{ saving ? '创建中…' : '创建用户' }}</button>
            </div>
          </form>
        </section>
      </div>
      <div v-if="current" class="sheet-backdrop" @click.self="closeSheet">
        <section class="sheet" role="dialog" aria-modal="true">
          <div class="sheet-grab" />
          <div class="sheet-head"><h2>{{ current.username }}</h2><button class="icon-btn" aria-label="关闭" @click="closeSheet"><AppIcon name="close" /></button></div>
          <div class="sheet-body">
            <p class="tiny muted" style="margin:-2px 0 10px">
              {{ current.is_super_admin ? '超级管理员' : current.role === 'admin' ? '管理员' : '普通用户' }} · {{ statusText(current.status) }}
              <template v-if="current.id === auth.user?.id"> · 这是你自己的账号</template>
            </p>

            <div v-if="!current.is_super_admin" class="field">
              <label>用户名</label>
              <div class="inline-field">
                <input v-model="editName" class="input" />
                <button class="btn btn-outline btn-sm" :disabled="saving || !editName.trim() || editName.trim() === current.username" @click="saveUsername">保存</button>
              </div>
            </div>

            <div v-if="resetMode" class="field">
              <label>新密码（{{ current.username }}）</label>
              <input v-model="newPassword" type="password" class="input" placeholder="至少 6 位" autocomplete="new-password" />
            </div>

            <div v-if="current.is_super_admin" class="super-note">
              <b>超级管理员</b>
              <p class="tiny muted">超级管理员是首次注册的账号，<b>任何账号都不能修改它的设置</b>——不能改名、改角色、禁用、删除或重置密码，包括它自己。<br />忘记密码时请在服务器上用命令行重置：<br /><code>medcare -data-dir &lt;数据目录&gt; -reset-admin-password</code></p>
            </div>

            <div v-else class="sheet-actions">
              <template v-if="!resetMode">
                <button v-if="current.status === 2" class="btn btn-primary btn-block" :disabled="saving" @click="approveUser">通过审核，允许登录</button>
                <button class="btn btn-outline btn-block" :disabled="saving" @click="toggleRole">
                  {{ current.role === 'admin' ? '设为普通用户' : '设为管理员' }}
                </button>
                <button class="btn btn-outline btn-block" :disabled="saving" @click="resetMode = true">重置密码</button>
                <button v-if="current.status !== 2" class="btn btn-outline btn-block" :disabled="saving" @click="toggleStatus">
                  {{ current.status === 1 ? '禁用该账号' : '启用该账号' }}
                </button>
                <button v-if="current.id !== auth.user?.id" class="btn btn-danger btn-block" :disabled="saving" @click="removeUser">删除用户</button>
                <p v-else class="tiny muted" style="line-height:1.6">不能删除或禁用自己，避免把自己锁在门外。</p>
              </template>
              <template v-else>
                <button class="btn btn-primary btn-block" :disabled="saving || newPassword.length < 6" @click="doResetPassword">{{ saving ? '保存中…' : '确认重置密码' }}</button>
                <button class="btn btn-outline btn-block" :disabled="saving" @click="resetMode = false; newPassword = ''">取消</button>
              </template>
            </div>
          </div>
        </section>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import AppIcon from '../components/AppIcon.vue'
import SheetSelect from '../components/SheetSelect.vue'
import { userApi, type AdminUser } from '../api/user'
import { useAuthStore } from '../stores/auth'
import { useUIStore } from '../stores/ui'

const router = useRouter()
const auth = useAuthStore()
const ui = useUIStore()

const users = ref<AdminUser[]>([])
const total = ref(0)
const search = ref('')
const loading = ref(false)
const page = ref(1)
const hasMore = ref(false)
const current = ref<AdminUser | null>(null)
const editName = ref('')
const showCreate = ref(false)
const createForm = reactive({ username: '', password: '', role: 'user' })
const roleOptions = [
  { value: 'user', label: '普通用户' },
  { value: 'admin', label: '管理员' },
]

function statusText(s: number) {
  return s === 1 ? '正常' : s === 2 ? '待审核' : '已禁用'
}
const resetMode = ref(false)
const newPassword = ref('')
const saving = ref(false)

async function load(reset = true) {
  loading.value = true
  const target = reset ? 1 : page.value + 1
  try {
    const res = await userApi.list({ page: target, pageSize: 20, search: search.value.trim() })
    const list = (res.data?.data?.items || []) as AdminUser[]
    users.value = reset ? list : users.value.concat(list)
    page.value = target
    total.value = Number(res.data?.data?.total) || 0
    hasMore.value = users.value.length < total.value
  } catch (e: any) {
    ui.showToast(e?.response?.data?.message || '读取用户失败', 'error')
  } finally { loading.value = false }
}

let timer: number | null = null
watch(search, () => {
  if (timer) window.clearTimeout(timer)
  timer = window.setTimeout(() => load(true), 250)
})

function openSheet(u: AdminUser) {
  current.value = u
  editName.value = u.username
  resetMode.value = false
  newPassword.value = ''
}
function openCreate() {
  createForm.username = ''
  createForm.password = ''
  createForm.role = 'user'
  showCreate.value = true
}
function closeCreate() { if (!saving.value) showCreate.value = false }

async function createUser() {
  if (!createForm.username.trim() || createForm.password.length < 6) return
  saving.value = true
  try {
    await userApi.create({ username: createForm.username.trim(), password: createForm.password, role: createForm.role })
    ui.showToast('已创建用户 ' + createForm.username.trim())
    showCreate.value = false
    await load(true)
  } catch (e: any) {
    ui.showToast(e?.response?.data?.message || '创建用户失败', 'error')
  } finally { saving.value = false }
}

async function saveUsername() {
  const u = current.value
  if (!u) return
  const name = editName.value.trim()
  if (!name || name === u.username) return
  saving.value = true
  try {
    await userApi.rename(u.id, name)
    ui.showToast('用户名已改为 ' + name)
    await load(true); refreshCurrent()
  } catch (e: any) {
    ui.showToast(e?.response?.data?.message || '修改用户名失败', 'error')
  } finally { saving.value = false }
}

// 审核通过：把「待审核」的账号置为正常。
async function approveUser() {
  const u = current.value
  if (!u) return
  saving.value = true
  try {
    await userApi.update(u.id, { status: 1 })
    ui.showToast('已通过 ' + u.username + ' 的审核')
    await load(true); refreshCurrent()
  } catch (e: any) {
    ui.showToast(e?.response?.data?.message || '审核失败', 'error')
  } finally { saving.value = false }
}
function closeSheet() { if (!saving.value) { current.value = null; resetMode.value = false } }
function refreshCurrent() {
  const fresh = users.value.find((x) => x.id === current.value?.id)
  if (fresh) current.value = fresh
}

async function toggleRole() {
  const u = current.value
  if (!u) return
  if (u.id === auth.user?.id && u.role === 'admin') { ui.showToast('不能取消自己的管理员身份', 'error'); return }
  saving.value = true
  try {
    await userApi.update(u.id, { role: u.role === 'admin' ? 'user' : 'admin' })
    ui.showToast(u.role === 'admin' ? '已设为普通用户' : '已设为管理员')
    await load(true); refreshCurrent()
  } catch (e: any) { ui.showToast(e?.response?.data?.message || '修改角色失败', 'error') }
  finally { saving.value = false }
}

async function toggleStatus() {
  const u = current.value
  if (!u) return
  if (u.id === auth.user?.id) { ui.showToast('不能禁用自己的账号', 'error'); return }
  saving.value = true
  try {
    await userApi.toggleStatus(u.id)
    ui.showToast(u.status === 1 ? '已禁用该账号' : '已启用该账号')
    await load(true); refreshCurrent()
  } catch (e: any) { ui.showToast(e?.response?.data?.message || '操作失败', 'error') }
  finally { saving.value = false }
}

async function doResetPassword() {
  const u = current.value
  if (!u) return
  saving.value = true
  try {
    await userApi.resetPassword(u.id, newPassword.value)
    ui.showToast('已重置 ' + u.username + ' 的密码')
    resetMode.value = false
    newPassword.value = ''
  } catch (e: any) { ui.showToast(e?.response?.data?.message || '重置密码失败', 'error') }
  finally { saving.value = false }
}

async function removeUser() {
  const u = current.value
  if (!u) return
  const ok = await ui.confirm({
    title: '删除「' + u.username + '」？',
    message: '账号与登录凭据会立即失效，该操作不可恢复。\n若只是暂时不用，建议改为「禁用」。',
    confirmText: '删除',
  })
  if (!ok) return
  saving.value = true
  try {
    await userApi.remove(u.id)
    ui.showToast('已删除 ' + u.username)
    current.value = null
    await load(true)
  } catch (e: any) { ui.showToast(e?.response?.data?.message || '删除失败', 'error') }
  finally { saving.value = false }
}

onMounted(() => load(true))
</script>

<style scoped>
.search-card { display: flex; align-items: center; gap: 8px; padding: 0 12px; margin-bottom: 10px; }
.search-card:focus-within { border-color: var(--mc-primary); box-shadow: 0 0 0 3px var(--mc-primary-soft); }
.search-icon { flex: 0 0 auto; color: var(--mc-text-muted); }
.search-input, .search-input:focus { border: 0 !important; background: transparent !important; box-shadow: none !important; border-radius: 0; padding-left: 0; }
.search-clear { flex: 0 0 auto; border: 0; background: var(--mc-surface-2); color: var(--mc-text-muted); width: 28px; height: 28px; border-radius: 999px; display: grid; place-items: center; }
.user-row { display: flex; align-items: center; gap: 10px; padding: 11px 12px; cursor: pointer; }
.avatar { flex: 0 0 auto; width: 38px; height: 38px; border-radius: 50%; background: var(--mc-primary-soft); color: var(--mc-primary-strong); font-weight: 800; display: grid; place-items: center; }
.user-body { min-width: 0; flex: 1 1 auto; }
.user-title { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.user-title b { font-size: 14px; overflow-wrap: anywhere; }
.user-arrow { flex: 0 0 auto; color: var(--mc-text-muted); }
.sheet-actions { display: grid; gap: 8px; }
.badge.super { background: linear-gradient(135deg, #f6e3b4, #ecd08a); color: #6b4a12; }
.super-note { display: grid; gap: 6px; padding: 10px 12px; border-radius: 12px; background: var(--mc-warning-soft); }
.super-note b { font-size: 14px; }
.super-note code { font-size: 11.5px; background: var(--mc-surface); padding: 2px 5px; border-radius: 6px; overflow-wrap: anywhere; }
.inline-field { display: flex; gap: 8px; align-items: center; }
.inline-field .input { flex: 1 1 auto; }
.inline-field .btn { flex: 0 0 auto; }
@media (min-width: 900px) {
  .user-row { padding: 13px 15px; }
  .user-title b { font-size: 15px; }
}
</style>
