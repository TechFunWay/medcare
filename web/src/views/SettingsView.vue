<template>
  <div class="page narrow">
    <div class="page-head"><div><div class="eyebrow">设置</div><h1>设置</h1><p>按分组管理提醒节奏、外观、AI 识别与邮件等配置。</p></div></div>
    <!-- 子菜单：每个分组是一个独立视图，地址栏带 ?tab= 便于直达与刷新 -->
    <div class="settings-tabs">
      <button v-for="t in visibleSettingTabs" :key="t.value" type="button" class="settings-tab" :class="{ active: tab === t.value }" @click="goTab(t.value)">
        <AppIcon :name="t.icon" :size="17" /><span>{{ t.label }}</span>
      </button>
    </div>
    <div v-if="loading" class="card muted">加载中…</div>
    <template v-else>
    <section v-if="tab === 'theme'" class="card" style="margin-bottom:10px">
      <div class="section-head" style="margin-top:0"><h2 style="font-size:16px">外观主题</h2><span class="badge">{{ theme.modeLabel }}</span></div>
      <div class="field"><label>亮色 / 暗色</label>
        <div class="tabs">
          <button v-for="m in modeOptions" :key="m.value" type="button" class="tab" :class="{ active: theme.mode === m.value }" @click="setMode(m.value)">{{ m.label }}</button>
        </div>
        <p class="tiny muted" style="margin-top:6px">跟随系统时，白天用亮色、晚上自动切到暗色。</p>
      </div>
      <div class="field"><label>主题配色</label>
        <div class="brand-grid">
          <button v-for="b in THEME_BRANDS" :key="b.value" type="button" class="brand-option" :class="{ active: theme.brand === b.value }" @click="setBrand(b.value)">
            <span class="brand-dot" :style="{ background: b.color }"></span>
            <span>{{ b.name }}</span>
          </button>
        </div>
      </div>
      <div class="preview-strip">
        <span class="pv-dot"></span>
        <div class="preview-text"><b>预览</b><div class="tiny muted">页面背景 / 卡片 / 主色按钮会随上面选择变化</div></div>
        <button class="btn btn-primary btn-xs">主按钮</button>
      </div>
    </section>
    <form v-if="tab === 'rhythm'" class="card" @submit.prevent="save">
      <div class="section-head" style="margin-top:0"><h2 style="font-size:16px">提醒节奏</h2><span class="badge">影响未来的提醒</span></div>
      <div class="field-row"><div class="field"><label>提前提醒（分钟）</label><input v-model.number="pref.lead_minutes" type="number" min="0" max="120" class="input" /></div><div class="field"><label>每日总结时间</label><TimeField v-model="pref.daily_summary_time" placeholder="选择时间" /></div></div>
      <div class="field-row"><div class="field"><label>首次逾期（分钟）</label><input v-model.number="pref.overdue_first_minutes" type="number" min="1" max="240" class="input" /></div><div class="field"><label>未完成重复提醒</label><SheetSelect v-model.number="pref.overdue_repeat_minutes" placeholder="提醒间隔" :options="repeatOptions" /></div></div>
      <p class="tiny muted" style="margin:-4px 0 10px">没按时吃药就按这个间隔一直提醒，直到点「完成」为止。</p>
      <div class="field" style="display:flex;justify-content:space-between;align-items:center"><label style="margin:0">每日总结</label><label class="switch"><input v-model="pref.daily_summary_enabled" type="checkbox" /><span /></label></div>
      <div class="field" style="display:flex;justify-content:space-between;align-items:center"><label style="margin:0">提醒中包含地点</label><label class="switch"><input v-model="pref.include_location_in_notifications" type="checkbox" /><span /></label></div>
      <div class="field" style="display:flex;justify-content:space-between;align-items:center"><label style="margin:0">提醒中包含备注</label><label class="switch"><input v-model="pref.include_notes_in_notifications" type="checkbox" /><span /></label></div>
      <div class="field" style="display:flex;justify-content:space-between;align-items:center"><label style="margin:0">库存不足提醒</label><label class="switch"><input v-model="pref.low_stock_notify_enabled" type="checkbox" /><span /></label></div>
      <p class="tiny muted" style="margin:-4px 0 10px">库存低于阈值或缺货时，经已开启的消息渠道提醒，同一药品每天最多一次。</p>
      <button class="btn btn-primary btn-block" :disabled="saving">{{ saving ? '保存中…' : '保存偏好' }}</button>
    </form>
    </template>
    <div v-if="tab === 'mail'" class="card" style="margin-top:10px"><router-link to="/settings/channels" class="btn btn-outline btn-block">管理通知渠道</router-link></div>

    <template v-if="auth.isAdmin">
      <div v-if="isAdminTab" class="section-head" style="margin-top:18px"><h2>管理员配置</h2><span class="badge">仅管理员</span></div>
      <div v-if="tab === 'theme'" class="card" style="margin-bottom:10px">
        <div class="section-head" style="margin-top:0"><h3 style="font-size:14.5px">新用户默认主题</h3><button class="btn btn-outline btn-xs" @click="saveDefaultTheme">保存</button></div>
        <div class="field-row">
          <div class="field"><label>默认模式</label><SheetSelect v-model="defaultTheme.mode" placeholder="默认模式" :options="modeOptions" /></div>
          <div class="field"><label>默认配色</label><SheetSelect v-model="defaultTheme.brand" placeholder="默认配色" :options="brandOptions" /></div>
        </div>
        <p class="tiny muted">只影响尚未自行设置过主题的新用户；老用户保留自己的选择。</p>
      </div>
      <div v-if="tab === 'dict'" class="card" style="margin-bottom:10px">
        <div class="section-head" style="margin-top:0"><h3 style="font-size:14.5px">药品字典</h3><button class="btn btn-outline btn-xs" @click="saveMedicineDict">保存</button></div>
        <p class="tiny muted" style="margin:-2px 0 10px;line-height:1.6">这里维护药品表单下拉的候选值，可添加、修改、删除。留空表示用内置默认；用户在表单里仍可自己填。</p>
        <div class="field"><label>剂型</label><DictEditor v-model="medicineDict.form" placeholder="例如：片剂、胶囊、注射液" /></div>
        <div class="field"><label>规格</label><DictEditor v-model="medicineDict.spec" placeholder="例如：5mg、0.2g×10片、100ml" /></div>
        <div class="field"><label>库存单位</label><DictEditor v-model="medicineDict.unit" placeholder="例如：片、盒、支、瓶" /></div>
        <div class="field" style="margin-bottom:0"><label>包装规格</label><DictEditor v-model="medicineDict.packageSpec" placeholder="例如：10片/盒、100片/瓶" /></div>
      </div>
      <div v-if="tab === 'security'" class="card" style="margin-bottom:10px">
        <div class="section-head" style="margin-top:0"><h3 style="font-size:14.5px">注册审核</h3><button class="btn btn-outline btn-xs" @click="saveRegisterApproval">保存</button></div>
        <label class="switch-row">
          <input v-model="registerApproval" type="checkbox" class="switch-input" />
          <span class="switch-body">
            <b>普通用户注册需审核</b>
            <span class="tiny muted">打开后，新注册的普通账号需管理员在「用户管理」里通过审核才能登录；首个注册的管理员不受影响。</span>
          </span>
        </label>
      </div>
      <div v-if="tab === 'mail'" class="card">
        <div class="section-head" style="margin-top:0"><h3 style="font-size:14.5px">SMTP 邮件</h3><button class="btn btn-outline btn-xs" @click="saveSMTP">保存</button></div>
        <div class="field-row"><div class="field"><label>服务器</label><input v-model="smtp.smtp_host" class="input" placeholder="smtp.qq.com" /></div><div class="field"><label>端口</label><input v-model.number="smtp.smtp_port" type="number" class="input" /></div></div>
        <div class="field-row"><div class="field"><label>加密</label><SheetSelect v-model="smtp.smtp_security" placeholder="加密方式" :options="smtpSecurityOptions" /></div><div class="field"><label>用户名</label><input v-model="smtp.smtp_username" class="input" /></div></div>
        <div class="field"><label>密码 / 授权码（留空不修改）</label><input v-model="smtp.smtp_password" type="password" class="input" /></div>
        <div class="field-row"><div class="field"><label>发件人</label><input v-model="smtp.from_address" class="input" /></div><div class="field"><label>公网地址</label><input v-model="smtp.public_base_url" class="input" placeholder="https://..." /></div></div>
        <button class="btn btn-outline btn-block" @click="testSMTP">发送测试邮件</button>
      </div>
      <div v-if="tab === 'ai'" class="card" style="margin-top:10px">
        <div class="section-head" style="margin-top:0">
          <h3 style="font-size:14.5px">AI 识别（处方单 / 缴费单 / 药盒）</h3>
          <button class="btn btn-xs" :class="aiDirty ? 'btn-primary' : 'btn-outline'" :disabled="savingAI" @click="saveAI">{{ savingAI ? '保存中…' : (aiDirty ? '保存修改' : '保存') }}</button>
        </div>
        <div class="field">
          <label>AI 服务商</label>
          <SheetSelect :model-value="aiProvider" placeholder="选择服务商，自动填好地址与常用模型" :options="aiProviderOptions" @update:model-value="applyAIProvider" />
          <p class="tiny muted" style="margin-top:6px">选服务商会自动填 Base URL 和常用模型；带套餐的服务商（小米 MiMo、智谱、百炼、豆包）选完后还要选计费方式；列表里没有的选「自定义」，起个名称、填地址与 API Key，Key 填好会自动拉模型列表供选择。</p>
        </div>
        <div v-if="aiModes.length" class="field">
          <label>计费方式</label>
          <div class="chip-row">
            <button v-for="m in aiModes" :key="m.value" type="button" class="chip" :class="{ active: aiMode === m.value }" @click="applyAIMode(m.value)">{{ m.label }}</button>
          </div>
          <p class="tiny muted" style="margin-top:6px">按量付费与套餐的 Base URL、API Key 不通用：切换后地址自动换，Key 也要用对应控制台里的那把。</p>
        </div>
        <div v-if="aiProvider === 'custom'" class="field">
          <label>服务商名称</label>
          <input v-model="ai.provider_name" class="input" placeholder="例如：小米 MiMo Token Plan" maxlength="60" autocomplete="off" />
          <p class="tiny muted" style="margin-top:6px">给这套自定义配置起个名字，保存后服务商下拉里会显示它。</p>
        </div>
        <div class="field"><label>Base URL</label><input v-model="ai.base_url" class="input" placeholder="https://api.example.com/v1" autocomplete="off" spellcheck="false" /></div>
        <div class="field"><label>API Key{{ ai.has_key ? '（已保存，留空不改）' : '' }}</label><input v-model="ai.api_key" type="password" class="input" :placeholder="aiKeyHint || 'sk-...'" autocomplete="new-password" />
          <p class="tiny muted" style="margin-top:6px">先填 Key：填好并停顿一下会自动获取模型列表，然后在下面选模型。</p>
        </div>
        <div class="field">
          <label>模型</label>
          <SheetSelect v-if="aiModelOptions.length && !modelManual" v-model="ai.model" placeholder="选择模型" :options="aiModelOptions" />
          <input v-else v-model="ai.model" class="input" placeholder="例如 mimo-v2.6-flash" autocomplete="off" spellcheck="false" />
          <p v-if="aiModelOptions.length && !modelManual" class="tiny muted">已获取 {{ aiModelOptions.length }} 个模型，<button type="button" class="link-btn" @click="modelManual = true">改为手动填写</button></p>
          <p v-else-if="aiModelOptions.length" class="tiny muted"><button type="button" class="link-btn" @click="modelManual = false">从已获取的 {{ aiModelOptions.length }} 个模型里选</button></p>
          <p v-else class="tiny muted">填好 API Key 后自动获取模型列表，获取到即可在这里下拉选择；也可点下方「获取模型列表」。</p>
        </div>
        <div class="field"><label>超时（秒）</label><input v-model.number="ai.timeout_seconds" type="number" min="30" max="300" class="input" /></div>
        <div class="field" style="display:flex;justify-content:space-between;align-items:center;margin-bottom:10px"><label style="margin:0">启用 AI</label><label class="switch"><input v-model="ai.enabled" type="checkbox" /><span /></label></div>
        <p class="tiny muted" style="margin-bottom:10px">测试连接与获取模型列表直接用上面填的内容，不用先保存；AI 识别（处方单 / 缴费单 / 药盒）真正生效需要保存并启用。</p>
        <div class="btn-row"><button class="btn btn-outline" :disabled="testingAI" @click="testAI">{{ testingAI ? '测试中…' : '测试连接' }}</button><button class="btn btn-outline" :disabled="loadingModels" @click="fetchAIModels()">{{ loadingModels ? '获取中…' : '获取模型列表' }}</button></div>
        <p v-if="aiDirty" class="tiny dirty-hint">有未保存的修改，点下方或右上角「保存修改」后生效。</p>
        <!-- 表单很长，底部再放一个保存按钮，填完直接在这里保存，不用滚回顶部 -->
        <button class="btn btn-block" :class="aiDirty ? 'btn-primary' : 'btn-outline'" style="margin-top:12px" :disabled="savingAI" @click="saveAI">{{ savingAI ? '保存中…' : (aiDirty ? '保存修改' : '保存') }}</button>
      </div>
      <div v-if="tab === 'ai'" class="card" style="margin-top:10px">
        <div class="section-head" style="margin-top:0">
          <h3 style="font-size:14.5px">OCR 辅助（可选）</h3>
          <button class="btn btn-xs" :class="ocrDirty ? 'btn-primary' : 'btn-outline'" :disabled="savingOCR" @click="saveOCR">{{ savingOCR ? '保存中…' : (ocrDirty ? '保存修改' : '保存') }}</button>
        </div>
        <p class="tiny muted" style="margin-bottom:10px;line-height:1.65">开启后识别先由 OCR 读字（秒级、不耗视觉额度），再让 AI 把文本整理成清单；OCR 失败自动回退现在的视觉识别。整单缴费单可从约 150 秒降到 10 秒内。</p>
        <div class="field" style="display:flex;justify-content:space-between;align-items:center"><label style="margin:0">启用 OCR 辅助</label><label class="switch"><input v-model="ocr.enabled" type="checkbox" /><span /></label></div>
        <div class="field"><label>OCR 服务商</label><SheetSelect v-model="ocr.provider" placeholder="选择 OCR 服务商，自动切换所需凭据" :options="ocrProviderOptions" />
          <p class="tiny muted" style="margin-top:6px">{{ ocrPreset.tip }}</p>
        </div>
        <div v-if="ocr.provider === 'custom'" class="field"><label>Base URL</label><input v-model="ocr.base_url" class="input" placeholder="https://api.example.com/v1（OpenAI 兼容）" autocomplete="off" spellcheck="false" /></div>
        <div class="field-row">
          <div class="field"><label>{{ ocrPreset.keyLabel }}</label><input v-model="ocr.api_key" class="input" :placeholder="ocrPreset.keyPlaceholder" autocomplete="off" spellcheck="false" /></div>
          <div class="field"><label>{{ ocrPreset.secretLabel }}{{ ocr.has_secret ? '（已保存，留空不改）' : '' }}</label><input v-model="ocr.secret_key" type="password" class="input" :placeholder="ocrPreset.secretPlaceholder" autocomplete="new-password" /></div>
        </div>
        <div v-if="ocr.provider === 'custom'" class="field"><label>模型</label><input v-model="ocr.model" class="input" placeholder="例如 qwen-vl-max / gpt-4o-mini / mimo-v2.6-flash" autocomplete="off" spellcheck="false" /></div>
        <div class="btn-row"><button class="btn btn-outline" :disabled="testingOCR" @click="testOCR">{{ testingOCR ? '测试中…' : '测试连接' }}</button></div>
        <p class="tiny muted" style="margin-top:8px;line-height:1.6">凭据加密保存。百度可直接验证双 Key；其它服务商需带图片才能验签，测试只做配置完整性检查，建议用一张单据实测一次。不开启则完全走原来的视觉识别，不影响任何功能。</p>
      </div>
      <div v-if="tab === 'backup'" class="card" style="margin-top:10px">
        <div class="section-head" style="margin-top:0">
          <h3 style="font-size:14.5px">数据备份</h3>
          <button class="btn btn-outline btn-sm" :disabled="creatingBackup" @click="createBackup">{{ creatingBackup ? '备份中…' : '立即备份' }}</button>
        </div>
        <p class="tiny muted" style="margin-bottom:10px">备份包含全部用药数据与图片。换机时先在旧机「下载」，再到新机「上传恢复」。</p>
        <div class="btn-row" style="margin-bottom:10px">
          <button class="btn btn-outline" :disabled="uploadingBackup" @click="pickBackupFile">{{ uploadingBackup ? '上传中…' : '上传恢复' }}</button>
          <button class="btn btn-outline" :disabled="loadingBackups" @click="loadBackups">{{ loadingBackups ? '刷新中…' : '刷新列表' }}</button>
        </div>
        <input ref="backupInput" type="file" accept=".db" class="hidden-input" @change="onBackupPicked" />
        <div v-if="loadingBackups && !backups.length" class="tiny muted">加载中…</div>
        <div v-else-if="!backups.length" class="tiny muted">还没有备份，点右上角「立即备份」生成第一份。</div>
        <div v-else class="list">
          <div v-for="b in backups" :key="b.name" class="backup-row">
            <div class="backup-meta">
              <div class="backup-name">{{ b.name }}</div>
              <div class="tiny muted">{{ formatSize(b.size) }} · {{ formatDateTime(b.created_at, true) }}</div>
            </div>
            <div class="backup-actions">
              <button class="btn btn-outline btn-sm" :disabled="downloading === b.name" @click="downloadBackup(b.name)">{{ downloading === b.name ? '下载中…' : '下载' }}</button>
              <button class="btn btn-primary btn-sm" :disabled="restoring === b.name" @click="restoreBackup(b.name)">{{ restoring === b.name ? '恢复中…' : '恢复' }}</button>
              <button class="btn btn-outline btn-sm" @click="removeBackup(b.name)">删除</button>
            </div>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppIcon from '../components/AppIcon.vue'
