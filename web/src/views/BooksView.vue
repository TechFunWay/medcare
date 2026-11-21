<template>
  <div class="page narrow">
    <div class="page-head">
      <div><div class="eyebrow">用药本</div><h1>我的用药本</h1><p>一个账号可以管理多本，邀请家人一起照护。</p></div>
      <button class="btn btn-primary" @click="openCreate"><AppIcon name="plus" :size="18" />新建</button>
    </div>

    <div class="card join-card">
      <AppIcon name="members" :size="20" class="join-icon" />
      <input v-model="joinCode" class="input join-input" placeholder="输入 6 位邀请码加入" />
      <button class="btn btn-outline" :disabled="!joinCode.trim()" @click="joinByCode">加入</button>
    </div>

    <div v-if="book.loading" class="card muted">加载中…</div>
    <div v-else-if="!book.books.length" class="empty card">
      <div class="empty-icon"><AppIcon name="book" :size="30" /></div>
      <h3>先建一本用药本</h3>
      <p>例如“李奶奶的用药本”或“爸爸的用药本”。</p>
      <button class="btn btn-primary" style="margin-top:12px" @click="openCreate">创建用药本</button>
    </div>
    <div v-else class="list">
      <article v-for="b in book.books" :key="b.id" class="card book-card">
        <button class="book-icon" style="border:0" @click="enter(b.id)">{{ b.care_recipient_name.slice(0,1) }}</button>
        <div class="book-meta" @click="enter(b.id)">
          <div class="book-name">{{ b.name }} <span v-if="b.id === book.currentId" class="badge done">当前</span></div>
          <div class="book-sub">{{ b.care_recipient_name }} · {{ roleLabel(b.role) }} · {{ b.member_count || 1 }} 位成员</div>
        </div>
        <button class="btn btn-outline btn-sm" @click="manage(b.id)">管理</button>
      </article>
    </div>

    <Teleport to="body">
      <div v-if="showCreate" class="sheet-backdrop" @click.self="showCreate = false">
        <section class="sheet" role="dialog" aria-modal="true">
          <div class="sheet-grab" />
          <div class="sheet-head"><h2>新建用药本</h2><button class="icon-btn" aria-label="关闭" @click="showCreate=false"><AppIcon name="close" /></button></div>
          <form class="sheet-body" @submit.prevent="createBook">
            <div class="field"><label>用药本名称</label><input v-model="form.name" class="input" placeholder="例如：李奶奶的用药本" required /></div>
            <div class="field"><label>用药人称呼</label><input v-model="form.care_recipient_name" class="input" placeholder="例如：李奶奶" required /></div>
            <div class="field"><label>用药人是谁</label>
              <div class="tabs">
                <button type="button" class="tab" :class="{ active: form.care_recipient_is_self }" @click="form.care_recipient_is_self = true">我自己是用药人</button>
                <button type="button" class="tab" :class="{ active: !form.care_recipient_is_self }" @click="form.care_recipient_is_self = false">我是监护人</button>
              </div>
              <p class="tiny muted" style="margin-top:6px">{{ form.care_recipient_is_self ? '你本人按医嘱用药，自己打卡。' : '用药人由家人照顾；创建后可邀请用药人加入，或在成员里指定。' }}</p>
            </div>
            <div class="field-row">
              <div class="field"><label>关系</label><SheetSelect v-model="form.relation" placeholder="关系" :options="relationOptions" /></div>
              <div class="field"><label>时区</label><div class="input" style="display:flex;align-items:center;color:var(--mc-text-muted);background:var(--mc-surface)">北京时间（固定）</div></div>
            </div>
            <div class="field"><label>备注</label><textarea v-model="form.notes" class="textarea" placeholder="可选" /></div>
            <div class="sheet-foot"><button type="button" class="btn btn-outline btn-block" @click="showCreate=false">取消</button><button class="btn btn-primary btn-block" :disabled="creating">{{ creating ? '创建中…' : '创建' }}</button></div>
          </form>
        </section>
      </div>

      <div v-if="manageId" class="sheet-backdrop" @click.self="closeManage">
        <section class="sheet" role="dialog" aria-modal="true">
          <div class="sheet-grab" />
          <div class="sheet-head"><div><div class="eyebrow">管理</div><h2>{{ managed?.name }}</h2></div><button class="icon-btn" aria-label="关闭" @click="closeManage"><AppIcon name="close" /></button></div>
          <div class="sheet-body">
            <template v-if="isOwnerOfBook">
              <div class="section-head" style="margin-top:0"><h2>用药本信息</h2></div>
              <div class="card info-card">
                <div class="field"><label>用药本名称</label><input v-model="bookForm.name" class="input" placeholder="例如：李奶奶的用药本" /></div>
                <div class="field-row">
                  <div class="field"><label>用药人称呼</label><input v-model="bookForm.care_recipient_name" class="input" placeholder="例如：李奶奶" /></div>
                  <div class="field"><label>关系</label><SheetSelect v-model="bookForm.relation" placeholder="关系" :options="relationOptions" /></div>
                </div>
                <div class="field"><label>备注</label><textarea v-model="bookForm.notes" class="textarea" placeholder="可选" /></div>
                <p class="tiny muted" style="margin:0 0 10px">新增：{{ managed?.created_by_name || '—' }}<template v-if="managed?.updated_by_name"> ｜ 最后更新：{{ managed.updated_by_name }}</template></p>
                <button type="button" class="btn btn-primary btn-block" :disabled="savingBook" @click="saveBookInfo">{{ savingBook ? '保存中…' : '保存用药本信息' }}</button>
              </div>
            </template>
            <template v-if="canViewMembers">
            <div class="section-head" style="margin-top:20px"><h2>成员与权限</h2><span class="badge">{{ members.length }} 人</span></div>
            <p class="tiny muted" style="margin:-2px 0 10px">逐格勾选，勾了才有：包括「能不能看」与「能不能改、能不能删」。老人不识字就由家人代管，把对应的格勾给实际动手的人；不想让其看到的（如处方单、检查报告）不勾「查看」即可。</p>
            <div class="list">
              <div v-for="m in members" :key="m.user_id" class="member-row">
                <div class="event-card" style="align-items:center;flex-wrap:wrap">
                  <div class="book-icon" style="width:38px;height:38px;border-radius:50%;font-size:14px">{{ m.username.slice(0,1) }}</div>
                  <div class="event-main" style="flex:1 1 130px">
                    <div class="event-title">{{ m.username }} <span v-if="m.is_self" class="badge done">我</span></div>
                    <div class="event-meta" style="gap:6px">
                      <span v-if="m.is_care_recipient" class="badge doctor">用药人</span>
                      <span class="badge">{{ roleLabel(m.role) }}</span>
                      <span class="badge" :class="permBadgeClass(m)">{{ permsLabel(m) }}</span>
                    </div>
                  </div>
                  <div v-if="managed?.role === 'owner' && m.role !== 'owner'" style="display:flex;gap:6px;flex-wrap:wrap">
                    <button class="btn btn-outline btn-xs" @click="openPerm(m)">{{ permToggleLabel(m) }}</button>
                    <button v-if="!m.is_care_recipient" class="btn btn-outline btn-xs" @click="setRecipient(m)">设为用药人</button>
                    <button class="btn btn-outline btn-xs" @click="toggleRole(m)">{{ m.role === 'manager' ? '设为成员' : '设为监护人' }}</button>
                    <button class="btn btn-outline btn-xs" @click="transferTo(m)">转交管理</button>
                    <button class="btn btn-danger btn-xs" @click="removeMember(m)">移除</button>
                  </div>
                  <label v-if="m.is_self" class="switch"><input type="checkbox" :checked="m.notifications_enabled" @change="toggleSelfNotify($event, m)" /><span /></label>
                </div>
                <div v-if="permTarget === m.user_id" class="card perm-panel">
                  <div class="perm-head"><b>{{ m.username }} 能做什么</b><span class="badge" :class="permBadgeClass(m)">{{ permModeLabel(m) }}</span></div>
                  <p class="tiny muted" style="margin:0 0 4px">{{ recipientHint(m) }}</p>
                  <div class="perm-presets">
                    <button v-for="preset in PERMISSION_PRESETS" :key="preset.key" class="chip" :class="{ active: presetActive(m, preset) }" :disabled="permSaving" :title="preset.hint" @click="applyPreset(m, preset)">{{ preset.label }}</button>
                  </div>
                  <div v-for="mod in moduleList" :key="mod.module" class="perm-row">
                    <div class="perm-row-head">
                      <span class="perm-row-label"><b>{{ mod.label }}</b><i>{{ mod.hint }}</i></span>
                      <button class="btn btn-ghost btn-xs" :disabled="permSaving" @click="toggleModule(m, mod)">{{ moduleState(mod, m) === 'all' ? '全不勾' : '全勾' }}</button>
                    </div>
                    <div class="perm-cells">
                      <label v-for="act in mod.actions" :key="act.key" class="perm-cell" :class="{ on: capOn(m, act.key) }">
                        <input type="checkbox" :checked="capOn(m, act.key)" :disabled="permSaving" @change="toggleCell(m, act.key)" />
                        <span>{{ act.label }}</span>
                      </label>
                    </div>
                  </div>
                  <div class="perm-actions">
                    <button class="btn btn-outline btn-sm" :disabled="permSaving || !m.custom_permissions" @click="resetPermissions(m)">恢复跟随角色</button>
                    <button class="btn btn-ghost btn-sm" @click="permTarget = null">收起</button>
                  </div>
                </div>
              </div>
            </div>
            </template>
            <template v-if="canInviteMembers">
            <div class="section-head" style="margin-top:20px"><h2>邀请家人</h2></div>
            <div class="field-row">
              <div v-if="roleOptions.length > 1" class="field"><label>角色</label><SheetSelect v-model="inviteRole" placeholder="角色" :options="roleOptions" /></div>
              <div v-else class="field"><label>角色</label><div class="input" style="display:flex;align-items:center;color:var(--mc-text-muted);background:var(--mc-surface)">成员</div></div>
              <div class="field"><label>有效期</label><SheetSelect v-model="inviteHours" placeholder="有效期" :options="expiryOptions" /></div>
            </div>
            <p class="tiny muted" style="margin:-4px 0 10px">监护人默认能管药品、计划与打卡，成员默认只能查看与提醒；加入后管理员还能在「设置权限」里逐人调整。</p>
            <label v-if="isOwnerOfBook" class="invite-check"><input v-model="inviteAsRecipient" type="checkbox" /> 对方加入后就是用药人</label>
            <button class="btn btn-primary btn-block" :disabled="creatingInvite" @click="createInvite">{{ creatingInvite ? '生成中…' : '生成邀请码' }}</button>
            <div v-if="createdInvite" class="card" style="margin-top:12px;background:var(--mc-primary-soft)">
              <div class="small"><b>邀请码：</b>{{ createdInvite.code }} <button class="btn btn-outline" style="min-height:28px;padding:0 8px;margin-left:6px;font-size:11px" @click="copy(createdInvite.code)">复制</button></div>
              <div class="small" style="margin-top:6px;overflow-wrap:anywhere"><b>链接：</b>{{ createdInvite.link }}</div>
              <button class="btn btn-outline btn-block" style="margin-top:10px" @click="copy(createdInvite.link)">复制链接</button>
            </div>
            <div v-if="invites.length" style="margin-top:14px">
              <div v-for="inv in invites" :key="inv.id" class="event-card" style="align-items:center">
                <div class="event-main">
                  <div class="event-title">{{ roleLabel(inv.role) }}<span v-if="inv.as_care_recipient" class="badge doctor" style="margin-left:6px">用药人</span></div>
                  <div class="event-meta">已用 {{ inv.used_count }}/{{ inv.max_uses }} 次 · {{ inviteStatusLabel(inv.status) }}</div>
                </div>
                <button v-if="inv.status === 'active' && (isOwnerOfBook || inv.is_mine)" class="btn btn-danger btn-xs" @click="revokeInvite(inv)">撤销</button>
              </div>
            </div>
            </template>
            <div class="list" style="margin-top:18px">
              <button class="btn btn-outline btn-block" @click="setDefaultBook">{{ book.defaultId === managed?.id ? '已是我的默认药本' : '设为我的默认药本' }}</button>
              <router-link to="/locations" class="btn btn-outline btn-block" @click="closeManage">地点库</router-link>
              <button class="btn btn-outline btn-block" @click="toggleBookNotify">{{ selfNotify ? '关闭本用药本提醒' : '开启本用药本提醒' }}</button>
            </div>
          </div>
        </section>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppIcon from '../components/AppIcon.vue'
