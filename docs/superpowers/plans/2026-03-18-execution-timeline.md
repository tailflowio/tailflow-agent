# Execution Timeline Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the DAG graph + step results list with a single vertical timeline component in ExecutionView.

**Architecture:** New `StepTimeline.vue` component receives all SSE reactive data as props and renders steps as compact rows (expandable on click). ExecutionView keeps the graph fetch for step ordering but removes all DAG/results rendering.

**Tech Stack:** Vue 3 (Composition API, `<script setup>`), TypeScript, Tailwind CSS, `@tailflow/shared` (JsonView, EventTimeline)

**Spec:** `docs/superpowers/specs/2026-03-18-execution-timeline-design.md`

---

## Chunk 1: StepTimeline Component

### Task 1: Create StepTimeline with compact rows

**Files:**
- Create: `web/frontend/src/components/StepTimeline.vue`

- [ ] **Step 1: Create StepTimeline.vue with props and compact row rendering**

```vue
<script setup lang="ts">
import { ref, computed } from 'vue'
import type { StepVolume, StepOutputEntry, PipelineEntry, WorkflowEvent } from '@/composables/useSSE'
import type { StepResult } from '@/composables/useWorkflowApi'
import { JsonView } from '@tailflow/shared'

const props = defineProps<{
  steps: [string, any][]
  stepStatuses: Record<string, string>
  stepVolumes: Record<string, StepVolume>
  stepIterations: Record<string, number>
  stepOutputHistory: Record<string, StepOutputEntry[]>
  stepPipelineProgress: Record<string, PipelineEntry[]>
  events: { type: string; step_id?: string; message?: string; data?: any; timestamp: string }[]
}>()

const expandedStep = ref<string | null>(null)
const deferredStep = ref<string | null>(null)

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

function stepStatus(stepId: string, result: any): string {
  return props.stepStatuses[stepId] ?? result?.status ?? 'pending'
}

function dotColor(status: string): string {
  if (status === 'success') return 'bg-emerald-400'
  if (status === 'failed') return 'bg-red-400'
  if (status === 'running' || status === 'waiting') return 'bg-amber-400'
  if (status === 'cancelled') return 'bg-orange-400'
  if (status === 'skipped') return 'bg-gray-500'
  return 'bg-g-6'
}

function isActive(status: string): boolean {
  return status === 'running' || status === 'waiting'
}

function rowClasses(status: string): string {
  if (status === 'running' || status === 'waiting') return 'border-l-2 border-l-amber-400 bg-amber-400/5'
  if (status === 'failed') return 'border-l-2 border-l-red-400 bg-red-400/5'
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

function stepAction(stepId: string): string {
  const step = props.steps.find(([id]) => id === stepId)
  return step?.[1]?.action ?? ''
}

function stepLogs(stepId: string): string[] {
  return props.events
    .filter(e => e.type === 'step.log' && e.step_id === stepId)
    .map(e => e.message ?? '')
    .filter(Boolean)
}
</script>

<template>
  <div class="bg-g-2 border border-g-5 rounded-lg overflow-hidden">
    <template v-for="[stepId, result] in steps" :key="stepId">
      <!-- Compact row -->
      <div
        :class="[
          'flex items-center gap-2.5 px-4 py-2.5 border-b border-g-5 last:border-b-0 cursor-pointer transition-colors hover:bg-g-3/50',
          rowClasses(stepStatus(stepId, result)),
          expandedStep === stepId ? 'bg-g-3/30' : ''
        ]"
        @click="toggleStep(stepId)"
      >
        <span
          :class="['w-1.5 h-1.5 rounded-full shrink-0', dotColor(stepStatus(stepId, result))]"
          :style="isActive(stepStatus(stepId, result)) ? 'box-shadow: 0 0 8px currentColor' : ''"
        />
        <span
          class="font-mono text-[13px] flex-1 truncate"
          :class="stepStatus(stepId, result) === 'pending' ? 'text-g-7' : isActive(stepStatus(stepId, result)) ? 'text-g-14 font-medium' : 'text-g-11'"
        >
          {{ stepId }}
        </span>
        <span
          v-if="stepIterations[stepId] && stepIterations[stepId] > 1"
          class="text-[10px] font-mono font-semibold rounded-full px-1.5 leading-[18px] bg-amber-400/15 text-amber-400 border border-amber-400/25"
        >
          x{{ stepIterations[stepId] }}
        </span>
        <span
          v-if="stepAction(stepId)"
          class="text-[10px] font-mono px-1.5 py-0.5 rounded bg-g-4 text-g-8"
        >
          {{ stepAction(stepId) }}
        </span>
        <span class="text-[12px] font-mono text-g-7 tabular-nums shrink-0">
          {{ formatDuration(result) }}
        </span>
        <span v-if="expandedStep === stepId" class="text-g-7 text-[10px]">&#9660;</span>
        <span v-else-if="stepStatus(stepId, result) !== 'pending'" class="text-g-7 text-[10px]">&#9654;</span>
      </div>

      <!-- Expanded content -->
      <div v-if="expandedStep === stepId" class="border-b border-g-5 bg-g-3/20">
        <div v-if="deferredStep !== stepId" class="flex items-center justify-center gap-2 py-4">
          <div class="w-3.5 h-3.5 border-2 border-g-5 border-t-g-9 rounded-full animate-spin" />
          <span class="text-[12px] text-g-7">Loading...</span>
        </div>
        <div v-else class="px-4 pb-3 pt-1" style="padding-left:32px">
          <!-- Input -->
          <div v-if="stepVolumes[stepId]?.input" class="mt-2">
            <span class="text-[10px] font-mono text-g-7 uppercase tracking-wider">Input</span>
            <div class="mt-1 bg-g-3 rounded px-3 py-2 overflow-x-auto max-h-[300px] overflow-y-auto">
              <JsonView :data="stepVolumes[stepId].input" />
            </div>
          </div>

          <!-- Output -->
          <div v-if="stepVolumes[stepId]?.output && !(stepOutputHistory[stepId]?.length > 1)" class="mt-2">
            <span class="text-[10px] font-mono text-g-7 uppercase tracking-wider">Output</span>
            <div class="mt-1 bg-g-3 rounded px-3 py-2 overflow-x-auto max-h-[300px] overflow-y-auto">
              <JsonView :data="stepVolumes[stepId].output ?? result.output" />
            </div>
          </div>

          <!-- Output multi-iteration -->
          <div v-if="stepOutputHistory[stepId]?.length > 1" class="mt-2">
            <span class="text-[10px] font-mono text-g-7 uppercase tracking-wider">Output</span>
            <div
              v-for="(entry, idx) in stepOutputHistory[stepId]"
              :key="idx"
              class="mt-1"
            >
              <div class="text-[11px] font-mono text-g-9 flex items-center gap-2">
                <span class="text-[10px] font-semibold rounded-full px-1.5 leading-[18px] bg-amber-400/15 text-amber-400 border border-amber-400/25">
                  x{{ entry.iteration }}
                </span>
                <span class="text-[10px] text-g-7">{{ new Date(entry.timestamp).toLocaleTimeString() }}</span>
              </div>
              <div class="mt-1 bg-g-3 rounded px-3 py-2 overflow-x-auto max-h-[200px] overflow-y-auto">
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
          <div v-if="stepLogs(stepId).length > 0" class="mt-2">
            <span class="text-[10px] font-mono text-g-7 uppercase tracking-wider">Logs</span>
            <pre class="mt-1 bg-g-3 rounded px-3 py-2 text-[11px] font-mono text-g-11 whitespace-pre-wrap overflow-x-auto max-h-[200px] overflow-y-auto leading-snug">{{ stepLogs(stepId).join('\n') }}</pre>
          </div>

          <!-- Pipeline progress -->
          <div v-if="stepPipelineProgress[stepId]?.length > 0" class="mt-2">
            <span class="text-[10px] font-mono text-amber-400 font-medium uppercase tracking-wider">Pipeline</span>
            <div class="mt-1 ml-1">
              <div
                v-for="(ps, psIdx) in stepPipelineProgress[stepId]"
                :key="psIdx"
                class="flex items-center gap-2 text-[11px] font-mono text-g-11 py-0.5"
              >
                <span
                  :class="[
                    'w-[6px] h-[6px] rounded-full inline-block shrink-0',
                    ps.status === 'ok' ? 'bg-emerald-400' : ps.status === 'failed' ? 'bg-red-400' : 'bg-g-12 animate-pulse'
                  ]"
                />
                <span class="text-g-12">{{ ps.action }}</span>
                <span v-if="ps.durationMs !== undefined" class="text-g-7">{{ ps.durationMs }}ms</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>
```

