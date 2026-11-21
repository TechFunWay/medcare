<template>
  <div class="page">
    <div class="page-head">
      <div style="display:flex;gap:8px;align-items:flex-start">
        <button class="icon-btn" aria-label="返回" @click="router.back()"><AppIcon name="arrowLeft" /></button>
        <div><div class="eyebrow">全部功能</div><h1>更多</h1><p>不常放在底部导航里的功能都集中在这里，按用途分组。</p></div>
      </div>
    </div>

    <template v-for="group in groups" :key="group.title">
      <template v-if="group.items.length">
        <div class="section-head" style="margin-top:16px">
          <h2>{{ group.title }}</h2>
          <span v-if="group.hint" class="tiny muted">{{ group.hint }}</span>
        </div>
        <div class="list">
          <router-link v-for="item in group.items" :key="item.to" :to="item.to" class="card link-row">
            <span class="link-icon"><AppIcon :name="item.icon" :size="20" /></span>
            <span class="link-body"><b>{{ item.label }}</b><span class="tiny muted">{{ item.desc }}</span></span>
            <AppIcon name="chevron" :size="17" class="link-arrow" />
          </router-link>
        </div>
      </template>
    </template>

    <div class="card about" style="margin-top:16px">
      <div class="tiny muted" style="line-height:1.7">
        用药管理 {{ version }}<br />
        本应用只帮助记录和提醒，不提供诊断、用药建议或剂量推荐，请遵医嘱。
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import AppIcon from '../components/AppIcon.vue'
import { useAuthStore } from '../stores/auth'
import { useBookStore } from '../stores/book'
import request from '../api/request'

const router = useRouter()
const auth = useAuthStore()
const book = useBookStore()
const version = ref('')

type Item = { to: string; label: string; desc: string; icon: string; perm?: string; anyPerm?: string[]; adminOnly?: boolean }

const allItems: { title: string; hint?: string; items: Item[] }[] = [
  {
    title: '常用',
    hint: '每天会用到',
    items: [
      { to: '/books', label: '用药本与成员', desc: '切换、邀请、角色与权限', icon: 'book' },
      { to: '/prescriptions', label: '处方识别历史', desc: 'AI 识别记录与重新核对', icon: 'prescription', perm: 'prescription:view' },
      { to: '/images', label: '图片管理', desc: '药品照片、处方单、检查报告', icon: 'camera', perm: 'image:view' },
      { to: '/settings/channels', label: '通知渠道', desc: '邮件、钉钉、飞书提醒', icon: 'bell' },
    ],
  },
  {
    title: '管理员',
    hint: '仅管理员',
    items: [
      { to: '/admin/users', label: '用户管理', desc: '新增、改角色、重置密码、禁用或删除账号', icon: 'user', adminOnly: true },
    ],
  },
  {
    title: '工具与记录',
    items: [
      { to: '/locations', label: '地点库', desc: '医院、诊所、居家地址', icon: 'location', perm: 'plan:view' },
      { to: '/stats', label: '统计', desc: '用药完成率、库存与趋势', icon: 'chart', perm: 'stats:view' },
      { to: '/audit', label: '操作日志', desc: '谁在什么时候做了什么', icon: 'clock', perm: 'stats:view' },
    ],
  },
  {
    title: '设置',
    items: [
      { to: '/settings', label: '偏好设置', desc: '提醒节奏、主题、AI、邮件', icon: 'settings' },
    ],
  },
]

function canSee(item: Item) {
  if (item.adminOnly && !auth.isAdmin) return false
  if (item.anyPerm?.length) return item.anyPerm.some((p) => book.can(p))
  if (item.perm) return book.can(item.perm)
  return true
}

const groups = computed(() => allItems.map((g) => ({ ...g, items: g.items.filter(canSee) })))

onMounted(async () => {
  try {
    const res = await request.get('/api/version')
    const v = String(res.data?.data?.version || '')
    const t = String(res.data?.data?.buildTime || '')
    // 开发构建会返回 dev/空值，别在页面上显示「dev · unknown」。
    if (v && v !== 'dev') version.value = v + (t && t !== 'unknown' ? ' · ' + t.slice(0, 10) : '')
  } catch { /* 版本拿不到就不显示，不影响使用 */ }
})
</script>

<style scoped>
.link-row { display: flex; align-items: center; gap: 10px; padding: 11px 12px; }
.link-icon { flex: 0 0 auto; width: 36px; height: 36px; border-radius: 12px; background: var(--mc-primary-soft); color: var(--mc-primary-strong); display: grid; place-items: center; }
.link-body { flex: 1 1 auto; min-width: 0; display: grid; gap: 2px; }
.link-body b { font-size: 14px; overflow-wrap: anywhere; }
.link-arrow { flex: 0 0 auto; color: var(--mc-text-muted); }
.about { padding: 12px; }
@media (min-width: 900px) {
  .link-row { padding: 13px 15px; }
  .link-body b { font-size: 15px; }
}
</style>
