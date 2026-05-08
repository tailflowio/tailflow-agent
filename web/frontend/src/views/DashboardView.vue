<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useWorkflowApi, type Graph, type Execution } from '@/composables/useWorkflowApi'
import { useGlobalEvents } from '@/composables/useGlobalEvents'
import { useRunTrigger } from '@/composables/useRunTrigger'
import { fmtDur, fmtAgo, statusDot } from '@/composables/useFormat'
import { useStepInspector } from '@/composables/useStepInspector'
import StatusBadge from '@/components/primitives/StatusBadge.vue'
import Icon from '@/components/primitives/Icon.vue'
import Kbd from '@/components/primitives/Kbd.vue'
import WorkflowDAGCustom from '@/components/WorkflowDAGCustom.vue'

const { t } = useI18n()
const router = useRouter()
const api = useWorkflowApi()
const { showRunDialog, workflowReady, workflowName, workflowParams } = useRunTrigger()
const stepInspector = useStepInspector()

const workflow = ref<Record<string, unknown> | null>(null)
const graph = ref<Graph | null>(null)
const executions = ref<Execution[]>([])
const stepMetrics = ref<Record<string, { total_executions: number; success_count: number; failure_count: number; avg_duration_ms: number }>>({})

async function load() {
  const [wf, gr, ex, sm] = await Promise.all([
    api.getWorkflow().catch(() => null),
    api.getWorkflowGraph().catch(() => null),
    api.listExecutions().then(r => r.items).catch(() => [] as Execution[]),
    api.getAllStepMetrics().catch(() => ({})),
  ])
  if (wf) workflow.value = wf
  if (gr) graph.value = gr
  executions.value = ex
  stepMetrics.value = sm
  if (wf) {
    workflowReady.value = true
    workflowName.value = (wf as any).name || ''
    workflowParams.value = (wf as any).params || []
  }
}

onMounted(load)

const { stepCounts, sysMetrics } = useGlobalEvents(() => {
  api.listExecutions().then(r => { executions.value = r.items }).catch(() => {})
})

const totalActive = computed(() => {
  let count = 0
  for (const sc of Object.values(stepCounts.value)) {
    count += sc.running.length + sc.waiting.length
  }
  return count
})

const tick = ref(0)
let tickTimer: ReturnType<typeof setInterval> | null = null
onMounted(() => { tickTimer = setInterval(() => { tick.value++ }, 1000) })
onUnmounted(() => { if (tickTimer) clearInterval(tickTimer) })

const nextRunLabel = computed(() => {
  tick.value
  const nr = (workflow.value as any)?.next_run
  if (!nr) return ''
  const s = Math.floor((new Date(nr).getTime() - Date.now()) / 1000)
  if (s <= 0) return t('dashboard.nextNow')
  if (s < 60) return t('dashboard.nextIn', { time: `${s}s` })
  if (s < 3600) return t('dashboard.nextIn', { time: `${Math.floor(s / 60)}m${String(s % 60).padStart(2, '0')}s` })
  if (s < 86400) return t('dashboard.nextIn', { time: `${Math.floor(s / 3600)}h${String(Math.floor((s % 3600) / 60)).padStart(2, '0')}m` })
  return t('dashboard.nextIn', { time: `${Math.floor(s / 86400)}d` })
})

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

const metrics = computed(() => {
  const all = executions.value
  const total = all.length
  const success = all.filter(e => e.status === 'success').length
  const failed = all.filter(e => e.status === 'failed').length
  const running = all.filter(e => e.status === 'running' || e.status === 'waiting').length
  let totalMs = 0, counted = 0
  for (const e of all) {
    if (e.finished_at && e.started_at) {
      totalMs += new Date(e.finished_at).getTime() - new Date(e.started_at).getTime()
      counted++
    }
  }
  return { total, success, failed, running, avgMs: counted > 0 ? Math.round(totalMs / counted) : 0 }
})

const successPct = computed(() => metrics.value.total > 0 ? ((metrics.value.success / metrics.value.total) * 100).toFixed(1) : '0')
const failPct = computed(() => metrics.value.total > 0 ? ((metrics.value.failed / metrics.value.total) * 100).toFixed(1) : '0')

