<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useWorkflowApi, type Graph, type Execution } from '@/composables/useWorkflowApi'
import { useGlobalEvents } from '@/composables/useGlobalEvents'
import { useRunTrigger } from '@/composables/useRunTrigger'
import WorkflowGraph from '@/components/WorkflowGraph.vue'

const { t } = useI18n()

const router = useRouter()
const api = useWorkflowApi()
const { workflowReady, workflowName, workflowParams } = useRunTrigger()

const workflow = ref<Record<string, unknown> | null>(null)
const graph = ref<Graph | null>(null)
const executions = ref<Execution[]>([])

async function load() {
  const [wf, gr, ex] = await Promise.all([
    api.getWorkflow().catch(() => null),
    api.getWorkflowGraph().catch(() => null),
    api.listExecutions().then(r => r.items).catch(() => [] as Execution[]),
  ])
  if (wf) workflow.value = wf
  if (gr) graph.value = gr
  executions.value = ex
  if (wf) {
    workflowReady.value = true
    workflowName.value = (wf as any).name || ''
    workflowParams.value = (wf as any).params || []
  }
}

onMounted(load)

// Global SSE with step tracking (hydrates from /api/workflow/activity on mount)
const { stepCounts, recentStatuses, stepExecCounts, sysMetrics } = useGlobalEvents(() => {
  api.listExecutions().then(r => { executions.value = r.items }).catch(() => {})
})

// Total active executions
const totalActive = computed(() => {
  let count = 0
  for (const sc of Object.values(stepCounts.value)) {
    count += sc.running.length + sc.waiting.length
  }
  return count
})

// Global metrics computed from executions
const metrics = computed(() => {
  const all = executions.value
  const total = all.length
  const success = all.filter(e => e.status === 'success').length
  const failed = all.filter(e => e.status === 'failed').length
  const running = all.filter(e => e.status === 'running' || e.status === 'waiting').length

  let totalMs = 0
  let counted = 0
  for (const e of all) {
    if (e.finished_at && e.started_at) {
      totalMs += new Date(e.finished_at).getTime() - new Date(e.started_at).getTime()
      counted++
    }
  }
  const avgMs = counted > 0 ? Math.round(totalMs / counted) : 0

  return { total, success, failed, running, avgMs }
})

function fmtMs(ms: number) {
  if (ms < 1000) return ms + 'ms'
  return (ms / 1000).toFixed(1) + 's'
}

const tick = ref(0)
let tickTimer: ReturnType<typeof setInterval> | null = null
onMounted(() => { tickTimer = setInterval(() => { tick.value++ }, 1000) })
onUnmounted(() => { if (tickTimer) clearInterval(tickTimer) })

// System metrics via SSE (pushed by backend every 1s)
const MAX_HISTORY = 30
const cpuHistory = ref<number[]>([])
const memHistory = ref<number[]>([])
const goroutineHistory = ref<number[]>([])
const netRxHistory = ref<number[]>([])
let prevNetRx = 0

function pushHistory(arr: number[], val: number) {
  arr.push(val)
  if (arr.length > MAX_HISTORY) arr.shift()
}

watch(sysMetrics, (m) => {
  if (!m) return
  pushHistory(cpuHistory.value, m.cpu_percent)
  pushHistory(memHistory.value, m.rss_kb > 0 ? m.rss_kb / 1024 : m.heap_mb)
  pushHistory(goroutineHistory.value, m.goroutines)
  const rxDelta = prevNetRx > 0 ? Math.max(0, m.net_rx_bytes - prevNetRx) : 0
  prevNetRx = m.net_rx_bytes
  pushHistory(netRxHistory.value, rxDelta)
})

function sparklinePath(data: number[], w: number, h: number): string {
  if (data.length < 2) return ''
  const max = Math.max(...data, 1)
  const min = Math.min(...data, 0)
  const range = max - min || 1
  const stepX = w / (MAX_HISTORY - 1)
  const points = data.map((v, i) => {
    const x = i * stepX
    const y = h - ((v - min) / range) * h
    return `${x.toFixed(1)},${y.toFixed(1)}`
  })
  return `M${points.join(' L')} L${((data.length - 1) * stepX).toFixed(1)},${h} L0,${h} Z`
}

function fmtBytes(bytes: number): string {
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  if (bytes < 1024 * 1024 * 1024) return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
  return (bytes / (1024 * 1024 * 1024)).toFixed(1) + ' GB'
}

