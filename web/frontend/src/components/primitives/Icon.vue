<script setup lang="ts">
const props = defineProps<{
  name: string
  className?: string
}>()

const paths: Record<string, string> = {
  home: 'M3 12l9-9 9 9M5 10v10h14V10',
  workflow: 'M5 6h6M13 6h6M5 12h6M13 12h6M5 18h6M13 18h6',
  play: 'M5 4l14 8-14 8V4z',
  history: 'M3 12a9 9 0 1 0 3-6.7M3 4v5h5M12 7v5l3 2',
  edit: 'M4 20h4l10-10-4-4L4 16v4z',
  search: 'M11 4a7 7 0 1 0 4.95 11.95L20 20M11 4a7 7 0 0 1 7 7',
  settings: 'M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM19 12c0-.6-.06-1.2-.18-1.78l2.05-1.6-2-3.46-2.42.97a7 7 0 0 0-3.06-1.78L12.97 2h-4l-.42 2.35a7 7 0 0 0-3.06 1.78L3.07 5.16l-2 3.46 2.05 1.6c-.12.58-.18 1.18-.18 1.78s.06 1.2.18 1.78L1.07 15.4l2 3.46 2.42-.97a7 7 0 0 0 3.06 1.78l.42 2.33h4l.42-2.35a7 7 0 0 0 3.06-1.78l2.42.97 2-3.46-2.05-1.6c.12-.58.18-1.18.18-1.78z',
  cpu: 'M5 5h14v14H5zM9 1v4M15 1v4M9 19v4M15 19v4M1 9h4M1 15h4M19 9h4M19 15h4',
  chevronR: 'M9 5l7 7-7 7',
  chevronD: 'M5 9l7 7 7-7',
  plus: 'M12 5v14M5 12h14',
  x: 'M18 6L6 18M6 6l12 12',
  arrowR: 'M5 12h14M13 5l7 7-7 7',
  docs: 'M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8zM14 2v6h6M9 13h6M9 17h6',
  repeat: 'M17 1l4 4-4 4M3 11V9a4 4 0 0 1 4-4h14M7 23l-4-4 4-4M21 13v2a4 4 0 0 1-4 4H3',
  branch: 'M6 3v12M18 9a3 3 0 1 0 0 6 3 3 0 0 0 0-6zM6 18a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM18 6a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM6 15a9 9 0 0 0 9-9',
  download: 'M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3',
  copy: 'M9 9h11v11H9zM5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1',
  bell: 'M18 8a6 6 0 0 0-12 0c0 7-3 9-3 9h18s-3-2-3-9M14 21a2 2 0 0 1-4 0',
  terminal: 'M4 17l6-6-6-6M12 19h8',
  cmd: 'M9 6a3 3 0 1 0-3 3h12a3 3 0 1 0-3-3v12a3 3 0 1 0 3-3H6a3 3 0 1 0 3 3z',
  bolt: 'M13 2L3 14h7l-1 8 10-12h-7z',
  check: 'M5 13l4 4L19 7',
  alert: 'M12 9v4M12 17h.01M12 2L1 22h22z',
  refresh: 'M21 12a9 9 0 1 1-3-6.7M21 4v5h-5',
  filter: 'M3 4h18M6 12h12M10 20h4',
  eye: 'M1 12s4-8 11-8 11 8 11 8-4 8-11 8S1 12 1 12zM12 9a3 3 0 1 0 0 6 3 3 0 0 0 0-6z',
  grip: 'M9 6h.01M9 12h.01M9 18h.01M15 6h.01M15 12h.01M15 18h.01',
  folder: 'M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z',
  db: 'M12 3c-4.4 0-8 1.3-8 3v12c0 1.7 3.6 3 8 3s8-1.3 8-3V6c0-1.7-3.6-3-8-3zM4 6c0 1.7 3.6 3 8 3s8-1.3 8-3M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3',
  globe: 'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18',
  grid: 'M3 3h7v7H3zM14 3h7v7h-7zM3 14h7v7H3zM14 14h7v7h-7z',
}
</script>

<template>
  <svg :class="props.className || 'w-4 h-4'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">
    <path :d="paths[props.name] || paths.docs" />
  </svg>
</template>
