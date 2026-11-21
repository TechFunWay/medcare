import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import { useBookStore } from '../stores/book'
import { useUIStore } from '../stores/ui'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    { path: '/login', name: 'Login', component: () => import('../views/LoginView.vue'), meta: { public: true } },
    { path: '/register', name: 'Register', component: () => import('../views/RegisterView.vue'), meta: { public: true } },
    { path: '/join', name: 'Join', component: () => import('../views/JoinView.vue'), meta: { public: true } },
    { path: '/forgot-password', name: 'ForgotPassword', component: () => import('../views/ForgotPasswordView.vue'), meta: { public: true } },
    {
      path: '/',
      component: () => import('../layouts/AppLayout.vue'),
      children: [
        { path: '', redirect: '/today' },
        // meta.bookless：账号级页面，不依赖当前用药本，首次注册后也能进去
        // （用药本 / 我的 / 设置 / 通知）；其余模块都要带用药本才能取数。
        { path: 'books', name: 'Books', component: () => import('../views/BooksView.vue'), meta: { bookless: true } },
        { path: 'today', name: 'Today', component: () => import('../views/TodayView.vue'), meta: { perm: 'today:view' } },
        { path: 'plans', name: 'Plans', component: () => import('../views/PlansView.vue'), meta: { anyPerm: ['plan:view', 'today:view'] } },
        { path: 'plans/new', name: 'PlanNew', component: () => import('../views/PlanFormView.vue'), meta: { anyPerm: ['plan:create', 'plan:edit'] } },
        { path: 'plans/:id', name: 'PlanDetail', component: () => import('../views/PlanFormView.vue'), meta: { anyPerm: ['plan:view', 'plan:edit'] } },
        { path: 'medicines', name: 'Medicines', component: () => import('../views/MedicinesView.vue'), meta: { perm: 'medicine:view' } },
        { path: 'medicines/:id', name: 'MedicineDetail', component: () => import('../views/MedicineDetailView.vue'), meta: { perm: 'medicine:view' } },
        { path: 'stock', name: 'Stock', component: () => import('../views/StockMovementsView.vue'), meta: { perm: 'medicine:view' } },
        { path: 'stock/detail', name: 'StockDetail', component: () => import('../views/StockDetailView.vue'), meta: { perm: 'medicine:view' } },
        { path: 'audit', name: 'Audit', component: () => import('../views/AuditLogView.vue'), meta: { perm: 'stats:view' } },
        { path: 'locations', name: 'Locations', component: () => import('../views/LocationsView.vue'), meta: { perm: 'plan:view' } },
        { path: 'images', name: 'Images', component: () => import('../views/ImageLibraryView.vue'), meta: { perm: 'image:view' } },
        { path: 'prescriptions', name: 'Prescriptions', component: () => import('../views/PrescriptionsView.vue'), meta: { perm: 'prescription:view' } },
        { path: 'stats', name: 'Stats', component: () => import('../views/StatsView.vue'), meta: { perm: 'stats:view' } },
        { path: 'inbox', name: 'Inbox', component: () => import('../views/InboxView.vue'), meta: { bookless: true } },
        { path: 'profile', name: 'Profile', component: () => import('../views/ProfileView.vue'), meta: { bookless: true } },
        { path: 'settings', name: 'Settings', component: () => import('../views/SettingsView.vue'), meta: { bookless: true } },
        { path: 'admin/users', name: 'AdminUsers', component: () => import('../views/AdminUsersView.vue'), meta: { bookless: true, adminOnly: true } },
        { path: 'more', name: 'More', component: () => import('../views/MoreView.vue'), meta: { bookless: true } },
        { path: 'settings/channels', name: 'Channels', component: () => import('../views/ChannelsView.vue') },
      ],
    },
  ],
})

router.beforeEach(async (to) => {
  const auth = useAuthStore()
  await auth.init()
  // 库里还没有账号时必须先去注册（第一个账号即管理员），
  // 这条要排在 public 判断前面，否则直接打开 /login 会绕过首次初始化。
  if (auth.setupRequired) {
    if (to.name === 'Register') return true
    return { name: 'Register', query: to.name === 'Join' ? { redirect: to.fullPath } : {} }
  }
  if (to.meta.public) {
    if (auth.isAuthenticated && (to.name === 'Login' || to.name === 'Register')) return { name: 'Today' }
    return true
  }
  if (!auth.isAuthenticated) return { name: 'Login', query: { redirect: to.fullPath } }
  // 仅管理员可进的页面（用户管理）：非管理员直接挡回去并说明原因。
  if (to.meta.adminOnly && !auth.isAdmin) {
    useUIStore().showToast('只有管理员可以管理用户', 'info')
    return { name: 'Today' }
  }
  const book = useBookStore()
  if (!book.books.length) await book.fetchBooks()
  // 还没有当前用药本时，只放开账号级页面（用药本、我的、设置、通知）；
  // 其余模块要带 book_id 才能取数，挡回去必须说一句为什么，否则看着像菜单坏了。
  if (!book.currentId && !to.meta.bookless) {
    if (to.name !== 'Books') useUIStore().showToast('先创建或加入一个用药本，再进其它模块', 'info')
    return { name: 'Books' }
  }
  return true
})

export default router
