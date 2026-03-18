<script setup lang="ts">
import { ref, computed } from 'vue'
import type { StepVolume, StepOutputEntry, PipelineEntry, WorkflowEvent } from '@/composables/useSSE'
import { JsonView } from '@tailflow/shared'

const props = defineProps<{
  steps: [string, any][]
  stepStatuses: Record<string, string>
  stepVolumes: Record<string, StepVolume>
  stepIterations: Record<string, number>
  stepOutputHistory: Record<string, StepOutputEntry[]>
  stepPipelineProgress: Record<string, PipelineEntry[]>
  events: WorkflowEvent[]
}>()

const expandedStep = ref<string | null>(null)
const deferredStep = ref<string | null>(null)
const expandedIterations = ref<Record<string, boolean>>({})

function toggleStep(stepId: string) {
  if (expandedStep.value === stepId) {
    deferredStep.value = null
    requestAnimationFrame(() => {
      requestAnimationFrame(() => { expandedStep.value = null })
    })

    return
  }

  expandedStep.value = stepId
  deferredStep.value = null
  requestAnimationFrame(() => {
    requestAnimationFrame(() => { deferredStep.value = stepId })
  })
}

function toggleIteration(key: string) {
  expandedIterations.value[key] = !expandedIterations.value[key]
}

function isIterationExpanded(key: string): boolean {
  return expandedIterations.value[key] ?? false
}

function isPipelineIterExpanded(stepId: string, iteration: number): boolean {
  const key = `${stepId}-pipe-${iteration}`
  return expandedIterations.value[key] ?? true
}

function status(stepId: string, result: any): string {
  return props.stepStatuses[stepId] ?? result?.status ?? 'pending'
}

function dotColor(s: string): string {
  if (s === 'success') return 'bg-emerald-400'
  if (s === 'failed') return 'bg-red-400'
  if (s === 'running' || s === 'waiting') return 'bg-amber-400'
  if (s === 'cancelled') return 'bg-orange-400'
  if (s === 'skipped') return 'bg-gray-500'
  return 'bg-g-6'
}

function isActive(s: string): boolean {
  return s === 'running' || s === 'waiting'
}

function rowAccent(s: string): string {
  if (s === 'running' || s === 'waiting') return 'border-l-2 border-l-amber-400 bg-amber-400/5'
  if (s === 'failed') return 'border-l-2 border-l-red-400 bg-red-400/5'
  return 'border-l-2 border-l-transparent'
}

function formatDuration(result: any): string {
  if (!result?.started_at) return ''
  const start = new Date(result.started_at).getTime()
  const end = result.finished_at ? new Date(result.finished_at).getTime() : Date.now()
  const ms = end - start
  if (ms < 1000) return ms + 'ms'
  if (ms < 60000) return (ms / 1000).toFixed(1) + 's'
  const minutes = Math.floor(ms / 60000)
  const seconds = Math.floor((ms % 60000) / 1000)
  return minutes + 'm' + (seconds > 0 ? seconds + 's' : '')
}

function formatIterationTime(timestamp: string): string {
  const d = new Date(timestamp)
  return d.toLocaleTimeString('fr-FR', { hour12: false, fractionalSecondDigits: 3 } as Intl.DateTimeFormatOptions)
}