// Pipeline DAG preview: reflect the latest run only.
//
// Steps absent from latest.steps are intentionally left unset (pending). We
// used to fall back to historical metrics here, but that painted not-yet-run
// steps as success during an in-progress run, which is misleading.
const pipelineStatuses = computed<Record<string, string>>(() => {
  const out: Record<string, string> = {}
  const latest = executions.value[0]
  if (latest?.steps) {
    for (const [id, s] of Object.entries(latest.steps)) {
      if (s.status) out[id] = s.status
    }
  }
  return out
})

const pipelineDurations = computed<Record<string, number>>(() => {
  const out: Record<string, number> = {}
  for (const id in stepMetrics.value) {
    const m = stepMetrics.value[id]
    if (m.avg_duration_ms > 0) out[id] = m.avg_duration_ms
  }
  return out
})

// System metrics history for sparklines
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
  pushHistory(memHistory.value, m.memory_bytes / (1024 * 1024))
  pushHistory(goroutineHistory.value, m.goroutines)
  const rxDelta = prevNetRx > 0 ? Math.max(0, m.net_rx_bytes - prevNetRx) : 0
  prevNetRx = m.net_rx_bytes
  pushHistory(netRxHistory.value, rxDelta)
})

function sparkAreaPath(data: number[], w = 200, h = 36): string {
  if (data.length < 2) return ''
  const max = Math.max(...data, 1)
  const min = Math.min(...data, 0)
  const range = max - min || 1
  const stepX = w / (Math.max(data.length - 1, 1))
  const points = data.map((v, i) => `${(i * stepX).toFixed(1)},${(h - ((v - min) / range) * h).toFixed(1)}`)
  return `M${points.join(' L')} L${((data.length - 1) * stepX).toFixed(1)},${h} L0,${h} Z`
}