import SheetSelect from '../components/SheetSelect.vue'
import { bookApi, inviteApi, type Book, type Member, type PermissionCatalog } from '../api/medcare'
import { PERMISSION_MODULES, PERMISSION_PRESETS, type PermModule, type PermissionPreset } from '../utils/permissions'
import { useBookStore } from '../stores/book'
import { useUIStore } from '../stores/ui'
const route = useRoute()
const router = useRouter()
const book = useBookStore()
const ui = useUIStore()
const showCreate = ref(false)
const creating = ref(false)
const form = reactive({ name: '', care_recipient_name: '', relation: 'self', timezone: 'Asia/Shanghai', notes: '', care_recipient_is_self: true })
const joinCode = ref('')
const manageId = ref<number | null>(null)
const members = ref<Member[]>([])
const invites = ref<any[]>([])
const inviteRole = ref('manager')
const inviteHours = ref(168)
const creatingInvite = ref(false)
const createdInvite = ref<any>(null)
const managed = computed<Book | undefined>(() => book.books.find((b) => b.id === manageId.value))
const savingBook = ref(false)
const bookForm = reactive({ name: '', care_recipient_name: '', relation: 'self', notes: '' })
const selfMember = computed(() => members.value.find((m) => m.is_self))
const selfNotify = computed(() => !!selfMember.value?.notifications_enabled)
const relationOptions = [
  { value: 'self', label: '本人' }, { value: 'spouse', label: '配偶' },
  { value: 'parent', label: '父母' }, { value: 'child', label: '子女' }, { value: 'other', label: '其他' },
]
const roleOptions = computed(() => {
  const isOwner = book.books.find((b) => b.id === manageId.value)?.role === 'owner'
  return isOwner ? [{ value: 'manager', label: '监护人' }, { value: 'member', label: '成员' }] : [{ value: 'member', label: '成员' }]
})
const inviteAsRecipient = ref(false)
const isOwnerOfBook = computed(() => book.books.find((b) => b.id === manageId.value)?.role === 'owner')
// 看成员列表、发邀请各是一格权限；管理员没勾就不显示入口，也根本不请求。
const canViewMembers = computed(() => isOwnerOfBook.value || (!!manageId.value && book.canIn(manageId.value, 'member:view')))
const canInviteMembers = computed(() => isOwnerOfBook.value || (!!manageId.value && book.canIn(manageId.value, 'member:invite')))
const expiryOptions = [{ value: 24, label: '1 天' }, { value: 168, label: '7 天' }, { value: 720, label: '30 天' }]
function roleLabel(role: string) { return role === 'owner' ? '管理员' : role === 'manager' ? '监护人' : '成员' }
function inviteStatusLabel(status: string) { return status === 'active' ? '有效' : status === 'expired' ? '已过期' : status === 'revoked' ? '已撤销' : status === 'used_up' ? '已用完' : status }
async function transferTo(m: Member) {
  if (!manageId.value) return
  const ok = await ui.confirm({ title: '转交管理权限', message: '转交后 ' + m.username + ' 成为管理员，你变成监护人。确定继续？', confirmText: '转交' })
  if (!ok) return
  try {
    await bookApi.transfer(manageId.value, m.user_id)
    await book.fetchBooks()
    await loadManage()
    ui.showToast('已转交管理权限')
  } catch (e: any) { ui.showToast(e?.response?.data?.message || '转交失败', 'error') }
}
async function setRecipient(m: Member) {
  if (!manageId.value) return
  try {
    await bookApi.setCareRecipient(manageId.value, m.user_id)
    await loadManage()
    ui.showToast('已把 ' + m.username + ' 设为用药人')
  } catch (e: any) { ui.showToast(e?.response?.data?.message || '设置失败', 'error') }
}
// ---------- 成员权限矩阵（管理员逐格勾选，勾了才有，不勾就没有） ----------
const permTarget = ref<number | null>(null)
const permSaving = ref(false)
// 清单以服务端为准（后端是权威），拉不到时用本地同名矩阵兜底，界面不会空。
const serverModules = ref<PermModule[]>([])
const moduleList = computed<PermModule[]>(() => (serverModules.value.length ? serverModules.value : PERMISSION_MODULES))
const cellList = computed(() => moduleList.value.flatMap((m) => m.actions))