- [ ] **Step 2: Verify it renders in isolation**

Temporarily import in ExecutionView below the existing results section to confirm rendering:
```typescript
import StepTimeline from '@/components/StepTimeline.vue'
```
```html
<StepTimeline
  :steps="orderedSteps"
  :step-statuses="stepStatuses"
  :step-volumes="stepVolumes"
  :step-iterations="stepIterations"
  :step-output-history="stepOutputHistory"
  :step-pipeline-progress="stepPipelineProgress"
  :events="events"
/>
```

Run the dev server and navigate to an execution. Verify the timeline renders alongside the old views.

- [ ] **Step 3: Commit**

```bash
git add web/frontend/src/components/StepTimeline.vue
git commit -m "feat(web): add StepTimeline component"
```

---

## Chunk 2: Rewrite ExecutionView

### Task 2: Strip DAG and results from ExecutionView, wire StepTimeline

**Files:**
- Modify: `web/frontend/src/views/ExecutionView.vue`

- [ ] **Step 1: Remove DAG-related imports and refs**

Remove from `<script setup>`:
- `import WorkflowGraph from '@/components/WorkflowGraph.vue'`
- `const dagAutoHeight = ref(400)`
- `const dagCollapsed = ref(localStorage.getItem(...))`
- `function toggleDag() { ... }`
- `const dagStepCount = computed(...)`
- `function onGraphReady(...) { ... }`
- `function onDagStepClick(...) { ... }`