function pipelineGrouped(stepId: string) {
  const entries = props.stepPipelineProgress[stepId]
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

function pipelineDot(s: string): string {
  if (s === 'ok') return 'bg-emerald-400'
  if (s === 'failed') return 'bg-red-400'
  return 'bg-g-12 animate-pulse'
}

const stepLogs = computed(() => {
  const map: Record<string, string[]> = {}
  for (const ev of props.events) {
    if (ev.type !== 'step.log' || !ev.step_id) continue
    if (!map[ev.step_id]) map[ev.step_id] = []
    if (ev.message) map[ev.step_id].push(ev.message)
  }
  return map
})
</script>

<template>
  <div class="bg-g-2 border border-g-5 rounded-lg overflow-hidden">
    <template v-for="[stepId, result] in steps" :key="stepId">
      <div
        :class="[
          'flex items-center gap-2.5 px-4 py-2.5 border-b border-g-5 last:border-b-0 cursor-pointer transition-colors hover:bg-g-3/50',
          rowAccent(status(stepId, result)),
          expandedStep === stepId ? 'bg-g-3/30' : ''
        ]"
        @click="toggleStep(stepId)"
      >
        <span
          :class="['w-1.5 h-1.5 rounded-full shrink-0', dotColor(status(stepId, result))]"
          :style="isActive(status(stepId, result)) ? 'box-shadow: 0 0 8px currentColor' : ''"
        />
        <span
          class="font-mono text-[13px] flex-1 truncate"
          :class="status(stepId, result) === 'pending' ? 'text-g-7' : isActive(status(stepId, result)) ? 'text-g-14 font-medium' : 'text-g-11'"
        >
          {{ stepId }}
        </span>
        <span
          v-if="stepIterations[stepId] && stepIterations[stepId] > 1"
          class="text-[10px] font-mono font-semibold rounded-full px-1.5 leading-[18px] bg-amber-400/15 text-amber-400 border border-amber-400/25"
        >
          x{{ stepIterations[stepId] }}
        </span>
        <span class="text-[12px] font-mono text-g-7 tabular-nums shrink-0">
          {{ formatDuration(result) }}
        </span>
        <span v-if="expandedStep === stepId" class="text-g-7 text-[10px]">&#9660;</span>
        <span v-else-if="status(stepId, result) !== 'pending'" class="text-g-7 text-[10px]">&#9654;</span>
      </div>

      <div v-if="expandedStep === stepId" class="border-b border-g-5 bg-g-3/20">
        <div v-if="deferredStep !== stepId" class="flex items-center justify-center gap-2 py-4">
          <div class="w-3.5 h-3.5 border-2 border-g-5 border-t-g-9 rounded-full animate-spin" />
          <span class="text-[12px] text-g-7">Loading...</span>
        </div>
        <div v-else class="px-4 pb-3 pt-1" style="padding-left:32px">

          <!-- Waiting -->
          <div
            v-if="stepVolumes[stepId]?.waiting"
            class="mt-2 inline-flex items-center gap-2 text-[11px] font-mono text-amber-400 bg-amber-400/10 border border-amber-400/20 rounded px-2 py-1"
          >
            <span class="w-1.5 h-1.5 bg-amber-400 rounded-full animate-pulse" />
            {{ stepVolumes[stepId].waiting!.type }}
          </div>

          <!-- Input -->
          <div v-if="stepVolumes[stepId]?.input" class="mt-2">
            <span class="text-[10px] font-mono text-g-7 uppercase tracking-wider">Input</span>
            <div class="mt-1 bg-g-3 rounded px-3 py-2 overflow-x-auto max-h-[300px] overflow-y-auto">
              <JsonView :data="stepVolumes[stepId].input" />
            </div>
          </div>

          <!-- Output (single) -->
          <div
            v-if="status(stepId, result) !== 'failed' && (stepVolumes[stepId]?.output || result.output) && !(stepOutputHistory[stepId]?.length > 1)"
            class="mt-2"
          >
            <span class="text-[10px] font-mono text-g-7 uppercase tracking-wider">Output</span>
            <div class="mt-1 bg-g-3 rounded px-3 py-2 overflow-x-auto max-h-[300px] overflow-y-auto">
              <JsonView :data="stepVolumes[stepId]?.output ?? result.output" />
            </div>
          </div>

          <!-- Output multi-iteration -->
          <div v-if="stepOutputHistory[stepId]?.length > 1" class="mt-2">
            <span class="text-[10px] font-mono text-g-7 uppercase tracking-wider">Output</span>
            <div
              v-for="(entry, idx) in stepOutputHistory[stepId]"
              :key="idx"
              class="mb-1 mt-1"
            >
              <button
                @click.stop="toggleIteration(`${stepId}-out-${idx}`)"
                class="text-[11px] font-mono text-g-9 hover:text-g-13 transition-colors flex items-center gap-2 cursor-pointer"
              >
                <span>{{ isIterationExpanded(`${stepId}-out-${idx}`) ? '&#9660;' : '&#9654;' }}</span>
                <span class="text-[10px] font-semibold rounded-full px-1.5 leading-[18px] bg-amber-400/15 text-amber-400 border border-amber-400/25">
                  x{{ entry.iteration }}
                </span>
                <span class="text-[10px] text-g-7">{{ formatIterationTime(entry.timestamp) }}</span>
              </button>
              <div
                v-if="isIterationExpanded(`${stepId}-out-${idx}`)"
                class="mt-1 bg-g-3 rounded px-3 py-2 overflow-x-auto max-h-[200px] overflow-y-auto"
              >
                <JsonView :data="entry.output" />
              </div>
            </div>
          </div>

          <!-- Error -->
          <div v-if="result.error" class="mt-2">
            <span class="text-[10px] font-mono text-red-400 uppercase tracking-wider">Error</span>
            <div class="mt-1 bg-red-400/5 border border-red-400/20 rounded px-3 py-2 overflow-x-auto max-h-[200px] overflow-y-auto">
              <JsonView :data="typeof result.error === 'object' ? result.error.message ?? result.error : result.error" />
            </div>
          </div>

          <!-- Logs -->
          <div v-if="stepLogs[stepId]?.length > 0" class="mt-2">
            <span class="text-[10px] font-mono text-g-7 uppercase tracking-wider">Logs</span>
            <pre class="mt-1 bg-g-3 rounded px-3 py-2 text-[11px] font-mono text-g-11 whitespace-pre-wrap overflow-x-auto max-h-[200px] overflow-y-auto leading-snug">{{ stepLogs[stepId].join('\n') }}</pre>
          </div>

          <!-- Pipeline progress -->
          <div v-if="pipelineGrouped(stepId).length > 0" class="mt-2">
            <span class="text-[10px] font-mono text-amber-400 font-medium uppercase tracking-wider">Pipeline</span>
            <div
              v-for="group in pipelineGrouped(stepId)"
              :key="group.iteration"
              class="mt-1.5"
            >
              <button
                @click.stop="toggleIteration(`${stepId}-pipe-${group.iteration}`)"
                class="text-[11px] font-mono text-g-9 hover:text-g-13 transition-colors flex items-center gap-2 cursor-pointer"
              >
                <span>{{ isPipelineIterExpanded(stepId, group.iteration) ? '&#9660;' : '&#9654;' }} iter {{ group.iteration }}/{{ group.totalIterations }}</span>
                <span class="text-[10px] text-g-7">&mdash; {{ group.steps.length }}/{{ group.totalSteps }} actions</span>
              </button>
              <div
                v-if="isPipelineIterExpanded(stepId, group.iteration)"
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
        </div>
      </div>
    </template>
  </div>
</template>
