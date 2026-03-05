<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useWorkflowApi, type Execution, type Graph } from '@/composables/useWorkflowApi'
import { useGlobalEvents } from '@/composables/useGlobalEvents'

const { t, locale } = useI18n()

const router = useRouter()
const api = useWorkflowApi()

const PAGE_SIZE = 20

const statusFilter = ref<string[]>([])
const sortBy = ref<'date' | 'duration'>('date')
const sortOrder = ref<'desc' | 'asc'>('desc')
const graph = ref<Graph | null>(null)
const executions = ref<Execution[]>([])
const total = ref(0)
const offset = ref(0)
const hasMore = computed(() => offset.value < total.value)
const loading = ref(false)

const statuses = [
  { key: 'running', label: 'executions.filterRunning' },
  { key: 'success', label: 'executions.filterSuccess' },
  { key: 'failed', label: 'executions.filterFailed' },
  { key: 'cancelled', label: 'executions.filterCancelled' },
  { key: 'waiting', label: 'executions.filterWaiting' },
]

function toggleStatus(s: string) {
  const idx = statusFilter.value.indexOf(s)
  if (idx >= 0) statusFilter.value.splice(idx, 1)
  else statusFilter.value.push(s)
}

function clearFilters() {
  statusFilter.value = []
}

function toggleSort(field: 'date' | 'duration') {
  if (sortBy.value === field) {
    sortOrder.value = sortOrder.value === 'desc' ? 'asc' : 'desc'
  } else {
    sortBy.value = field
    sortOrder.value = 'desc'
  }
}

async function fetchPage(reset = false) {
  if (loading.value) return
  if (!reset && !hasMore.value) return
  loading.value = true
  try {
    if (reset) {
      offset.value = 0
      executions.value = []
    }
    const resp = await api.listExecutions({
      status: statusFilter.value.length ? statusFilter.value : undefined,
      sort: sortBy.value,
      order: sortOrder.value,
      limit: PAGE_SIZE,
      offset: offset.value,
    })
    if (reset) executions.value = resp.items
    else executions.value.push(...resp.items)
    total.value = resp.total
    offset.value += resp.items.length
  } catch {
    // ignore
  } finally {
    loading.value = false
  }
}

// Re-fetch on filter/sort change
watch([statusFilter, sortBy, sortOrder], () => fetchPage(true), { deep: true })

onMounted(() => {
  fetchPage(true)
  api.getWorkflowGraph().then(g => { graph.value = g }).catch(() => {})
})

// SSE: refresh current view (debounced)
useGlobalEvents(() => fetchPage(true))

// Infinite scroll sentinel
const sentinel = ref<HTMLElement>()
let observer: IntersectionObserver | null = null

onMounted(() => {
  nextTick(() => {
    observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting && hasMore.value && !loading.value) fetchPage()
      },
      { rootMargin: '200px' }
    )
    if (sentinel.value) observer.observe(sentinel.value)
  })
})

onUnmounted(() => observer?.disconnect())

function dot(s: string) {
  if (s === 'success') return 'bg-emerald-400'
  if (s === 'failed') return 'bg-red-400'
  if (s === 'cancelled') return 'bg-g-9'
  if (s === 'running') return 'bg-g-12 animate-pulse'
  if (s === 'waiting') return 'bg-amber-400 animate-pulse'
  return 'bg-g-8'
}
function stepDot(s: string) {
  if (s === 'success') return 'bg-emerald-400'
  if (s === 'failed') return 'bg-red-400'
  if (s === 'running') return 'bg-g-11 animate-pulse'
  if (s === 'waiting') return 'bg-amber-400 animate-pulse'
  if (s === 'skipped') return 'bg-g-6'
  return 'bg-g-5'
}
function badge(s: string) {
  if (s === 'success') return 'bg-emerald-400/15 text-emerald-400'
  if (s === 'failed') return 'bg-red-400/15 text-red-400'
  if (s === 'cancelled') return 'bg-g-7/20 text-g-9'
  if (s === 'running') return 'bg-amber-400/15 text-amber-400'
  if (s === 'waiting') return 'bg-amber-400/15 text-amber-400'
  return 'bg-g-7/20 text-g-9'
}
function isAnimated(s: string) {
  return s === 'running' || s === 'waiting'
}
function hashCode(str: string): number {
  let hash = 0
  for (let i = 0; i < str.length; i++) {
    hash = ((hash << 5) - hash + str.charCodeAt(i)) | 0
  }
  return Math.abs(hash)
}
function execGradient(id: string): string {
  const h1 = hashCode(id) % 360
  const h2 = (h1 + 40 + (hashCode(id + 'x') % 80)) % 360
  return `linear-gradient(135deg, hsl(${h1}, 70%, 60%), hsl(${h2}, 70%, 50%))`
}

