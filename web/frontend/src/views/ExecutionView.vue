<script setup lang="ts">
import { ref, nextTick, onMounted, computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useWorkflowApi, type Graph, type Execution } from '@/composables/useWorkflowApi'

const { t } = useI18n()
import { useSSE } from '@/composables/useSSE'
import type { EventRow } from '@tailflow/shared'
import { JsonView } from '@tailflow/shared'
import WorkflowGraph from '@/components/WorkflowGraph.vue'
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
    // SSE dropped without workflow.completed — re-fetch to check if execution finished
    try {
      const fresh = await api.getExecution(executionId) as Execution
      if (fresh.finished_at) {
        execution.value = fresh
        // Sync step statuses from final data
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

  // Retry once if the initial fetch failed (execution may not be persisted yet)
  if (!execution.value && !fetchError.value) {
    await new Promise(r => setTimeout(r, 500))
    await fetchExecution()
  }

  initialLoading.value = false

  if (execution.value) {
    // Always connect SSE: replays stored events for finished executions,
    // streams live events for running ones
    connect()
  }
})

// Re-fetch execution when SSE stream finishes and sync step statuses
watch(finished, (v) => {
  if (v) {
    api.getExecution(executionId).then(e => {
      execution.value = e as Execution
      // Sync stepStatuses from final execution data to fix any missed SSE events
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
  // Use graph node order (matches workflow YAML order) so ALL steps appear
  // even before they start, and the order is stable.
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
    // Re-fetch to get updated status
    execution.value = await api.getExecution(executionId) as Execution
  } catch {} finally {
    cancelling.value = false
  }
}

function statusBadge(s: string) {
  if (s === 'success') return 'bg-emerald-400/15 text-emerald-400'
  if (s === 'failed') return 'bg-red-400/15 text-red-400'
  if (s === 'cancelled') return 'bg-g-7/20 text-g-9'
  if (s === 'running') return 'bg-amber-400/15 text-amber-400'
  if (s === 'waiting') return 'bg-amber-400/15 text-amber-400'
  return 'bg-g-7/20 text-g-9'
}
function isStatusAnimated(s: string) {
  return s === 'running' || s === 'waiting'
}
function statusDot(s: string) {
  if (s === 'success') return 'bg-emerald-400'
  if (s === 'failed') return 'bg-red-400'
  if (s === 'running' || s === 'waiting') return 'bg-amber-400 animate-pulse'
  if (s === 'skipped' || s === 'cancelled') return 'bg-g-7'
  return 'bg-g-7'
}

function stepStatus(stepId: string, result: any): string {
  return stepStatuses.value[stepId] ?? result?.status ?? 'pending'
}

const dagAutoHeight = ref(400)
const dagCollapsed = ref(localStorage.getItem('tailflow-exec-dag-collapsed') !== 'false')
const resultsCollapsed = ref(localStorage.getItem('tailflow-exec-results-collapsed') !== 'false')

function toggleResults() {
  resultsCollapsed.value = !resultsCollapsed.value
  if (resultsCollapsed.value) collapseAll()
  localStorage.setItem('tailflow-exec-results-collapsed', String(resultsCollapsed.value))
}

function toggleDag() {
  dagCollapsed.value = !dagCollapsed.value
  localStorage.setItem('tailflow-exec-dag-collapsed', String(dagCollapsed.value))
}

const dagStepCount = computed(() => graph.value?.nodes?.length ?? 0)

function onGraphReady(payload: { maxY: number }) {
  dagAutoHeight.value = Math.max(300, payload.maxY + 120)
}

const expandedSteps = ref<Record<string, boolean>>({})
const deferredSteps = ref<Record<string, boolean>>({})
const expandedIterations = ref<Record<string, boolean>>({})
const focusedStepId = ref<string | null>(null)

function isStepExpanded(stepId: string): boolean {
  return expandedSteps.value[stepId] ?? false
}

function isStepContentReady(stepId: string): boolean {
  return deferredSteps.value[stepId] ?? false
}

function deferContent(stepId: string) {
  // Double rAF: first rAF lets browser paint the spinner, second rAF triggers heavy render
  requestAnimationFrame(() => {
    requestAnimationFrame(() => { deferredSteps.value[stepId] = true })
  })
}

function toggleStep(stepId: string) {
  const opening = !isStepExpanded(stepId)
  if (opening) {
    expandedSteps.value[stepId] = true
    deferredSteps.value[stepId] = false
    deferContent(stepId)
  } else {
    // Hide heavy content first (instant), then collapse container after paint
    deferredSteps.value[stepId] = false
    requestAnimationFrame(() => {
      requestAnimationFrame(() => { expandedSteps.value[stepId] = false })
    })
  }
}

function onDagStepClick(stepId: string) {
  expandedSteps.value[stepId] = true
  deferredSteps.value[stepId] = false
  deferContent(stepId)
  focusedStepId.value = stepId
  nextTick(() => {
    const el = document.getElementById('step-' + stepId)
    if (el) {
      el.scrollIntoView({ behavior: 'smooth', block: 'center' })
    }
    setTimeout(() => { focusedStepId.value = null }, 1500)
  })
}

function collapseAll() {
  expandedIterations.value = {}
  const collapsed: Record<string, boolean> = {}
  for (const [stepId] of orderedSteps.value) {
    collapsed[stepId] = false
  }
  expandedSteps.value = collapsed
  deferredSteps.value = {}
}

function toggleIteration(key: string) {
  expandedIterations.value[key] = !expandedIterations.value[key]
}

function isIterationExpanded(key: string): boolean {
  return expandedIterations.value[key] ?? false
}

function formatIterationTime(timestamp: string): string {
  const d = new Date(timestamp)
  return d.toLocaleTimeString('fr-FR', { hour12: false, fractionalSecondDigits: 3 } as Intl.DateTimeFormatOptions)
}

function isPipelineIterExpanded(stepId: string, iteration: number): boolean {
  const key = `${stepId}-pipe-${iteration}`
  return expandedIterations.value[key] ?? true
}

function pipelineGrouped(stepId: string) {
  const entries = stepPipelineProgress.value[stepId]
  if (!entries || entries.length === 0) return []
  const grouped: Record<number, typeof entries> = {}
  for (const e of entries) {
    if (!grouped[e.iteration]) grouped[e.iteration] = []
    grouped[e.iteration].push(e)
  }
  return Object.entries(grouped).map(([iter, steps]) => ({
    iteration: parseInt(iter),
    totalIterations: steps[0].totalIterations,
    totalSteps: steps[0].totalSteps,
    steps,
  }))
}

function pipelineDot(status: string) {
  if (status === 'ok') return 'bg-emerald-400'
  if (status === 'failed') return 'bg-red-400'
  return 'bg-g-12 animate-pulse'
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
  <!-- Loading state -->
  <div v-if="initialLoading && !execution" class="flex flex-col items-center justify-center py-32">
    <div class="w-6 h-6 border-2 border-g-5 border-t-g-9 rounded-full animate-spin" />
    <span class="mt-3 text-sm text-g-7">{{ t('executions.loading') }}</span>
  </div>

  <!-- Error state -->
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

  <div v-else>
    <!-- Header -->
    <div class="mb-6 space-y-3">
      <!-- Breadcrumb -->
      <div class="flex items-center gap-4">
        <button @click="router.push('/')" class="text-g-8 hover:text-g-12 text-sm transition-colors cursor-pointer">&larr; {{ t('executions.title') }}</button>
      </div>
      <!-- Title + status -->
      <div class="flex items-start justify-between">
        <div>
          <h1 class="text-lg font-semibold text-g-14">{{ execution?.workflow_name || 'Execution' }}</h1>
          <div class="flex items-center gap-3 mt-1">
            <span class="text-[12px] font-mono text-g-7">{{ executionId.slice(0, 8) }}</span>
            <span class="text-[12px] text-g-9 font-mono">{{ duration }}</span>
          </div>
        </div>
        <div class="flex items-center gap-2">
          <span v-if="connected" class="flex items-center gap-1.5 text-[11px] text-emerald-400 font-medium mr-1">
            <span class="w-1.5 h-1.5 bg-emerald-400 rounded-full animate-pulse" />
            {{ t('execution.live') }}
          </span>
          <span :class="['inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium', statusBadge(statusLabel)]">
            <span :class="['w-1.5 h-1.5 rounded-full bg-current', isStatusAnimated(statusLabel) ? 'pulse-dot' : 'opacity-50']" />
            {{ statusLabel }}
          </span>
          <button
            v-if="canCancel"
            @click="cancelExec"
            :disabled="cancelling"
            class="ml-1 px-3 py-1 text-[12px] font-medium rounded-md border border-red-400/30 text-red-400 hover:bg-red-400/10 transition-colors disabled:opacity-50 cursor-pointer"
          >
            {{ cancelling ? t('execution.cancelling') : t('execution.cancel') }}
          </button>
        </div>
      </div>
    </div>

    <!-- Error -->
    <div
      v-if="execution?.error"
      class="mb-5 px-4 py-3 rounded-lg text-[13px] bg-red-400/5 text-red-400 border border-red-400/20 font-mono"
    >
      {{ execution.error }}
    </div>

    <!-- DAG -->
    <div class="bg-g-2 border border-g-5 rounded-lg mb-6 overflow-hidden lm-card">
      <button
        @click="toggleDag"
        class="w-full px-4 py-2.5 flex items-center gap-2 text-left cursor-pointer hover:bg-g-3 transition-colors"
      >
        <span class="text-[10px] text-g-7">{{ dagCollapsed ? '▸' : '▾' }}</span>
        <span class="text-sm font-medium text-g-12">{{ t('execution.graph') }}</span>
        <span class="text-[11px] text-g-7 font-mono">{{ dagStepCount }} steps</span>
        <span class="ml-auto text-[10px] text-g-7 font-mono uppercase tracking-wider">{{ dagCollapsed ? t('execution.expand') : t('execution.collapse') }}</span>
      </button>
      <div v-if="!dagCollapsed" :style="{ height: dagAutoHeight + 'px' }">
        <WorkflowGraph
          v-if="graph"
          :graph="graph"
          :step-statuses="stepStatuses"
          :step-iterations="stepIterations"
          @nodeClick="onDagStepClick"
          @layout-ready="onGraphReady"
        />
        <div v-else class="flex items-center justify-center h-full">
          <div class="w-5 h-5 border-2 border-g-7 border-t-g-12 rounded-full animate-spin" />
        </div>
      </div>
    </div>

    <!-- Step results with volume inspector -->
    <div v-if="orderedSteps.length > 0" class="bg-g-2 border border-g-5 rounded-lg mb-6 overflow-hidden lm-card">
      <button
        @click="toggleResults"
        class="w-full px-4 py-2.5 flex items-center gap-2 text-left cursor-pointer hover:bg-g-3 transition-colors"
      >
        <span class="text-[10px] text-g-7">{{ resultsCollapsed ? '▸' : '▾' }}</span>
        <span class="text-sm font-medium text-g-12">{{ t('execution.results') }}</span>
        <span class="text-[11px] text-g-7 font-mono">{{ orderedSteps.length }} steps</span>
        <span class="ml-auto text-[10px] text-g-7 font-mono uppercase tracking-wider">{{ resultsCollapsed ? t('execution.expand') : t('execution.collapse') }}</span>
      </button>
      <div v-if="!resultsCollapsed">
        <div
          v-for="[stepId, result] in orderedSteps"
          :key="stepId"
          :id="'step-' + stepId"
          :class="[
            'px-4 py-3 border-b border-g-5 last:border-b-0',
            focusedStepId === stepId
              ? 'ring-1 ring-white/60 bg-g-3 transition-all duration-700'
              : ''
          ]"
        >
          <button
            @click="toggleStep(stepId as string)"
            class="flex items-center gap-2.5 mb-1 w-full text-left cursor-pointer group"
          >
            <span class="text-[14px] font-mono text-g-12">{{ stepId }}</span>
            <span :class="['inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-[11px] font-medium', statusBadge(stepStatus(stepId as string, result))]">
              <span :class="['w-1.5 h-1.5 rounded-full bg-current', isStatusAnimated(stepStatus(stepId as string, result)) ? 'pulse-dot' : 'opacity-50']" />
              {{ stepStatus(stepId as string, result) }}
            </span>
            <span
              v-if="stepIterations[stepId as string] && stepIterations[stepId as string] > 1"
              class="text-[10px] font-mono font-semibold rounded-full px-1.5 leading-[18px] bg-amber-400/15 text-amber-400 border border-amber-400/25"
            >
              x{{ stepIterations[stepId as string] }}
            </span>
          </button>

          <template v-if="isStepExpanded(stepId as string)">
          <!-- Deferred loading -->
          <div v-if="!isStepContentReady(stepId as string)" class="flex items-center justify-center gap-2 py-4">
            <div class="w-3.5 h-3.5 border-2 border-g-5 border-t-g-9 rounded-full animate-spin" />
            <span class="text-[12px] text-g-7">{{ t('execution.loading') }}</span>
          </div>
          <template v-else>
          <!-- Waiting badge -->
          <div
            v-if="stepVolumes[stepId as string]?.waiting"
            class="ml-[18px] mt-1 mb-2 inline-flex items-center gap-2 text-[11px] font-mono text-amber-400 bg-amber-400/10 border border-amber-400/20 rounded px-2 py-1"
          >
            <span class="w-1.5 h-1.5 bg-amber-400 rounded-full animate-pulse" />
            {{ t('execution.waiting', { type: stepVolumes[stepId as string].waiting!.type }) }}
          </div>

          <!-- Input -->
          <div v-if="stepVolumes[stepId as string]?.input" class="ml-[18px] mt-2">
            <span class="text-[11px] font-mono text-g-9 uppercase tracking-wider">{{ t('execution.input') }}</span>
            <div class="mt-1 bg-g-3 rounded px-3 py-2 overflow-x-auto max-h-[400px] overflow-y-auto">
              <JsonView :data="stepVolumes[stepId as string].input" />
            </div>
          </div>

          <!-- Output (single) — hidden when step failed -->
          <div
            v-if="stepStatus(stepId as string, result) !== 'failed' && (stepVolumes[stepId as string]?.output || result.output) && !(stepOutputHistory[stepId as string]?.length > 1)"
            class="ml-[18px] mt-2"
          >
            <span class="text-[11px] font-mono text-g-9 uppercase tracking-wider">{{ t('execution.output') }}</span>
            <div class="mt-1 bg-g-3 rounded px-3 py-2 overflow-x-auto max-h-[400px] overflow-y-auto">
              <JsonView :data="stepVolumes[stepId as string]?.output ?? result.output" />
            </div>
          </div>

          <!-- Output per-iteration history (multi-iteration) -->
          <div v-if="stepOutputHistory[stepId as string]?.length > 1" class="ml-[18px] mt-2">
            <span class="text-[11px] font-mono text-g-9 uppercase tracking-wider">{{ t('execution.output') }}</span>
            <div
              v-for="(entry, idx) in stepOutputHistory[stepId as string]"
              :key="idx"
              class="mb-1 mt-1"
            >
              <button
                @click="toggleIteration(`${stepId}-out-${idx}`)"
                class="text-[11px] font-mono text-g-9 hover:text-g-13 transition-colors flex items-center gap-2"
              >
                <span>{{ isIterationExpanded(`${stepId}-out-${idx}`) ? '▾' : '▸' }}</span>
                <span class="text-[10px] font-semibold rounded-full px-1.5 leading-[18px] bg-amber-400/15 text-amber-400 border border-amber-400/25">
                  x{{ entry.iteration }}
                </span>
                <span class="text-[10px] text-g-7">{{ formatIterationTime(entry.timestamp) }}</span>
              </button>
              <div
                v-if="isIterationExpanded(`${stepId}-out-${idx}`)"
                class="mt-1 bg-g-3 rounded px-3 py-2 overflow-x-auto"
              >
                <JsonView :data="entry.output" />
              </div>
            </div>
          </div>

          <!-- Pipeline progress -->
          <div v-if="pipelineGrouped(stepId as string).length > 0" class="ml-[18px] mt-2">
            <span class="text-[11px] font-mono text-amber-400 font-medium uppercase tracking-wider">{{ t('execution.pipeline') }}</span>
            <div
              v-for="group in pipelineGrouped(stepId as string)"
              :key="group.iteration"
              class="mt-1.5"
            >
              <button
                @click="toggleIteration(`${stepId}-pipe-${group.iteration}`)"
                class="text-[11px] font-mono text-g-9 hover:text-g-13 transition-colors flex items-center gap-2"
              >
                <span>{{ isPipelineIterExpanded(stepId as string, group.iteration) ? '▾' : '▸' }} iter {{ group.iteration }}/{{ group.totalIterations }}</span>
                <span class="text-[10px] text-g-7">&mdash; {{ group.steps.length }}/{{ group.totalSteps }} actions</span>
              </button>
              <div
                v-if="isPipelineIterExpanded(stepId as string, group.iteration)"
                class="ml-3 mt-0.5"
              >
                <div
                  v-for="(ps, psIdx) in group.steps"
                  :key="psIdx"
                  class="flex items-center gap-2 text-[11px] font-mono text-g-11 py-0.5"
                >
                  <span :class="['w-[6px] h-[6px] rounded-full inline-block shrink-0', pipelineDot(ps.status)]" />
                  <span class="text-g-12">{{ ps.action }}</span>
                  <span v-if="ps.durationMs !== undefined" class="text-g-7">{{ ps.durationMs }}ms</span>
                </div>
              </div>
            </div>
          </div>

          <div v-if="result.error" class="ml-[18px] mt-2">
            <span class="text-[11px] font-mono text-red-400 uppercase tracking-wider">{{ t('execution.error') }}</span>
            <div class="mt-1 bg-red-400/5 border border-red-400/20 rounded px-3 py-2 overflow-x-auto max-h-[400px] overflow-y-auto">
              <JsonView :data="typeof result.error === 'object' ? result.error.message : result.error" />
            </div>
          </div>
          </template>
          </template>
        </div>
      </div>
    </div>

    <!-- Event Log -->
    <EventTimeline :events="eventRows" />
  </div>
</template>
