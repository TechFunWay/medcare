<template>
  <div class="section">
    <div class="section-head">
      <h2>用药计划</h2>
      <div class="head-links">
        <button class="link-btn" @click="router.push('/plans')">全部计划</button>
        <button v-if="book.canPlanCreate && book.canPlanView" class="btn btn-primary btn-xs" @click="router.push('/plans/new?medicine_id=' + medicineId)">
          <AppIcon name="plus" :size="14" />新建
        </button>
      </div>
    </div>

    <div v-if="loading" class="card muted">计划加载中…</div>
    <div v-else-if="!plans.length" class="card muted plan-empty">
      <span>这个药品还没有用药计划。</span>
      <button v-if="book.canPlanCreate && book.canPlanView" class="link-btn" @click="router.push('/plans/new?medicine_id=' + medicineId)">去建一个</button>
    </div>
    <div v-else class="list">
      <router-link v-for="p in plans" :key="p.id" :to="'/plans/' + p.id" class="card plan-row">
        <div class="plan-body">
          <div class="plan-title"><b>{{ p.title || '用药计划' }}</b><span class="badge" :class="planStatusBadge(p.status)">{{ planStatusLabel(p.status) }}</span></div>
          <div class="tiny muted plan-meta">
            <span>{{ p.dose_amount }} {{ p.dose_unit }}</span><span class="dot">·</span>
            <span>{{ frequencyLabel(p) }}</span>
          </div>
          <div class="tiny muted plan-meta">
            <span>{{ p.start_date }} 起<template v-if="p.end_date"> · {{ p.end_date }} 止</template></span>
            <template v-if="planProgressText(p)"><span class="dot">·</span><span>{{ planProgressText(p) }}</span></template>
            <template v-if="p.next_event_at"><span class="dot">·</span><span>下次 {{ formatDateTime(p.next_event_at) }}</span></template>
          </div>
        </div>
        <AppIcon name="chevron" :size="18" class="plan-chevron" />
      </router-link>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import AppIcon from './AppIcon.vue'
import { planApi, type Plan } from '../api/medcare'
import { frequencyLabel, planProgressText, planStatusBadge, planStatusLabel } from '../utils/plan'
import { useBookStore } from '../stores/book'
import { useUIStore } from '../stores/ui'

const props = defineProps<{ medicineId: number }>()
const router = useRouter()
const book = useBookStore()
const ui = useUIStore()
const plans = ref<Plan[]>([])
const loading = ref(false)

// 后端的计划时间统一按北京时间返回，直接按该时区格式化，避免设备时区差异。
function formatDateTime(v: string) {
  const d = new Date(v)
  if (Number.isNaN(d.getTime())) return v
  return d.toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' })
}

async function load() {
  if (!props.medicineId || !book.currentId || !book.canPlanView) return
  loading.value = true
  try {
    const res = await planApi.list(book.currentId, { medicine_id: props.medicineId, pageSize: 100 })
    plans.value = (res.data?.data?.items || []) as Plan[]
  } catch (e: any) {
    ui.showToast(e?.response?.data?.message || '读取用药计划失败', 'error')
  } finally { loading.value = false }
}

onMounted(load)
watch(() => [props.medicineId, book.currentId], load)
</script>

<style scoped>
.head-links { display: flex; align-items: center; gap: 10px; }
.link-btn { border: 0; background: transparent; color: var(--mc-primary-strong); font-size: 12.5px; font-weight: 700; padding: 4px 2px; }
.plan-empty { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.plan-row { display: flex; align-items: center; gap: 10px; padding: 10px 12px; }
.plan-body { min-width: 0; flex: 1 1 auto; }
.plan-title { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.plan-title b { font-size: 14px; overflow-wrap: anywhere; }
.plan-meta { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 3px; }
.plan-meta .dot { color: var(--mc-border); }
.plan-chevron { flex: 0 0 auto; color: var(--mc-text-muted); }
@media (min-width: 900px) { .plan-row { padding: 12px 15px; } .plan-title b { font-size: 15px; } }
</style>
