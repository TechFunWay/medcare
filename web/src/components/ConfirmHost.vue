<template>
  <Teleport to="body">
    <Transition name="modal">
      <div v-if="ui.confirmState" class="modal-backdrop" @click.self="ui.resolveConfirm(false)">
        <section class="modal" role="dialog" aria-modal="true">
          <h3>{{ ui.confirmState.title }}</h3>
          <p>{{ ui.confirmState.message }}</p>
          <div class="modal-actions">
            <button v-if="!ui.confirmState.hideCancel" class="btn btn-outline btn-block" @click="ui.resolveConfirm(false)">取消</button>
            <button class="btn btn-block" :class="ui.confirmState.danger ? 'btn-danger' : 'btn-primary'" @click="ui.resolveConfirm(true)">
              {{ ui.confirmState.confirmText }}
            </button>
          </div>
        </section>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { useUIStore } from '../stores/ui'
const ui = useUIStore()
</script>

<style scoped>
/* 确认文案里常带多行清单（删除前的绑定关系、盘点提示等），保留换行才读得清。 */
.modal p { white-space: pre-line; }
.modal-enter-active, .modal-leave-active { transition: opacity .18s ease; }
.modal-enter-from, .modal-leave-to { opacity: 0; }
</style>
