<template>
  <AuthShell>
    <div class="eyebrow">用药本邀请</div>
    <h1 style="font-size:24px;margin:6px 0 8px">加入一起管理</h1>
    <template v-if="!auth.isAuthenticated">
      <p class="muted small" style="margin-bottom:18px">先登录或注册，再确认加入这个用药本。</p>
      <div class="list">
        <router-link :to="{ name: 'Login', query: { redirect: route.fullPath } }" class="btn btn-primary btn-block">登录后加入</router-link>
        <router-link :to="{ name: 'Register', query: { redirect: route.fullPath } }" class="btn btn-outline btn-block">注册账号</router-link>
      </div>
    </template>
    <template v-else>
      <div v-if="loading" class="muted small">正在读取邀请…</div>
      <div v-else-if="error" class="form-error">{{ error }}</div>
      <template v-else-if="preview">
        <div class="invite-box">
          <div class="book-icon" style="width:54px;height:54px;font-size:22px">{{ preview.care_recipient_name.slice(0,1) }}</div>
          <div>
            <div class="book-name" style="font-size:18px">{{ preview.book_name }}</div>
            <div class="muted small">{{ preview.care_recipient_name }} · 邀请人 {{ preview.inviter_name }}</div>
            <div class="badge doctor" style="margin-top:6px">{{ roleLabel(preview.role) }}</div>
          </div>
        </div>
        <p class="muted small" style="margin:12px 0">加入后可以查看用药安排、参与打卡，并在自己的通知渠道接收提醒。</p>
        <button class="btn btn-primary btn-block" :disabled="joining" @click="join">{{ joining ? '加入中…' : preview.already_member ? '进入用药本' : '确认加入' }}</button>
      </template>
    </template>
  </AuthShell>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AuthShell from '../components/AuthShell.vue'
import { inviteApi } from '../api/medcare'
import { useAuthStore } from '../stores/auth'
import { useBookStore } from '../stores/book'
import { useUIStore } from '../stores/ui'
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const book = useBookStore()
const ui = useUIStore()
const token = String(route.params.token || '')
const preview = ref<any>(null)
const loading = ref(false)
const joining = ref(false)
const error = ref('')
function roleLabel(role: string) { return role === 'manager' ? '监护人' : '成员' }
async function load() {
  if (!auth.isAuthenticated) return
  loading.value = true
  try {
    const res = await inviteApi.preview(token)
    if (res.data?.code !== 0) throw new Error(res.data?.message || '邀请无效')
    preview.value = res.data.data
  } catch (e: any) { error.value = e?.response?.data?.message || e?.message || '邀请无效或已过期' }
  finally { loading.value = false }
}
async function join() {
  if (!preview.value) return
  joining.value = true
  try {
    const res = await inviteApi.join(token)
    if (res.data?.code !== 0) throw new Error(res.data?.message || '加入失败')
    await book.fetchBooks()
    book.select(res.data.data.id)
    ui.showToast('已加入用药本')
    router.push('/today')
  } catch (e: any) { error.value = e?.response?.data?.message || e?.message || '加入失败' }
  finally { joining.value = false }
}
onMounted(load)
</script>

<style scoped>
.invite-box { display: flex; align-items: center; gap: 14px; padding: 16px; border-radius: 18px; background: var(--mc-primary-soft); }
</style>
