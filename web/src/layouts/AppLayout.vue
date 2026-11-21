<template>
  <div class="app-bg">
    <div class="app-shell">
      <aside class="sidebar">
        <div class="brand">
          <div class="brand-mark">药</div>
          <div>
            <div class="brand-name">用药管理</div>
            <div class="tiny muted">家庭用药本</div>
          </div>
        </div>
        <nav class="list">
          <!-- 与底栏同一套权限过滤：没有那一格权限就不显示入口，免得点了被挡回来 -->
          <router-link v-for="item in visibleNavItems" :key="item.to" :to="item.to" class="nav-link">
            <AppIcon :name="item.icon" :size="19" />{{ item.label }}
          </router-link>
        </nav>
        <!-- 还没有当前用药本时，业务模块都进不去，这里直接给一条去建本的提示 -->
        <div v-if="!book.currentId" class="nav-need-book">
          <AppIcon name="book" :size="17" />
          <span>还没有用药本</span>
          <router-link to="/books">{{ book.books.length ? '选一本' : '先建一本' }}</router-link>
        </div>
        <div style="margin-top:auto;padding-top:24px">
          <router-link to="/settings" class="nav-link"><AppIcon name="settings" :size="19" />设置</router-link>
        </div>
      </aside>

      <div class="main-wrap">
        <header class="topbar">
          <div class="topbar-inner">
            <button class="book-switch" @click="showBooks = true">
              <div class="name">{{ book.current?.name || '选择用药本' }}</div>
              <div class="sub">{{ book.current?.care_recipient_name || '创建或加入一个用药本' }}</div>
            </button>
            <button class="icon-btn" :aria-label="'切换主题（当前：' + theme.modeLabel + '）'" @click="toggleTheme">
              <AppIcon :name="theme.isDark ? 'moon' : 'sun'" :size="20" />
            </button>
            <router-link to="/inbox" class="icon-btn" aria-label="通知">
              <AppIcon name="bell" :size="21" />
              <span v-if="unread > 0" class="unread-dot">{{ unread > 9 ? '9+' : unread }}</span>
            </router-link>
            <router-link to="/more" class="icon-btn more-trigger" :class="{ active: route.path === '/more' }" aria-label="更多功能">
              <AppIcon name="more" :size="21" />
              <span class="more-trigger-label">更多</span>
            </router-link>
            <router-link to="/profile" class="avatar" aria-label="我的">{{ auth.user?.username?.slice(0, 1) || '我' }}</router-link>
          </div>
        </header>

        <div v-if="support.bannerVisible" class="support-banner">
          <span class="support-banner-text">❤ 如果这个应用对你有帮助，欢迎 <button class="support-banner-link" @click="support.show = true">请作者喝杯咖啡</button>（不赞赏不影响任何功能）。</span>
          <button class="support-banner-close" aria-label="关闭支持提示" @click="support.dismissBanner()"><AppIcon name="close" :size="15" /></button>
        </div>

        <main>
          <NoPermission v-if="blockedPerm" :perm="blockedPerm" />
          <router-view v-else />
        </main>
      </div>
    </div>

    <nav class="bottom-nav">
      <router-link v-for="item in bottomNavItems" :key="item.to" :to="item.to" class="nav-item">
        <AppIcon :name="item.icon" :size="21" />
        <span>{{ item.label }}</span>
      </router-link>
    </nav>

    <div v-if="scanPill" class="scan-pill" :class="{ done: scanPill.status === 'done' }">
      <button type="button" class="scan-pill-main" @click="openScanPill">
        <span v-if="scanPill.status === 'running'" class="scan-pill-spinner" />
        <AppIcon v-else name="check" :size="15" />
        <span>{{ scanPill.status === 'running' ? 'AI 识别中 · ' + pillElapsed : '识别完成 · 去核对' }}</span>
      </button>
      <button type="button" class="scan-pill-x" aria-label="忽略该识别任务" @click="dismissScanPill"><AppIcon name="close" :size="14" /></button>
    </div>

    <SupportModal />

    <button v-if="showFab" class="fab" aria-label="快捷新建" @click="showActions = true"><AppIcon name="plus" :size="26" /></button>

    <Teleport to="body">
      <div v-if="showBooks" class="sheet-backdrop" @click.self="showBooks = false">
        <section class="sheet" role="dialog" aria-modal="true">
          <div class="sheet-grab" />
          <div class="sheet-head">
            <div><div class="eyebrow">用药本</div><h2>切换用药本</h2></div>
            <button class="icon-btn" aria-label="关闭" @click="showBooks = false"><AppIcon name="close" /></button>
          </div>
          <div class="sheet-body list">
            <button v-for="b in book.books" :key="b.id" class="book-card card" style="text-align:left;width:100%" @click="switchBook(b.id)">
              <div class="book-icon">{{ b.care_recipient_name.slice(0, 1) }}</div>
              <div class="book-meta">
                <div class="book-name">{{ b.name }} <span v-if="b.id === book.currentId" class="badge done">当前</span></div>
                <div class="book-sub">{{ b.care_recipient_name }} · {{ roleLabel(b.role) }} · {{ b.member_count || 1 }} 位成员</div>
              </div>
              <AppIcon name="chevron" :size="18" />
            </button>
            <button class="btn btn-primary btn-block" @click="goCreateBook"><AppIcon name="plus" :size="18" />新建用药本</button>
            <button class="btn btn-outline btn-block" @click="goBooks">管理用药本 / 邀请成员</button>
          </div>
        </section>
      </div>

      <div v-if="showActions" class="sheet-backdrop" @click.self="showActions = false">
        <section class="sheet" role="dialog" aria-modal="true">
          <div class="sheet-grab" />
          <div class="sheet-head"><h2>{{ canCreate ? '新建' : '新建用药本' }}</h2><button class="icon-btn" aria-label="关闭" @click="showActions = false"><AppIcon name="close" /></button></div>
          <div class="sheet-body list">
            <template v-if="canCreate">
              <button v-if="book.canPlanCreate" class="btn btn-block btn-outline" @click="go('/plans/new')"><AppIcon name="calendar" :size="18" />新建用药计划</button>
              <button v-if="book.canPrescCreate" class="btn btn-block btn-outline" @click="go('/prescriptions')"><AppIcon name="camera" :size="18" />AI 识别处方单</button>
              <button v-if="book.canMedicineCreate" class="btn btn-block btn-outline" @click="go('/medicines')"><AppIcon name="pill" :size="18" />添加药品 / 入库</button>
            </template>
            <button class="btn btn-block btn-outline" @click="go('/books')"><AppIcon name="book" :size="18" />新建用药本</button>
          </div>
        </section>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppIcon from '../components/AppIcon.vue'
