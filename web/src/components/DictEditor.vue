<template>
  <div class="dict-editor">
    <div v-if="list.length" class="dict-chips">
      <span v-for="(v, i) in list" :key="v + i" class="dict-chip">
        <input v-model="list[i]" class="dict-input" @blur="commit" @keyup.enter="commit" />
        <button type="button" class="dict-x" :aria-label="'删除 ' + v" @click="removeAt(i)"><AppIcon name="close" :size="13" /></button>
      </span>
    </div>
    <p v-else class="tiny muted" style="margin:2px 0 8px">还没有自定义项，用下面的输入框添加。</p>
    <div class="dict-add">
      <input v-model="draft" class="input" :placeholder="placeholder || '输入后点添加'" @keyup.enter="add" />
      <button type="button" class="btn btn-outline btn-sm" :disabled="!draft.trim()" @click="add">添加</button>
    </div>
  </div>
</template>

<script setup lang="ts">
// 字典编辑器：值以逗号分隔的字符串存取，界面上是「可改可删的胶囊 + 新增输入框」。
// 超级管理员在「偏好设置 → 药品字典」里用它维护剂型 / 规格 / 单位 / 包装规格。
import { ref, watch } from 'vue'
import AppIcon from './AppIcon.vue'

const props = defineProps<{ modelValue: string; placeholder?: string }>()
const emit = defineEmits<{ (e: 'update:modelValue', value: string): void }>()

const list = ref<string[]>([])
const draft = ref('')

function parse(raw: string): string[] {
  return String(raw || '').split(/[,，\n]/).map((v) => v.trim()).filter(Boolean)
}
watch(() => props.modelValue, (v) => {
  const next = parse(v)
  // 内容一致就不重建，避免正在编辑的输入框被重置。
  if (next.join('|') !== list.value.join('|')) list.value = next
}, { immediate: true })

function commit() {
  const cleaned = list.value.map((v) => v.trim()).filter(Boolean)
  const seen = new Set<string>()
  const dedup = cleaned.filter((v) => (seen.has(v) ? false : (seen.add(v), true)))
  list.value = dedup
  emit('update:modelValue', dedup.join(','))
}
function add() {
  const v = draft.value.trim()
  if (!v) return
  if (list.value.includes(v)) { draft.value = ''; return }
  list.value = [...list.value, v]
  draft.value = ''
  commit()
}
function removeAt(i: number) {
  list.value = list.value.filter((_, idx) => idx !== i)
  commit()
}
</script>

<style scoped>
.dict-chips { display: flex; flex-wrap: wrap; gap: 6px; margin-bottom: 8px; }
.dict-chip { display: inline-flex; align-items: center; gap: 2px; padding: 2px 2px 2px 8px; border-radius: 999px; border: 1px solid var(--mc-border); background: var(--mc-surface-2); }
.dict-input { width: 5.6em; min-width: 3em; border: 0; background: transparent; font-size: 12.5px; font-weight: 650; color: var(--mc-text); outline: none; }
.dict-x { display: grid; place-items: center; width: 22px; height: 22px; border: 0; border-radius: 50%; background: transparent; color: var(--mc-text-muted); }
.dict-x:hover { background: var(--mc-danger-soft); color: var(--mc-danger); }
.dict-add { display: flex; gap: 8px; align-items: center; }
.dict-add .input { flex: 1 1 auto; min-width: 0; }
.dict-add .btn { flex: 0 0 auto; }
</style>