function fmtUptime(seconds: number): string {
  if (seconds < 60) return seconds + 's'
  if (seconds < 3600) return Math.floor(seconds / 60) + 'm ' + (seconds % 60) + 's'
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  return h + 'h ' + (m > 0 ? m + 'm' : '')
}

function cpuColor(pct: number): string {
  if (pct >= 80) return 'text-red-400'
  if (pct >= 50) return 'text-amber-400'
  return 'text-emerald-400'
}

const nextRunLabel = computed(() => {
  tick.value // reactive dependency
  const nr = (workflow.value as any)?.next_run
  if (!nr) return ''
  const s = Math.floor((new Date(nr).getTime() - Date.now()) / 1000)
  if (s <= 0) return t('dashboard.nextNow')
  if (s < 60) return t('dashboard.nextIn', { time: `${s}s` })
  if (s < 3600) return t('dashboard.nextIn', { time: `${Math.floor(s / 60)}m${String(s % 60).padStart(2, '0')}s` })
  if (s < 86400) return t('dashboard.nextIn', { time: `${Math.floor(s / 3600)}h${String(Math.floor((s % 3600) / 60)).padStart(2, '0')}m` })
  return t('dashboard.nextIn', { time: `${Math.floor(s / 86400)}d` })
})

// Re-fetch next_run when countdown hits "now" (cron has fired)
let nextRunRefreshTimer: ReturnType<typeof setTimeout> | null = null
watch(nextRunLabel, (v) => {
  if (v === t('dashboard.nextNow')) {
    if (nextRunRefreshTimer) clearTimeout(nextRunRefreshTimer)
    nextRunRefreshTimer = setTimeout(() => {
      api.getWorkflow().then(wf => {
        if (wf) workflow.value = wf
      }).catch(() => {})
    }, 3000)
  }
})

const dagHeight = computed(() => {
  const nodes = graph.value?.nodes?.length ?? 0
  return Math.min(700, Math.max(300, 200 + nodes * 70))
})

function onStepClick(stepId: string) {
  router.push({ name: 'step-detail', params: { id: stepId } })
}

function dot(s: string) {
  if (s === 'success') return 'bg-emerald-400'
  if (s === 'failed') return 'bg-red-400'
  if (s === 'cancelled') return 'bg-g-9'
  if (s === 'running') return 'bg-g-12 animate-pulse'
  if (s === 'waiting') return 'bg-amber-400 animate-pulse'
  return 'bg-g-8'
}
function badge(s: string) {
  if (s === 'success') return 'bg-emerald-400/15 text-emerald-400'
  if (s === 'failed') return 'bg-red-400/15 text-red-400'
  if (s === 'cancelled') return 'bg-g-5 text-g-9'
  if (s === 'running') return 'bg-amber-400/15 text-amber-400'
  if (s === 'waiting') return 'bg-amber-400/15 text-amber-400'
  return 'bg-g-5 text-g-9'
}
function stepDot(s: string) {
  if (s === 'success') return 'bg-emerald-400'
  if (s === 'failed') return 'bg-red-400'
  if (s === 'running') return 'bg-g-11 animate-pulse'
  if (s === 'waiting') return 'bg-amber-400 animate-pulse'
  if (s === 'skipped') return 'bg-g-6'
  return 'bg-g-5'
}
function ago(d: string) {
  if (!d) return ''
  const s = Math.floor((Date.now() - new Date(d).getTime()) / 1000)
  if (s < 5) return 'now'
  if (s < 60) return s + 's'
  if (s < 3600) return Math.floor(s / 60) + 'm'
  if (s < 86400) return Math.floor(s / 3600) + 'h'
  return Math.floor(s / 86400) + 'd'
}
</script>