function cellOf(key: string) {
  for (const module of moduleList.value) {
    const hit = module.actions.find((a) => a.key === key)
    if (hit) return { module, cell: hit }
  }
  return null
}
function cellTitle(key: string): string {
  const hit = cellOf(key)
  return hit ? hit.module.label + " · " + hit.cell.label : key
}
function capOn(m: Member, key: string): boolean {
  return (m.permissions || []).includes(key)
}
function moduleKeys(mod: PermModule): string[] {
  return mod.actions.map((a) => a.key)
}
function moduleState(mod: PermModule, m: Member): "all" | "part" | "none" {
  const keys = moduleKeys(mod)
  const on = keys.filter((k) => capOn(m, k)).length
  if (!on) return "none"
  return on === keys.length ? "all" : "part"
}
function permsLabel(m: Member): string {
  const total = cellList.value.length
  const owned = (m.permissions || []).filter((k) => cellList.value.some((c) => c.key === k))
  if (m.role === "owner" || owned.length === total) return "全部权限"
  if (!owned.length) return "无权限"
  const parts = moduleList.value
    .map((mod) => {
      const on = moduleKeys(mod).filter((k) => owned.includes(k))
      if (!on.length) return ""
      const viewOnly = on.length === 1 && on[0].endsWith(":view")
      return (viewOnly ? "看" : "") + mod.label.replace(/与.*/, "") + (on.length > 1 && !viewOnly ? "·改" : "")
    })
    .filter(Boolean)
  return parts.join(" ") || owned.length + " 格"
}
function permModeLabel(m: Member): string {
  return m.custom_permissions ? "已单独勾选" : "跟随" + roleLabel(m.role) + "默认"
}
function permBadgeClass(m: Member): string {
  return m.custom_permissions ? "self" : "pending"
}
function permToggleLabel(m: Member): string {
  return permTarget.value === m.user_id ? "收起权限" : "设置权限"
}
function recipientHint(m: Member): string {
  return m.is_care_recipient
    ? "用药人：是（一个用药本只有一位，就是按医嘱吃药的那个人）"
    : "用药人：不是，需要时用上方「设为用药人」指定"
}
function presetActive(m: Member, preset: PermissionPreset): boolean {
  const owned = (m.permissions || []).filter((k) => cellList.value.some((c) => c.key === k))
  return !!m.custom_permissions && owned.length === preset.keys.length && preset.keys.every((k) => owned.includes(k))
}
function openPerm(m: Member) {
  permTarget.value = permTarget.value === m.user_id ? null : m.user_id
}
async function savePermissions(m: Member, next: string[] | null, tip: string) {
  if (!manageId.value) return
  permSaving.value = true
  try {
    const res = await bookApi.updatePermissions(manageId.value, m.user_id, next)
    if (res.data?.code !== 0) throw new Error(res.data?.message || "保存失败")
    const data = res.data.data as { permissions?: string[]; custom_permissions?: boolean }
    m.permissions = data.permissions || []
    m.custom_permissions = !!data.custom_permissions
    // 改的是自己时立刻刷新当前用药本的权限，按钮和页面跟着变。
    if (m.is_self) await book.fetchBooks()
    ui.showToast(tip)
  } catch (e: any) {
    ui.showToast(e?.response?.data?.message || e?.message || "保存权限失败", "error")
    await loadManage()
  } finally {
    permSaving.value = false
  }
}
async function toggleCell(m: Member, key: string) {
  const current = m.permissions || []
  const had = current.includes(key)
  const order = cellList.value.map((c) => c.key)
  const next = had ? current.filter((k) => k !== key) : [...current, key].sort((a, b) => order.indexOf(a) - order.indexOf(b))
  await savePermissions(m, next, (had ? "已取消「" : "已勾选「") + cellTitle(key) + "」")
}
// 勾整行：这一行已全勾就整行取消，否则整行勾上（"看 + 改"一步到位）。
async function toggleModule(m: Member, mod: PermModule) {
  const keys = moduleKeys(mod)
  const state = moduleState(mod, m)
  const current = m.permissions || []
  const next = state === "all"
    ? current.filter((k) => !keys.includes(k))
    : [...new Set([...current, ...keys])].sort((a, b) => cellList.value.findIndex((c) => c.key === a) - cellList.value.findIndex((c) => c.key === b))
  await savePermissions(m, next, mod.label + (state === "all" ? "：全部取消" : "：已全部勾选"))
}
async function applyPreset(m: Member, preset: PermissionPreset) {
  await savePermissions(m, preset.keys, m.username + " 已套用「" + preset.label + "」")
}
async function resetPermissions(m: Member) {
  await savePermissions(m, null, m.username + " 的权限已恢复跟随" + roleLabel(m.role))
}
async function loadPermissionMatrix(id: number) {
  try {
    const res = await bookApi.permissions(id)
    const data = res.data?.data as PermissionCatalog | undefined
    serverModules.value = data?.modules?.length ? (data.modules as unknown as PermModule[]) : []
  } catch { /* 用本地矩阵兜底，不打扰用户 */ }
}
function openCreate() { showCreate.value = true }
function enter(id: number) { book.select(id); router.push('/today') }
async function createBook() {
  if (!form.name.trim() || !form.care_recipient_name.trim()) return
  creating.value = true
  try { await book.create({ ...form }); showCreate.value = false; ui.showToast('用药本已创建'); router.push('/today') }
  catch (e: any) { ui.showToast(e?.message || '创建失败', 'error') }
  finally { creating.value = false }
}
async function joinByCode() {
  try { const res = await inviteApi.joinByCode(joinCode.value.trim()); if (res.data?.code !== 0) throw new Error(res.data?.message || '加入失败'); await book.fetchBooks(); book.select(res.data.data.id); joinCode.value = ''; ui.showToast('已加入用药本'); router.push('/today') }
  catch (e: any) { ui.showToast(e?.response?.data?.message || e?.message || '邀请码无效', 'error') }
}
async function manage(id: number) {
  manageId.value = id
  permTarget.value = null
  const target = book.books.find((b) => b.id === id)
  // 成员只能邀请成员，默认角色按身份给。
  inviteRole.value = target?.role === 'owner' ? 'manager' : 'member'
  inviteAsRecipient.value = false
  if (target) {
    Object.assign(bookForm, { name: target.name, care_recipient_name: target.care_recipient_name, relation: target.relation || 'other', notes: target.notes || '' })
  }
  await loadManage()
  loadPermissionMatrix(id)
}
async function setDefaultBook() {
  if (!manageId.value) return
  await book.setDefault(manageId.value)
  ui.showToast('已设为默认药本')
}
async function saveBookInfo() {
  if (!manageId.value) return
  if (!bookForm.name.trim() || !bookForm.care_recipient_name.trim()) { ui.showToast('名称和用药人称呼不能为空', 'error'); return }
  savingBook.value = true
  try {
    await bookApi.update(manageId.value, { name: bookForm.name.trim(), care_recipient_name: bookForm.care_recipient_name.trim(), relation: bookForm.relation, notes: bookForm.notes })
    await book.fetchBooks()
    ui.showToast('用药本信息已保存')
  } catch (e: any) { ui.showToast(e?.response?.data?.message || '保存失败', 'error') }
  finally { savingBook.value = false }
}
async function loadManage() {
  if (!manageId.value) return
  // 「成员与邀请 · 查看」没勾时不发请求：这一页对他只剩切药本与看信息。
  if (!book.canIn(manageId.value, 'member:view')) { members.value = []; invites.value = []; return }
  const [m, i] = await Promise.all([bookApi.members(manageId.value), bookApi.invites(manageId.value)])
  members.value = m.data?.data || []
  invites.value = i.data?.data || []
}
function closeManage() { manageId.value = null; createdInvite.value = null; permTarget.value = null }
async function createInvite() {
  if (!manageId.value) return
  creatingInvite.value = true
  try { const res = await bookApi.createInvite(manageId.value, { role: inviteRole.value, expires_hours: inviteHours.value, max_uses: 10, as_care_recipient: inviteAsRecipient.value }); createdInvite.value = res.data.data; await loadManage(); ui.showToast('邀请已生成') }
  catch (e: any) { ui.showToast(e?.response?.data?.message || '生成邀请失败', 'error') }
  finally { creatingInvite.value = false }
}
async function revokeInvite(inv: any) {
  if (!manageId.value) return
  if (!(await ui.confirm({ title: '撤销邀请？', message: '撤销后该链接和邀请码将无法再加入。', confirmText: '撤销' }))) return
  await bookApi.revokeInvite(manageId.value, inv.id); await loadManage(); ui.showToast('已撤销')
}
async function toggleRole(m: Member) {
  if (!manageId.value) return
  await bookApi.updateMember(manageId.value, m.user_id, m.role === 'manager' ? 'member' : 'manager')
  await loadManage(); ui.showToast('角色已更新，该成员的权限恢复为新角色的默认')
}
async function removeMember(m: Member) {
  if (!manageId.value) return
  if (!(await ui.confirm({ title: '移除成员？', message: `移除 ${m.username} 后，对方将立即失去该用药本的访问权限。`, confirmText: '移除' }))) return
  await bookApi.removeMember(manageId.value, m.user_id); await loadManage(); ui.showToast('已移除')
}
async function toggleSelfNotify(event: Event, m: Member) {
  if (!manageId.value) return
  const enabled = (event.target as HTMLInputElement).checked
  await bookApi.myNotifications(manageId.value, enabled); m.notifications_enabled = enabled; ui.showToast(enabled ? '已开启本提醒' : '已关闭本提醒')
}
async function toggleBookNotify() {
  if (!manageId.value) return
  const next = !selfNotify.value
  await bookApi.myNotifications(manageId.value, next); if (selfMember.value) selfMember.value.notifications_enabled = next; ui.showToast(next ? '已开启本提醒' : '已关闭本提醒')
}
async function copy(text: string) { try { await navigator.clipboard.writeText(text); ui.showToast('已复制') } catch { ui.showToast('复制失败，请手动选择', 'error') } }
onMounted(async () => { await book.fetchBooks(); if (route.query.create === '1') showCreate.value = true })
</script>

