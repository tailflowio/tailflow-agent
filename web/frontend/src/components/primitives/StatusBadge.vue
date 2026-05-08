<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  status: string
  className?: string
}>()

const badgeClass = computed(() => {
  const s = props.status
  if (s === 'success') return 'bg-emerald-400/15 text-emerald-400'
  if (s === 'completed_with_errors') return 'bg-amber-400/15 text-amber-400'
  if (s === 'failed') return 'bg-red-400/15 text-red-400'
  if (s === 'cancelled') return 'bg-orange-400/15 text-orange-400'
  if (s === 'running') return 'bg-amber-400/15 text-amber-400'
  if (s === 'waiting') return 'bg-amber-400/15 text-amber-400'
  if (s === 'scheduled') return 'bg-violet-400/15 text-violet-400'
  if (s === 'pending') return 'bg-g-4 text-g-9'
  if (s === 'skipped') return 'bg-g-4 text-g-8'
  return 'bg-g-4 text-g-9'
})

const dotClass = computed(() => {
  const s = props.status
  if (s === 'success') return 'bg-emerald-400'
  if (s === 'completed_with_errors') return 'bg-amber-400'
  if (s === 'failed') return 'bg-red-400'
  if (s === 'cancelled') return 'bg-orange-400'
  if (s === 'running') return 'bg-amber-400 animate-pulse'
  if (s === 'waiting') return 'bg-amber-400 animate-pulse'
  if (s === 'scheduled') return 'bg-violet-400'
  if (s === 'skipped') return 'bg-g-7'
  return 'bg-g-6'
})
</script>

<template>
  <span :class="['inline-flex items-center gap-1.5 px-2 py-0.5 rounded text-[11px] font-medium font-mono', badgeClass, props.className || '']">
    <span :class="['w-1.5 h-1.5 rounded-full', dotClass]" />
    {{ props.status }}
  </span>
</template>