<template>
  <div v-if="workflow">
    <!-- Header -->
    <div class="flex items-start justify-between mb-6">
      <div>
        <h1 class="text-lg font-semibold text-g-14 mb-1 tracking-tight">{{ workflow.name }}</h1>
        <p v-if="workflow.description" class="text-sm text-g-10 leading-relaxed">{{ workflow.description }}</p>
      </div>
      <div class="flex gap-2" />
    </div>

    <!-- Tags + Trigger + Active count -->
    <div class="flex flex-wrap items-center gap-2 mb-5">
      <span
        v-for="tag in (workflow.tags as string[])"
        :key="tag"
        class="text-[12px] px-2.5 py-1 rounded-md bg-g-4 text-g-9 font-mono"
      >
        {{ tag }}
      </span>
      <!-- Trigger detail -->
      <span
        v-if="(workflow as any).trigger?.http"
        class="text-[12px] font-mono font-medium text-g-11 bg-g-5 px-2.5 py-1 rounded-md flex items-center gap-1.5"
      >
        <svg class="w-3 h-3 text-g-8" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101" />
          <path stroke-linecap="round" stroke-linejoin="round" d="M10.172 13.828a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.102 1.101" />
        </svg>
        {{ (workflow as any).trigger.http.method }} {{ (workflow as any).trigger.http.path }}
      </span>
      <span
        v-else-if="(workflow as any).trigger?.webhook"
        class="text-[12px] font-mono font-medium text-g-11 bg-g-5 px-2.5 py-1 rounded-md flex items-center gap-1.5"
      >
        <svg class="w-3 h-3 text-g-8" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101" />
          <path stroke-linecap="round" stroke-linejoin="round" d="M10.172 13.828a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.102 1.101" />
        </svg>
        webhook {{ (workflow as any).trigger.webhook.path }}
      </span>
      <span
        v-else-if="(workflow as any).trigger?.schedule"
        class="text-[12px] font-mono font-medium text-g-11 bg-g-5 px-2.5 py-1 rounded-md flex items-center gap-1.5"
      >
        <svg class="w-3 h-3 text-g-8" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="10" />
          <path stroke-linecap="round" stroke-linejoin="round" d="M12 6v6l4 2" />
        </svg>
        cron {{ (workflow as any).trigger.schedule.cron }}
      </span>
      <span
        v-if="nextRunLabel"
        class="text-[12px] font-mono text-g-9 flex items-center gap-1.5"
      >
        <svg class="w-3 h-3 text-g-8" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M12 6v6h4.5m4.5 0a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        {{ nextRunLabel }}
      </span>
      <span v-if="totalActive > 0" class="ml-2 flex items-center gap-1.5 text-[12px] font-mono text-g-12">
        <span class="w-1.5 h-1.5 bg-g-12 rounded-full animate-pulse" />
        {{ t('dashboard.active', { count: totalActive }) }}
      </span>
    </div>

    <!-- System metrics -->
    <div v-if="sysMetrics" class="mb-6">
      <h2 class="text-sm font-medium text-g-12 mb-3">{{ t('dashboard.systemMetrics') }}</h2>
      <div v-if="sysMetrics.available" class="grid grid-cols-5 gap-3">
        <div class="bg-g-2 border border-g-5 rounded-lg p-4 lm-card relative overflow-hidden anim-enter">
          <svg v-if="cpuHistory.length > 1" class="absolute bottom-0 left-0 w-full h-1/2" preserveAspectRatio="none">
            <path :d="sparklinePath(cpuHistory, 200, 80)" fill="currentColor" class="text-g-12/5" />
          </svg>
          <div class="relative">
            <p class="text-xs text-g-9 mb-1.5">{{ t('dashboard.cpu') }}</p>
            <p :class="['text-2xl font-semibold font-mono tabular-nums', cpuColor(sysMetrics.cpu_percent)]">{{ sysMetrics.cpu_percent.toFixed(1) }}%</p>
          </div>
        </div>
        <div class="bg-g-2 border border-g-5 rounded-lg p-4 lm-card relative overflow-hidden anim-enter delay-1">
          <svg v-if="memHistory.length > 1" class="absolute bottom-0 left-0 w-full h-1/2" preserveAspectRatio="none">
            <path :d="sparklinePath(memHistory, 200, 80)" fill="currentColor" class="text-g-12/5" />
          </svg>
          <div class="relative">
            <p class="text-xs text-g-9 mb-1.5">{{ t('dashboard.memory') }}</p>
            <p class="text-2xl font-semibold text-g-14 font-mono tabular-nums">{{ Math.round(sysMetrics.rss_kb / 1024) }} MB</p>
          </div>
        </div>
        <div class="bg-g-2 border border-g-5 rounded-lg p-4 lm-card relative overflow-hidden anim-enter delay-2">
          <svg v-if="goroutineHistory.length > 1" class="absolute bottom-0 left-0 w-full h-1/2" preserveAspectRatio="none">
            <path :d="sparklinePath(goroutineHistory, 200, 80)" fill="currentColor" class="text-g-12/5" />
          </svg>
          <div class="relative">
            <p class="text-xs text-g-9 mb-1.5">{{ t('dashboard.goroutines') }}</p>
            <p class="text-2xl font-semibold text-g-14 font-mono tabular-nums">{{ sysMetrics.goroutines }}</p>
          </div>
        </div>
        <div class="bg-g-2 border border-g-5 rounded-lg p-4 lm-card relative overflow-hidden anim-enter delay-3">
          <svg v-if="netRxHistory.length > 1" class="absolute bottom-0 left-0 w-full h-1/2" preserveAspectRatio="none">
            <path :d="sparklinePath(netRxHistory, 200, 80)" fill="currentColor" class="text-g-12/5" />
          </svg>
          <div class="relative">
            <p class="text-xs text-g-9 mb-1.5">{{ t('dashboard.network') }}</p>
            <p class="text-base font-semibold text-g-14 font-mono tabular-nums leading-tight">
              <span class="text-g-9">&darr;</span> {{ fmtBytes(sysMetrics.net_rx_bytes) }}
              <br>
              <span class="text-g-9">&uarr;</span> {{ fmtBytes(sysMetrics.net_tx_bytes) }}
            </p>
          </div>
        </div>
        <div class="bg-g-2 border border-g-5 rounded-lg p-4 lm-card anim-enter delay-4">
          <p class="text-xs text-g-9 mb-1.5">{{ t('dashboard.uptime') }}</p>
          <p class="text-2xl font-semibold text-g-14 font-mono tabular-nums">{{ fmtUptime(sysMetrics.uptime_s) }}</p>
        </div>
      </div>
      <div v-else class="grid grid-cols-5 gap-3">
        <div class="bg-g-2 border border-g-5 rounded-lg p-4 lm-card relative overflow-hidden anim-enter">
          <svg v-if="goroutineHistory.length > 1" class="absolute bottom-0 left-0 w-full h-1/2" preserveAspectRatio="none">
            <path :d="sparklinePath(goroutineHistory, 200, 80)" fill="currentColor" class="text-g-12/5" />
          </svg>
          <div class="relative">
            <p class="text-xs text-g-9 mb-1.5">{{ t('dashboard.goroutines') }}</p>
            <p class="text-2xl font-semibold text-g-14 font-mono tabular-nums">{{ sysMetrics.goroutines }}</p>
          </div>
        </div>
        <div class="bg-g-2 border border-g-5 rounded-lg p-4 lm-card relative overflow-hidden anim-enter delay-1">
          <svg v-if="memHistory.length > 1" class="absolute bottom-0 left-0 w-full h-1/2" preserveAspectRatio="none">
            <path :d="sparklinePath(memHistory, 200, 80)" fill="currentColor" class="text-g-12/5" />
          </svg>
          <div class="relative">
            <p class="text-xs text-g-9 mb-1.5">Heap</p>
            <p class="text-2xl font-semibold text-g-14 font-mono tabular-nums">{{ sysMetrics.heap_mb.toFixed(1) }} MB</p>
          </div>
        </div>
        <div class="bg-g-2 border border-g-5 rounded-lg p-4 lm-card anim-enter delay-2">
          <p class="text-xs text-g-9 mb-1.5">{{ t('dashboard.uptime') }}</p>
          <p class="text-2xl font-semibold text-g-14 font-mono tabular-nums">{{ fmtUptime(sysMetrics.uptime_s) }}</p>
        </div>
        <div class="col-span-2 bg-g-2 border border-g-5 rounded-lg p-4 flex items-center lm-card anim-enter delay-3">
          <span class="text-sm text-g-8">{{ t('dashboard.metricsUnavailable') }}</span>
        </div>
      </div>
    </div>

    <!-- Workflow metrics -->
    <div class="mb-6">
      <h2 class="text-sm font-medium text-g-12 mb-3">{{ t('dashboard.workflowMetrics') }}</h2>
      <div class="grid grid-cols-5 gap-3">
        <div class="bg-g-2 border border-g-5 rounded-lg p-4 lm-card anim-enter">
          <p class="text-xs text-g-9 mb-1.5">{{ t('dashboard.total') }}</p>
          <p class="text-2xl font-semibold text-g-14 font-mono tabular-nums">{{ metrics.total }}</p>
        </div>
        <div class="bg-g-2 border border-g-5 rounded-lg p-4 lm-card anim-enter delay-1">
          <p class="text-xs text-g-9 mb-1.5">{{ t('dashboard.success') }}</p>
          <p class="text-2xl font-semibold text-emerald-400 font-mono tabular-nums">{{ metrics.success }}</p>
        </div>
        <div class="bg-g-2 border border-g-5 rounded-lg p-4 lm-card anim-enter delay-2">
          <p class="text-xs text-g-9 mb-1.5">{{ t('dashboard.failed') }}</p>
          <p class="text-2xl font-semibold text-red-400 font-mono tabular-nums">{{ metrics.failed }}</p>
        </div>
        <div class="bg-g-2 border border-g-5 rounded-lg p-4 lm-card anim-enter delay-3">
          <p class="text-xs text-g-9 mb-1.5">{{ t('dashboard.running') }}</p>
          <div class="flex items-baseline gap-1.5">
            <p class="text-2xl font-semibold text-g-14 font-mono tabular-nums">{{ metrics.running }}</p>
            <span v-if="metrics.running > 0" class="w-1.5 h-1.5 bg-g-12 rounded-full animate-pulse" />
          </div>
        </div>
        <div class="bg-g-2 border border-g-5 rounded-lg p-4 lm-card anim-enter delay-4">
          <p class="text-xs text-g-9 mb-1.5">{{ t('dashboard.avgDuration') }}</p>
          <p class="text-2xl font-semibold text-g-14 font-mono tabular-nums">{{ metrics.avgMs ? fmtMs(metrics.avgMs) : '-' }}</p>
        </div>
      </div>
    </div>

    <!-- DAG -->
    <div class="bg-g-2 border border-g-5 rounded-lg mb-6 overflow-hidden lm-card" :style="{ height: dagHeight + 'px' }">
      <WorkflowGraph
        v-if="graph"
        :graph="graph"
        :step-counts="stepCounts"
        :recent-statuses="recentStatuses"
        :step-exec-counts="stepExecCounts"
        @node-click="onStepClick"
      />
    </div>

    <!-- Recent executions -->
    <div class="flex items-baseline justify-between mb-3">
      <h2 class="text-sm font-medium text-g-12">{{ t('dashboard.recentExecs') }}</h2>
      <RouterLink to="/executions" class="text-xs text-g-9 hover:text-g-12 transition-colors">{{ t('dashboard.viewAll') }}</RouterLink>
    </div>

    <div v-if="executions.length === 0" class="bg-g-2 border border-g-5 rounded-lg py-10 text-center text-g-9 text-sm lm-card">
      {{ t('dashboard.noExecs') }}
    </div>
    <div v-else class="bg-g-2 border border-g-5 rounded-lg overflow-hidden lm-card">
      <div
        v-for="e in executions.slice(0, 10)" :key="e.id"
        @click="router.push({ name: 'execution', params: { id: e.id } })"
        class="flex items-center gap-3 px-4 py-3 cursor-pointer hover:bg-g-3 transition-colors border-b border-g-5 last:border-b-0"
      >
        <span :class="['w-[7px] h-[7px] rounded-full flex-shrink-0', dot(e.status)]" />
        <span :class="['text-[12px] font-mono px-2 py-0.5 rounded', badge(e.status)]">{{ e.status }}</span>
        <template v-if="e.steps && Object.keys(e.steps).length > 0 && graph">
          <div class="flex items-center gap-1.5 ml-1">
            <div class="flex gap-[2px]">
              <span
                v-for="n in graph.nodes"
                :key="n.id"
                v-show="e.steps[n.id]"
                :class="['w-[12px] h-[4px] rounded-sm', stepDot(e.steps[n.id]?.status || 'pending')]"
                :title="`${n.id}: ${e.steps[n.id]?.status || 'pending'}`"
              />
            </div>
          </div>
        </template>
        <span class="flex-1" />
        <span class="text-[12px] text-g-8 font-mono tabular-nums">{{ ago(e.started_at) }}</span>
        <span class="text-[12px] text-g-7 font-mono">{{ e.id.slice(0, 8) }}</span>
      </div>
    </div>

  </div>

  <div v-else-if="api.loading.value" class="flex items-center justify-center py-20">
    <div class="w-5 h-5 border-2 border-g-7 border-t-g-12 rounded-full animate-spin" />
  </div>
  <div v-else-if="api.error.value" class="text-red-400 text-sm">{{ api.error.value }}</div>
</template>
