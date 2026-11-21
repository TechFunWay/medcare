<template>
  <Transition name="toast">
    <div v-if="ui.toast" class="toast" :class="ui.toast.type" role="status">
      {{ ui.toast.message }}
    </div>
  </Transition>
</template>

<script setup lang="ts">
import { useUIStore } from '../stores/ui'
const ui = useUIStore()
</script>

<style scoped>
.toast-enter-active, .toast-leave-active { transition: opacity .18s ease, transform .18s ease; }
.toast-enter-from, .toast-leave-to { opacity: 0; transform: translate(-50%, 8px); }
</style>