- [ ] **Step 2: Remove step results-related refs and functions**

Remove from `<script setup>`:
- `const resultsCollapsed = ref(localStorage.getItem(...))`
- `function toggleResults() { ... }`
- `const expandedSteps = ref<Record<string, boolean>>({})`
- `const deferredSteps = ref<Record<string, boolean>>({})`
- `const expandedIterations = ref<Record<string, boolean>>({})`
- `const focusedStepId = ref<string | null>(null)`
- `function isStepExpanded(...) { ... }`
- `function isStepContentReady(...) { ... }`
- `function deferContent(...) { ... }`
- `function toggleStep(...) { ... }`
- `function onDagStepClick(...) { ... }`
- `function collapseAll() { ... }`
- `function toggleIteration(...) { ... }`
- `function isIterationExpanded(...) { ... }`
- `function formatIterationTime(...) { ... }`
- `function isPipelineIterExpanded(...) { ... }`
- `function pipelineGrouped(...) { ... }`
- `function pipelineDot(...) { ... }`

Keep: `graph`, `orderedSteps`, `statusBadge`, `isStatusAnimated`, `stepStatus`, `statusLabel`, `duration`, `eventRows`, all SSE refs, all cancel logic.

- [ ] **Step 3: Remove DAG and results template sections**

Remove from `<template>`:
- The entire DAG collapsible card (`<!-- DAG -->` section with `WorkflowGraph`)
- The entire step results card (`<!-- Step results with volume inspector -->` section)

- [ ] **Step 4: Add StepTimeline import and template usage**

Add import:
```typescript
import StepTimeline from '@/components/StepTimeline.vue'
```

Add in template between the stats grid and the Event Timeline:
```html
<!-- Step Timeline -->
<StepTimeline
  :steps="orderedSteps"
  :step-statuses="stepStatuses"
  :step-volumes="stepVolumes"
  :step-iterations="stepIterations"
  :step-output-history="stepOutputHistory"
  :step-pipeline-progress="stepPipelineProgress"
  :events="events"
/>
```

- [ ] **Step 5: Remove unused imports**

Remove `JsonView` import if no longer used directly in ExecutionView (it's now used inside StepTimeline).

- [ ] **Step 6: Verify the full page works**

Run dev server. Navigate to an execution:
- Verify header with badges renders
- Verify stats row renders
- Verify StepTimeline renders all steps
- Verify clicking a step expands it with input/output/error/logs
- Verify accordion behavior (only one expanded at a time)
- Verify running step has amber highlight
- Verify event timeline still renders at bottom
- Verify SSE live updates work (steps transition in real time)

- [ ] **Step 7: Commit**

```bash
git add web/frontend/src/views/ExecutionView.vue
git commit -m "feat(web): replace DAG and results with StepTimeline in ExecutionView"
```

---

## Chunk 3: Build and verify

### Task 3: Build frontend and verify no regressions

**Files:**
- Modify: `web/frontend/src/views/ExecutionView.vue` (if fixes needed)
- Modify: `web/frontend/src/components/StepTimeline.vue` (if fixes needed)

- [ ] **Step 1: Run TypeScript check**

```bash
cd web/frontend && npx vue-tsc --noEmit 2>&1
```

Fix any type errors.

- [ ] **Step 2: Build production bundle**

```bash
cd web/frontend && npx vite build 2>&1
```

Verify build succeeds.

- [ ] **Step 3: Verify WorkflowView still works**

Navigate to the workflow overview page. Verify the DAG graph still renders correctly — WorkflowGraph.vue is untouched.

- [ ] **Step 4: Verify ExecutionsView still works**

Navigate to the executions list. Verify it renders correctly — ExecutionsView.vue is untouched.

- [ ] **Step 5: Final commit with build output**

```bash
git add web/
git commit -m "build(web): rebuild frontend with timeline view"
```
