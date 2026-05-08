<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useWorkflowApi, type Execution, type Graph } from '@/composables/useWorkflowApi'
import { useRunTrigger } from '@/composables/useRunTrigger'
import { useStepInspector } from '@/composables/useStepInspector'
import { fmtDur, statusDot } from '@/composables/useFormat'
import Icon from './primitives/Icon.vue'
import Kbd from './primitives/Kbd.vue'

interface PaletteItem {
  id: string
  section: string
  label: string
  hint?: string
  kbd?: string[]
  type: 'action' | 'step' | 'run'
  status?: string
  action?: string
}

const props = defineProps<{
  open: boolean
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

const router = useRouter()
const api = useWorkflowApi()
const { showRunDialog, workflowReady } = useRunTrigger()
const inspector = useStepInspector()

const q = ref('')
const sel = ref(0)
const inputEl = ref<HTMLInputElement | null>(null)

const recentRuns = ref<Execution[]>([])
const graph = ref<Graph | null>(null)

watch(() => props.open, async (open) => {
  if (open) {
    q.value = ''
    sel.value = 0
    nextTick(() => inputEl.value?.focus())
    if (recentRuns.value.length === 0) {
      try {
        const r = await api.listExecutions({ limit: 8 })
        recentRuns.value = r.items
      } catch {}
    }
    if (!graph.value) {
      try {
        graph.value = await api.getWorkflowGraph()
      } catch {}
    }
  }
})

const items = computed<PaletteItem[]>(() => {
  const base: PaletteItem[] = [
    { id: 'run', section: 'Workflow', label: 'Run workflow', hint: 'with current params', kbd: ['⌘', '↵'], type: 'action' },
    { id: 'shortcuts', section: 'Settings', label: 'Show keyboard shortcuts', kbd: ['?'], type: 'action' },
    { id: 'theme', section: 'Settings', label: 'Toggle theme', hint: 'dark / light', type: 'action' },
    { id: 'history', section: 'Workflow', label: 'Open history', hint: 'recent executions', type: 'action' },
  ]
  const stepItems: PaletteItem[] = (graph.value?.nodes || []).map(n => ({
    id: 'step:' + n.id,
    section: 'Jump to step',
    label: n.id,
    hint: (n.action || '') + (n.label && n.label !== n.id ? ' · ' + n.label : ''),
    type: 'step',
    action: n.action,
  }))
  const runItems: PaletteItem[] = recentRuns.value.slice(0, 8).map(r => ({
    id: 'run:' + r.id,
    section: 'Recent runs',
    label: r.id,
    hint: r.status + (r.finished_at && r.started_at ? ' · ' + fmtDur(new Date(r.finished_at).getTime() - new Date(r.started_at).getTime()) : ' · running'),
    type: 'run',
    status: r.status,
  }))
  return [...base, ...stepItems, ...runItems]
})

const filtered = computed<PaletteItem[]>(() => {
  if (!q.value) return items.value
  const ql = q.value.toLowerCase()
  return items.value.filter(i => (i.label + ' ' + (i.hint || '') + ' ' + i.section).toLowerCase().includes(ql))
})

const grouped = computed<Record<string, PaletteItem[]>>(() => {
  const map: Record<string, PaletteItem[]> = {}
  for (const it of filtered.value) {
    if (!map[it.section]) map[it.section] = []
    map[it.section].push(it)
  }
  return map
})

watch(q, () => { sel.value = 0 })

function activate(item: PaletteItem) {
  if (item.id === 'run') {
    if (workflowReady.value) showRunDialog.value = true
  } else if (item.id === 'history') {
    router.push('/executions')
  } else if (item.id === 'shortcuts') {
    window.dispatchEvent(new CustomEvent('toggle-shortcuts'))
  } else if (item.id === 'theme') {
    window.dispatchEvent(new CustomEvent('toggle-theme'))
  } else if (item.id.startsWith('step:')) {
    inspector.inspect(item.label)
  } else if (item.id.startsWith('run:')) {
    router.push({ name: 'execution', params: { id: item.label } })
  }
  emit('close')
}

function onKey(e: KeyboardEvent) {
  if (e.key === 'ArrowDown') { e.preventDefault(); sel.value = Math.min(filtered.value.length - 1, sel.value + 1) }
  else if (e.key === 'ArrowUp') { e.preventDefault(); sel.value = Math.max(0, sel.value - 1) }
  else if (e.key === 'Enter') { e.preventDefault(); const it = filtered.value[sel.value]; if (it) activate(it) }
  else if (e.key === 'Escape') { e.preventDefault(); emit('close') }
}
</script>

<template>
  <Teleport to="body">
    <div
      v-if="props.open"
      class="fixed inset-0 z-50 flex items-start justify-center pt-[14vh] backdrop-blur-sm"
      style="background: rgba(0, 0, 0, 0.6)"
      @click.self="emit('close')"
    >
      <div class="w-[640px] bg-g-2 border border-g-6 rounded-lg shadow-2xl overflow-hidden anim-enter" @click.stop>
        <div class="flex items-center gap-2 px-4 py-3 border-b border-g-5">
          <Icon name="search" class-name="w-4 h-4 text-g-8" />
          <input
            ref="inputEl"
            v-model="q"
            @keydown="onKey"
            placeholder="Run · jump to step · open run · search…"
            class="flex-1 bg-transparent outline-none text-[14px] text-g-14 placeholder:text-g-7 font-mono"
          />
          <span class="text-[10px] text-g-8 font-mono"><Kbd>esc</Kbd></span>
        </div>
        <div class="max-h-[50vh] overflow-y-auto">
          <template v-for="(itemList, section) in grouped" :key="section">
            <div class="px-4 pt-2.5 pb-1 text-[10px] uppercase tracking-wider font-mono text-g-8">{{ section }}</div>
            <div
              v-for="item in itemList"
              :key="item.id"
              :class="['px-4 py-2 flex items-center gap-3 cursor-pointer', filtered.indexOf(item) === sel ? 'bg-g-4' : 'hover:bg-g-3']"
              @mouseenter="sel = filtered.indexOf(item)"
              @click="activate(item)"
            >
              <div :class="['w-1 h-4 rounded', filtered.indexOf(item) === sel ? 'bg-g-13' : 'bg-transparent']" />
              <span v-if="item.type === 'run'" :class="['w-1.5 h-1.5 rounded-full', statusDot(item.status || '')]" />
              <Icon v-else-if="item.type === 'step'" name="bolt" class-name="w-3.5 h-3.5 text-g-9" />
              <Icon v-else name="bolt" class-name="w-3.5 h-3.5 text-g-9" />
              <span class="text-[13px] text-g-13 font-mono">{{ item.label }}</span>
              <span class="text-[11px] text-g-8 truncate flex-1">{{ item.hint }}</span>
              <div v-if="item.kbd" class="flex gap-1">
                <Kbd v-for="k in item.kbd" :key="k">{{ k }}</Kbd>
              </div>
            </div>
          </template>
          <div v-if="filtered.length === 0" class="px-4 py-8 text-center text-[12px] text-g-8 font-mono">
            No matches for "{{ q }}"
          </div>
        </div>
        <div class="border-t border-g-5 px-4 py-2 flex items-center justify-between text-[10px] text-g-8 font-mono">
          <div class="flex gap-3">
            <span><Kbd>↵</Kbd> select</span>
            <span><Kbd>↑</Kbd><Kbd>↓</Kbd> navigate</span>
          </div>
          <span>cmd-k anywhere</span>
        </div>
      </div>
    </div>
  </Teleport>
</template>
