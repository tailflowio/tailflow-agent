<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useWorkflowApi, type Graph } from '@/composables/useWorkflowApi'
import { useGlobalEvents } from '@/composables/useGlobalEvents'
import { useRunTrigger } from '@/composables/useRunTrigger'
import WorkflowGraph from '@/components/WorkflowGraph.vue'

const { t } = useI18n()
const router = useRouter()
const api = useWorkflowApi()
const { workflowReady, workflowName, workflowParams } = useRunTrigger()


const workflow = ref<Record<string, unknown> | null>(null)
const graph = ref<Graph | null>(null)

const viewMode = ref<'graph' | 'list'>(
  (localStorage.getItem('workflowViewMode') as 'graph' | 'list') || 'graph'
)

watch(viewMode, (v) => localStorage.setItem('workflowViewMode', v))

async function load() {
  const [wf, gr] = await Promise.all([
    api.getWorkflow().catch(() => null),
    api.getWorkflowGraph().catch(() => null),
  ])
  if (wf) workflow.value = wf
  if (gr) graph.value = gr
  if (wf) {
    workflowReady.value = true
    workflowName.value = (wf as any).name || ''
    workflowParams.value = (wf as any).params || []
  }
}

onMounted(load)

const { stepCounts, recentStatuses, stepExecCounts } = useGlobalEvents(() => {})

// Auto-height: compute from dagre node positions
const dagAutoHeight = ref(400)

function onGraphReady(payload: { maxY: number }) {
  dagAutoHeight.value = Math.max(300, payload.maxY + 120)
}

const steps = () => (workflow.value?.steps as any[]) || []

function onStepClick(stepId: string) {
  router.push({ name: 'step-detail', params: { id: stepId } })
}
</script>

<template>
  <div v-if="workflow">
    <div class="mb-5">
      <h2 class="text-lg font-semibold text-g-14">{{ workflow.name }}</h2>
      <p class="text-sm text-g-8 mt-0.5">{{ t('steps.count', { count: steps().length, name: workflow.name }) }}</p>
    </div>

    <!-- View toggle -->
    <div class="flex items-center gap-1 mb-4">
      <button
        @click="viewMode = 'graph'"
        :class="[
          'px-3 py-1.5 text-[12px] font-medium rounded-md transition-colors',
          viewMode === 'graph'
            ? 'bg-g-5 text-g-14'
            : 'text-g-9 hover:text-g-12 hover:bg-g-3'
        ]"
      >
        {{ t('workflow.graph') }}
      </button>
      <button
        @click="viewMode = 'list'"
        :class="[
          'px-3 py-1.5 text-[12px] font-medium rounded-md transition-colors',
          viewMode === 'list'
            ? 'bg-g-5 text-g-14'
            : 'text-g-9 hover:text-g-12 hover:bg-g-3'
        ]"
      >
        {{ t('workflow.list') }}
      </button>
    </div>

    <!-- Graph mode -->
    <div v-if="viewMode === 'graph'" class="bg-g-2 border border-g-5 rounded-lg overflow-hidden lm-card" :style="{ minHeight: '300px', height: dagAutoHeight + 'px' }">
      <WorkflowGraph
        v-if="graph"
        :graph="graph"
        :step-counts="stepCounts"
        :recent-statuses="recentStatuses"
        :step-exec-counts="stepExecCounts"
        @node-click="onStepClick"
        @layout-ready="onGraphReady"
      />
    </div>

    <!-- List mode -->
    <div v-else>
      <div class="bg-g-2 border border-g-5 rounded-lg overflow-hidden lm-card">
        <div
          v-for="step in steps()"
          :key="step.id"
          @click="onStepClick(step.id)"
          class="flex items-center gap-4 px-5 py-4 cursor-pointer hover:bg-g-3 transition-colors border-b border-g-5 last:border-b-0"
        >
          <div class="flex-1 min-w-0">
            <div class="flex items-center gap-2.5 mb-1">
              <span class="font-mono text-[14px] font-medium text-g-13">{{ step.id }}</span>
              <span class="text-[11px] font-mono font-medium text-g-9 bg-g-5 px-2 py-0.5 rounded">
                {{ step.action }}
              </span>
            </div>
            <div v-if="step.title" class="text-[13px] text-g-9 truncate">{{ step.title }}</div>
          </div>
          <div v-if="step.depends_on?.length" class="flex items-center gap-1.5 flex-shrink-0">
            <span class="text-[11px] text-g-8 mr-1">{{ t('steps.depends') }}</span>
            <span
              v-for="dep in step.depends_on"
              :key="dep"
              class="text-[11px] font-mono text-g-10 bg-g-4 border border-g-5 px-2 py-0.5 rounded"
            >
              {{ dep }}
            </span>
          </div>
          <svg class="w-4 h-4 text-g-7 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
          </svg>
        </div>
      </div>
    </div>
  </div>

  <div v-else-if="api.loading.value" class="flex flex-col items-center justify-center py-32">
    <div class="w-6 h-6 border-2 border-g-5 border-t-g-9 rounded-full animate-spin" />
    <span class="mt-3 text-sm text-g-7">{{ t('executions.loading') }}</span>
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
