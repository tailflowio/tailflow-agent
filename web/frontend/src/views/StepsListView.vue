<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useWorkflowApi } from '@/composables/useWorkflowApi'

const { t } = useI18n()

const router = useRouter()
const api = useWorkflowApi()

const workflow = ref<Record<string, unknown> | null>(null)

onMounted(async () => {
  try {
    workflow.value = await api.getWorkflow()
  } catch {}
})

const steps = () => (workflow.value?.steps as any[]) || []

function onStepClick(id: string) {
  router.push({ name: 'step-detail', params: { id } })
}
</script>

<template>
  <div v-if="workflow">
    <h1 class="text-lg font-semibold text-g-14 mb-1 tracking-tight">{{ t('steps.title') }}</h1>
    <p class="text-sm text-g-10 mb-6">{{ t('steps.count', { count: steps().length, name: workflow.name }) }}</p>

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

  <div v-else-if="api.loading.value" class="flex items-center justify-center py-20">
    <div class="w-5 h-5 border-2 border-g-7 border-t-g-12 rounded-full animate-spin" />
  </div>
  <div v-else-if="api.error.value" class="text-red-400 text-sm">{{ api.error.value }}</div>
</template>
