<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useWorkflowApi, type Execution } from '@/composables/useWorkflowApi'
import { useGlobalEvents } from '@/composables/useGlobalEvents'
import Icon from './primitives/Icon.vue'

const props = defineProps<{
  currentRunId: string
}>()

const router = useRouter()
const api = useWorkflowApi()

const activeRuns = ref<Execution[]>([])

async function refresh() {
  try {
    const r = await api.listExecutions({ status: ['running', 'waiting'], limit: 20 })
    activeRuns.value = r.items
  } catch {}
}

useGlobalEvents(refresh)

onMounted(refresh)

const liveCount = computed(() => activeRuns.value.filter(r => r.status === 'running').length)
const waitCount = computed(() => activeRuns.value.filter(r => r.status === 'waiting').length)
const view = ref<'strip' | 'tile'>('strip')

const tick = ref(0)
let timer: ReturnType<typeof setInterval> | null = null
onMounted(() => { timer = setInterval(() => { tick.value++ }, 1000) })
onUnmounted(() => { if (timer) clearInterval(timer) })

function elapsedSeconds(r: Execution): number {
  void tick.value
  return Math.floor((Date.now() - new Date(r.started_at).getTime()) / 1000)
}

function fmtElapsed(s: number): string {
  if (s < 60) return s + 's'
  return Math.floor(s / 60) + 'm' + (s % 60) + 's'
}

function progress(r: Execution): number {
  if (!r.steps) return 0
  const all = Object.values(r.steps)
  const done = all.filter(s => s.status === 'success' || s.status === 'failed' || s.status === 'skipped').length
  return all.length > 0 ? (done / all.length) * 100 : 0
}

function dotColor(status: string): string {
  if (status === 'running') return 'bg-amber-400'
  if (status === 'waiting') return 'bg-violet-400'
  return 'bg-g-7'
}

function go(id: string) {
  router.push({ name: 'execution', params: { id } })
}
</script>

<template>
  <div v-if="activeRuns.length > 0" class="border-b border-g-5 bg-g-1 shrink-0">
    <div class="flex items-center px-3 h-10 gap-2">
      <div class="flex items-center gap-1.5 pr-2 border-r border-g-5 mr-1">
        <span class="text-[10px] font-semibold uppercase tracking-wider text-g-9">Active</span>
        <span class="font-mono text-[11px] text-emerald-400 flex items-center gap-1">
          <span class="relative flex h-1.5 w-1.5">
            <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75" />
            <span class="relative inline-flex rounded-full h-1.5 w-1.5 bg-emerald-400" />
          </span>
          {{ liveCount }}
        </span>
        <span v-if="waitCount > 0" class="font-mono text-[11px] text-violet-400 flex items-center gap-1">
          <span class="w-1.5 h-1.5 rounded-full bg-violet-400" />
          {{ waitCount }}w
        </span>
      </div>
      <div v-if="view === 'strip'" class="flex-1 flex items-center gap-1 overflow-x-auto scrollbar-hide">
        <button
          v-for="r in activeRuns"
          :key="r.id"
          @click="go(r.id)"
          :class="[
            'group shrink-0 flex items-center gap-2 h-7 px-2 rounded text-[11px] font-mono border transition',
            r.id === props.currentRunId
              ? 'bg-g-3 border-g-7 text-g-14'
              : 'bg-g-2 border-g-5 text-g-10 hover:border-g-6 hover:text-g-12',
          ]"
          :title="`${r.workflow_name} · ${r.id}`"
        >
          <span :class="['relative w-1.5 h-1.5 rounded-full', dotColor(r.status)]">
            <span v-if="r.status === 'running'" :class="['absolute inset-0 rounded-full animate-ping opacity-75', dotColor(r.status)]" />
          </span>
          <span :class="r.id === props.currentRunId ? 'text-g-14 font-medium' : 'text-g-12'">{{ r.workflow_name }}</span>
          <span class="text-g-7">·</span>
          <span class="text-g-8 tabular-nums">{{ r.id.slice(-6) }}</span>
          <span class="w-10 h-1 bg-g-5 rounded-sm overflow-hidden">
            <span :class="['block h-full', r.status === 'waiting' ? 'bg-violet-400' : 'bg-emerald-400']" :style="{ width: progress(r) + '%' }" />
          </span>
          <span class="text-g-8 tabular-nums">{{ fmtElapsed(elapsedSeconds(r)) }}</span>
        </button>
      </div>
      <div v-else class="flex-1" />
      <div class="flex items-center gap-1 pl-2 border-l border-g-5 ml-1 shrink-0">
        <button
          @click="view = view === 'strip' ? 'tile' : 'strip'"
          :class="['h-6 w-6 grid place-items-center hover:bg-g-3 rounded', view === 'tile' ? 'text-g-13' : 'text-g-9 hover:text-g-13']"
          :title="view === 'strip' ? 'Tile all runs' : 'Show as strip'"
        >
          <Icon name="grid" class-name="w-3.5 h-3.5" />
        </button>
        <button
          @click="refresh"
          class="h-6 w-6 grid place-items-center text-g-9 hover:text-g-13 hover:bg-g-3 rounded"
          title="Refresh"
        >
          <Icon name="refresh" class-name="w-3.5 h-3.5" />
        </button>
      </div>
    </div>

    <!-- Tile view -->
    <div v-if="view === 'tile'" class="px-3 pb-3 grid grid-cols-3 gap-2">
      <button
        v-for="r in activeRuns"
        :key="r.id"
        @click="go(r.id)"
        :class="[
          'text-left bg-g-2 border rounded p-2.5 transition hover:border-g-6',
          r.id === props.currentRunId ? 'border-g-7' : 'border-g-5',
        ]"
      >
        <div class="flex items-center gap-2 mb-1">
          <span :class="['relative w-1.5 h-1.5 rounded-full', dotColor(r.status)]">
            <span v-if="r.status === 'running'" :class="['absolute inset-0 rounded-full animate-ping opacity-75', dotColor(r.status)]" />
          </span>
          <span class="font-mono text-[12px] text-g-13 truncate flex-1">{{ r.workflow_name }}</span>
          <span class="font-mono text-[10px] text-g-8 tabular-nums">{{ fmtElapsed(elapsedSeconds(r)) }}</span>
        </div>
        <div class="flex items-center gap-1.5 mb-1.5">
          <span class="font-mono text-[10px] text-g-7">{{ r.id.slice(-12) }}</span>
        </div>
        <div class="w-full h-1 bg-g-5 rounded-sm overflow-hidden">
          <span :class="['block h-full', r.status === 'waiting' ? 'bg-violet-400' : 'bg-emerald-400']" :style="{ width: progress(r) + '%' }" />
        </div>
        <div class="flex items-center justify-between mt-1.5">
          <span class="font-mono text-[10px] text-g-9">{{ r.status }}</span>
          <span class="font-mono text-[10px] text-g-8 tabular-nums">{{ Math.round(progress(r)) }}%</span>
        </div>
      </button>
    </div>
  </div>
</template>
