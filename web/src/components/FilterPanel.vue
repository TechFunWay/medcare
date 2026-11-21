<template>
  <div class="filter-panel">
    <div class="fp-head">
      <div class="fp-search"><slot name="search" /></div>
      <button type="button" class="fp-go" @click="emit('search')">查询</button>
      <button type="button" class="fp-toggle" :class="{ active: open || props.count > 0 }" :aria-expanded="open" @click="open = !open">
        <AppIcon name="filter" :size="16" />
        <span class="fp-label">{{ props.count > 0 ? '筛选 · ' + props.count : '筛选' }}</span>
        <AppIcon name="down" :size="14" class="fp-caret" :class="{ up: open }" />
      </button>
    </div>
    <div v-show="open" class="fp-body">
      <slot />
      <div class="fp-foot">
        <button type="button" class="btn btn-outline btn-block" @click="emit('reset')">重置条件</button>
        <button type="button" class="btn btn-primary btn-block" @click="emit('search'); open = false">查询</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
// 查询条件默认折叠：一行只留搜索框、「查询」按钮与「筛选」按钮（带已选条数），
// 点开才显示更多条件。条件改动不自动查询，统一由「查询」按钮触发，避免边选边刷新。
import AppIcon from './AppIcon.vue'

const props = withDefaults(defineProps<{ count?: number }>(), { count: 0 })
const emit = defineEmits<{ (e: 'search'): void; (e: 'reset'): void }>()
const open = defineModel<boolean>('open', { default: false })
</script>

<style scoped>
.filter-panel { margin-bottom: 10px; }
.fp-head { display: flex; align-items: center; gap: 6px; }
.fp-search { flex: 1 1 auto; min-width: 0; }
/* 搜索框与右侧「查询 / 筛选」按钮齐平：按钮 40px，输入框 38px + 卡片 1px 边框 = 40px。 */
.fp-search :deep(.card) { margin-bottom: 0; padding-top: 0; padding-bottom: 0; }
.fp-search :deep(input) { min-height: 38px; }
.fp-go { flex: 0 0 auto; min-height: 40px; padding: 0 12px; border-radius: 10px; border: 1px solid var(--mc-primary); background: var(--mc-primary); color: #fff; font-size: 12.5px; font-weight: 700; white-space: nowrap; }
.fp-toggle { flex: 0 0 auto; display: inline-flex; align-items: center; gap: 4px; min-height: 40px; padding: 0 10px; border-radius: 10px; border: 1px solid var(--mc-border); background: var(--mc-surface); color: var(--mc-text-muted); font-size: 12.5px; font-weight: 650; white-space: nowrap; }
.fp-toggle.active { border-color: var(--mc-primary); background: var(--mc-primary-soft); color: var(--mc-primary-strong); }
.fp-caret { transition: transform .15s ease; }
.fp-caret.up { transform: rotate(180deg); }
.fp-body { margin-top: 8px; padding: 10px 12px; border: 1px solid var(--mc-border); border-radius: 12px; background: var(--mc-surface); }
.fp-body :deep(.chip-row) { margin-bottom: 8px; }
.fp-foot { display: grid; grid-template-columns: 1fr 1fr; gap: 8px; margin-top: 4px; }
</style>
