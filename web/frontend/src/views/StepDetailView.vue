<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useWorkflowApi, type StepDetail } from '@/composables/useWorkflowApi'

const { t } = useI18n()

const route = useRoute()
const router = useRouter()
const api = useWorkflowApi()

const detail = ref<StepDetail | null>(null)
let pollTimer: ReturnType<typeof setInterval> | null = null

async function load() {
  const id = route.params.id as string
  try {
    detail.value = await api.getStepDetail(id)
  } catch {}
}

onMounted(() => {
  load()
  pollTimer = setInterval(load, 5000)
})

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer)
})

const step = computed(() => detail.value?.step)
const history = computed(() => detail.value?.history || [])
const metrics = computed(() => detail.value?.metrics)

const successRate = computed(() => {
  if (!metrics.value || metrics.value.total_executions === 0) return '-'
  return ((metrics.value.success_count / metrics.value.total_executions) * 100).toFixed(1) + '%'
})

const activeExecs = computed(() => history.value.filter(h => h.status === 'running' || h.status === 'waiting'))
const pastExecs = computed(() => history.value.filter(h => h.status !== 'running' && h.status !== 'waiting'))

function dot(s: string) {
  if (s === 'success') return 'bg-emerald-400'
  if (s === 'failed') return 'bg-red-400'
  if (s === 'cancelled') return 'bg-orange-400'
  if (s === 'running') return 'bg-g-12 animate-pulse'
  if (s === 'waiting') return 'bg-amber-400 animate-pulse'
  if (s === 'pending') return 'bg-violet-400'
  if (s === 'skipped') return 'bg-g-6'
  return 'bg-g-5'
}

function badge(s: string) {
  if (s === 'success') return 'bg-emerald-400/15 text-emerald-400'
  if (s === 'failed') return 'bg-red-400/15 text-red-400'
  if (s === 'cancelled') return 'bg-orange-400/15 text-orange-400'
  if (s === 'running') return 'bg-amber-400/15 text-amber-400'
  if (s === 'waiting') return 'bg-amber-400/15 text-amber-400'
  if (s === 'pending') return 'bg-violet-400/15 text-violet-400'
  return 'bg-g-5 text-g-9'
}

function ago(d: string) {
  if (!d) return ''
  const s = Math.floor((Date.now() - new Date(d).getTime()) / 1000)
  if (s < 5) return 'now'
  if (s < 60) return s + 's ago'
  if (s < 3600) return Math.floor(s / 60) + 'm ago'
  if (s < 86400) return Math.floor(s / 3600) + 'h ago'
  return Math.floor(s / 86400) + 'd ago'
}

function formatMs(ms: number) {
  if (ms < 1000) return ms + 'ms'
  return (ms / 1000).toFixed(1) + 's'
}

