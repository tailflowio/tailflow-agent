<script setup lang="ts">
import { ref, onMounted, computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useWorkflowApi, type Graph, type Execution } from '@/composables/useWorkflowApi'

const { t } = useI18n()
import { useSSE } from '@/composables/useSSE'
import type { EventRow } from '@tailflow/shared'
import LogsDock from '@/components/LogsDock.vue'
import ActiveRunsStrip from '@/components/ActiveRunsStrip.vue'
import WorkflowDAGCustom from '@/components/WorkflowDAGCustom.vue'
import { useStepInspector } from '@/composables/useStepInspector'
import StatusBadge from '@/components/primitives/StatusBadge.vue'
import Icon from '@/components/primitives/Icon.vue'
import { fmtAgo } from '@/composables/useFormat'

const route = useRoute()
useRouter()
const api = useWorkflowApi()
const inspector = useStepInspector()

const executionId = route.params.id as string
const execution = ref<Execution | null>(null)
const graph = ref<Graph | null>(null)
const fetchError = ref(false)
const initialLoading = ref(true)

const { events, connected, finished, stepStatuses, stepVolumes, stepIterations, connect } = useSSE(executionId, {
  onDisconnect: async () => {
    try {
      const fresh = await api.getExecution(executionId) as Execution
      if (fresh.finished_at) {
        execution.value = fresh
        if (fresh.steps) {
          for (const [id, step] of Object.entries(fresh.steps)) {
            if (step.status) stepStatuses.value[id] = step.status
          }
        }
      }
    } catch {}
  },
})

async function fetchExecution() {
  fetchError.value = false
  try {
    execution.value = await api.getExecution(executionId) as Execution
  } catch {
    fetchError.value = true
    return
  }

  try {
    graph.value = await api.getWorkflowGraph()
  } catch {}

  if (execution.value?.steps) {
    for (const [id, step] of Object.entries(execution.value.steps)) {
      if (step.status) stepStatuses.value[id] = step.status === 'success' ? 'success'
        : step.status === 'failed' ? 'failed'
        : step.status === 'skipped' ? 'skipped'
        : step.status === 'running' ? 'running'
        : step.status === 'waiting' ? 'waiting'
        : step.status
      if (step.input || step.output) {
        if (!stepVolumes.value[id]) stepVolumes.value[id] = {}
        if (step.input) stepVolumes.value[id].input = step.input
        if (step.output) stepVolumes.value[id].output = step.output
      }
    }
  }
}

async function retryFetch() {
  initialLoading.value = true
  fetchError.value = false
  await fetchExecution()
  if (execution.value) connect()
  initialLoading.value = false
}

onMounted(async () => {
  await fetchExecution()

  if (!execution.value && !fetchError.value) {
    await new Promise(r => setTimeout(r, 500))
    await fetchExecution()
  }

  initialLoading.value = false

  if (execution.value) {
    if (execution.value.finished_at) {
      fetchEventPage(false)
    }

    connect()
  }
})

watch(finished, (v) => {
  if (!v) return
  fetchEventPage(false)

  // Recovery path: refresh execution from API. The SSE bus is drop-on-full
  // (bus.go:73) so a slow SSE subscriber may have missed events; the API
  // snapshot is updated by a separate subscriber and acts as the safety net.
  api.getExecution(executionId).then(e => {
    execution.value = e as Execution
    if (e.steps) {
      for (const [id, step] of Object.entries(e.steps)) {
        if (step.status) stepStatuses.value[id] = step.status
      }
    }
  }).catch(() => {})
})

const statusLabel = computed(() => execution.value?.status || 'pending')
const duration = computed(() => {
  if (!execution.value) return '-'
  if (!execution.value.finished_at) return t('execution.inProgress')
  const ms = new Date(execution.value.finished_at).getTime() - new Date(execution.value.started_at).getTime()
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
})

const orderedSteps = computed(() => {
  const nodes = graph.value?.nodes ?? []
  const apiSteps = execution.value?.steps ?? {}
  return nodes.map(n => {
    const result = apiSteps[n.id] ?? { status: stepStatuses.value[n.id] ?? 'pending' }
    return [n.id, result] as [string, typeof result]
  })
})

const cancelling = ref(false)
const canCancel = computed(() => {
  const s = execution.value?.status
  return s === 'running' || s === 'waiting'
})

async function cancelExec() {
  if (!canCancel.value || cancelling.value) return
  cancelling.value = true
  try {
    await api.cancelExecution(executionId)
    execution.value = await api.getExecution(executionId) as Execution
  } catch {} finally {
    cancelling.value = false
  }
}