<style scoped>
.invite-check { display: flex; align-items: center; gap: 8px; font-size: 12.5px; color: var(--mc-text-muted); margin-bottom: 10px; }
/* 成员行与其下方的权限面板算一组：组内 6px，组间靠 .list 的 7px 拉开层次 */
.member-row { display: grid; gap: 6px; }
.perm-panel { padding: 10px 12px; background: var(--mc-surface-2); box-shadow: none; }
.perm-head { display: flex; align-items: center; justify-content: space-between; gap: 8px; margin-bottom: 4px; }
.perm-head b { font-size: 13px; overflow-wrap: anywhere; }
.perm-presets { display: flex; flex-wrap: wrap; gap: 6px; margin: 8px 0 10px; }
.perm-presets .chip { min-height: 34px; font-size: 12px; }
.perm-row { border-top: 1px dashed var(--mc-border); padding: 8px 0; }
.perm-row-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 8px; }
.perm-row-label { min-width: 0; }
.perm-row-label b { display: block; font-size: 13px; line-height: 1.35; }
.perm-row-label i { display: block; font-style: normal; font-size: 11.5px; line-height: 1.45; color: var(--mc-text-muted); }
.perm-cells { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 7px; }
.perm-cell { display: inline-flex; align-items: center; gap: 5px; min-height: 34px; padding: 0 10px; border: 1px solid var(--mc-border); border-radius: 11px; background: var(--mc-surface); font-size: 12.5px; font-weight: 650; color: var(--mc-text-muted); cursor: pointer; white-space: nowrap; }
.perm-cell input { width: 17px; height: 17px; accent-color: var(--mc-primary); margin: 0; }
.perm-cell.on { border-color: var(--mc-primary); background: var(--mc-primary-soft); color: var(--mc-primary-strong); }
.perm-line { display: flex; align-items: flex-start; gap: 10px; min-height: 44px; padding: 6px 0; border-bottom: 1px dashed var(--mc-border); cursor: pointer; }
.perm-line:last-of-type { border-bottom: 0; }
.perm-line input { flex: 0 0 auto; width: 20px; height: 20px; margin: 2px 0 0; accent-color: var(--mc-primary); }
.perm-text { min-width: 0; }
.perm-text b { display: block; font-size: 13px; line-height: 1.35; }
.perm-text i { display: block; font-style: normal; font-size: 11.5px; line-height: 1.45; color: var(--mc-text-muted); }
.perm-actions { display: flex; gap: 8px; margin-top: 10px; }
.perm-actions .btn { flex: 1 1 0; min-width: 0; }
.join-card { display: flex; gap: 8px; align-items: center; margin-bottom: 10px; }
.join-icon { color: var(--mc-primary); flex: 0 0 auto; }
.join-input { flex: 1 1 auto; min-width: 0; min-height: 44px; }
.info-card { padding: 12px; margin-bottom: 4px; }
.info-card .field:last-of-type { margin-bottom: 10px; }
@media (max-width: 380px) { .join-icon { display: none; } }
.switch { position: relative; display: inline-flex; width: 42px; height: 26px; flex: 0 0 auto; }
.switch input { opacity: 0; width: 0; height: 0; }
.switch span { position: absolute; inset: 0; border-radius: 999px; background: var(--mc-surface-2); border: 1px solid var(--mc-border); transition: .15s; }
.switch span::after { content: ''; position: absolute; width: 20px; height: 20px; left: 2px; top: 2px; border-radius: 50%; background: #fff; box-shadow: 0 1px 4px rgba(0,0,0,.15); transition: .15s; }
.switch input:checked + span { background: var(--mc-primary); border-color: var(--mc-primary); }
.switch input:checked + span::after { transform: translateX(16px); }
</style>