import NoPermission from '../components/NoPermission.vue'
import SupportModal from '../components/SupportModal.vue'
import { useSupportStore } from '../stores/support'
import { useScanStore } from '../stores/scan'
import { useAuthStore } from '../stores/auth'
import { useBookStore } from '../stores/book'
import { useUIStore } from '../stores/ui'
import { logOnly } from '../utils/errors'
import { useNotifyStore } from '../stores/notify'
import { useThemeStore } from '../stores/theme'
import { notificationApi, preferenceApi } from '../api/medcare'

const auth = useAuthStore()
const support = useSupportStore()
const scan = useScanStore()
const book = useBookStore()
const ui = useUIStore()
const theme = useThemeStore()
const router = useRouter()
const route = useRoute()
const showBooks = ref(false)
const showActions = ref(false)
const fabRoutes = ['Today', 'Plans', 'Medicines']
// 能不能建东西取决于管理员给的能力：计划管理或药品管理有其一才显示新建入口。
const canCreate = computed(() => book.canPlanCreate || book.canMedicineCreate || book.canPrescCreate)
const showFab = computed(() => {
  const name = String(route.name || '')
  if (!fabRoutes.includes(name)) return false
  if (canCreate.value) return true
  return name === 'Today'
})
// 未读数放进共享 store：通知中心标记已读后顶栏红点立即变化。
const notify = useNotifyStore()
const unread = computed(() => notify.unread)
let timer: number | null = null
// 后台 AI 识别胶囊：弹层关着且有「在跑 / 跑完没核对」的任务时挂在左下角，
// 点击回药品页打开核对弹层；× 忽略该任务（记录仍留在处方识别历史）。
const scanPill = computed(() => (book.currentId ? scan.pillJob(book.currentId) : null))
// 胶囊上带已等秒数：后台识别久的时候用户能知道它还在跑、跑了多久。
const pillNow = ref(Date.now())
let pillTimer: number | null = null
watch(() => scanPill.value?.status, (s) => {
  if (s === 'running' && pillTimer === null) pillTimer = window.setInterval(() => { pillNow.value = Date.now() }, 1000)
  else if (s !== 'running' && pillTimer !== null) { window.clearInterval(pillTimer); pillTimer = null }
}, { immediate: true })
const pillElapsed = computed(() => {
  const j = scanPill.value
  if (!j) return ''
  const s = Math.max(0, Math.round((pillNow.value - j.startedAt) / 1000))
  return s < 60 ? s + 's' : Math.floor(s / 60) + 'm' + (s % 60) + 's'
})
function openScanPill() {
  scan.requestOpen = true
  // 库存页发起的识别要回库存页核对（那里才能入库），药品页发起的回药品页。
  router.push(scanPill.value?.mode === 'stock' ? '/stock' : '/medicines')
}
function dismissScanPill() { if (scanPill.value) scan.clear(scanPill.value.id) }
// 进本 / 切本 / 刷新后把服务端还在识别中的任务接回来，关 App 再开也不丢。
watch(() => book.currentId, (id) => { if (id) void scan.resume(id) }, { immediate: true })