import { adminApi, backupApi, preferenceApi } from '../api/medcare'
import { formatDateTime } from '../utils/date'
import TimeField from '../components/TimeField.vue'
import SheetSelect from '../components/SheetSelect.vue'
import DictEditor from '../components/DictEditor.vue'
import { applyMedicineDict } from '../utils/medicineMeta'
import { useAuthStore } from '../stores/auth'
import { THEME_BRANDS, useThemeStore } from '../stores/theme'
import type { ThemeBrand, ThemeMode } from '../stores/theme'
import { useUIStore } from '../stores/ui'
import { errMessage, reportError } from '../utils/errors'
const auth = useAuthStore()
const theme = useThemeStore()
const ui = useUIStore()
const loading = ref(true)
// 设置分组：每个 tab 是一页，地址栏 ?tab= 记住位置（可直接分享 / 刷新保持）。
type SettingTab = 'rhythm' | 'theme' | 'ai' | 'dict' | 'mail' | 'security' | 'backup'
const route = useRoute()
const router = useRouter()
const settingTabs: { value: SettingTab; label: string; icon: string; adminOnly?: boolean }[] = [
  { value: 'rhythm', label: '提醒节奏', icon: 'bell' },
  { value: 'theme', label: '外观主题', icon: 'sparkles' },
  { value: 'ai', label: 'AI 识别', icon: 'camera', adminOnly: true },
  { value: 'dict', label: '药品字典', icon: 'pill', adminOnly: true },
  { value: 'mail', label: '邮件与渠道', icon: 'mail', adminOnly: true },
  { value: 'security', label: '注册与安全', icon: 'shield', adminOnly: true },
  { value: 'backup', label: '数据备份', icon: 'download', adminOnly: true },
]
function normalizeTab(v: any): SettingTab {
  const raw = String(v || '')
  const ok = settingTabs.some((t) => t.value === raw)
  return (ok ? raw : 'rhythm') as SettingTab
}
const tab = computed<SettingTab>(() => {
  const t = normalizeTab(route.query.tab)
  // 非管理员访问管理员分组时回落到提醒节奏，避免空白页。
  const def = settingTabs.find((x) => x.value === t)
  if (def?.adminOnly && !auth.isAdmin) return 'rhythm'
  return t
})
const isAdminTab = computed(() => !!settingTabs.find((t) => t.value === tab.value)?.adminOnly)
const visibleSettingTabs = computed(() => settingTabs.filter((t) => !t.adminOnly || auth.isAdmin))
function goTab(v: SettingTab) { router.replace({ path: '/settings', query: { tab: v } }) }
const defaultTheme = reactive<{ mode: string; brand: string }>({ mode: 'system', brand: 'a' })
// 普通用户注册是否需要管理员审核（系统级配置，仅管理员可见）。
const registerApproval = ref(false)
// 药品字典（剂型/规格/单位/包装规格）：系统级配置，超级管理员维护，前端下拉用。
const medicineDict = reactive({ form: '', spec: '', unit: '', packageSpec: '' })
const brandOptions = THEME_BRANDS.map((b) => ({ value: b.value, label: b.name }))
const modeOptions: { value: ThemeMode; label: string }[] = [
  { value: 'system', label: '跟随系统' },
  { value: 'light', label: '亮色' },
  { value: 'dark', label: '暗色' },
]
function setMode(value: ThemeMode) { theme.setMode(value); theme.persist() }
function setBrand(value: ThemeBrand) { theme.setBrand(value); theme.persist() }
const saving = ref(false)
const repeatOptions = [
  { value: 1, label: '每 1 分钟' }, { value: 2, label: '每 2 分钟' }, { value: 5, label: '每 5 分钟' },
  { value: 10, label: '每 10 分钟' }, { value: 15, label: '每 15 分钟' }, { value: 30, label: '每 30 分钟' }, { value: 60, label: '每 60 分钟' },
]
const pref = reactive<any>({ lead_minutes: 10, overdue_first_minutes: 15, overdue_repeat_minutes: 60, overdue_max_repeats: 2, daily_summary_enabled: true, daily_summary_time: '21:00', include_location_in_notifications: true, include_notes_in_notifications: false, low_stock_notify_enabled: true })
const smtp = reactive<any>({ smtp_host: '', smtp_port: 587, smtp_security: 'starttls', smtp_username: '', smtp_password: '', from_address: '', from_name: '用药管理', public_base_url: '' })
const ai = reactive<any>({ enabled: false, provider_type: '', provider_name: '', base_url: '', model: '', api_key: '', timeout_seconds: 60, has_key: false })
// 市面常见、提供 OpenAI 兼容接口的 AI 服务商。选一项会自动填 Base URL 与常用模型，
// 用户只需要再填自己的 API Key；没列出的服务商选「自定义」手动填。
const AI_PROVIDERS: { value: string; label: string; base_url: string; model: string; modes?: { value: string; label: string; base_url: string; model?: string; key_hint?: string }[] }[] = [
  { value: 'openai', label: 'OpenAI 官方', base_url: 'https://api.openai.com/v1', model: 'gpt-4o-mini' },
  { value: 'deepseek', label: 'DeepSeek 深度求索', base_url: 'https://api.deepseek.com/v1', model: 'deepseek-chat' },
  { value: 'dashscope', label: '通义千问（阿里云百炼）', base_url: 'https://dashscope.aliyuncs.com/compatible-mode/v1', model: 'qwen-plus', modes: [
    { value: 'payg', label: '按量付费', base_url: 'https://dashscope.aliyuncs.com/compatible-mode/v1', model: 'qwen-plus', key_hint: 'sk-…（百炼普通 Key）' },
    { value: 'coding', label: 'Coding Plan 套餐', base_url: 'https://coding.dashscope.aliyuncs.com/v1', model: 'qwen-plus', key_hint: 'Coding Plan 专属 Key' },
  ] },
  { value: 'ark', label: '豆包（火山方舟）', base_url: 'https://ark.cn-beijing.volces.com/api/v3', model: 'doubao-1-5-pro-32k-250115', modes: [
    { value: 'payg', label: '按量付费', base_url: 'https://ark.cn-beijing.volces.com/api/v3', model: 'doubao-1-5-pro-32k-250115', key_hint: '方舟普通 Key' },
    { value: 'coding', label: 'Coding Plan 套餐', base_url: 'https://ark.cn-beijing.volces.com/api/coding/v3', model: 'doubao-1-5-pro-32k-250115', key_hint: 'Coding Plan 专属 Key（用按量地址不扣套餐额度）' },
  ] },
  { value: 'zhipu', label: '智谱 GLM', base_url: 'https://open.bigmodel.cn/api/paas/v4', model: 'glm-4-flash', modes: [
    { value: 'payg', label: '按量付费', base_url: 'https://open.bigmodel.cn/api/paas/v4', model: 'glm-4-flash', key_hint: '普通 API Key' },
    { value: 'coding', label: 'Coding Plan 套餐', base_url: 'https://open.bigmodel.cn/api/coding/paas/v4', model: 'glm-4-flash', key_hint: 'Coding Plan 专属 Key' },
  ] },
  { value: 'moonshot', label: 'Kimi（月之暗面）', base_url: 'https://api.moonshot.cn/v1', model: 'moonshot-v1-8k' },
  { value: 'siliconflow', label: '硅基流动 SiliconFlow', base_url: 'https://api.siliconflow.cn/v1', model: 'Qwen/Qwen2.5-7B-Instruct' },
  { value: 'baichuan', label: '百川智能', base_url: 'https://api.baichuan-ai.com/v1', model: 'Baichuan4-Air' },
  { value: 'stepfun', label: '阶跃星辰 StepFun', base_url: 'https://api.stepfun.com/v1', model: 'step-1-flash' },
  { value: 'hunyuan', label: '腾讯混元', base_url: 'https://api.hunyuan.cloud.tencent.com/v1', model: 'hunyuan-turbos-latest' },
  { value: 'qianfan', label: '百度文心（千帆）', base_url: 'https://qianfan.baidubce.com/v2', model: 'ernie-4.0-8k' },
  { value: 'spark', label: '讯飞星火', base_url: 'https://spark-api-open.xf-yun.com/v1', model: 'generalv3.5' },
  { value: 'lingyiwanwu', label: '零一万物', base_url: 'https://api.lingyiwanwu.com/v1', model: 'yi-lightning' },
  { value: 'mimo', label: '小米 MiMo', base_url: 'https://api.xiaomimimo.com/v1', model: 'mimo-v2.6-flash', modes: [
    { value: 'payg', label: '按量付费', base_url: 'https://api.xiaomimimo.com/v1', model: 'mimo-v2.6-flash', key_hint: 'sk-…（按量付费 Key）' },
    { value: 'plan', label: 'Token Plan 套餐', base_url: 'https://token-plan-cn.xiaomimimo.com/v1', model: 'mimo-v2.6-flash', key_hint: 'tp-… / ttp-…（套餐专属 Key）' },
  ] },
  { value: 'openrouter', label: 'OpenRouter', base_url: 'https://openrouter.ai/api/v1', model: 'openai/gpt-4o-mini' },
  { value: 'groq', label: 'Groq', base_url: 'https://api.groq.com/openai/v1', model: 'llama-3.3-70b-versatile' },
  { value: 'together', label: 'Together AI', base_url: 'https://api.together.xyz/v1', model: 'meta-llama/Llama-3.3-70B-Instruct-Turbo' },
  { value: 'ollama', label: 'Ollama（本机）', base_url: 'http://127.0.0.1:11434/v1', model: 'llama3.2' },
  { value: 'lmstudio', label: 'LM Studio（本机）', base_url: 'http://127.0.0.1:1234/v1', model: 'local-model' },
  { value: 'custom', label: '自定义 / 未列出（手动填写）', base_url: '', model: '' },
]
const aiProvider = ref('custom')
// 自定义档的显示名用用户自填的服务商名称，下拉里不再只是一句「自定义」。
const aiProviderOptions = computed(() => AI_PROVIDERS.map((p) => ({
  value: p.value,
  label: p.value === 'custom' && ai.provider_name ? ai.provider_name + '（自定义）' : p.label,
})))
function normalizeBaseUrl(v: string) { return String(v || '').trim().replace(/\/+$/, '').toLowerCase() }
function detectAIProvider(baseUrl: string, providerType: string) {
  const byKey = AI_PROVIDERS.find((p) => p.value === providerType && p.value !== 'custom')
  if (byKey) return byKey.value
  const norm = normalizeBaseUrl(baseUrl)
  const byUrl = AI_PROVIDERS.find((p) => p.value !== 'custom' && p.base_url && (normalizeBaseUrl(p.base_url) === norm || (p.modes || []).some((m) => normalizeBaseUrl(m.base_url) === norm)))
  return byUrl ? byUrl.value : 'custom'
}
// 套餐 / 按量的地址不同：按已存地址反推计费方式，刷新后开关停在正确档位。
function detectAIMode(baseUrl: string, providerValue: string) {
  const modes = AI_PROVIDERS.find((p) => p.value === providerValue)?.modes || []
  if (!modes.length) return ''
  const norm = normalizeBaseUrl(baseUrl)
  return modes.find((m) => normalizeBaseUrl(m.base_url) === norm)?.value || modes[0].value
}
const aiMode = ref('')
const aiModes = computed(() => AI_PROVIDERS.find((p) => p.value === aiProvider.value)?.modes || [])
const aiKeyHint = computed(() => aiModes.value.find((m) => m.value === aiMode.value)?.key_hint || '')
function applyAIMode(value: string) {
  const mode = aiModes.value.find((m) => m.value === value)
  if (!mode) return
  aiMode.value = mode.value
  ai.base_url = mode.base_url
  if (mode.model) ai.model = mode.model
  aiModelOptions.value = []
  modelManual.value = true
}
function applyAIProvider(value: string | number) {
  aiProvider.value = String(value || 'custom')
  ai.provider_type = aiProvider.value
  const preset = AI_PROVIDERS.find((p) => p.value === aiProvider.value)
  if (!preset || preset.value === 'custom') { aiMode.value = ''; return }
  // 带计费方式的服务商默认落在第一档（按量付费），地址与模型随档位填。
  const mode = preset.modes?.[0]
  aiMode.value = mode?.value || ''
  ai.base_url = mode?.base_url || preset.base_url
  ai.model = mode?.model || preset.model
  aiModelOptions.value = []
  modelManual.value = true
}
const savingAI = ref(false)
const testingAI = ref(false)
const loadingModels = ref(false)
const modelManual = ref(false)
const aiModelOptions = ref<{ value: string; label: string }[]>([])
const aiBaseline = ref('')
const aiDirty = computed(() => (ai.api_key ? true : aiSnapshot() !== aiBaseline.value))
function aiSnapshot() {
  return JSON.stringify({
    enabled: !!ai.enabled,
    base_url: String(ai.base_url || '').trim(),
    model: String(ai.model || '').trim(),
    provider_type: String(ai.provider_type || ''),
    provider_name: String(ai.provider_name || ''),
    timeout_seconds: Number(ai.timeout_seconds) || 0,
  })
}
function markAISaved() { aiBaseline.value = aiSnapshot() }
function aiPayload() {
  return {
    base_url: String(ai.base_url || '').trim(),
    api_key: String(ai.api_key || '').trim(),
    model: String(ai.model || '').trim(),
    timeout_seconds: Number(ai.timeout_seconds) || 0,
  }
}
const smtpSecurityOptions = [
  { value: 'starttls', label: 'STARTTLS' },
  { value: 'ssl', label: 'SSL' },
  { value: 'none', label: '不加密' },
]
async function load() {
  try { const res = await preferenceApi.get(); if (res.data?.code === 0) Object.assign(pref, res.data.data) } catch (e) { reportError(e, '读取偏好失败') }
  if (auth.isAdmin) {
    try { const s = await adminApi.getSMTP(); if (s.data?.code === 0) Object.assign(smtp, s.data.data) } catch (e) { reportError(e, '读取邮件配置失败') }
    try {
      const a = await adminApi.getAI()
      if (a.data?.code === 0) {
        Object.assign(ai, a.data.data)
        aiProvider.value = detectAIProvider(ai.base_url, ai.provider_type)
        aiMode.value = detectAIMode(ai.base_url, aiProvider.value)
        if (!ai.provider_type) ai.provider_type = aiProvider.value
        markAISaved()
      }
    } catch (e) { reportError(e, '读取 AI 配置失败') }
    try {
      const o = await adminApi.getOCR()
      if (o.data?.code === 0) { Object.assign(ocr, o.data.data); ocr.secret_key = ''; markOCRSaved() }
    } catch (e) { reportError(e, '读取 OCR 配置失败') }
    await loadDefaultTheme()
    await loadBackups()
  }
  loading.value = false
}
async function save() {
  saving.value = true
  try {
    await preferenceApi.update({ ...pref, theme_mode: theme.mode, theme_brand: theme.brand })
    ui.showToast('偏好已保存')
  } catch (e: any) {
    ui.showToast(e?.response?.data?.message || '保存失败', 'error')
  } finally {
    saving.value = false
  }
}
async function loadDefaultTheme() {
  try {
    const res = await adminApi.getSystemConfigs()
    const data = res.data?.data || {}
    if (data.medcare_default_theme_mode) defaultTheme.mode = data.medcare_default_theme_mode
    if (data.medcare_default_theme_brand) defaultTheme.brand = data.medcare_default_theme_brand
    registerApproval.value = String(data.medcare_register_approval || 'false') === 'true'
    medicineDict.form = String(data.medcare_form_options || '')
    medicineDict.spec = String(data.medcare_spec_options || '')
    medicineDict.unit = String(data.medcare_unit_options || '')
    medicineDict.packageSpec = String(data.medcare_package_spec_options || '')
    applyMedicineDict(medicineDict)
  } catch (e) { reportError(e, '读取默认主题失败') }
}
async function saveDefaultTheme() {
  try {
    await adminApi.saveConfig('medcare_default_theme_mode', defaultTheme.mode)
    await adminApi.saveConfig('medcare_default_theme_brand', defaultTheme.brand)
    ui.showToast('默认主题已保存')
  } catch (e: any) {
    ui.showToast(e?.response?.data?.message || '保存失败', 'error')
  }
}
async function saveMedicineDict() {
  try {
    await adminApi.saveConfig('medcare_form_options', medicineDict.form)
    await adminApi.saveConfig('medcare_spec_options', medicineDict.spec)
    await adminApi.saveConfig('medcare_unit_options', medicineDict.unit)
    await adminApi.saveConfig('medcare_package_spec_options', medicineDict.packageSpec)
    // 立即应用到本页运行时字典，保存后新建药品就能看到新选项。
    applyMedicineDict(medicineDict)
    ui.showToast('药品字典已保存')
  } catch (e: any) {
    ui.showToast(e?.response?.data?.message || '保存失败', 'error')
  }
}
async function saveRegisterApproval() {
  try {
    await adminApi.saveConfig('medcare_register_approval', registerApproval.value ? 'true' : 'false')
    ui.showToast(registerApproval.value ? '已开启注册审核' : '已关闭注册审核，注册后可直接登录')
  } catch (e: any) {
    ui.showToast(e?.response?.data?.message || '保存失败', 'error')
  }
}
async function saveSMTP() { try { await adminApi.saveSMTP({ ...smtp }); smtp.smtp_password = ''; ui.showToast('SMTP 已保存') } catch (e: any) { ui.showToast(e?.response?.data?.message || '保存失败', 'error') } }
async function testSMTP() { try { await adminApi.testSMTP(smtp.from_address); ui.showToast('测试邮件已发送') } catch (e: any) { ui.showToast(e?.response?.data?.message || '测试失败', 'error') } }
async function saveAI() {
  if (savingAI.value) return
  savingAI.value = true
  try {
    await adminApi.saveAI({ ...ai })
    ai.api_key = ''
    ai.has_key = true
    markAISaved()
    ui.showToast('AI 配置已保存')
  } catch (e: any) {
    ui.showToast(e?.response?.data?.message || '保存失败', 'error')
  } finally {
    savingAI.value = false
  }
}
// 测试连接直接用页面上填的值，不必先保存；只校验填没填。
async function testAI() {
  if (testingAI.value) return
  if (!String(ai.base_url || '').trim()) return ui.showToast('请先填写 Base URL', 'error')
  if (!String(ai.model || '').trim()) return ui.showToast('请先填写模型名称', 'error')
  if (!String(ai.api_key || '').trim() && !ai.has_key) return ui.showToast('请先填写 API Key', 'error')
  testingAI.value = true
  try {
    await adminApi.testAI(aiPayload())
    ui.showToast('AI 连接成功')
  } catch (e: any) {
    ui.showToast(e?.response?.data?.message || '测试失败', 'error')
  } finally {
    testingAI.value = false
  }
}
// 用页面上的 Base URL 与 API Key 拉 OpenAI 兼容的模型列表，供选择。
async function fetchAIModels(silent = false) {
  if (loadingModels.value) return
  if (!String(ai.base_url || '').trim()) { if (!silent) ui.showToast('请先填写 Base URL', 'error'); return }
  if (!String(ai.api_key || '').trim() && !ai.has_key) { if (!silent) ui.showToast('请先填写 API Key', 'error'); return }
  loadingModels.value = true
  try {
    const res = await adminApi.listAIModels(aiPayload())
    const list: string[] = res.data?.data?.models || []
    if (!list.length) throw new Error('服务返回的模型列表为空')
    const current = String(ai.model || '').trim()
    const options = list.map((id) => ({ value: id, label: id }))
    if (current && !list.includes(current)) options.unshift({ value: current, label: current })
    aiModelOptions.value = options
    modelManual.value = false
    if (!current) ai.model = list[0]
    if (!silent) ui.showToast(`已获取 ${list.length} 个模型`)
  } catch (e: any) {
    if (!silent) ui.showToast(e?.response?.data?.message || e?.message || '获取模型列表失败', 'error')
  } finally {
    loadingModels.value = false
  }
}
// 先填 Key 再选模型：Key / 地址填好后自动拉一次模型列表（静默，失败不打扰），
// 拉到了模型字段就变成下拉供选择；手动按钮仍保留。
let autoFetchTimer: number | null = null
watch([() => ai.api_key, () => ai.base_url], () => {
  if (autoFetchTimer) window.clearTimeout(autoFetchTimer)
  autoFetchTimer = window.setTimeout(() => {
    const key = String(ai.api_key || '').trim()
    const url = String(ai.base_url || '').trim()
    if (key.length >= 8 && url) fetchAIModels(true)
  }, 900)
})
// ---------- OCR 辅助（可选，仅管理员）----------
// 先 OCR 读字再让文本模型整理清单：秒级、省视觉额度；失败回退视觉识别。
const ocr = reactive<any>({ enabled: false, provider: 'baidu', base_url: '', api_key: '', secret_key: '', model: '', has_secret: false })
// OCR 服务商预设：像文本模型一样可选，按服务商切换所需凭据与提示。
const OCR_PROVIDERS: { value: string; label: string; keyLabel: string; keyPlaceholder: string; secretLabel: string; secretPlaceholder: string; tip: string }[] = [
  { value: 'baidu', label: '百度 OCR（通用文字识别高精度）', keyLabel: 'API Key', keyPlaceholder: '百度智能云 API Key', secretLabel: 'Secret Key', secretPlaceholder: '百度智能云 Secret Key', tip: '百度智能云 → 文字识别 → 创建应用，拿 API Key 与 Secret Key；通用文字识别高精度版有免费额度。' },
  { value: 'tencent', label: '腾讯云 OCR（通用印刷体高精度）', keyLabel: 'SecretId', keyPlaceholder: '腾讯云 SecretId', secretLabel: 'SecretKey', secretPlaceholder: '腾讯云 SecretKey', tip: '腾讯云控制台 → 访问管理 → API 密钥，拿 SecretId 与 SecretKey；需开通文字识别服务。' },
  { value: 'aliyun', label: '阿里云 OCR（RecognizeGeneral）', keyLabel: 'AccessKey ID', keyPlaceholder: '阿里云 AccessKey ID', secretLabel: 'AccessKey Secret', secretPlaceholder: '阿里云 AccessKey Secret', tip: '阿里云控制台 → AccessKey 管理，拿 AccessKey ID / Secret；需开通文字识别 OCR 服务。' },
  { value: 'youdao', label: '有道智云 OCR（通用文字识别）', keyLabel: '应用 ID', keyPlaceholder: '有道应用 ID', secretLabel: '应用密钥', secretPlaceholder: '有道应用密钥', tip: '有道智云控制台 → 应用管理 → 创建应用（选通用 OCR），拿应用 ID 与应用密钥。' },
  { value: 'deepseek', label: 'DeepSeek OCR（按页识别）', keyLabel: 'API Key', keyPlaceholder: 'DeepSeek API Key（sk-...）', secretLabel: 'Secret Key（本档不用）', secretPlaceholder: '可留空', tip: 'DeepSeek → API Keys 创建 API Key；图片按页识别，1.5 元/1000 页。官方文档 api-docs.deepseek.com/zh-cn/guides/ocr。' },
  { value: 'custom', label: '自定义（OpenAI 兼容视觉模型）', keyLabel: 'API Key', keyPlaceholder: 'sk-...', secretLabel: 'Secret Key（本档不用）', secretPlaceholder: '可留空', tip: '任何 OpenAI 兼容的多模态模型都能当 OCR 用：填 Base URL、API Key 与模型名（如 qwen-vl-max、mimo-v2.6-flash）。' },
]
const ocrProviderOptions = OCR_PROVIDERS.map((p) => ({ value: p.value, label: p.label }))
const ocrPreset = computed(() => OCR_PROVIDERS.find((p) => p.value === ocr.provider) || OCR_PROVIDERS[0])
const savingOCR = ref(false)
const testingOCR = ref(false)
const ocrBaseline = ref('')
function ocrSnapshot() {
  return JSON.stringify({ enabled: !!ocr.enabled, provider: String(ocr.provider || ''), base_url: String(ocr.base_url || '').trim(), api_key: String(ocr.api_key || '').trim(), model: String(ocr.model || '').trim() })
}
const ocrDirty = computed(() => (ocr.secret_key ? true : ocrSnapshot() !== ocrBaseline.value))
function markOCRSaved() { ocrBaseline.value = ocrSnapshot() }
async function saveOCR() {
  if (savingOCR.value) return
  savingOCR.value = true
  try {
    const res = await adminApi.saveOCR({ enabled: !!ocr.enabled, provider: ocr.provider, base_url: String(ocr.base_url || '').trim(), api_key: String(ocr.api_key || '').trim(), secret_key: String(ocr.secret_key || '').trim(), model: String(ocr.model || '').trim() })
    if (res.data?.code !== 0) throw new Error(res.data?.message || '保存失败')
    if (ocr.secret_key) ocr.has_secret = true
    ocr.secret_key = ''
    markOCRSaved()
    ui.showToast('OCR 配置已保存')
  } catch (e: any) {
    ui.showToast(e?.response?.data?.message || e?.message || '保存 OCR 配置失败', 'error')
  } finally {
    savingOCR.value = false
  }
}
async function testOCR() {
  if (testingOCR.value) return
  // 用当前表单里的值直接测试，不必先保存（与 AI 测试一致）。
  if (!String(ocr.api_key || '').trim() && !ocr.has_key) return ui.showToast('请先填写 API Key', 'error')
  testingOCR.value = true
  try {
    const res = await adminApi.testOCR({ provider: ocr.provider, base_url: String(ocr.base_url || '').trim(), api_key: String(ocr.api_key || '').trim(), secret_key: String(ocr.secret_key || '').trim(), model: String(ocr.model || '').trim() })
    if (res.data?.code !== 0) throw new Error(res.data?.message || '测试失败')
    ui.showToast('OCR 连接正常（配置检查通过）')
  } catch (e: any) {
    ui.showToast(e?.response?.data?.message || e?.message || 'OCR 连接测试失败', 'error')
  } finally {
    testingOCR.value = false
  }
}