function sparkLinePath(data: number[], w = 200, h = 36): string {
  if (data.length < 2) return ''
  const max = Math.max(...data, 1)
  const min = Math.min(...data, 0)
  const range = max - min || 1
  const stepX = w / (Math.max(data.length - 1, 1))
  const points = data.map((v, i) => `${(i * stepX).toFixed(1)},${(h - ((v - min) / range) * h).toFixed(1)}`)
  return `M${points.join(' L')}`
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

function execDuration(e: Execution): number | null {
  if (e.finished_at && e.started_at) return new Date(e.finished_at).getTime() - new Date(e.started_at).getTime()
  return null
}

function paramsLabel(e: Execution): string {
  if (!e.params || Object.keys(e.params).length === 0) return ''
  return Object.entries(e.params).map(([k, v]) => `${k}=${v}`).join(' ')
}

const tags = computed<string[]>(() => ((workflow.value as any)?.tags) || [])
const trigger = computed(() => (workflow.value as any)?.trigger)

function openRun() {
  if (workflowReady.value) showRunDialog.value = true
}
</script>

<template>
  <div v-if="workflow" class="px-8 py-6">
    <!-- Sparkline gradient defs -->
    <svg class="absolute w-0 h-0 overflow-hidden text-g-12" aria-hidden="true">
      <defs>
        <linearGradient id="spark-grad" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stop-color="currentColor" stop-opacity="0" />
          <stop offset="100%" stop-color="currentColor" stop-opacity="0.1" />
        </linearGradient>
      </defs>
    </svg>

    <!-- Header -->
    <div class="flex items-start justify-between mb-5">
      <div>
        <div class="flex items-center gap-3 mb-1.5">
          <h1 class="text-[22px] font-semibold text-g-14 tracking-tight">{{ workflow.name }}</h1>
          <StatusBadge v-if="totalActive > 0" status="running" />
          <StatusBadge v-else status="success" />
        </div>
        <p v-if="workflow.description" class="text-[13px] text-g-10">{{ workflow.description }}</p>
        <div class="flex flex-wrap items-center gap-2 mt-3">
          <span
            v-for="tag in tags"
            :key="tag"
            class="text-[11px] font-mono px-2 py-0.5 rounded bg-g-3 border border-g-5 text-g-10"
          >{{ tag }}</span>
          <span
            v-if="trigger?.http"
            class="text-[11px] font-mono px-2 py-0.5 rounded bg-g-3 border border-g-5 text-g-10 flex items-center gap-1.5"
          >
            <Icon name="globe" class-name="w-3 h-3 text-g-8" />
            {{ trigger.http.method }} {{ trigger.http.path }}
          </span>
          <span
            v-else-if="trigger?.webhook"
            class="text-[11px] font-mono px-2 py-0.5 rounded bg-g-3 border border-g-5 text-g-10 flex items-center gap-1.5"
          >
            <Icon name="bolt" class-name="w-3 h-3 text-g-8" />
            webhook {{ trigger.webhook.path }}
          </span>
          <span
            v-else-if="trigger?.schedule"
            class="text-[11px] font-mono px-2 py-0.5 rounded bg-g-3 border border-g-5 text-g-10 flex items-center gap-1.5"
          >
            <Icon name="history" class-name="w-3 h-3 text-g-8" />
            cron · {{ trigger.schedule.cron }}
          </span>
          <span
            v-if="nextRunLabel"
            class="text-[11px] font-mono text-g-9 flex items-center gap-1.5"
          >
            <Icon name="history" class-name="w-3 h-3 text-g-8" />
            {{ nextRunLabel }}
          </span>
          <span
            v-if="(workflow as any).recovery"
            class="text-[11px] font-mono text-emerald-400 flex items-center gap-1.5"
          >
            <span class="w-1.5 h-1.5 bg-emerald-400 rounded-full pulse-dot" />
            recovery on
          </span>
        </div>
      </div>
      <div class="flex items-center gap-2">
        <button
          @click="openRun"
          :disabled="!workflowReady"
          class="h-8 px-3 rounded text-[12px] font-medium bg-g-14 text-g-1 hover:bg-g-15 disabled:opacity-50 flex items-center gap-1.5 cursor-pointer"
        >
          <Icon name="play" class-name="w-3.5 h-3.5" />
          {{ t('nav.run') }}
          <span class="ml-1 flex gap-0.5"><Kbd>⌘</Kbd><Kbd>↵</Kbd></span>
        </button>
      </div>
    </div>

    <!-- Agent metrics row -->
    <h2 class="text-[11px] font-semibold uppercase tracking-wider text-g-9 mb-2.5">{{ t('dashboard.systemMetrics') }}</h2>
    <div v-if="sysMetrics?.available !== false" class="grid grid-cols-6 gap-2 mb-5">
      <div class="bg-g-2 border border-g-5 rounded p-3 relative overflow-hidden">
        <svg v-if="cpuHistory.length > 1" class="absolute bottom-0 left-0 w-full h-1/2 text-g-12" preserveAspectRatio="none" viewBox="0 0 200 36">
          <path :d="sparkAreaPath(cpuHistory, 200, 36)" class="sparkline-fill" />
          <path :d="sparkLinePath(cpuHistory, 200, 36)" fill="none" stroke="currentColor" stroke-width="1" opacity="0.3" />
        </svg>
        <div class="relative">
          <p class="text-[10px] uppercase tracking-wider text-g-8 mb-1 font-mono">cpu</p>
          <p :class="['text-[18px] font-semibold font-mono tabular-nums', cpuColor(sysMetrics?.cpu_percent || 0)]">{{ sysMetrics?.cpu_percent?.toFixed(1) ?? '0' }}%</p>
        </div>
      </div>
      <div class="bg-g-2 border border-g-5 rounded p-3 relative overflow-hidden">
        <svg v-if="memHistory.length > 1" class="absolute bottom-0 left-0 w-full h-1/2 text-g-12" preserveAspectRatio="none" viewBox="0 0 200 36">
          <path :d="sparkAreaPath(memHistory, 200, 36)" class="sparkline-fill" />
          <path :d="sparkLinePath(memHistory, 200, 36)" fill="none" stroke="currentColor" stroke-width="1" opacity="0.3" />
        </svg>
        <div class="relative">
          <p class="text-[10px] uppercase tracking-wider text-g-8 mb-1 font-mono">memory</p>
          <p class="text-[18px] font-semibold text-g-14 font-mono tabular-nums">{{ sysMetrics ? fmtBytes(sysMetrics.memory_bytes) : '—' }}</p>
        </div>
      </div>
      <div class="bg-g-2 border border-g-5 rounded p-3 relative overflow-hidden">
        <svg v-if="goroutineHistory.length > 1" class="absolute bottom-0 left-0 w-full h-1/2 text-g-12" preserveAspectRatio="none" viewBox="0 0 200 36">
          <path :d="sparkAreaPath(goroutineHistory, 200, 36)" class="sparkline-fill" />
          <path :d="sparkLinePath(goroutineHistory, 200, 36)" fill="none" stroke="currentColor" stroke-width="1" opacity="0.3" />
        </svg>
        <div class="relative">
          <p class="text-[10px] uppercase tracking-wider text-g-8 mb-1 font-mono">goroutines</p>
          <p class="text-[18px] font-semibold text-g-14 font-mono tabular-nums">{{ sysMetrics?.goroutines ?? '—' }}</p>
        </div>
      </div>
      <div class="bg-g-2 border border-g-5 rounded p-3 relative overflow-hidden">
        <div class="relative">
          <p class="text-[10px] uppercase tracking-wider text-g-8 mb-1 font-mono">net rx</p>
          <p class="text-[18px] font-semibold text-g-14 font-mono tabular-nums">{{ sysMetrics ? fmtBytes(sysMetrics.net_rx_bytes) : '—' }}</p>
          <p v-if="sysMetrics" class="text-[10px] text-g-8 font-mono mt-0.5">↑ {{ fmtBytes(sysMetrics.net_tx_bytes) }}</p>
        </div>
      </div>
      <div class="bg-g-2 border border-g-5 rounded p-3">
        <p class="text-[10px] uppercase tracking-wider text-g-8 mb-1 font-mono">uptime</p>
        <p class="text-[18px] font-semibold text-g-14 font-mono tabular-nums">{{ sysMetrics ? fmtUptime(sysMetrics.uptime_s) : '—' }}</p>
      </div>
      <div class="bg-g-2 border border-g-5 rounded p-3">
        <p class="text-[10px] uppercase tracking-wider text-g-8 mb-1 font-mono">version</p>
        <p class="text-[18px] font-semibold text-g-14 font-mono tabular-nums">agent</p>
      </div>
    </div>

    <!-- Workflow metrics row -->
    <h2 class="text-[11px] font-semibold uppercase tracking-wider text-g-9 mb-2.5">{{ t('dashboard.workflowMetrics') }} · last 30 days</h2>
    <div class="grid grid-cols-5 gap-2 mb-5">
      <div class="bg-g-2 border border-g-5 rounded p-3">
        <p class="text-[10px] uppercase tracking-wider text-g-8 mb-1 font-mono">{{ t('dashboard.total') }}</p>
        <p class="text-[20px] font-semibold text-g-14 font-mono tabular-nums">{{ metrics.total }}</p>
      </div>
      <div class="bg-g-2 border border-g-5 rounded p-3">
        <p class="text-[10px] uppercase tracking-wider text-g-8 mb-1 font-mono">{{ t('dashboard.success') }}</p>
        <p class="text-[20px] font-semibold text-emerald-400 font-mono tabular-nums">{{ metrics.success }}</p>
        <p class="text-[10px] text-g-8 font-mono mt-0.5">{{ successPct }}%</p>
      </div>
      <div class="bg-g-2 border border-g-5 rounded p-3">
        <p class="text-[10px] uppercase tracking-wider text-g-8 mb-1 font-mono">{{ t('dashboard.failed') }}</p>
        <p class="text-[20px] font-semibold text-red-400 font-mono tabular-nums">{{ metrics.failed }}</p>
        <p class="text-[10px] text-g-8 font-mono mt-0.5">{{ failPct }}%</p>
      </div>
      <div class="bg-g-2 border border-g-5 rounded p-3">
        <p class="text-[10px] uppercase tracking-wider text-g-8 mb-1 font-mono">{{ t('dashboard.running') }}</p>
        <div class="flex items-baseline gap-1.5">
          <p class="text-[20px] font-semibold text-amber-400 font-mono tabular-nums">{{ metrics.running }}</p>
          <span v-if="metrics.running > 0" class="w-1.5 h-1.5 bg-amber-400 rounded-full pulse-dot" />
        </div>
        <p v-if="metrics.running > 0" class="text-[10px] text-g-8 font-mono mt-0.5">live</p>
      </div>
      <div class="bg-g-2 border border-g-5 rounded p-3">
        <p class="text-[10px] uppercase tracking-wider text-g-8 mb-1 font-mono">{{ t('dashboard.avgDuration') }}</p>
        <p class="text-[20px] font-semibold text-g-14 font-mono tabular-nums">{{ metrics.avgMs ? fmtDur(metrics.avgMs) : '—' }}</p>
      </div>
    </div>

    <!-- Pipeline preview (DAG) -->
    <div v-if="graph" class="flex items-baseline justify-between mb-2.5">
      <h2 class="text-[11px] font-semibold uppercase tracking-wider text-g-9">Pipeline</h2>
      <RouterLink to="/executions" class="text-[11px] text-g-9 hover:text-g-13 font-mono">open history →</RouterLink>
    </div>
    <div v-if="graph" class="bg-g-2 border border-g-5 rounded p-3 mb-5 overflow-hidden">
      <WorkflowDAGCustom
        :graph="graph"
        :step-statuses="pipelineStatuses"
        :step-durations="pipelineDurations"
        :show-stages="true"
        :height="320"
        @node-click="(id: string) => stepInspector.inspect(id)"
      />
    </div>

    <!-- Recent runs table -->
    <div class="flex items-baseline justify-between mb-2.5">
      <h2 class="text-[11px] font-semibold uppercase tracking-wider text-g-9">{{ t('dashboard.recentExecs') }}</h2>
      <RouterLink to="/executions" class="text-[11px] text-g-9 hover:text-g-13 font-mono">{{ t('dashboard.viewAll') }} →</RouterLink>
    </div>
    <div v-if="executions.length === 0" class="bg-g-2 border border-g-5 rounded py-10 text-center text-g-9 text-sm">
      {{ t('dashboard.noExecs') }}
    </div>
    <div v-else class="bg-g-2 border border-g-5 rounded overflow-hidden">
      <div class="grid grid-cols-[20px_140px_90px_80px_1fr_80px_80px] gap-3 px-3 py-2 border-t border-g-4 text-[10px] uppercase tracking-wider text-g-8 font-mono">
        <div></div><div>id</div><div>status</div><div>duration</div><div>params</div><div>trigger</div><div>started</div>
      </div>
      <div
        v-for="r in executions.slice(0, 8)"
        :key="r.id"
        @click="router.push({ name: 'execution', params: { id: r.id } })"
        class="grid grid-cols-[20px_140px_90px_80px_1fr_80px_80px] gap-3 px-3 py-2 hover:bg-g-3 cursor-pointer border-t border-g-4 items-center"
      >
        <span :class="['w-1.5 h-1.5 rounded-full', statusDot(r.status)]" />
        <span class="font-mono text-[12px] text-g-12 truncate">{{ r.id }}</span>
        <StatusBadge :status="r.status" />
        <span class="font-mono text-[12px] text-g-11 tabular-nums">{{ execDuration(r) ? fmtDur(execDuration(r)!) : '— running —' }}</span>
        <span class="font-mono text-[11px] text-g-9 truncate">{{ paramsLabel(r) }}</span>
        <span class="font-mono text-[11px] text-g-9">{{ (r as any).trigger || 'manual' }}</span>
        <span class="font-mono text-[11px] text-g-8 tabular-nums">{{ fmtAgo(r.started_at) }}</span>
      </div>
    </div>
  </div>

  <div v-else-if="api.loading.value" class="flex items-center justify-center py-32">
    <div class="flex flex-col items-center">
      <div class="w-6 h-6 border-2 border-g-5 border-t-g-9 rounded-full animate-spin" />
      <span class="mt-3 text-sm text-g-7">{{ t('executions.loading') }}</span>
    </div>
  </div>
  <div v-else-if="api.error.value" class="flex flex-col items-center justify-center py-32">
    <div class="w-12 h-12 rounded-full bg-red-400/10 flex items-center justify-center mb-4">
      <svg class="w-6 h-6 text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
        <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z" />
      </svg>
    </div>
    <p class="text-sm font-medium text-g-12 mb-1">{{ t('executions.connectionLost') }}</p>
    <p class="text-[13px] text-g-7 text-center max-w-sm">{{ t('executions.connectionLostDesc') }}</p>
  </div>
</template>
