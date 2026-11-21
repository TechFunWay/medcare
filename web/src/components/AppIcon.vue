<template>
  <svg :width="size" :height="size" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
    <path v-for="(p, i) in paths" :key="i" :d="p" />
  </svg>
</template>

<script setup lang="ts">
import { computed } from 'vue'
const props = withDefaults(defineProps<{ name: string; size?: number }>(), { size: 22 })
const icons: Record<string, string[]> = {
  home: ['M3 11.5 12 4l9 7.5', 'M5 10v10h14V10'],
  calendar: ['M5 5h14a1 1 0 0 1 1 1v13H4V6a1 1 0 0 1 1-1Z', 'M8 3v4M16 3v4M4 10h16'],
  pill: ['M8.5 5.5a4 4 0 0 1 5.7 0l4.3 4.3a4 4 0 0 1-5.7 5.7L8.5 11.2a4 4 0 0 1 0-5.7Z', 'M7 13 13 7'],
  chart: ['M4 20V10M10 20V4M16 20v-7M22 20H2'],
  user: ['M12 12a4 4 0 1 0 0-8 4 4 0 0 0 0 8Z', 'M4 21a8 8 0 0 1 16 0'],
  plus: ['M12 5v14M5 12h14'],
  bell: ['M18 8a6 6 0 0 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9', 'M10 21h4'],
  chevron: ['m9 5 7 7-7 7'],
  down: ['m6 9 6 6 6-6'],
  check: ['m5 12 4 4L19 6'],
  clock: ['M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18Z', 'M12 7v5l3 2'],
  heart: ['M12 20.5s-7.5-4.6-7.5-9.6A4.4 4.4 0 0 1 12 7.9a4.4 4.4 0 0 1 7.5 3c0 5-7.5 9.6-7.5 9.6Z'],
  location: ['M20 10c0 5-8 11-8 11S4 15 4 10a8 8 0 1 1 16 0Z', 'M12 12a2.5 2.5 0 1 0 0-5 2.5 2.5 0 0 0 0 5Z'],
  prescription: ['M7 3h8l4 4v14H7a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2Z', 'M14 3v5h5', 'M8 13h8M8 17h5'],
  book: ['M5 4h11a2 2 0 0 1 2 2v14H7a2 2 0 0 1-2-2V4Z', 'M5 18a2 2 0 0 1 2-2h11'],
  members: ['M9 11a3 3 0 1 0 0-6 3 3 0 0 0 0 6Z', 'M3 21a6 6 0 0 1 12 0', 'M17 8a3 3 0 1 1 0 6', 'M21 21a5 5 0 0 0-4-4.9'],
  lock: ['M6.5 10.5V8a5.5 5.5 0 1 1 11 0v2.5', 'M5 10.5h14V21H5z', 'M12 14v3.5'],
  settings: ['M12 15.5a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7Z', 'M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1-2.8 2.8-.1-.1a1.7 1.7 0 0 0-1.9-.3 1.7 1.7 0 0 0-1 1.6v.2h-4v-.2a1.7 1.7 0 0 0-1-1.6 1.7 1.7 0 0 0-1.9.3l-.1.1L4.2 17l.1-.1a1.7 1.7 0 0 0 .3-1.9 1.7 1.7 0 0 0-1.6-1H2.8v-4H3a1.7 1.7 0 0 0 1.6-1 1.7 1.7 0 0 0-.3-1.9L4.2 7 7 4.2l.1.1A1.7 1.7 0 0 0 9 4.6a1.7 1.7 0 0 0 1-1.6v-.2h4V3a1.7 1.7 0 0 0 1 1.6 1.7 1.7 0 0 0 1.9-.3l.1-.1L19.8 7l-.1.1a1.7 1.7 0 0 0-.3 1.9 1.7 1.7 0 0 0 1.6 1h.2v4H21a1.7 1.7 0 0 0-1.6 1Z'],
  search: ['M11 19a8 8 0 1 0 0-16 8 8 0 0 0 0 16Z', 'm21 21-4.3-4.3'],
  trash: ['M4 7h16M9 7V4h6v3M6 7l1 14h10l1-14', 'M10 11v6M14 11v6'],
  edit: ['M4 20h4L19 9a2.1 2.1 0 0 0-3-3L5 17v3Z', 'm14 7 3 3'],
  close: ['m6 6 12 12M18 6 6 18'],
  copy: ['M8 8h12v12H8Z', 'M4 16V4h12'],
  map: ['m3 6 6-3 6 3 6-3v15l-6 3-6-3-6 3V6Z', 'M9 3v15M15 6v15'],
  alert: ['M12 9v4M12 17h.01', 'M10.3 3.9 2.6 17.3A2 2 0 0 0 4.3 20h15.4a2 2 0 0 0 1.7-2.7L13.7 3.9a2 2 0 0 0-3.4 0Z'],
  camera: ['M4 8h3l2-3h6l2 3h3v12H4V8Z', 'M12 17a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7Z'],
  download: ['M12 4v10', 'M8 11l4 4 4-4', 'M5 19h14'],
  maximize: ['M8 3H3v5', 'M16 3h5v5', 'M8 21H3v-5', 'M16 21h5v-5'],
  more: ['M5 12h.01', 'M12 12h.01', 'M19 12h.01'],
  // 九宫格：底部「更多」用，比三点省略号更像功能入口。
  grid: ['M4 4h6v6H4z', 'M14 4h6v6h-6z', 'M4 14h6v6H4z', 'M14 14h6v6h-6z'],
  sparkles: ['m12 3 1.5 4.5L18 9l-4.5 1.5L12 15l-1.5-4.5L6 9l4.5-1.5L12 3Z', 'm19 15 .8 2.2L22 18l-2.2.8L19 21l-.8-2.2L16 18l2.2-.8L19 15Z'],
  filter: ['M4 5h16l-6 7v6l-4 2v-8L4 5Z'],
  arrowLeft: ['m15 6-6 6 6 6'],
  refresh: ['M20 11a8 8 0 1 0-2.3 6.3', 'M20 4v7h-7'],
  sun: ['M12 17a5 5 0 1 0 0-10 5 5 0 0 0 0 10Z', 'M12 2v2M12 20v2M4.2 4.2l1.4 1.4M18.4 18.4l1.4 1.4M2 12h2M20 12h2M4.2 19.8l1.4-1.4M18.4 5.6l1.4-1.4'],
  moon: ['M21 12.8A8.5 8.5 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8Z'],
}
const paths = computed(() => icons[props.name] || icons.pill)
</script>
