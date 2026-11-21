<template>
  <div class="page narrow">
    <div class="page-head"><div><div class="eyebrow">消息</div><h1>通知中心</h1><p>站内提醒与投递记录。</p></div><button class="btn btn-outline" @click="readAll">全部已读</button></div>
    <div class="tabs" style="margin-bottom:10px"><button class="tab" :class="{ active: tab === 'notes' }" @click="tab = 'notes'">站内通知</button><button class="tab" :class="{ active: tab === 'deliveries' }" @click="tab = 'deliveries'; loadDeliveries()">投递记录</button></div>
    <div v-if="loading" class="card muted">加载中…</div>
    <template v-else>
      <div v-if="tab === 'notes'">
        <div v-if="!notes.length" class="empty card"><div class="empty-icon"><AppIcon name="bell" :size="30" /></div><h3>这里很安静</h3><p>到点提醒、逾期提醒和每日总结会出现在这里。</p></div>
        <div v-else class="list">
          <article v-for="n in notes" :key="n.id" class="event-card" :style="!n.read_at ? 'border-color:var(--mc-primary-soft)' : ''" @click="read(n)">
            <div class="type-dot"><AppIcon name="bell" :size="18" /></div>
            <div class="event-main"><div class="event-title-row"><span class="event-title">{{ n.title }}</span><span v-if="!n.read_at" class="badge done">未读</span></div><div class="event-meta" style="overflow-wrap:anywhere">{{ n.body }}</div><div class="tiny muted" style="margin-top:3px">{{ n.book_name }} · {{ formatDate(n.created_at) }}</div></div>
          </article>
        </div>
      </div>
      <div v-else>
        <div v-if="!deliveries.length" class="card muted">暂无投递记录</div>
        <div v-else class="list">
          <div v-for="d in deliveries" :key="d.id" class="card delivery-card">
            <div class="delivery-head"><b>{{ channelLabel(d.channel_type) }}</b><span class="badge" :class="d.status === 'sent' ? 'done' : d.status === 'failed' ? 'missed' : 'pending'">{{ statusLabel(d.status) }}</span><span class="tiny muted delivery-time">{{ formatDate(d.run_at) }}</span></div>
            <div class="tiny muted" style="margin-top:3px">{{ d.book_name }} · {{ kindLabel(d.kind) }} · 尝试 {{ d.attempts }} 次</div>
            <div v-if="d.last_error" class="tiny delivery-error">{{ d.last_error }}</div>
            <button v-if="d.status === 'failed'" class="btn btn-outline btn-xs" style="margin-top:6px" @click="retry(d)">重试</button>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import AppIcon from '../components/AppIcon.vue'
import { notificationApi } from '../api/medcare'
import { useNotifyStore } from '../stores/notify'
import { useBookStore } from '../stores/book'
import { useUIStore } from '../stores/ui'
import { reportError } from '../utils/errors'
const book = useBookStore()
const ui = useUIStore()
const notify = useNotifyStore()
const tab = ref('notes')
const notes = ref<any[]>([])
const deliveries = ref<any[]>([])
const loading = ref(true)
import { formatDateTime as formatDate } from '../utils/date'
function channelLabel(t: string) { return t === 'inapp' ? '站内' : t === 'email' ? '邮件' : t === 'dingtalk' ? '钉钉' : t === 'feishu' ? '飞书' : t }
function statusLabel(s: string) { return s === 'sent' ? '已发送' : s === 'failed' ? '失败' : s === 'retry' ? '重试中' : s === 'canceled' ? '已取消' : '待发送' }
function kindLabel(k: string) { return k === 'lead' ? '提前提醒' : k === 'due' ? '到点提醒' : k === 'overdue_1' || k === 'overdue_2' ? '逾期提醒' : k === 'daily_summary' ? '每日总结' : k }
async function load() {
  loading.value = true
  try { const res = await notificationApi.list({ pageSize: 100 }); notes.value = res.data?.data?.items || [] } catch (e: any) { ui.showToast(e?.response?.data?.message || '读取通知失败', 'error') } finally { loading.value = false }
}
async function loadDeliveries() { try { const res = await notificationApi.deliveries({ pageSize: 100 }); deliveries.value = res.data?.data?.items || [] } catch (e) { reportError(e, '读取投递记录失败') } }
async function read(n: any) { if (n.read_at) return; try { await notificationApi.read(n.id); n.read_at = new Date().toISOString(); notify.markRead() } catch (e) { reportError(e, '标记已读失败') } }
async function readAll() { try { await notificationApi.readAll(); notes.value.forEach((n) => n.read_at = new Date().toISOString()); notify.markAllRead(); ui.showToast('已全部已读') } catch (e) { reportError(e, '全部标记已读失败') } }
async function retry(d: any) { try { await notificationApi.retry(d.id); ui.showToast('已重新排队'); loadDeliveries() } catch (e: any) { ui.showToast(e?.response?.data?.message || '重试失败', 'error') } }
onMounted(async () => { await load(); await loadDeliveries(); void notify.refresh() })
</script>

<style scoped>
.delivery-card { padding: 10px 12px; }
.delivery-head { display: flex; gap: 6px; align-items: center; min-width: 0; }
.delivery-head b { min-width: 0; overflow-wrap: anywhere; }
.delivery-time { margin-left: auto; flex: 0 0 auto; }
.delivery-error { color: var(--mc-danger); margin-top: 3px; overflow-wrap: anywhere; }
@media (min-width: 900px) {
  .delivery-card { padding: 12px 14px; }
  .delivery-head { gap: 8px; }
  .delivery-error { margin-top: 4px; }
}
</style>
