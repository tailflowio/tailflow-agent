<script setup lang="ts">
import { ref, nextTick, onMounted, computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useWorkflowApi, type Graph, type Execution } from '@/composables/useWorkflowApi'

const { t } = useI18n()
import { useSSE } from '@/composables/useSSE'
import WorkflowGraph from '@/components/WorkflowGraph.vue'
import ExecutionLog from '@/components/ExecutionLog.vue'
import JsonView from '@/components/JsonView.vue'

const route = useRoute()
const router = useRouter()
const api = useWorkflowApi()

const executionId = route.params.id as string
const execution = ref<Execution | null>(null)
const graph = ref<Graph | null>(null)

const { events, connected, finished, stepStatuses, stepVolumes, stepIterations, stepOutputHistory, stepPipelineProgress, connect } = useSSE(executionId)

onMounted(async () => {
  try {
    execution.value = await api.getExecution(executionId) as Execution
    graph.value = await api.getWorkflowGraph()
    if (execution.value?.steps) {
      for (const [id, step] of Object.entries(execution.value.steps)) {
        // Restore step statuses (including running/waiting for live executions)
        if (step.status) stepStatuses.value[id] = step.status === 'success' ? 'success'
          : step.status === 'failed' ? 'failed'
          : step.status === 'skipped' ? 'skipped'
          : step.status === 'running' ? 'running'
          : step.status === 'waiting' ? 'waiting'
          : step.status
        // Restore volumes (input/output) from persisted step data
        if (step.input || step.output) {
          if (!stepVolumes.value[id]) stepVolumes.value[id] = {}
          if (step.input) stepVolumes.value[id].input = step.input
          if (step.output) stepVolumes.value[id].output = step.output
        }
      }
    }
  } catch {}

  // Always connect SSE: replays stored events for finished executions,
  // streams live events for running ones
  connect()
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
  if (!execution.value?.steps) return []
  return Object.entries(execution.value.steps)
    .sort(([, a], [, b]) => {
      const ta = a.started_at ? new Date(a.started_at).getTime() : Infinity
      const tb = b.started_at ? new Date(b.started_at).getTime() : Infinity
      return ta - tb
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
  if (s === 'cancelled') return 'bg-g-5 text-g-9'
  if (s === 'running') return 'bg-amber-400/15 text-amber-400'
  if (s === 'waiting') return 'bg-amber-400/15 text-amber-400'
  return 'bg-g-5 text-g-9'
}
function statusDot(s: string) {
  if (s === 'success') return 'bg-emerald-400'
  if (s === 'failed') return 'bg-red-400'
  if (s === 'cancelled') return 'bg-g-9'
  if (s === 'running') return 'bg-g-12 animate-pulse'
  if (s === 'waiting') return 'bg-amber-400 animate-pulse'
  return 'bg-g-8'
}

const dagHeight = computed(() => {
  const nodes = graph.value?.nodes?.length ?? 0
  return Math.min(700, Math.max(300, 200 + nodes * 70))
})

const expandedSteps = ref<Record<string, boolean>>({})
const expandedIterations = ref<Record<string, boolean>>({})
const focusedStepId = ref<string | null>(null)

function isStepExpanded(stepId: string): boolean {
  return expandedSteps.value[stepId] ?? true
}

function toggleStep(stepId: string) {
  expandedSteps.value[stepId] = !isStepExpanded(stepId)
}

function onDagStepClick(stepId: string) {
  // Expand + scroll to the step row and flash a white border
  expandedSteps.value[stepId] = true
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


</script>

<template>
  <div>
    <!-- Header -->
    <div class="flex items-start justify-between mb-6">
      <div>
        <div class="flex items-center gap-3 mb-2">
          <button @click="router.push('/')" class="text-g-8 hover:text-g-13 transition-colors">
            <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M15 19l-7-7 7-7" />
            </svg>
          </button>
          <h1 class="text-lg font-semibold text-g-14 tracking-tight">
            {{ execution?.workflow_name || 'Execution' }}
          </h1>
          <span class="text-[13px] font-mono text-g-7">{{ executionId.slice(0, 8) }}</span>
        </div>
        <!-- Cancel button -->
        <button
          v-if="canCancel"
          @click="cancelExec"
          :disabled="cancelling"
          class="ml-7 mb-1 px-3 py-1 text-[12px] font-medium rounded-md border border-red-400/30 text-red-400 hover:bg-red-400/10 transition-colors disabled:opacity-50"
        >
          {{ cancelling ? t('execution.cancelling') : t('execution.cancel') }}
        </button>
        <div class="flex items-center gap-3 ml-7">
          <span class="flex items-center gap-1.5">
            <span :class="['w-[6px] h-[6px] rounded-full', statusDot(statusLabel)]" />
            <span :class="['text-[12px] font-mono px-2 py-0.5 rounded', statusBadge(statusLabel)]">{{ statusLabel }}</span>
          </span>
          <span class="text-[12px] text-g-8 font-mono">{{ duration }}</span>
          <span v-if="connected" class="flex items-center gap-1.5 text-[11px] text-g-10">
            <span class="w-1.5 h-1.5 bg-emerald-400 rounded-full animate-pulse" />
            {{ t('execution.live') }}
          </span>
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
    <div class="bg-g-2 border border-g-5 rounded-lg mb-6 overflow-hidden lm-card" :style="{ height: dagHeight + 'px' }">
      <WorkflowGraph
        v-if="graph"
        :graph="graph"
        :step-statuses="stepStatuses"
        :step-iterations="stepIterations"
        @nodeClick="onDagStepClick"
      />
      <div v-else class="flex items-center justify-center h-full">
        <div class="w-5 h-5 border-2 border-g-7 border-t-g-12 rounded-full animate-spin" />
      </div>
    </div>

    <!-- Step results with volume inspector -->
    <div v-if="execution?.steps && Object.keys(execution.steps).length > 0" class="bg-g-2 border border-g-5 rounded-lg mb-6 lm-card">
      <div class="px-4 py-2.5 border-b border-g-5 flex items-center justify-between sticky top-0 z-10 bg-g-2 rounded-t-lg lm-sticky">
        <span class="text-sm font-medium text-g-12 mb-0">{{ t('execution.results') }}</span>
        <button
          @click="collapseAll"
          class="text-[11px] font-mono text-g-8 hover:text-g-12 transition-colors"
        >{{ t('execution.collapseAll') }}</button>
      </div>
      <div>
        <div
          v-for="[stepId, result] in orderedSteps"
          :key="stepId"
          :id="'step-' + stepId"
          :class="[
            'px-4 py-3 transition-all duration-700 border-b border-g-5 last:border-b-0',
            focusedStepId === stepId
              ? 'ring-1 ring-white/60 bg-g-3'
              : ''
          ]"
        >
          <button
            @click="toggleStep(stepId as string)"
            class="flex items-center gap-2.5 mb-1 w-full text-left cursor-pointer group"
          >
            <span class="text-[10px] text-g-7 group-hover:text-g-11 transition-colors">{{ isStepExpanded(stepId as string) ? '▾' : '▸' }}</span>
            <span :class="['w-[6px] h-[6px] rounded-full', statusDot(result.status)]" />
            <span class="text-[14px] font-mono text-g-12">{{ stepId }}</span>
            <span :class="['text-[11px] font-mono px-2 py-0.5 rounded', statusBadge(result.status)]">{{ result.status }}</span>
            <span
              v-if="stepIterations[stepId as string] && stepIterations[stepId as string] > 1"
              class="text-[10px] font-mono font-semibold rounded-full px-1.5 leading-[18px] bg-amber-400/15 text-amber-400 border border-amber-400/25"
            >
              x{{ stepIterations[stepId as string] }}
            </span>
          </button>

          <template v-if="isStepExpanded(stepId as string)">
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
            v-if="result.status !== 'failed' && (stepVolumes[stepId as string]?.output || result.output) && !(stepOutputHistory[stepId as string]?.length > 1)"
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
              <JsonView :data="result.error?.message || result.error" />
            </div>
          </div>
          </template>
        </div>
      </div>
    </div>

    <!-- Event Log -->
    <div class="bg-g-2 border border-g-5 rounded-lg lm-card">
      <div class="px-4 py-2.5 border-b border-g-5 flex items-center justify-between sticky top-0 z-10 bg-g-2 rounded-t-lg lm-sticky">
        <span class="text-sm font-medium text-g-12">{{ t('execution.events') }}</span>
        <span class="text-[11px] text-g-7 font-mono">{{ events.length }} events</span>
      </div>
      <ExecutionLog :events="events" />
    </div>
  </div>
</template>