// ---------- 数据备份（仅管理员）----------
// 上传与恢复两步分离：上传只把文件存成一份备份，恢复仍走列表行的「恢复」，
// 完整复用确认弹窗与服务端的恢复前快照。
const backups = ref<any[]>([])
const loadingBackups = ref(false)
const creatingBackup = ref(false)
const uploadingBackup = ref(false)
const restoring = ref('')
const downloading = ref('')
const backupInput = ref<HTMLInputElement | null>(null)

function formatSize(bytes: number): string {
  const n = Number(bytes) || 0
  if (n < 1024) return n + ' B'
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB'
  return (n / 1024 / 1024).toFixed(1) + ' MB'
}
async function loadBackups() {
  loadingBackups.value = true
  try {
    const res = await backupApi.list()
    backups.value = res.data?.data?.items || []
  } catch (e: any) {
    ui.showToast(errMessage(e, '读取备份列表失败'), 'error')
  } finally {
    loadingBackups.value = false
  }
}
async function createBackup() {
  if (creatingBackup.value) return
  creatingBackup.value = true
  try {
    await backupApi.create()
    ui.showToast('备份已生成')
    await loadBackups()
  } catch (e: any) {
    ui.showToast(errMessage(e, '备份失败'), 'error')
  } finally {
    creatingBackup.value = false
  }
}
function pickBackupFile() {
  if (!uploadingBackup.value) backupInput.value?.click()
}
async function onBackupPicked(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = '' // 允许再次选择同一个文件
  if (!file) return
  // 前端只做后缀预检，服务端校验（SQLite 头 / 完整性 / users 表）才是权威。
  if (!file.name.toLowerCase().endsWith('.db')) {
    ui.showToast('只支持 .db 备份文件', 'error')
    return
  }
  uploadingBackup.value = true
  try {
    const form = new FormData()
    form.append('file', file)
    await backupApi.upload(form)
    ui.showToast('上传成功，点该行的「恢复」完成恢复', 'success', 5000)
    await loadBackups()
  } catch (e: any) {
    ui.showToast(errMessage(e, '上传失败'), 'error')
  } finally {
    uploadingBackup.value = false
  }
}
async function restoreBackup(name: string) {
  const ok = await ui.confirm({
    title: '确认恢复这份备份？',
    message: `将用 ${name} 覆盖当前全部数据（用药本、药品、计划、用药记录与图片）。系统会先自动留一份恢复前快照，恢复后应用会重启，约半分钟后可刷新使用。`,
    confirmText: '确认恢复',
    danger: true,
  })
  if (!ok) return
  restoring.value = name
  try {
    await backupApi.restore(name)
    ui.showToast('恢复完成，应用正在重启，请稍后刷新页面', 'success', 8000)
  } catch (e: any) {
    ui.showToast(errMessage(e, '恢复失败'), 'error')
  } finally {
    restoring.value = ''
  }
}
async function downloadBackup(name: string) {
  if (downloading.value) return
  downloading.value = name
  try {
    const res = await backupApi.download(name)
    const url = URL.createObjectURL(res.data)
    const a = document.createElement('a')
    a.href = url
    a.download = name
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(url)
  } catch (e: any) {
    ui.showToast(errMessage(e, '下载失败'), 'error')
  } finally {
    downloading.value = ''
  }
}
async function removeBackup(name: string) {
  const ok = await ui.confirm({
    title: '删除这份备份？',
    message: `将删除 ${name}，删除后不可找回。当前数据不受影响。`,
    confirmText: '删除',
    danger: true,
  })
  if (!ok) return
  try {
    await backupApi.remove(name)
    ui.showToast('备份已删除')
    await loadBackups()
  } catch (e: any) {
    ui.showToast(errMessage(e, '删除失败'), 'error')
  }
}