// 底部与侧栏顺序：药品在最左，今日居中，符合「先看药、再看今天要吃什么」的习惯。
// perm / anyPerm 是该入口对应的权限格；管理员没勾的入口直接不显示（路由也会挡住）。
interface NavItem { to: string; label: string; icon: string; perm?: string; anyPerm?: string[] }
const navItems: NavItem[] = [
  { to: '/medicines', label: '药品', icon: 'pill', perm: 'medicine:view' },
  { to: '/stock', label: '库存', icon: 'filter', perm: 'medicine:view' },
  { to: '/plans', label: '计划', icon: 'calendar', anyPerm: ['plan:view', 'today:view'] },
  { to: '/today', label: '今日', icon: 'home', perm: 'today:view' },
  { to: '/stats', label: '统计', icon: 'chart', perm: 'stats:view' },
  { to: '/profile', label: '我的', icon: 'user' },
]
function visible(item: NavItem): boolean {
  if (item.anyPerm) return item.anyPerm.some((key) => book.can(key))
  return !item.perm || book.can(item.perm)
}
const visibleNavItems = computed(() => navItems.filter(visible))
// 手机底部只放 5 个最高频入口；「库存」在侧栏与「更多」里，避免底部挤成 6 个。
const bottomPaths = ['/medicines', '/stock', '/today', '/plans']
// 底部 5 项：药品 / 库存 / 今日 / 计划 / 我的（「今日」居中，计划紧随其后）。
// 「更多」功能收进「我的 → 更多功能」，顶栏的「⋯ 更多」也仍在，底部不再放它。
const bottomNavItems = computed(() => [
  ...visibleNavItems.value.filter((i) => bottomPaths.includes(i.to)).sort((a, b) => bottomPaths.indexOf(a.to) - bottomPaths.indexOf(b.to)),
  ...visibleNavItems.value.filter((i) => i.to === '/profile'),
])
// 直接敲网址或从旧链接进来时，也按当前用药本的权限挡住，页面根本不发请求。
const blockedPerm = computed<string | null>(() => {
  const meta = route.meta as { perm?: string; anyPerm?: string[] }
  if (meta.anyPerm?.length) return meta.anyPerm.some((key) => book.can(key)) ? null : meta.anyPerm[0]
  if (meta.perm) return book.can(meta.perm) ? null : meta.perm
  return null
})

function roleLabel(role: string) { return role === 'owner' ? '管理员' : role === 'manager' ? '监护人' : '成员' }
function switchBook(id: number) { book.select(id); showBooks.value = false; router.push('/today') }
function go(path: string) { showActions.value = false; router.push(path) }
function goCreateBook() { showBooks.value = false; router.push('/books?create=1') }
function goBooks() { showBooks.value = false; router.push('/books') }
function loadUnread() {
  // 未读数是 30 秒一次的后台轮询，失败只记日志、由 store 统一处理。
  void notify.refresh()
}
async function toggleTheme() {
  theme.cycleMode()
  ui.showToast('主题：' + theme.modeLabel)
  await theme.persist()
}
onMounted(async () => {
  if (!book.books.length) await book.fetchBooks()
  await loadUnread()
  try {
    const res = await preferenceApi.get()
    const pref = res.data?.data
    if (pref) {
      theme.hydrate(pref.theme_mode, pref.theme_brand)
      book.applyDefault(pref.last_book_id)
    }
  } catch (e) { logOnly('启动时读取偏好', e) }
  timer = window.setInterval(loadUnread, 30000)
})
onUnmounted(() => { if (timer) window.clearInterval(timer); if (pillTimer !== null) window.clearInterval(pillTimer) })
// 赞赏提示只对管理员展示，且要等有了用药本、真正进入应用后才弹：
// 首次注册后的引导页（还没有任何用药本）不打扰用户。
watch([() => book.books.length, () => route.name, () => auth.isAdmin], () => {
  if (book.books.length > 0 && auth.isAdmin) support.init(true)
}, { immediate: true })
watch(() => route.fullPath, () => { showBooks.value = false; showActions.value = false; loadUnread() })
</script>

<style scoped>
/* 支持提示条：一句话就该只有一行高。通用 .icon-btn 是 44×44，会把整条撑到 62px，
   所以这里用专属小关闭按钮（24px 视觉），触控热区靠 ::after 外扩到 44px，不占布局高度。 */