const paginatedEvents = ref<EventRow[]>([])
const paginatedTotal = ref(0)
const paginatedHasMore = ref(false)
const paginatedLoading = ref(false)
const isFinished = computed(() => !!execution.value?.finished_at)

const stepStageMap = computed(() => {
  const map: Record<string, string> = {}
  const stages = graph.value?.stages
  if (!stages) return map
  for (const stage of stages) {
    for (const stepId of stage.steps) {
      map[stepId] = stage.name
    }
  }
  return map
})

function mapEvent(ev: any): EventRow {
  const stepId = ev.step_id || ''
  return {
    event_type: ev.type || ev.event_type || '',
    step_id: stepId,
    message: ev.message || '',
    data: ev.data ? (typeof ev.data === 'string' ? ev.data : JSON.stringify(ev.data)) : '',
    event_timestamp: ev.timestamp || ev.event_timestamp || '',
    execution_id: ev.execution_id || '',
    seq: ev.seq,
    stage: stepStageMap.value[stepId] || '',
  }
}

// Live event rows are maintained incrementally to avoid re-mapping the full
// array on every SSE batch. Only newly-arrived events are mapped.
const liveEventRows = ref<EventRow[]>([])

watch(() => events.value.length, (newLen, oldLen = 0) => {
  if (newLen === 0) {
    liveEventRows.value = []
    return
  }
  if (newLen < oldLen) {
    // SSE reconnect or reset → rebuild from scratch
    liveEventRows.value = events.value.map(mapEvent)
    return
  }
  // Append only new events
  const additions: EventRow[] = []
  for (let i = oldLen; i < newLen; i++) {
    additions.push(mapEvent(events.value[i]))
  }
  if (additions.length) liveEventRows.value.push(...additions)
}, { immediate: true })

// When the graph (and therefore the stage map) loads after events have started
// arriving, re-map so previously-loaded rows pick up their stage.
watch(stepStageMap, () => {
  if (events.value.length === 0) return
  liveEventRows.value = events.value.map(mapEvent)
})

const eventRows = computed<EventRow[]>(() => {
  if (isFinished.value) return paginatedEvents.value
  return liveEventRows.value
})

const eventTotal = computed(() => {
  if (isFinished.value) return paginatedTotal.value
  return events.value.length
})

const eventHasMore = computed(() => {
  if (isFinished.value) return paginatedHasMore.value
  return false
})

async function fetchEventPage(append = false) {
  paginatedLoading.value = true
  try {
    const offset = append ? paginatedEvents.value.length : 0
    const resp = await api.listExecutionEvents(executionId, offset, 50)
    const rows = (resp.events ?? []).map(mapEvent)
    if (append) {
      paginatedEvents.value.push(...rows)
    } else {
      paginatedEvents.value = rows
    }
    paginatedTotal.value = resp.total
    paginatedHasMore.value = resp.hasMore
  } catch {} finally {
    paginatedLoading.value = false
  }
}

function onLoadMore() {
  fetchEventPage(true)
}

function copyShareUrl() {
  try { navigator.clipboard?.writeText(window.location.href) } catch {}
}
</script>