onMounted(load)
</script>

<style scoped>
/* 设置分组：一排可横向滑动的按钮，手机上也不会挤成两行 */
.settings-tabs { display: flex; gap: 6px; overflow-x: auto; padding: 2px 0 10px; scrollbar-width: none; }
.settings-tabs::-webkit-scrollbar { display: none; }
.settings-tab { flex: 0 0 auto; display: inline-flex; align-items: center; gap: 6px; min-height: 38px; padding: 0 12px; border-radius: 999px; border: 1px solid var(--mc-border); background: var(--mc-surface); color: var(--mc-text-muted); font-size: 13px; font-weight: 650; }
.settings-tab.active { border-color: var(--mc-primary); background: var(--mc-primary-soft); color: var(--mc-primary-strong); }
.switch-row { display: flex; align-items: flex-start; gap: 10px; padding: 4px 0 2px; cursor: pointer; }
.switch-input { flex: 0 0 auto; width: 22px; height: 22px; margin-top: 2px; accent-color: var(--mc-primary); }
.switch-body { display: grid; gap: 3px; min-width: 0; }
.switch-body b { font-size: 14px; }
.btn-row { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; }
.btn-row .btn { width: 100%; }
.hidden-input { display: none; }
.backup-row { display: grid; gap: 8px; padding: 10px 12px; border-radius: 12px; background: var(--mc-surface-2); }
.backup-meta { min-width: 0; }
.backup-name { font-size: 13px; font-weight: 700; overflow-wrap: anywhere; }
.backup-actions { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 6px; }
/* 手机端触控目标不小于 44px（家族手机端规范） */
.backup-actions .btn { width: 100%; min-height: 40px; }
.link-btn { display: inline-block; padding: 13px 4px; margin: -13px -4px; border: 0; background: transparent; color: var(--mc-primary); font-size: 11px; font-weight: 700; text-decoration: underline; text-underline-offset: 2px; white-space: nowrap; }
.link-btn:disabled { opacity: .5; }
.dirty-hint { margin-top: 8px; color: var(--mc-warning); font-weight: 650; }
.brand-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 6px; }
.brand-option { display: flex; align-items: center; gap: 6px; min-height: 34px; padding: 0 10px; border-radius: 11px; border: 1px solid var(--mc-border); background: var(--mc-surface); font-size: 12.5px; font-weight: 650; color: var(--mc-text-muted); white-space: nowrap; overflow: hidden; }
.brand-option > span:last-child { overflow: hidden; text-overflow: ellipsis; }
.brand-option.active { border-color: var(--mc-primary); background: var(--mc-primary-soft); color: var(--mc-primary-strong); }
.brand-dot { width: 16px; height: 16px; border-radius: 6px; flex: 0 0 auto; }
.preview-strip { display: flex; align-items: center; gap: 8px; padding: 10px; border-radius: 12px; background: var(--mc-surface-2); }
.preview-text { flex: 1; min-width: 0; }
.pv-dot { width: 30px; height: 30px; border-radius: 11px; background: var(--mc-primary-soft); box-shadow: inset 0 0 0 7px var(--mc-primary); flex: 0 0 auto; }
/* 外壳撑到 44×44 保证触控，视觉轨道仍是 42×26（靠 inset 居中） */
.switch { position: relative; display: inline-flex; width: 44px; height: 44px; flex: 0 0 auto; }
.switch input { opacity: 0; width: 0; height: 0; }
.switch span { position: absolute; inset: 9px 1px; border-radius: 999px; background: var(--mc-surface-2); border: 1px solid var(--mc-border); transition: .15s; }
.switch span::after { content: ''; position: absolute; width: 20px; height: 20px; left: 2px; top: 2px; border-radius: 50%; background: #fff; box-shadow: 0 1px 4px rgba(0,0,0,.15); transition: .15s; }
.switch input:checked + span { background: var(--mc-primary); border-color: var(--mc-primary); }
.switch input:checked + span::after { transform: translateX(16px); }
@media (min-width: 900px) {
  .backup-row { grid-template-columns: minmax(0, 1fr) auto; align-items: center; gap: 12px; }
  .backup-actions { grid-template-columns: repeat(3, auto); }
  .backup-actions .btn { width: auto; }
  .brand-grid { gap: 8px; }
  .brand-option { min-height: 44px; padding: 0 12px; border-radius: 12px; font-size: 13px; gap: 8px; }
  .brand-dot { width: 18px; height: 18px; }
  .preview-strip { gap: 10px; padding: 12px; border-radius: 14px; }
  .pv-dot { width: 34px; height: 34px; border-radius: 12px; box-shadow: inset 0 0 0 8px var(--mc-primary); }
}
</style>