function esc(s: string) {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

function yamlHtml(obj: unknown, indent = 0): string {
  const pad = '  '.repeat(indent)
  if (obj === null || obj === undefined) return '<span class="yaml-null">null</span>'
  if (typeof obj === 'boolean') return `<span class="yaml-bool">${obj}</span>`
  if (typeof obj === 'number') return `<span class="yaml-num">${obj}</span>`
  if (typeof obj === 'string') return `<span class="yaml-str">${esc(obj)}</span>`
  if (Array.isArray(obj)) {
    if (obj.length === 0) return '<span class="yaml-null">[]</span>'
    return obj.map(item => {
      if (typeof item === 'object' && item !== null) {
        const inner = yamlHtml(item, indent + 1).trimStart()
        return `${pad}<span class="yaml-dash">-</span> ${inner}`
      }
      return `${pad}<span class="yaml-dash">-</span> ${yamlHtml(item, indent)}`
    }).join('\n')
  }
  if (typeof obj === 'object') {
    const entries = Object.entries(obj as Record<string, unknown>)
    if (entries.length === 0) return '<span class="yaml-null">{}</span>'
    return entries.map(([key, val]) => {
      if (val !== null && typeof val === 'object') {
        return `${pad}<span class="yaml-key">${esc(key)}</span><span class="yaml-colon">:</span>\n${yamlHtml(val, indent + 1)}`
      }
      return `${pad}<span class="yaml-key">${esc(key)}</span><span class="yaml-colon">:</span> ${yamlHtml(val, indent)}`
    }).join('\n')
  }
  return esc(String(obj))
}

const configHtml = computed(() => step.value?.config ? yamlHtml(step.value.config) : '')
</script>

<template>
  <div v-if="detail && step">
    <!-- Header -->
    <div class="flex items-start justify-between mb-6">
      <div class="flex items-center gap-3">
        <button
          @click="router.push({ name: 'workflow' })"
          class="text-g-8 hover:text-g-13 transition-colors"
        >
          <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M15 19l-7-7 7-7" />
          </svg>
        </button>
        <div>
          <h1 class="text-lg font-semibold text-g-14 tracking-tight">{{ step.title || step.id }}</h1>
          <div class="flex items-center gap-2 mt-1">
            <span class="text-[13px] font-mono text-g-9">{{ step.id }}</span>
            <span class="text-[11px] font-mono font-medium text-g-9 bg-g-5 px-2 py-0.5 rounded">
              {{ step.action }}
            </span>
          </div>
        </div>
      </div>
    </div>

    <!-- Main content grid -->
    <div class="grid grid-cols-1 lg:grid-cols-3 gap-6 mb-6">
      <!-- Config panel -->
      <div class="lg:col-span-2 bg-g-2 border border-g-5 rounded-lg p-5 lm-card">
        <h2 class="text-sm font-medium text-g-12 mb-3">{{ t('stepDetail.config') }}</h2>

        <!-- Config YAML -->
        <div v-if="step.config" class="mb-5">
          <pre class="bg-g-3 rounded-md p-4 overflow-x-auto border border-g-5 text-[13px] font-mono leading-[1.7] whitespace-pre-wrap yaml-block" v-html="configHtml" />
        </div>

        <!-- Properties grid -->
        <div class="grid grid-cols-[auto_1fr] gap-x-5 gap-y-2.5 items-center text-[13px]">
          <template v-if="(step.depends_on as string[])?.length">
            <span class="text-g-8 text-[12px]">{{ t('stepDetail.dependencies') }}</span>
            <div class="flex flex-wrap gap-1.5">
              <span
                v-for="dep in (step.depends_on as string[])"
                :key="dep"
                @click="router.push({ name: 'step-detail', params: { id: dep } })"
                class="font-mono text-[12px] text-g-12 bg-g-4 border border-g-5 px-2.5 py-1 rounded-md cursor-pointer hover:bg-g-5 hover:border-g-7 transition-colors"
              >
                {{ dep }}
              </span>
            </div>
          </template>

          <template v-if="step.on_recovery && step.on_recovery !== 'retry'">
            <span class="text-g-8 text-[12px]">{{ t('stepDetail.onRecovery') }}</span>
            <span class="font-mono text-[12px] px-2.5 py-1 rounded-md w-fit"
              :class="step.on_recovery === 'skip' ? 'text-amber-400 bg-amber-400/10 border border-amber-400/20' : 'text-red-400 bg-red-400/10 border border-red-400/20'"
            >
              {{ step.on_recovery }}
            </span>
          </template>

          <template v-if="step.error_policy">
            <span class="text-g-8 text-[12px]">{{ t('stepDetail.errorPolicy') }}</span>
            <span class="font-mono text-[12px] px-2.5 py-1 rounded-md w-fit"
              :class="step.error_policy === 'continue' ? 'text-amber-400 bg-amber-400/10 border border-amber-400/20' : step.error_policy === 'ignore' ? 'text-g-10 bg-g-4 border border-g-5' : 'text-red-400 bg-red-400/10 border border-red-400/20'"
            >
              {{ step.error_policy }}
            </span>
          </template>

          <template v-if="step.retry">
            <span class="text-g-8 text-[12px]">{{ t('stepDetail.retry') }}</span>
            <div class="flex items-center gap-2">
              <span class="font-mono text-[12px] text-g-12 bg-g-4 border border-g-5 px-2.5 py-1 rounded-md">
                {{ (step.retry as any).max_attempts }}x
              </span>
              <span v-if="(step.retry as any).delay" class="font-mono text-[12px] text-g-10 bg-g-4 border border-g-5 px-2.5 py-1 rounded-md">
                {{ (step.retry as any).delay }} {{ t('stepDetail.delay') }}
              </span>
            </div>
          </template>

          <template v-if="step.timeout">
            <span class="text-g-8 text-[12px]">{{ t('stepDetail.timeout') }}</span>
            <span class="font-mono text-[12px] text-g-12 bg-g-4 border border-g-5 px-2.5 py-1 rounded-md w-fit">
              {{ step.timeout }}
            </span>
          </template>
        </div>
      </div>

      <!-- Metrics panel -->
      <div class="bg-g-2 border border-g-5 rounded-lg p-5 lm-card">
        <h2 class="text-sm font-medium text-g-12 mb-3">{{ t('stepDetail.metrics') }}</h2>

        <div v-if="metrics" class="space-y-4">
          <div>
            <p class="text-xs text-g-9 mb-1">{{ t('stepDetail.totalExecs') }}</p>
            <p class="text-2xl font-semibold text-g-14 font-mono tabular-nums">{{ metrics.total_executions }}</p>
          </div>
          <div>
            <p class="text-xs text-g-9 mb-1">{{ t('stepDetail.successRate') }}</p>
            <p class="text-2xl font-semibold text-emerald-400 font-mono tabular-nums">{{ successRate }}</p>
          </div>
          <div>
            <p class="text-xs text-g-9 mb-1">{{ t('stepDetail.avgDuration') }}</p>
            <p class="text-2xl font-semibold text-g-14 font-mono tabular-nums">
              {{ metrics.avg_duration_ms ? formatMs(metrics.avg_duration_ms) : '-' }}
            </p>
          </div>
          <div class="flex items-center gap-4 text-[14px] font-mono tabular-nums">
            <span class="text-emerald-400">{{ metrics.success_count }} ok</span>
            <span class="text-red-400">{{ metrics.failure_count }} err</span>
          </div>
          <div v-if="metrics.last_execution" class="text-[12px] text-g-7 font-mono">
            {{ t('stepDetail.last') }} {{ ago(metrics.last_execution) }}
          </div>
        </div>
        <div v-else class="text-sm text-g-8">{{ t('stepDetail.noData') }}</div>
      </div>
    </div>

    <!-- Active executions -->
    <div v-if="activeExecs.length > 0" class="bg-g-2 border border-g-5 rounded-lg overflow-hidden mb-6 lm-card">
      <div class="px-4 py-2.5 border-b border-g-5">
        <span class="text-sm font-medium text-g-12">
          {{ t('stepDetail.activeExecs', { count: activeExecs.length }) }}
        </span>
      </div>
      <div>
        <div
          v-for="h in activeExecs"
          :key="h.execution_id"
          @click="router.push({ name: 'execution', params: { id: h.execution_id } })"
          class="flex items-center gap-3 px-4 py-3 cursor-pointer hover:bg-g-3 transition-colors border-b border-g-5 last:border-b-0"
        >
          <span :class="['w-[7px] h-[7px] rounded-full flex-shrink-0', dot(h.status)]" />
          <span :class="['text-[12px] font-mono font-medium px-2 py-0.5 rounded', badge(h.status)]">{{ h.status }}</span>
          <span class="flex-1" />
          <span v-if="h.started_at" class="text-[12px] text-g-8 font-mono tabular-nums">
            {{ t('stepDetail.since') }} {{ ago(h.started_at) }}
          </span>
          <span class="text-[12px] text-g-7 font-mono">{{ h.execution_id.slice(0, 8) }}</span>
        </div>
      </div>
    </div>

    <!-- History -->
    <div class="bg-g-2 border border-g-5 rounded-lg overflow-hidden lm-card">
      <div class="px-4 py-2.5 border-b border-g-5">
        <span class="text-sm font-medium text-g-12">
          {{ t('stepDetail.history') }}
        </span>
      </div>
      <div v-if="pastExecs.length === 0" class="py-10 text-center text-g-8 text-sm">
        {{ t('stepDetail.noHistory') }}
      </div>
      <div v-else>
        <div
          v-for="h in pastExecs"
          :key="h.execution_id"
          @click="router.push({ name: 'execution', params: { id: h.execution_id } })"
          class="flex items-center gap-3 px-4 py-3 cursor-pointer hover:bg-g-3 transition-colors border-b border-g-5 last:border-b-0"
        >
          <span :class="['w-[7px] h-[7px] rounded-full flex-shrink-0', dot(h.status)]" />
          <span :class="['text-[12px] font-mono font-medium w-[70px] px-2 py-0.5 rounded', badge(h.status)]">{{ h.status }}</span>
          <span class="text-[13px] text-g-10 font-mono tabular-nums w-[80px]">
            {{ h.duration_ms ? formatMs(h.duration_ms) : '-' }}
          </span>
          <span v-if="h.error" class="text-[13px] text-red-400 truncate max-w-[300px]">{{ h.error }}</span>
          <span class="flex-1" />
          <span class="text-[12px] text-g-8 font-mono tabular-nums">{{ ago(h.started_at) }}</span>
          <span class="text-[12px] text-g-7 font-mono">{{ h.execution_id.slice(0, 8) }}</span>
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
