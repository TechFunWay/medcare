<template>
  <div class="page narrow">
    <section class="card empty">
      <div class="empty-icon"><AppIcon name="lock" :size="30" /></div>
      <h3>没有「{{ label }}」的权限</h3>
      <p class="why">
        用药管理是逐格授权的：管理员在「用药本 → 成员与权限 → 设置权限」里，
        对每个人分别勾选能看哪几块、能动哪几块，勾了才有。
      </p>
      <p class="mine">你在「{{ book.current?.name || '本用药本' }}」里能用的是：<b>{{ mineText }}</b></p>
      <div class="empty-actions">
        <router-link :to="homeRoute" class="btn btn-primary">去能看的地方</router-link>
        <router-link to="/books" class="btn btn-outline">我的用药本</router-link>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
// 没有那一格权限时，页面根本不挂载（路由 meta 挡住），这里只负责说清楚「缺哪格、找谁勾」。
import { computed } from 'vue'
import AppIcon from '../components/AppIcon.vue'
import { cellLabel, shortCaps } from '../utils/permissions'
import { useBookStore } from '../stores/book'

const props = defineProps<{ perm: string }>()
const book = useBookStore()

const label = computed(() => cellLabel(props.perm) || '相应权限')
const mineText = computed(() => shortCaps(book.permissionKeys).join('、') || '暂时什么都没开放，请联系管理员')
// 「回今日」可能被同一格权限挡住，所以挑第一个真正能进的页面。
const homeRoute = computed(() => {
  if (book.canView('today')) return '/today'
  if (book.canView('medicine')) return '/medicines'
  if (book.canView('plan')) return '/plans'
  if (book.canView('image')) return '/images'
  if (book.canView('prescription')) return '/prescriptions'
  if (book.canView('stats')) return '/stats'
  return '/books'
})
</script>

<style scoped>
.why, .mine { margin-top: 6px; }
.mine b { color: var(--mc-text); font-weight: 700; }
</style>
