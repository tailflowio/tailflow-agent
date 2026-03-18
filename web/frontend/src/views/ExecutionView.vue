<script setup lang="ts">
import { ref, onMounted, computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useWorkflowApi, type Graph, type Execution } from '@/composables/useWorkflowApi'

const { t } = useI18n()
import { useSSE } from '@/composables/useSSE'
import type { EventRow } from '@tailflow/shared'
import StepTimeline from '@/components/StepTimeline.vue'
import EventTimeline from '@tailflow/shared/components/EventTimeline.vue'

const route = useRoute()
const router = useRouter()
const api = useWorkflowApi()

const executionId = route.params.id as string
const execution = ref<Execution | null>(null)
const graph = ref<Graph | null>(null)
const fetchError = ref(false)
const initialLoading = ref(true)

const { events, connected, finished, stepStatuses, stepVolumes, stepIterations, stepOutputHistory, stepPipelineProgress, connect } = useSSE(executionId, {
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
    connect()
  }
})

watch(finished, (v) => {
  if (v) {
    api.getExecution(executionId).then(e => {
      execution.value = e as Execution
      if (e.steps) {
        for (const [id, step] of Object.entries(e.steps)) {
          if (step.status) {
            stepStatuses.value[id] = step.status
          }
        }
      }
    }).catch(() => {})
  }
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

function statusBadge(s: string) {
  if (s === 'success') return 'bg-emerald-400/15 text-emerald-400'
  if (s === 'failed') return 'bg-red-400/15 text-red-400'
  if (s === 'cancelled') return 'bg-orange-400/15 text-orange-400'
  if (s === 'running') return 'bg-amber-400/15 text-amber-400'
  if (s === 'waiting') return 'bg-amber-400/15 text-amber-400'
  if (s === 'pending') return 'bg-violet-400/15 text-violet-400'
  return 'bg-g-7/20 text-g-9'
}

function isStatusAnimated(s: string) {
  return s === 'running' || s === 'waiting'
}

const eventRows = computed<EventRow[]>(() =>
  events.value.map(ev => ({
    event_type: ev.type,
    step_id: ev.step_id || '',
    message: ev.message || '',
    data: ev.data ? JSON.stringify(ev.data) : '',
    event_timestamp: ev.timestamp,
    execution_id: ev.execution_id,
    seq: ev.seq,
  }))
)
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

  <div v-else class="space-y-6">
    <!-- Header -->
    <div>
      <div class="flex items-center gap-4 mb-3">
        <button @click="router.push('/')" class="text-g-8 hover:text-g-12 text-sm transition-colors cursor-pointer">&larr; {{ t('executions.title') }}</button>
      </div>
      <div class="flex items-start justify-between">
        <div>
          <h1 class="text-lg font-semibold text-g-14">{{ execution?.workflow_name || 'Execution' }}</h1>
          <div class="flex items-center gap-3 mt-1">
            <span class="text-[12px] font-mono text-g-7">{{ executionId.slice(0, 8) }}</span>
            <span class="text-[12px] text-g-9 font-mono">{{ duration }}</span>
          </div>
        </div>
        <div class="flex items-center gap-2">
          <span
            v-if="connected && !finished"
            class="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium bg-emerald-400/15 text-emerald-400"
          >
            <span class="relative flex h-1.5 w-1.5">
              <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
              <span class="relative inline-flex rounded-full h-1.5 w-1.5 bg-emerald-400"></span>
            </span>
            {{ t('execution.live') }}
          </span>
          <span :class="['inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium', statusBadge(statusLabel)]">
            <span :class="['w-1.5 h-1.5 rounded-full bg-current', isStatusAnimated(statusLabel) ? 'pulse-dot' : 'opacity-50']" />
            {{ statusLabel }}
          </span>
        </div>
      </div>
    </div>

    <!-- Cancel banner -->
    <div
      v-if="canCancel"
      class="flex items-center justify-between px-4 py-3 rounded-lg border border-amber-400/20 bg-amber-400/5"
    >
      <div class="flex items-center gap-3">
        <svg class="w-4 h-4 text-amber-400 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20zM12 6v6l4 2"/></svg>
        <p class="text-sm text-g-11">Execution is currently running.</p>
      </div>
      <button
        @click="cancelExec"
        :disabled="cancelling"
        class="shrink-0 ml-4 px-3 py-1.5 text-xs font-medium rounded-md bg-red-400/10 text-red-400 hover:bg-red-400/20 transition-colors disabled:opacity-50 cursor-pointer"
      >
        <span v-if="cancelling" class="flex items-center gap-1.5">
          <svg class="w-3 h-3 animate-spin" viewBox="0 0 24 24" fill="none"><circle cx="12" cy="12" r="10" stroke="currentColor" stroke-width="2.5" class="opacity-25"/><path d="M12 2a10 10 0 0110 10" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" class="opacity-75"/></svg>
          {{ t('execution.cancelling') }}
        </span>
        <span v-else>{{ t('execution.cancel') }}</span>
      </button>
    </div>

    <!-- Error -->
    <div
      v-if="execution?.error"
      class="px-4 py-3 rounded-lg text-[13px] bg-red-400/5 text-red-400 border border-red-400/20 font-mono"
    >
      {{ execution.error }}
    </div>

    <!-- Step Timeline -->
    <StepTimeline
      v-if="orderedSteps.length > 0"
      :steps="orderedSteps"
      :step-statuses="stepStatuses"
      :step-volumes="stepVolumes"
      :step-iterations="stepIterations"
      :step-output-history="stepOutputHistory"
      :step-pipeline-progress="stepPipelineProgress"
      :events="events"
    />

    <!-- Event Log -->
    <EventTimeline :events="eventRows" />
  </div>
</template>
