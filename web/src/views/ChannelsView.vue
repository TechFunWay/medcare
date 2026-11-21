<template>
  <div class="page narrow">
    <div class="page-head"><div><div class="eyebrow">通知</div><h1>通知渠道</h1><p>每个成员配置自己的渠道，互不共享密钥。</p></div><button class="btn btn-primary" @click="openAdd"><AppIcon name="plus" :size="18" />添加</button></div>
    <div v-if="loading" class="card muted">加载中…</div>
    <div v-else-if="!channels.length" class="empty card"><div class="empty-icon"><AppIcon name="bell" :size="30" /></div><h3>还没有通知渠道</h3><p>添加邮箱、钉钉群机器人或飞书群机器人，到点才会收到提醒。</p><button class="btn btn-primary" style="margin-top:12px" @click="openAdd">添加渠道</button></div>
    <div v-else class="list">
      <article v-for="ch in channels" :key="ch.id" class="card ch-card">
        <div class="ch-main">
          <div class="type-dot"><AppIcon :name="ch.type === 'email' ? 'bell' : 'members'" :size="18" /></div>
          <div class="ch-body"><div class="ch-title-row"><b>{{ ch.name }}</b><span class="badge" :class="ch.status === 'active' ? 'done' : 'missed'">{{ ch.status === 'active' ? '已启用' : '异常' }}</span></div><div class="tiny muted" style="margin-top:2px;overflow-wrap:anywhere">{{ typeLabel(ch.type) }} · {{ ch.target_masked }}</div><div v-if="ch.last_error_code" class="tiny" style="color:var(--mc-danger);margin-top:2px">{{ ch.last_error_code }}</div></div>
          <button class="icon-btn ch-more" aria-label="更多" @click="openMenu(ch)"><AppIcon name="chevron" :size="18" /></button>
        </div>
        <div v-if="menuId === ch.id" class="ch-actions">
          <button class="btn btn-outline btn-xs" @click="test(ch)">发送测试</button>
          <button class="btn btn-danger btn-xs" @click="remove(ch)">删除</button>
        </div>
      </article>
    </div>
    <div class="card" style="margin-top:12px"><div class="tiny muted" style="line-height:1.7">钉钉/飞书请使用群自定义机器人 Webhook；QQ 不在本应用范围内。密钥加密保存，接口不会回显。</div></div>

    <Teleport to="body">
      <div v-if="show" class="sheet-backdrop" @click.self="show=false">
        <section class="sheet"><div class="sheet-grab" /><div class="sheet-head"><h2>添加通知渠道</h2><button class="icon-btn" @click="show=false"><AppIcon name="close" /></button></div>
          <form class="sheet-body" @submit.prevent="save">
            <div class="field"><label>渠道类型</label><div class="chip-row"><button type="button" class="chip" :class="{ active: type === 'email' }" @click="type = 'email'">邮件</button><button type="button" class="chip" :class="{ active: type === 'dingtalk' }" @click="type = 'dingtalk'">钉钉</button><button type="button" class="chip" :class="{ active: type === 'feishu' }" @click="type = 'feishu'">飞书</button></div></div>
            <div class="field"><label>名称</label><input v-model="form.name" class="input" required placeholder="例如：家庭群 / 我的邮箱" /></div>
            <template v-if="type === 'email'"><div class="field"><label>收件邮箱</label><input v-model="form.address" type="email" class="input" required /></div></template>
            <template v-if="type === 'dingtalk'"><div class="field"><label>钉钉 Webhook</label><input v-model="form.webhook" class="input" required placeholder="https://oapi.dingtalk.com/robot/send?access_token=…" /></div><div class="field"><label>自定义关键词（可选）</label><input v-model="form.keyword" class="input" placeholder="机器人安全设置里的关键词，如 yygl" /></div><div class="field"><label>加签密钥（可选）</label><input v-model="form.secret" class="input" placeholder="SEC..." /></div></template>
            <template v-if="type === 'feishu'"><div class="field"><label>飞书 Webhook</label><input v-model="form.webhook" class="input" required placeholder="https://open.feishu.cn/open-apis/bot/v2/hook/…" /></div><div class="field"><label>自定义关键词（可选）</label><input v-model="form.keyword" class="input" placeholder="机器人安全设置里的关键词，如 yygl" /></div><div class="field"><label>签名密钥（可选）</label><input v-model="form.secret" class="input" /></div></template>
            <div class="sheet-foot"><button type="button" class="btn btn-outline btn-block" @click="show=false">取消</button><button class="btn btn-primary btn-block" :disabled="saving">{{ saving ? '保存中…' : '保存并启用' }}</button></div>
          </form>
        </section>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import AppIcon from '../components/AppIcon.vue'
import { channelApi } from '../api/medcare'
import { useUIStore } from '../stores/ui'
const ui = useUIStore()
const channels = ref<any[]>([])
const loading = ref(true)
const show = ref(false)
const saving = ref(false)
const type = ref('email')
const menuId = ref(0)
const form = reactive<any>({ name: '', address: '', webhook: '', secret: '', keyword: '' })
function typeLabel(t: string) { return t === 'email' ? '邮件' : t === 'dingtalk' ? '钉钉群机器人' : t === 'feishu' ? '飞书群机器人' : t }
function openAdd() { type.value = 'email'; Object.assign(form, { name: '', address: '', webhook: '', secret: '', keyword: '' }); show.value = true }
function openMenu(ch: any) { menuId.value = menuId.value === ch.id ? 0 : ch.id }
async function load() { loading.value = true; try { const res = await channelApi.list(); channels.value = res.data?.data || [] } catch (e: any) { ui.showToast(e?.response?.data?.message || '读取渠道失败', 'error') } finally { loading.value = false } }
async function save() {
  saving.value = true
  try {
    const config: any = type.value === 'email' ? { address: form.address } : { webhook: form.webhook, secret: form.secret, keyword: form.keyword }
    const res = await channelApi.create({ type: type.value, name: form.name, enabled: true, config })
    if (res.data?.code !== 0) throw new Error(res.data?.message || '保存失败')
    show.value = false; ui.showToast('渠道已添加'); load()
  } catch (e: any) { ui.showToast(e?.response?.data?.message || e?.message || '保存失败', 'error') }
  finally { saving.value = false }
}
async function test(ch: any) { try { await channelApi.test(ch.id); ui.showToast('测试消息已发送') } catch (e: any) { ui.showToast(e?.response?.data?.message || '测试失败', 'error') } }
async function remove(ch: any) { if (!(await ui.confirm({ title: '删除渠道？', message: '删除后不再通过该渠道接收提醒。', confirmText: '删除' }))) return; await channelApi.remove(ch.id); ui.showToast('已删除'); load() }
onMounted(load)
</script>

<style scoped>
.ch-card { display: grid; gap: 9px; }
.ch-main { display: flex; gap: 10px; align-items: center; min-width: 0; }
.ch-body { flex: 1; min-width: 0; }
.ch-title-row { display: flex; gap: 6px; align-items: center; flex-wrap: wrap; min-width: 0; }
.ch-title-row b { font-size: 14.5px; overflow-wrap: anywhere; }
.ch-more { width: 36px; height: 36px; }
.ch-actions { display: flex; gap: 6px; flex-wrap: wrap; }
@media (min-width: 900px) {
  .ch-card { gap: 10px; }
  .ch-main { gap: 12px; }
  .ch-title-row { gap: 8px; }
  .ch-title-row b { font-size: 15px; }
  .ch-more { width: 40px; height: 40px; }
  .ch-actions { gap: 8px; }
}
</style>
