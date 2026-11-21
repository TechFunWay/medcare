<template>
  <div class="section collapse card" :class="{ open }">
    <button
      type="button"
      class="collapse-head"
      :aria-expanded="open ? 'true' : 'false'"
      :aria-controls="panelId"
      @click="open = !open"
    >
      <span v-if="icon" class="collapse-icon"><AppIcon :name="icon" :size="19" /></span>
      <span class="collapse-head-main">
        <span class="collapse-title">{{ title }}</span>
        <span v-if="hint && !open" class="collapse-hint">{{ hint }}</span>
      </span>
      <span class="collapse-tail">
        <slot name="tail" :open="open" />
        <AppIcon name="chevron" :size="17" class="collapse-chevron" />
      </span>
    </button>
    <div v-show="open" :id="panelId" class="collapse-body">
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
// 低频表单区块的折叠外壳：默认收起成一张标题卡（和「我的」页的入口卡同样式），点标题展开表单。
// 用法：<CollapseSection v-model:open="sqOpen" title="安全问题" icon="lock" hint="忘记密码时凭答案重置">
//         <template #tail><span class="badge done">已设置</span></template>
//         …表单内容…
//       </CollapseSection>
// 不绑 v-model 时由组件自己维护开合（默认收起），方便其他页面直接复用。
import { useId } from 'vue'
import AppIcon from './AppIcon.vue'

defineProps<{ title: string; hint?: string; icon?: string }>()
const open = defineModel<boolean>({ default: false })
const panelId = useId()
</script>

<style scoped>
/* 卡片内边距交给标题行与内容块各自负责，展开时才出现两者之间的分隔线。 */
.collapse { padding: 0; }
.collapse-head {
  display: flex; align-items: center; gap: 10px; width: 100%; min-height: 56px; padding: 10px 12px;
  background: none; border: 0; border-radius: inherit; text-align: left; color: inherit;
  transition: background .15s ease;
}
.collapse-head:hover, .collapse-head:active { background: var(--mc-surface-2); }
.collapse-icon {
  display: inline-flex; align-items: center; justify-content: center; flex: 0 0 auto;
  width: 34px; height: 34px; border-radius: 11px;
  background: var(--mc-primary-soft); color: var(--mc-primary-strong);
}
.collapse-head-main { display: grid; gap: 2px; flex: 1 1 auto; min-width: 0; }
.collapse-title { font-size: 14.5px; font-weight: 750; overflow-wrap: anywhere; }
.collapse-hint { font-size: 11.5px; color: var(--mc-text-muted); line-height: 1.5; overflow-wrap: anywhere; }
.collapse-tail { display: inline-flex; align-items: center; gap: 8px; flex: 0 0 auto; color: var(--mc-text-muted); }
.collapse-chevron { flex: 0 0 auto; transition: transform .18s ease; }
.collapse.open .collapse-chevron { transform: rotate(90deg); }
.collapse-body { padding: 0 12px 12px; }
.collapse.open .collapse-body { margin-top: 2px; padding: 12px; border-top: 1px dashed var(--mc-border); }
@media (min-width: 900px) {
  .collapse-head { padding: 12px 14px; }
  .collapse-body { padding: 0 14px 14px; }
  .collapse.open .collapse-body { padding: 14px; }
  .collapse-title { font-size: 15px; }
}
</style>