<template>
  <div v-if="initialLoading && !execution" class="flex flex-col items-center justify-center py-32">
    <div class="w-6 h-6 border-2 border-g-5 border-t-g-9 rounded-full animate-spin" />
    <span class="mt-3 text-sm text-g-7">{{ t('executions.loading') }}</span>
  </div>

  <div v-else-if="fetchError && !execution" class="flex flex-col items-center justify-center py-32">
    <div class="w-12 h-12 rounded-full bg-red-400/10 flex items-center justify-center mb-4">
      <svg class="w-6 h-6 text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
        <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z" />
      </svg>
    </div>
    <p class="text-sm font-medium text-g-12 mb-1">{{ t('executions.connectionLost') }}</p>
    <p class="text-[13px] text-g-7 text-center max-w-sm mb-5">{{ t('executions.connectionLostDesc') }}</p>
    <button
      @click="retryFetch"
      class="px-4 py-2 text-sm font-medium rounded-lg bg-g-3 border border-g-5 text-g-12 hover:bg-g-4 hover:border-g-6 transition-colors cursor-pointer"
    >
      {{ t('executions.retry') }}
    </button>
  </div>

  <div v-else class="flex flex-col h-full">
    <!-- Active runs strip -->
    <ActiveRunsStrip :current-run-id="executionId" />

    <!-- Header -->
    <div class="px-6 pt-5 pb-3 border-b border-g-5 shrink-0">
      <div class="flex items-start justify-between gap-4">
        <div class="min-w-0">
          <div class="flex items-center gap-3 mb-1 flex-wrap">
            <h1 class="text-[16px] font-semibold text-g-14">{{ execution?.workflow_name || 'Execution' }}</h1>
            <span class="font-mono text-[12px] text-g-8">{{ executionId }}</span>
            <StatusBadge :status="statusLabel" />
            <span
              v-if="connected && !isFinished && statusLabel === 'running'"
              class="text-[11px] font-mono text-emerald-400 flex items-center gap-1.5"
            >
              <span class="relative flex h-2 w-2">
                <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
                <span class="relative inline-flex rounded-full h-2 w-2 bg-emerald-400"></span>
              </span>
              live
            </span>
            <span
              v-if="statusLabel === 'waiting'"
              class="text-[11px] font-mono text-violet-400 flex items-center gap-1.5"
            >
              <span class="w-2 h-2 rounded-full bg-violet-400" />
              waiting
            </span>
          </div>
          <div class="flex items-center gap-3 text-[11px] font-mono text-g-9 flex-wrap">
            <span>started {{ fmtAgo(execution?.started_at || 0) }}</span>
            <span class="text-g-7">·</span>
            <span>elapsed <span class="text-g-12 tabular-nums">{{ duration }}</span></span>
            <template v-if="execution?.params && Object.keys(execution.params).length > 0">
              <template v-for="(v, k) in execution.params" :key="k">
                <span class="text-g-7">·</span>
                <span>{{ k }}=<span class="text-g-11">{{ v }}</span></span>
              </template>
            </template>
          </div>
        </div>
        <div class="flex items-center gap-2 shrink-0">
          <button
            class="h-8 px-2.5 rounded text-[12px] font-medium bg-g-3 border border-g-5 text-g-11 hover:bg-g-4 flex items-center gap-1.5"
            @click="copyShareUrl"
            title="Copy URL"
          >
            <Icon name="copy" class-name="w-3.5 h-3.5" />
            Share
          </button>
          <button
            v-if="canCancel"
            @click="cancelExec"
            :disabled="cancelling"
            class="h-8 px-2.5 rounded text-[12px] font-medium bg-red-400/10 text-red-400 hover:bg-red-400/20 disabled:opacity-50"
          >
            {{ cancelling ? t('execution.cancelling') : t('execution.cancel') }}
          </button>
        </div>
      </div>

      <!-- Progress strip -->
      <div v-if="orderedSteps.length > 0" class="mt-3 flex items-center gap-4">
        <div class="flex items-center gap-1.5">
          <div
            v-for="[id, s] in orderedSteps"
            :key="id"
            :title="`${id} · ${s.status}`"
            :class="[
              'w-7 h-1.5 rounded-sm',
              s.status === 'success' ? 'bg-emerald-400' :
              s.status === 'running' ? 'bg-amber-400 animate-pulse' :
              s.status === 'waiting' ? 'bg-violet-400 animate-pulse' :
              s.status === 'failed' ? 'bg-red-400' :
              s.status === 'cancelled' ? 'bg-orange-400' :
              s.status === 'skipped' ? 'bg-g-7' : 'bg-g-5'
            ]"
          />
        </div>
        <span class="text-[11px] font-mono text-g-9">{{ orderedSteps.filter(([, s]) => s.status === 'success').length }}/{{ orderedSteps.length }} steps</span>
      </div>

      <!-- Error detail -->
      <div
        v-if="execution?.error"
        class="mt-3 px-3 py-2 rounded text-[12px] bg-red-400/5 text-red-400 border border-red-400/20 font-mono"
      >
        {{ execution.error }}
      </div>
    </div>

    <!-- Body: live DAG (top) + LogsDock (bottom) — full-width grid like the design -->
    <div class="flex-1 min-h-0 grid grid-rows-[minmax(360px,1fr)_minmax(480px,40%)]">
      <!-- Live DAG with animated edges -->
      <div v-if="graph" class="bg-g-1 border-b border-g-5 relative overflow-hidden">
        <div class="absolute inset-0 p-4">
          <WorkflowDAGCustom
            :graph="graph"
            :step-statuses="stepStatuses"
            :step-iterations="stepIterations"
            :selected-id="inspector.stepId.value"
            :show-stages="true"
            :fill-height="true"
            @node-click="(id: string) => inspector.inspect(id)"
          />
        </div>
      </div>

      <!-- Event Log dock (Events / Logs / I/O tabs + streaming indicator) -->
      <LogsDock
        :events="eventRows"
        :total="eventTotal"
        :has-more="eventHasMore"
        :loading="paginatedLoading"
        :server-side="isFinished"
        :connected="connected"
        :finished="isFinished"
        :fill-height="true"
        class="!rounded-none !border-0"
        @load-more="onLoadMore"
      />
    </div>
  </div>
</template>
