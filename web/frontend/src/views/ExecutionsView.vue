<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useWorkflowApi, type Execution, type Graph } from '@/composables/useWorkflowApi'
import { useGlobalEvents } from '@/composables/useGlobalEvents'
import { fmtDur, fmtAgo, statusDot } from '@/composables/useFormat'
import StatusBadge from '@/components/primitives/StatusBadge.vue'
import Icon from '@/components/primitives/Icon.vue'

const { t } = useI18n()
const router = useRouter()
const api = useWorkflowApi()

const PAGE_SIZE = 30

const filter = ref<'all' | 'success' | 'failed' | 'cancelled' | 'running' | 'waiting'>('all')
const search = ref('')
const compareIds = ref<string[]>([])
const graph = ref<Graph | null>(null)
const executions = ref<Execution[]>([])
const total = ref(0)
const offset = ref(0)
const hasMore = computed(() => offset.value < total.value)
const loading = ref(false)

const filterChips: Array<{ key: 'all' | 'success' | 'failed' | 'cancelled' | 'running' | 'waiting'; label: string }> = [
  { key: 'all', label: 'all' },
  { key: 'success', label: 'success' },
  { key: 'failed', label: 'failed' },
  { key: 'cancelled', label: 'cancelled' },
  { key: 'running', label: 'running' },
  { key: 'waiting', label: 'waiting' },
]

async function fetchPage(reset = false) {
  if (loading.value) return
  if (!reset && !hasMore.value) return
  loading.value = true
  try {
    if (reset) {
      offset.value = 0
      executions.value = []
    }
    const status = filter.value === 'all' ? undefined : [filter.value]
    const resp = await api.listExecutions({ status, sort: 'date', order: 'desc', limit: PAGE_SIZE, offset: offset.value })
    if (reset) executions.value = resp.items
    else executions.value.push(...resp.items)
    total.value = resp.total
    offset.value += resp.items.length
  } finally {
    loading.value = false
  }
}

watch([filter], () => fetchPage(true))

onMounted(() => {
  fetchPage(true)
  api.getWorkflowGraph().then(g => { graph.value = g }).catch(() => {})
})

useGlobalEvents(() => fetchPage(true))

const sentinel = ref<HTMLElement>()
let observer: IntersectionObserver | null = null
onMounted(() => {
  nextTick(() => {
    observer = new IntersectionObserver(
      ([entry]) => { if (entry.isIntersecting && hasMore.value && !loading.value) fetchPage() },
      { rootMargin: '200px' }
    )
    if (sentinel.value) observer.observe(sentinel.value)
  })
})
onUnmounted(() => observer?.disconnect())

const filtered = computed(() => {
  const s = search.value.toLowerCase().trim()
  if (!s) return executions.value
  return executions.value.filter(e => {
    if (e.id.toLowerCase().includes(s)) return true
    const params = e.params ? Object.entries(e.params).map(([k, v]) => `${k}=${v}`).join(' ') : ''
    return params.toLowerCase().includes(s)
  })
})

function toggleCompare(id: string) {
  const idx = compareIds.value.indexOf(id)
  if (idx >= 0) compareIds.value.splice(idx, 1)
  else if (compareIds.value.length >= 2) compareIds.value = [compareIds.value[1], id]
  else compareIds.value.push(id)
}

const showCompare = computed(() => compareIds.value.length === 2)
const compareA = computed(() => executions.value.find(r => r.id === compareIds.value[0]))
const compareB = computed(() => executions.value.find(r => r.id === compareIds.value[1]))

function execDuration(e: Execution): number | null {
  if (e.finished_at && e.started_at) return new Date(e.finished_at).getTime() - new Date(e.started_at).getTime()
  return null
}

function compareDelta(): string {
  const a = compareA.value, b = compareB.value
  if (!a || !b) return ''
  const da = execDuration(a), db = execDuration(b)
  if (da == null || db == null) return ''
  const delta = db - da
  const sign = delta >= 0 ? '+' : ''
  return `duration ${sign}${fmtDur(Math.abs(delta))}`
}

function paramsLabel(e: Execution): string {
  if (!e.params || Object.keys(e.params).length === 0) return ''
  return Object.entries(e.params).map(([k, v]) => `${k}=${v}`).join(' ')
}

function stepCells(exec: Execution): string[] {
  if (!exec.steps) return []
  if (graph.value) return graph.value.nodes.filter(n => exec.steps![n.id]).map(n => exec.steps![n.id].status)
  return Object.values(exec.steps).map(s => s.status)
}