function formatDate(d: string) {
  return new Date(d).toLocaleString(locale.value, {
    day: '2-digit', month: '2-digit', year: 'numeric',
    hour: '2-digit', minute: '2-digit', second: '2-digit'
  })
}

function duration(exec: Execution) {
  if (!exec.finished_at) return '...'
  const ms = new Date(exec.finished_at).getTime() - new Date(exec.started_at).getTime()
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

function stepProgress(exec: Execution): { done: number; total: number } | null {
  if (!exec.steps || Object.keys(exec.steps).length === 0) return null
  const entries = Object.values(exec.steps)
  const total = entries.length
  const done = entries.filter(s => s.status === 'success' || s.status === 'failed' || s.status === 'skipped').length
  return { done, total }
}

function sortArrow(field: 'date' | 'duration') {
  if (sortBy.value !== field) return ''
  return sortOrder.value === 'desc' ? '\u2193' : '\u2191'
}

function orderedSteps(exec: Execution): [string, { status: string; started_at?: string }][] {
  if (!exec.steps) return []
  if (graph.value) {
    // Use DAG node order
    return graph.value.nodes
      .filter(n => exec.steps![n.id])
      .map(n => [n.id, exec.steps![n.id]] as [string, { status: string; started_at?: string }])
  }
  // Fallback: sort by started_at
  return Object.entries(exec.steps)
    .sort(([, a], [, b]) => {
      const ta = a.started_at ? new Date(a.started_at).getTime() : Infinity
      const tb = b.started_at ? new Date(b.started_at).getTime() : Infinity
      return ta - tb
    })
}
</script>

<template>
  <div>
    <div class="mb-5">
      <h2 class="text-lg font-semibold text-g-14">{{ t('executions.title') }}</h2>
      <p class="text-sm text-g-8 mt-0.5">{{ t('executions.description') }}</p>
    </div>

    <!-- Toolbar: filters + sort -->
    <div class="flex flex-wrap items-center gap-2 mb-4">
      <!-- Status filter pills -->
      <button
        @click="clearFilters"
        :class="[
          'px-3 py-1.5 rounded-md text-[12px] font-medium transition-colors',
          statusFilter.length === 0
            ? 'bg-g-6 text-g-14'
            : 'bg-g-3 text-g-9 hover:bg-g-4'
        ]"
      >{{ t('executions.all') }}</button>
      <button
        v-for="s in statuses"
        :key="s.key"
        @click="toggleStatus(s.key)"
        :class="[
          'px-3 py-1.5 rounded-md text-[12px] font-medium transition-colors',
          statusFilter.includes(s.key)
            ? 'bg-g-6 text-g-14'
            : 'bg-g-3 text-g-9 hover:bg-g-4'
        ]"
      >{{ t(s.label) }}</button>

      <span class="flex-1" />

      <!-- Sort controls -->
      <div class="flex rounded-md overflow-hidden border border-g-5">
        <button
          @click="toggleSort('date')"
          :class="[
            'px-3 py-1.5 text-[12px] font-medium transition-colors',
            sortBy === 'date' ? 'bg-g-5 text-g-14' : 'bg-g-2 text-g-9 hover:bg-g-3'
          ]"
        >{{ t('executions.sortDate') }} {{ sortArrow('date') }}</button>
        <button
          @click="toggleSort('duration')"
          :class="[
            'px-3 py-1.5 text-[12px] font-medium transition-colors border-l border-g-5',
            sortBy === 'duration' ? 'bg-g-5 text-g-14' : 'bg-g-2 text-g-9 hover:bg-g-3'
          ]"
        >{{ t('executions.sortDuration') }} {{ sortArrow('duration') }}</button>
      </div>
    </div>

    <!-- Empty state -->
    <div v-if="!loading && executions.length === 0" class="bg-g-2 border border-g-5 rounded-lg py-16 text-center text-g-9 text-sm lm-card">
      {{ t('executions.none') }}
    </div>

    <!-- Table -->
    <div v-else class="bg-g-2 border border-g-5 rounded-lg overflow-hidden lm-card">
      <!-- Header -->
      <div class="grid grid-cols-[7rem_10rem_1fr_5rem_10rem] gap-3 px-5 py-2.5 border-b border-g-5 text-[12px] font-medium text-g-9 uppercase tracking-wider">
        <span>{{ t('executions.status') }}</span>
        <span>ID</span>
        <span>{{ t('executions.steps') }}</span>
        <span class="text-right">{{ t('executions.duration') }}</span>
        <span class="text-right">{{ t('executions.date') }}</span>
      </div>
      <!-- Rows -->
      <div>
        <div
          v-for="exec in executions"
          :key="exec.id"
          @click="router.push({ name: 'execution', params: { id: exec.id } })"
          class="grid grid-cols-[7rem_10rem_1fr_5rem_10rem] gap-3 items-center px-5 py-3 cursor-pointer hover:bg-g-3 transition-colors border-b border-g-5 last:border-b-0"
        >
          <span :class="['inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium w-fit', badge(exec.status)]">
            <span :class="['w-1.5 h-1.5 rounded-full bg-current', isAnimated(exec.status) ? 'pulse-dot' : 'opacity-50']" />
            {{ exec.status }}
          </span>
          <span class="inline-flex items-center gap-1.5">
            <span class="w-3 h-3 rounded-sm shrink-0" :style="{ background: execGradient(exec.id) }" />
            <span class="text-[11px] text-g-10 font-mono">{{ exec.id.slice(0, 8) }}</span>
          </span>
          <div class="flex items-center gap-2">
            <template v-if="stepProgress(exec)">
              <div class="flex gap-[3px]">
                <span
                  v-for="[sid, step] in orderedSteps(exec)"
                  :key="sid"
                  :class="['w-[14px] h-[5px] rounded-sm', stepDot(step.status)]"
                  :title="`${sid}: ${step.status}`"
                />
              </div>
              <span class="text-[11px] text-g-8 font-mono tabular-nums">
                {{ stepProgress(exec)!.done }}/{{ stepProgress(exec)!.total }}
              </span>
            </template>
            <span v-else class="text-[11px] text-g-7 font-mono">-</span>
          </div>
          <span class="text-[12px] text-g-10 font-mono tabular-nums text-right">{{ duration(exec) }}</span>
          <span class="text-[12px] text-g-9 whitespace-nowrap text-right">{{ formatDate(exec.started_at) }}</span>
        </div>
      </div>

      <!-- Footer: showing count + loading -->
      <div v-if="executions.length > 0" class="px-5 py-2.5 border-t border-g-5 text-[12px] text-g-8 flex items-center gap-3">
        <span>{{ t('executions.showing', { count: executions.length, total }) }}</span>
        <div v-if="loading" class="w-3.5 h-3.5 border-2 border-g-7 border-t-g-12 rounded-full animate-spin" />
      </div>
    </div>

    <!-- Infinite scroll sentinel -->
    <div ref="sentinel" class="h-1" />

    <!-- Initial loading -->
    <div v-if="loading && executions.length === 0" class="flex items-center justify-center py-16">
      <div class="w-5 h-5 border-2 border-g-7 border-t-g-12 rounded-full animate-spin" />
    </div>
  </div>
</template>
