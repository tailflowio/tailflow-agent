<script setup lang="ts">
import { ref, computed, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import type { WorkflowEvent } from '@/composables/useSSE'

const { t } = useI18n()

const props = defineProps<{
  events: WorkflowEvent[]
}>()

const logContainer = ref<HTMLElement | null>(null)
const search = ref('')
const hiddenTypes = ref<Set<string>>(new Set())

const eventTypes = computed(() => {
  const types = new Set<string>()
  for (const ev of props.events) {
    types.add(ev.type)
  }
  return Array.from(types).sort()
})

function toggleType(type: string) {
  const s = new Set(hiddenTypes.value)
  if (s.has(type)) s.delete(type)
  else s.add(type)
  hiddenTypes.value = s
}

const filteredEvents = computed(() => {
  const q = search.value.toLowerCase()
  return props.events.filter((ev) => {
    if (hiddenTypes.value.has(ev.type)) return false
    if (q && !(
      (ev.message || '').toLowerCase().includes(q) ||
      (ev.step_id || '').toLowerCase().includes(q)
    )) return false
    return true
  })
})

watch(
  () => filteredEvents.value.length,
  async () => {
    await nextTick()
    if (logContainer.value) {
      logContainer.value.scrollTop = logContainer.value.scrollHeight
    }
  }
)

function logLevelColor(event: WorkflowEvent): string {
  const level = event.data?.level as string | undefined
  if (level === 'error') return 'text-red-400'
  if (level === 'warn') return 'text-amber-400'
  return 'text-g-11'
}

function logLevelDot(event: WorkflowEvent): string {
  const level = event.data?.level as string | undefined
  if (level === 'error') return 'bg-red-400'
  if (level === 'warn') return 'bg-amber-400'
  return 'bg-g-10'
}

function logLevelBadge(event: WorkflowEvent): string {
  const level = event.data?.level as string | undefined
  if (level === 'error') return 'text-red-400 bg-red-400/10'
  if (level === 'warn') return 'text-amber-400 bg-amber-400/10'
  return 'text-g-9 bg-g-5'
}

function eventColor(type: string) {
  if (type === 'step.completed' || type === 'workflow.completed') return 'text-emerald-400'
  if (type === 'step.failed') return 'text-red-400'
  if (type === 'step.skipped') return 'text-g-8'
  if (type === 'step.started' || type === 'workflow.started') return 'text-g-12'
  if (type === 'step.waiting') return 'text-amber-400'
  if (type === 'step.goto') return 'text-amber-400'
  if (type === 'step.input') return 'text-indigo-400'
  if (type === 'step.output') return 'text-cyan-400'
  return 'text-g-9'
}

function eventDot(type: string) {
  if (type === 'step.completed' || type === 'workflow.completed') return 'bg-emerald-400'
  if (type === 'step.failed') return 'bg-red-400'
  if (type === 'step.started' || type === 'workflow.started') return 'bg-g-12'
  if (type === 'step.waiting') return 'bg-amber-400'
  if (type === 'step.goto') return 'bg-amber-400'
  if (type === 'step.input') return 'bg-indigo-400'
  if (type === 'step.output') return 'bg-cyan-400'
  return 'bg-g-7'
}

function chipColor(type: string, hidden: boolean) {
  if (hidden) return 'bg-g-4 text-g-7 border-g-5'
  if (type === 'step.completed' || type === 'workflow.completed') return 'bg-emerald-400/15 text-emerald-400 border-emerald-400/20'
  if (type === 'step.failed') return 'bg-red-400/15 text-red-400 border-red-400/20'
  if (type === 'step.started' || type === 'workflow.started') return 'bg-g-5 text-g-12 border-g-5'
  if (type === 'step.waiting') return 'bg-amber-400/15 text-amber-400 border-amber-400/20'
  if (type === 'step.goto') return 'bg-amber-400/15 text-amber-400 border-amber-400/20'
  if (type === 'step.log') return 'bg-g-5 text-g-10 border-g-5'
  if (type === 'step.input') return 'bg-indigo-400/15 text-indigo-400 border-indigo-400/20'
  if (type === 'step.output') return 'bg-cyan-400/15 text-cyan-400 border-cyan-400/20'
  return 'bg-g-5 text-g-9 border-g-5'
}

function formatTime(timestamp: string) {
  const d = new Date(timestamp)
  return d.toLocaleTimeString('fr-FR', { hour12: false, fractionalSecondDigits: 3 } as Intl.DateTimeFormatOptions)
}

function shortType(type: string) {
  return type.replace('workflow.', 'wf.').replace('step.', '')
}
</script>

<template>
  <div>
    <!-- Toolbar: search + type filters -->
    <div class="px-4 py-2.5 border-b border-g-5 flex flex-col gap-2">
      <!-- Search bar -->
      <div class="relative">
        <svg class="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-g-7" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
        </svg>
        <input
          v-model="search"
          type="text"
          :placeholder="t('search.placeholder')"
          class="w-full bg-g-3 border border-g-5 rounded-md text-[12px] text-g-12 placeholder-g-7 pl-8 pr-3 py-1.5 focus:outline-none focus:border-g-8 transition-colors"
        />
      </div>
      <!-- Type filter chips -->
      <div v-if="eventTypes.length > 0" class="flex flex-wrap gap-1.5">
        <button
          v-for="type in eventTypes"
          :key="type"
          @click="toggleType(type)"
          :class="['text-[10px] font-mono px-2 py-0.5 rounded-full border transition-all cursor-pointer select-none', chipColor(type, hiddenTypes.has(type))]"
        >
          {{ shortType(type) }}
        </button>
      </div>
    </div>

    <!-- Event list -->
    <div ref="logContainer" class="max-h-72 overflow-y-auto">
      <div v-if="filteredEvents.length === 0" class="px-4 py-8 text-center text-g-8 text-sm">
        {{ events.length === 0 ? t('log.waitingEvents') : t('log.noMatch') }}
      </div>

      <template v-for="(event, i) in filteredEvents" :key="i">
        <!-- step.log: dedicated style with level badge -->
        <div
          v-if="event.type === 'step.log'"
          class="flex items-start gap-3 px-4 py-1.5 text-[13px] hover:bg-g-3 transition-colors"
        >
          <span :class="['w-1.5 h-1.5 rounded-full flex-shrink-0 mt-[5px]', logLevelDot(event)]" />
          <span class="text-g-7 font-mono whitespace-nowrap w-[85px] flex-shrink-0">
            {{ formatTime(event.timestamp) }}
          </span>
          <span :class="['font-mono whitespace-nowrap text-[11px] rounded px-1.5 py-0.5 leading-tight flex-shrink-0', logLevelBadge(event)]">
            {{ (event.data?.level as string) || 'info' }}
          </span>
          <span v-if="event.step_id" class="font-mono text-g-10 whitespace-nowrap flex-shrink-0">
            {{ event.step_id }}
          </span>
          <span :class="['break-all whitespace-pre-wrap', logLevelColor(event)]">{{ event.message }}</span>
        </div>

        <!-- All other events: standard style -->
        <div
          v-else
          class="flex items-start gap-3 px-4 py-1.5 text-[13px] hover:bg-g-3 transition-colors"
        >
          <span :class="['w-1.5 h-1.5 rounded-full flex-shrink-0 mt-[5px]', eventDot(event.type)]" />
          <span class="text-g-7 font-mono whitespace-nowrap w-[85px] flex-shrink-0">
            {{ formatTime(event.timestamp) }}
          </span>
          <span :class="['font-mono whitespace-nowrap w-[90px] flex-shrink-0', eventColor(event.type)]">
            {{ shortType(event.type) }}
          </span>
          <span v-if="event.step_id" class="font-mono text-g-10 whitespace-nowrap flex-shrink-0">
            {{ event.step_id }}
          </span>
          <span class="text-g-9 break-all whitespace-pre-wrap">{{ event.message }}</span>
        </div>
      </template>
    </div>
  </div>
</template>