function stepCellClass(status: string): string {
  if (status === 'success') return 'bg-emerald-400'
  if (status === 'failed') return 'bg-red-400'
  if (status === 'running') return 'bg-amber-400 animate-pulse'
  if (status === 'waiting') return 'bg-amber-400 animate-pulse'
  if (status === 'skipped') return 'bg-g-6'
  if (status === 'cancelled') return 'bg-orange-400'
  return 'bg-g-5'
}

const counts = computed(() => {
  const all = executions.value
  return {
    total: total.value,
    success: all.filter(e => e.status === 'success').length,
    failed: all.filter(e => e.status === 'failed').length,
    cancelled: all.filter(e => e.status === 'cancelled').length,
  }
})
</script>

<template>
  <div class="px-8 py-6">
    <!-- Header -->
    <div class="flex items-center justify-between mb-5">
      <div>
        <h1 class="text-[20px] font-semibold text-g-14">{{ t('executions.title') }}</h1>
        <p class="text-[12px] text-g-10 mt-0.5">{{ counts.total }} total · {{ counts.success }} success · {{ counts.failed }} failed · {{ counts.cancelled }} cancelled</p>
      </div>
      <div class="flex items-center gap-2">
        <span v-if="compareIds.length > 0" class="text-[11px] font-mono text-g-10">{{ compareIds.length }}/2 selected</span>
        <button
          v-if="showCompare"
          class="h-8 px-3 rounded text-[12px] font-medium bg-g-14 text-g-1 hover:bg-g-15"
          @click="router.push({ name: 'execution', params: { id: compareIds[0] } })"
        >Open A ↗</button>
      </div>
    </div>

    <!-- Compare panel -->
    <div
      v-if="showCompare && compareA && compareB"
      class="bg-g-2 border border-g-6 rounded mb-4 overflow-hidden anim-enter"
    >
      <div class="flex items-center justify-between px-4 py-2 border-b border-g-5">
        <div class="flex items-center gap-2">
          <span class="text-[11px] font-mono uppercase tracking-wider text-g-9">Compare</span>
        </div>
        <button @click="compareIds = []" class="text-g-9 hover:text-g-12">
          <Icon name="x" class-name="w-4 h-4" />
        </button>
      </div>
      <div class="grid grid-cols-2 divide-x divide-g-5">
        <div class="p-4">
          <div class="flex items-center gap-2 mb-2">
            <span class="text-[10px] uppercase font-mono text-g-8">A</span>
            <span class="font-mono text-[12px] text-g-13">{{ compareA.id }}</span>
            <StatusBadge :status="compareA.status" />
            <span class="font-mono text-[11px] text-g-9 ml-auto tabular-nums">{{ execDuration(compareA) ? fmtDur(execDuration(compareA)!) : '—' }}</span>
          </div>
          <div class="flex gap-1 mb-2">
            <div
              v-for="(s, i) in stepCells(compareA)"
              :key="i"
              :class="['flex-1 h-2 rounded-sm', stepCellClass(s)]"
            />
          </div>
          <div class="font-mono text-[11px] text-g-9 leading-relaxed">
            <div>started <span class="text-g-12">{{ fmtAgo(compareA.started_at) }}</span></div>
            <div v-if="paramsLabel(compareA)">params <span class="text-g-12">{{ paramsLabel(compareA) }}</span></div>
            <div v-if="compareA.error" class="text-red-400/90 mt-1">{{ compareA.error }}</div>
          </div>
        </div>
        <div class="p-4">
          <div class="flex items-center gap-2 mb-2">
            <span class="text-[10px] uppercase font-mono text-g-8">B</span>
            <span class="font-mono text-[12px] text-g-13">{{ compareB.id }}</span>
            <StatusBadge :status="compareB.status" />
            <span class="font-mono text-[11px] text-g-9 ml-auto tabular-nums">{{ execDuration(compareB) ? fmtDur(execDuration(compareB)!) : '—' }}</span>
          </div>
          <div class="flex gap-1 mb-2">
            <div
              v-for="(s, i) in stepCells(compareB)"
              :key="i"
              :class="['flex-1 h-2 rounded-sm', stepCellClass(s)]"
            />
          </div>
          <div class="font-mono text-[11px] text-g-9 leading-relaxed">
            <div>started <span class="text-g-12">{{ fmtAgo(compareB.started_at) }}</span></div>
            <div v-if="paramsLabel(compareB)">params <span class="text-g-12">{{ paramsLabel(compareB) }}</span></div>
            <div v-if="compareB.error" class="text-red-400/90 mt-1">{{ compareB.error }}</div>
          </div>
        </div>
      </div>
      <div class="px-4 py-2 border-t border-g-5 flex items-center gap-2 bg-g-1">
        <span class="text-[10px] font-mono text-g-8 uppercase tracking-wider">Δ</span>
        <span class="font-mono text-[11px] text-amber-400">{{ compareDelta() }}</span>
      </div>
    </div>

    <!-- Filters + search -->
    <div class="flex items-center gap-2 mb-4">
      <div class="flex items-center bg-g-2 border border-g-5 rounded">
        <button
          v-for="s in filterChips"
          :key="s.key"
          @click="filter = s.key"
          :class="['px-3 py-1.5 text-[11px] font-mono', filter === s.key ? 'bg-g-4 text-g-14' : 'text-g-9 hover:text-g-12']"
        >
          {{ s.label }}
          <span v-if="filter === s.key && s.key === 'all'" class="text-g-8">· {{ counts.total }}</span>
        </button>
      </div>
      <div class="flex-1" />
      <div class="flex items-center gap-2 bg-g-2 border border-g-5 rounded px-2.5 h-8">
        <Icon name="search" class-name="w-3.5 h-3.5 text-g-8" />
        <input
          v-model="search"
          placeholder="exe_… or group=…"
          class="bg-transparent outline-none text-[12px] font-mono text-g-12 placeholder:text-g-7 w-56"
        />
      </div>
      <button
        class="h-8 px-3 rounded text-[11px] font-mono text-g-10 bg-g-2 border border-g-5 hover:bg-g-3 flex items-center gap-1.5"
        @click="fetchPage(true)"
      >
        <Icon name="refresh" class-name="w-3.5 h-3.5" />
        replay
      </button>
    </div>

    <!-- Table -->
    <div v-if="filtered.length === 0 && !loading" class="bg-g-2 border border-g-5 rounded-lg py-16 text-center text-g-9 text-sm">
      {{ t('executions.none') }}
    </div>
    <div v-else class="bg-g-2 border border-g-5 rounded overflow-hidden">
      <div class="grid grid-cols-[24px_24px_140px_90px_80px_1fr_140px_80px_80px] gap-3 px-3 py-2 border-t border-g-4 text-[10px] uppercase tracking-wider text-g-8 font-mono">
        <div></div>
        <div></div>
        <div>id</div>
        <div>status</div>
        <div>duration</div>
        <div>params / error</div>
        <div>steps</div>
        <div>trigger</div>
        <div>started</div>
      </div>
      <div
        v-for="r in filtered"
        :key="r.id"
        :class="[
          'grid grid-cols-[24px_24px_140px_90px_80px_1fr_140px_80px_80px] gap-3 px-3 py-2 hover:bg-g-3 border-t border-g-4 items-center',
          compareIds.includes(r.id) ? 'bg-g-3' : ''
        ]"
      >
        <input
          type="checkbox"
          :checked="compareIds.includes(r.id)"
          @change="toggleCompare(r.id)"
          class="cursor-pointer"
        />
        <span :class="['w-1.5 h-1.5 rounded-full', statusDot(r.status)]" />
        <span
          @click="router.push({ name: 'execution', params: { id: r.id } })"
          class="font-mono text-[12px] text-g-12 cursor-pointer hover:text-g-14 truncate"
        >{{ r.id }}</span>
        <StatusBadge :status="r.status" />
        <span class="font-mono text-[12px] text-g-11 tabular-nums">{{ execDuration(r) ? fmtDur(execDuration(r)!) : '—' }}</span>
        <span v-if="r.error" class="font-mono text-[11px] text-red-400/90 truncate">{{ typeof r.error === 'string' ? r.error : (r.error as any).message }}</span>
        <span v-else class="font-mono text-[11px] text-g-9 truncate">{{ paramsLabel(r) }}</span>
        <div class="flex gap-[2px]">
          <div
            v-for="(s, i) in stepCells(r)"
            :key="i"
            :class="['w-2.5 h-3 rounded-sm', stepCellClass(s)]"
          />
        </div>
        <span class="font-mono text-[11px] text-g-9">{{ (r as any).trigger || 'manual' }}</span>
        <span class="font-mono text-[11px] text-g-8 tabular-nums">{{ fmtAgo(r.started_at) }}</span>
      </div>
    </div>

    <div ref="sentinel" class="h-1" />
    <div v-if="loading && executions.length === 0" class="flex items-center justify-center py-16">
      <div class="w-5 h-5 border-2 border-g-7 border-t-g-12 rounded-full animate-spin" />
    </div>
  </div>
</template>
