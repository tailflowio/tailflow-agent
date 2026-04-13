<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import type { Execution, Graph } from '@/composables/useWorkflowApi'

const { t, locale } = useI18n()
const router = useRouter()

const props = withDefaults(defineProps<{
  executions: Execution[]
  graph?: Graph | null
  loading?: boolean
  total?: number
  showFooter?: boolean
}>(), {
  loading: false,
  total: 0,
  showFooter: false,
})

function badge(s: string) {
  if (s === 'success') return 'bg-emerald-400/15 text-emerald-400'
  if (s === 'failed') return 'bg-red-400/15 text-red-400'
  if (s === 'cancelled') return 'bg-orange-400/15 text-orange-400'
  if (s === 'running') return 'bg-amber-400/15 text-amber-400'
  if (s === 'waiting') return 'bg-amber-400/15 text-amber-400'
  if (s === 'pending') return 'bg-violet-400/15 text-violet-400'
  return 'bg-g-7/20 text-g-9'
}

function isAnimated(s: string) {
  return s === 'running' || s === 'waiting'
}

function stepDot(s: string) {
  if (s === 'success') return 'bg-emerald-400'
  if (s === 'failed') return 'bg-red-400'
  if (s === 'running') return 'bg-g-11 animate-pulse'
  if (s === 'waiting') return 'bg-amber-400 animate-pulse'
  if (s === 'skipped') return 'bg-g-6'
  return 'bg-g-5'
}

function hashCode(str: string): number {
  let hash = 0
  for (let i = 0; i < str.length; i++) {
    hash = ((hash << 5) - hash + str.charCodeAt(i)) | 0
  }
  return Math.abs(hash)
}

function execGradient(id: string): string {
  const h1 = hashCode(id) % 360
  const h2 = (h1 + 40 + (hashCode(id + 'x') % 80)) % 360
  return `linear-gradient(135deg, hsl(${h1}, 70%, 60%), hsl(${h2}, 70%, 50%))`
}

function formatDate(d: string) {
  return new Date(d).toLocaleString(locale.value, {
    day: '2-digit', month: '2-digit', year: 'numeric',
    hour: '2-digit', minute: '2-digit', second: '2-digit'
  })
}

function duration(exec: Execution) {
  if (!exec.finished_at) return '...'
  const ms = new Date(exec.finished_at).getTime() - new Date(exec.started_at).getTime()
  if (ms < 1000) return `${ms}ms`
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`
  const m = Math.floor(ms / 60000)
  const sec = Math.floor((ms % 60000) / 1000)
  return `${m}m${sec > 0 ? sec + 's' : ''}`
}

function stepProgress(exec: Execution): { done: number; total: number } | null {
  if (!exec.steps || Object.keys(exec.steps).length === 0) return null
  const entries = Object.values(exec.steps)
  const total = entries.length
  const done = entries.filter(s => s.status === 'success' || s.status === 'failed' || s.status === 'skipped').length
  return { done, total }
}

function orderedSteps(exec: Execution): [string, { status: string }][] {
  if (!exec.steps) return []
  if (props.graph?.stages) {
    const ordered: [string, { status: string }][] = []
    for (const stage of props.graph.stages) {
      for (const stepId of stage.steps) {
        if (exec.steps![stepId]) {
          ordered.push([stepId, exec.steps![stepId]])
        }
      }
    }
    return ordered
  }
  if (props.graph) {
    return props.graph.nodes
      .filter(n => exec.steps![n.id])
      .map(n => [n.id, exec.steps![n.id]] as [string, { status: string }])
  }
  return Object.entries(exec.steps)
    .sort(([, a], [, b]) => {
      const ta = (a as any).started_at ? new Date((a as any).started_at).getTime() : Infinity
      const tb = (b as any).started_at ? new Date((b as any).started_at).getTime() : Infinity
      return ta - tb
    })
}
</script>

<template>
  <div class="bg-g-2 border border-g-5 rounded-lg overflow-hidden">
    <!-- Header -->
    <div class="grid grid-cols-[7rem_10rem_1fr_5rem_10rem] gap-3 px-5 py-2.5 border-b border-g-5 text-[12px] font-medium text-g-9 uppercase tracking-wider">
      <span>{{ t('executions.status') }}</span>
      <span>ID</span>
      <span>{{ t('executions.steps') }}</span>
      <span class="text-right">{{ t('executions.duration') }}</span>
      <span class="text-right">{{ t('executions.date') }}</span>
    </div>
    <!-- Rows -->
    <div>
      <div
        v-for="exec in executions"
        :key="exec.id"
        @click="router.push({ name: 'execution', params: { id: exec.id } })"
        class="grid grid-cols-[7rem_10rem_1fr_5rem_10rem] gap-3 items-center px-5 py-3 cursor-pointer hover:bg-g-3 transition-colors border-b border-g-5 last:border-b-0"
      >
        <span :class="['inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium w-fit', badge(exec.status)]">
          <span :class="['w-1.5 h-1.5 rounded-full bg-current', isAnimated(exec.status) ? 'pulse-dot' : 'opacity-50']" />
          {{ exec.status }}
        </span>
        <span class="inline-flex items-center gap-1.5">
          <span class="w-3 h-3 rounded-sm shrink-0" :style="{ background: execGradient(exec.id) }" />
          <span class="text-[11px] text-g-10 font-mono">{{ exec.id.slice(0, 8) }}</span>
        </span>
        <div class="flex items-center gap-2">
          <template v-if="stepProgress(exec)">
            <div class="flex gap-[3px]">
              <span
                v-for="[sid, step] in orderedSteps(exec)"
                :key="sid"
                :class="['w-[14px] h-[5px] rounded-sm', stepDot(step.status)]"
                :title="`${sid}: ${step.status}`"
              />
            </div>
            <span class="text-[11px] text-g-8 font-mono tabular-nums">
              {{ stepProgress(exec)!.done }}/{{ stepProgress(exec)!.total }}
            </span>
          </template>
          <span v-else class="text-[11px] text-g-7 font-mono">-</span>
        </div>
        <span class="text-[12px] text-g-10 font-mono tabular-nums text-right">{{ duration(exec) }}</span>
        <span class="text-[12px] text-g-9 whitespace-nowrap text-right">{{ formatDate(exec.started_at) }}</span>
      </div>
    </div>

    <!-- Footer -->
    <div v-if="showFooter && executions.length > 0" class="px-5 py-2.5 border-t border-g-5 text-[12px] text-g-8 flex items-center gap-3">
      <span>{{ t('executions.showing', { count: executions.length, total }) }}</span>
      <div v-if="loading" class="w-3.5 h-3.5 border-2 border-g-7 border-t-g-12 rounded-full animate-spin" />
    </div>
  </div>
</template>
