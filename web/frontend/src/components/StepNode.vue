<script setup lang="ts">
import { Handle, Position } from '@vue-flow/core'
import { ref, onUnmounted, watch } from 'vue'

const props = defineProps<{
  data: {
    label: string
    action?: string
    status: string
    activeCount?: number
    iteration?: number
    pipeline?: { action: string; title?: string }[]
    execCount?: number
    when?: string
    hasConditionalOutputs?: boolean
  }
}>()

const elapsed = ref('')
let timer: ReturnType<typeof setInterval> | null = null
let startTime: number | null = null

function startTimer() {
  startTime = Date.now()
  timer = setInterval(() => {
    if (startTime) {
      const s = Math.floor((Date.now() - startTime) / 1000)
      if (s < 60) elapsed.value = `${s}s`
      else elapsed.value = `${Math.floor(s / 60)}m${s % 60}s`
    }
  }, 1000)
}

function stopTimer() {
  if (timer) {
    clearInterval(timer)
    timer = null
  }
}

watch(() => props.data.status, (s) => {
  if (s === 'waiting') startTimer()
  else stopTimer()
}, { immediate: true })

onUnmounted(stopTimer)

function nodeClass(s: string) {
  if (s === 'running') return 'border-g-10 bg-g-4 anim-glow'
  if (s === 'success') return 'border-emerald-400/50 bg-emerald-400/5'
  if (s === 'failed') return 'border-red-400/50 bg-red-400/5'
  if (s === 'skipped') return 'border-g-7 bg-g-3 opacity-60'
  if (s === 'waiting') return 'border-amber-400/50 bg-amber-400/5 anim-glow'
  return 'border-g-6 bg-g-3'
}

function dotClass(s: string) {
  if (s === 'running') return 'bg-g-12 animate-pulse'
  if (s === 'success') return 'bg-emerald-400'
  if (s === 'failed') return 'bg-red-400'
  if (s === 'skipped') return 'bg-g-7'
  if (s === 'waiting') return 'bg-amber-400 animate-pulse'
  return 'bg-g-7'
}

function labelClass(s: string) {
  if (s === 'running') return 'text-g-14'
  if (s === 'success') return 'text-emerald-400'
  if (s === 'failed') return 'text-red-400'
  if (s === 'skipped') return 'text-g-8'
  if (s === 'waiting') return 'text-amber-400'
  return 'text-g-10'
}

function countBg(s: string) {
  if (s === 'waiting') return 'bg-amber-400/15 text-amber-400 border border-amber-400/25'
  if (s === 'running') return 'bg-g-6 text-g-13 border border-g-7'
  return 'bg-g-6 text-g-11 border border-g-7'
}
</script>

<template>
  <div :class="['relative rounded-2xl border-2 px-8 py-7 w-[320px] transition-all duration-500 cursor-pointer select-none', nodeClass(data.status)]">
    <!-- Exec count badge — top-right notification style -->
    <span
      v-if="data.execCount"
      class="absolute -top-3.5 -right-3.5 text-[14px] font-mono font-bold rounded-full px-3 min-w-[34px] h-[34px] flex items-center justify-center bg-white text-black shadow-lg shadow-black/40 z-10 transition-all duration-300"
    >
      {{ data.execCount }}
    </span>
    <Handle type="target" :position="Position.Top" />

    <div class="flex items-center gap-3.5">
      <span v-if="data.action" class="text-[10px] font-mono font-medium px-1.5 py-px rounded bg-g-5 text-g-9 border border-g-6 flex-shrink-0">
        {{ data.action }}
      </span>
      <span :class="['text-[19px] font-semibold truncate transition-colors duration-500', labelClass(data.status)]">
        {{ data.label }}
      </span>
      <span
        v-if="data.iteration && data.iteration > 1"
        class="ml-auto text-[11px] font-mono font-semibold rounded-full px-1.5 min-w-[22px] text-center leading-[20px] bg-amber-400/15 text-amber-400 border border-amber-400/25"
      >
        x{{ data.iteration }}
      </span>
      <span
        v-else-if="data.activeCount"
        :class="['ml-auto text-[11px] font-mono font-semibold rounded-full px-1.5 min-w-[22px] text-center leading-[20px]', countBg(data.status)]"
      >
        {{ data.activeCount }}
      </span>
    </div>
    <div v-if="data.status === 'waiting' && elapsed" class="flex items-center gap-2 mt-1.5 pl-[26px]">
      <span class="text-[11px] font-mono text-amber-400/70">
        {{ elapsed }}
      </span>
    </div>
    <div v-if="data.pipeline && data.pipeline.length > 0" class="mt-2 pl-[26px] flex flex-col gap-1">
      <div
        v-for="(pa, i) in data.pipeline"
        :key="i"
        class="flex items-center gap-2"
      >
        <span class="text-[10px] font-mono font-medium px-1.5 py-px rounded bg-amber-400/10 text-amber-400 border border-amber-400/20">
          {{ pa.action }}
        </span>
        <span v-if="pa.title" class="text-[10px] font-mono text-g-9 truncate">{{ pa.title }}</span>
      </div>
    </div>

    <Handle v-if="!data.hasConditionalOutputs" id="default" type="source" :position="Position.Bottom" />

    <!-- Conditional outputs: labels positioned above each handle point -->
    <span v-if="data.hasConditionalOutputs" class="absolute bottom-[10px] text-[9px] font-mono font-semibold text-emerald-400" style="left: 25%; transform: translateX(-50%)">true</span>
    <span v-if="data.hasConditionalOutputs" class="absolute bottom-[10px] text-[9px] font-mono font-semibold text-g-7" style="left: 75%; transform: translateX(-50%)">false</span>
    <Handle v-if="data.hasConditionalOutputs" id="when-true" type="source" :position="Position.Bottom" :style="{ left: '25%' }" />
    <Handle v-if="data.hasConditionalOutputs" id="when-false" type="source" :position="Position.Bottom" :style="{ left: '75%' }" />
  </div>
</template>