.support-banner { display: flex; align-items: center; gap: 6px; margin: 6px 14px 0; padding: 4px 10px; border-radius: 10px; border: 1px solid rgba(195,146,63,.4); background: var(--mc-warning-soft); color: var(--mc-text); font-size: 12px; line-height: 1.45; }
.support-banner-text { flex: 1; min-width: 0; }
/* 手机端支持横幅压成一行，超出省略，不占两行高度 */
@media (max-width: 560px) {
  .support-banner { margin: 4px 12px 0; padding: 2px 8px; border-radius: 8px; }
  .support-banner-text { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .support-banner-close { width: 20px; height: 20px; }
}
/* 句子里的文字链接：按钮即使写 display:inline 仍是原子行内盒，纵向内边距照样撑高行盒，
   所以纵向不留内边距，触控热区交给 ::after 绝对定位外扩（不参与布局）。 */
.support-banner-link { position: relative; display: inline-block; border: 0; background: transparent; padding: 0 4px; margin: 0 -4px; color: var(--mc-primary-strong); font-weight: 700; font-size: 12px; line-height: inherit; text-decoration: underline; text-underline-offset: 2px; }
.support-banner-link::after { content: ''; position: absolute; inset: -13px -4px; }
.support-banner-close { position: relative; flex: 0 0 auto; display: inline-flex; align-items: center; justify-content: center; width: 24px; height: 24px; border: 0; border-radius: 8px; background: transparent; color: var(--mc-text-muted); }
.support-banner-close::after { content: ''; position: absolute; inset: -10px; }
.support-banner-close:hover { background: var(--mc-surface-2); color: var(--mc-text); }
.unread-dot { position: absolute; margin-left: 18px; margin-top: -24px; min-width: 17px; height: 17px; border-radius: 999px; background: var(--mc-danger); color: #fff; font-size: 10px; font-weight: 800; display: grid; place-items: center; padding: 0 4px; }
.avatar { width: 38px; height: 38px; border-radius: 50%; background: var(--mc-primary-soft); color: var(--mc-primary-strong); font-weight: 800; display: grid; place-items: center; }
.more-trigger { width: auto; gap: 4px; padding: 0 8px; }
.more-trigger.active { color: var(--mc-primary-strong); background: var(--mc-primary-soft); }
.more-trigger-label { font-size: 12.5px; font-weight: 700; }
@media (max-width: 420px) { .more-trigger-label { display: none; } }
.nav-need-book { display: flex; align-items: center; gap: 6px; margin-top: 10px; padding: 10px 12px; border: 1px dashed var(--mc-border); border-radius: 12px; color: var(--mc-text-muted); font-size: 12.5px; line-height: 1.5; }
.nav-need-book a { color: var(--mc-primary-strong); font-weight: 750; text-decoration: underline; text-underline-offset: 2px; }
.scan-pill { position: fixed; left: 12px; bottom: calc(64px + env(safe-area-inset-bottom)); z-index: 60; display: flex; align-items: center; gap: 2px; padding: 2px 4px 2px 14px; border-radius: 999px; background: var(--mc-surface); border: 1px solid var(--mc-border); box-shadow: var(--mc-shadow); }
.scan-pill-main { display: inline-flex; align-items: center; gap: 7px; min-height: 44px; border: 0; background: none; color: var(--mc-text); font-size: 12.5px; font-weight: 700; }
.scan-pill.done .scan-pill-main { color: var(--mc-primary-strong); }
.scan-pill-x { display: grid; place-items: center; width: 44px; height: 44px; border: 0; border-radius: 50%; background: none; color: var(--mc-text-muted); }
.scan-pill-spinner { width: 14px; height: 14px; border-radius: 50%; border: 2px solid var(--mc-primary-soft); border-top-color: var(--mc-primary); animation: mc-pill-spin .9s linear infinite; }
@keyframes mc-pill-spin { to { transform: rotate(360deg); } }
@media (min-width: 900px) { .scan-pill { left: 244px; bottom: 24px; } }
.more-menu { position: fixed; top: calc(50px + env(safe-area-inset-top)); right: 12px; z-index: 65; width: min(310px, calc(100vw - 24px)); padding: 12px; border: 1px solid var(--mc-border); border-radius: 14px; background: var(--mc-surface); box-shadow: 0 16px 40px rgba(30, 45, 38, .18); }
.more-menu-head { display: flex; align-items: baseline; justify-content: space-between; margin-bottom: 9px; }
.more-menu-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 7px; }
.more-item { display: flex; align-items: center; gap: 8px; padding: 9px 10px; border-radius: 10px; color: var(--mc-text); font-size: 13px; font-weight: 650; }
.more-item:active, .more-item.router-link-active { background: var(--mc-primary-soft); color: var(--mc-primary-strong); }
.more-icon { width: 26px; height: 26px; border-radius: 8px; background: var(--mc-surface-2); display: grid; place-items: center; color: var(--mc-primary-strong); flex: 0 0 auto; }
@media (min-width: 900px) { .more-menu { top: 56px; right: max(16px, calc((100vw - 980px) / 2)); } }
</style>
