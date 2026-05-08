<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useWorkflowApi, type StepDetail } from '@/composables/useWorkflowApi'
import { JsonView } from '@tailflow/shared'
import { fmtDur } from '@/composables/useFormat'
import StatusBadge from './primitives/StatusBadge.vue'
import Icon from './primitives/Icon.vue'
import Kbd from './primitives/Kbd.vue'

const props = defineProps<{
  stepId: string | null
  open: boolean
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

const api = useWorkflowApi()

const detail = ref<StepDetail | null>(null)
const lastExecParams = ref<Record<string, unknown>>({})
const tab = ref<'config' | 'output' | 'logs' | 'runs'>('config')

interface LogLine {
  time: string
  level: 'INFO' | 'DBUG' | 'WARN' | 'ERR' | 'IN' | 'OUT' | 'GOTO'
  message: string
}

const logLines = ref<LogLine[]>([])
const logsLoading = ref(false)

watch(() => [props.stepId, props.open], async ([id, open]) => {
  if (!open || !id) {
    detail.value = null
    lastExecParams.value = {}
    logLines.value = []
    return
  }
  try {
    detail.value = await api.getStepDetail(id as string)
  } catch {
    detail.value = null
  }
  const lastExecId = (detail.value?.history || [])[0]?.execution_id
  if (lastExecId) {
    try {
      const exec = await api.getExecution(lastExecId) as any
      lastExecParams.value = exec?.params || {}
    } catch {
      lastExecParams.value = {}
    }
  } else {
    lastExecParams.value = {}
  }
  // Reset logs; loaded lazily when tab is opened
  logLines.value = []
})

async function loadLogs() {
  const sid = props.stepId
  const execId = (detail.value?.history || [])[0]?.execution_id
  if (!sid || !execId) {
    logLines.value = []
    return
  }
  logsLoading.value = true
  try {
    const resp = await api.listExecutionEvents(execId, 0, 500)
    const events = resp.events ?? []
    const lines: LogLine[] = []
    for (const ev of events) {
      if (ev.step_id !== sid) continue
      const type = ev.type || ev.event_type
      let level: LogLine['level'] | null = null
      let message = ev.message || ''
      if (type === 'step.log') {
        const lvl = ev.data?.level || 'info'
        level = lvl === 'error' ? 'ERR' : lvl === 'warn' ? 'WARN' : lvl === 'debug' ? 'DBUG' : 'INFO'
      } else if (type === 'step.input') {
        level = 'IN'
        if (!message) message = 'input received'
      } else if (type === 'step.output') {
        level = 'OUT'
        if (!message) message = 'output produced'
      } else if (type === 'step.goto') {
        level = 'GOTO'
        const tgt = ev.data?.target ?? '?'
        const it = ev.data?.iteration
        const max = ev.data?.max_iterations
        if (!message) message = `goto ${tgt}${it != null ? ` · iter ${it}/${max}` : ''}`
      } else if (type === 'step.failed') {
        level = 'ERR'
        if (!message) message = ev.data?.error || 'failed'
      }
      if (!level) continue
      lines.push({
        time: formatTime(ev.timestamp || ev.event_timestamp),
        level,
        message,
      })
    }
    logLines.value = lines
  } catch {
    logLines.value = []
  } finally {
    logsLoading.value = false
  }
}

function formatTime(ts: string): string {
  if (!ts) return ''
  const d = new Date(ts)
  if (isNaN(d.getTime())) return ts
  const pad = (n: number, len = 2) => String(n).padStart(len, '0')
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}.${pad(d.getMilliseconds(), 3)}`
}

function levelClass(lvl: LogLine['level']): string {
  switch (lvl) {
    case 'INFO': return 'text-g-10'
    case 'DBUG': return 'text-g-8'
    case 'WARN': return 'text-amber-400'
    case 'ERR':  return 'text-red-400'
    case 'IN':   return 'text-violet-400'
    case 'OUT':  return 'text-emerald-400'
    case 'GOTO': return 'text-amber-400'
  }
}

function messageClass(lvl: LogLine['level']): string {
  if (lvl === 'ERR') return 'text-red-400'
  if (lvl === 'WARN' || lvl === 'GOTO') return 'text-amber-400'
  if (lvl === 'DBUG') return 'text-g-9'
  return 'text-g-12'
}

watch(tab, (t) => {
  if (t === 'logs' && logLines.value.length === 0 && !logsLoading.value) {
    loadLogs()
  }
})

const step = computed(() => detail.value?.step as Record<string, any> | undefined)
const metrics = computed(() => detail.value?.metrics)
const history = computed(() => detail.value?.history || [])

interface TplDiff { path: string; before: string; after: string }

const TPL_REGEX = /\{\{\s*([^}]+?)\s*\}\}/g

function flatten(obj: any, prefix = '', acc: { path: string; value: string }[] = []): { path: string; value: string }[] {
  if (obj == null) return acc
  if (typeof obj === 'string') {
    acc.push({ path: prefix || 'value', value: obj })
    return acc
  }
  if (typeof obj !== 'object') return acc
  if (Array.isArray(obj)) {
    obj.forEach((v, i) => flatten(v, `${prefix}[${i}]`, acc))
    return acc
  }
  for (const [k, v] of Object.entries(obj)) {
    flatten(v, prefix ? `${prefix}.${k}` : k, acc)
  }
  return acc
}

const tplDiffs = computed<TplDiff[]>(() => {
  const cfg = (step.value as any)?.config
  if (!cfg) return []
  const params = lastExecParams.value
  const items = flatten(cfg)
  const diffs: TplDiff[] = []
  for (const it of items) {
    if (typeof it.value !== 'string') continue
    TPL_REGEX.lastIndex = 0
    if (!TPL_REGEX.test(it.value)) continue
    TPL_REGEX.lastIndex = 0
    const resolved = it.value.replace(TPL_REGEX, (_m, expr) => {
      const path = String(expr).trim()
      if (path.startsWith('params.')) {
        const k = path.slice('params.'.length)
        const v = (params as any)[k]
        return v != null ? String(v) : `<${k}>`
      }
      return `<${path}>`
    })
    if (resolved && resolved !== it.value) {
      diffs.push({ path: it.path, before: it.value, after: resolved })
    }
  }
  return diffs.slice(0, 10)
})

const status = computed(() => {
  const last = history.value[0]
  return last?.status || 'pending'
})

const action = computed(() => (step.value?.action as string) || '')
const title = computed(() => (step.value?.title as string) || (step.value?.id as string) || '')
const stage = computed(() => (step.value?.stage as string) || '')

const tabs: { id: 'config' | 'output' | 'logs' | 'runs'; label: string }[] = [
  { id: 'config', label: 'Config' },
  { id: 'output', label: 'Output' },
  { id: 'logs', label: 'Logs' },
  { id: 'runs', label: 'Runs' },
]

function lastDuration(): string {
  const h = history.value.find(x => x.duration_ms != null)
  return h ? fmtDur(h.duration_ms) : '—'
}

function copyId() {
  if (props.stepId) {
    try { navigator.clipboard?.writeText(props.stepId) } catch {}
  }
}

function actionGlyph(a: string): string {
  if (a === 'http') return 'globe'
  if (a === 'exec') return 'terminal'
  if (a === 'loop') return 'repeat'
  if (a === 'condition') return 'branch'
  if (a === 'delay' || a === 'schedule') return 'history'
  if (a === 'set' || a === 'template') return 'edit'
  if (a?.startsWith('array.') || a?.startsWith('string.')) return 'filter'
  if (a?.startsWith('json.')) return 'docs'
  if (a === 'log' || a === 'table') return 'docs'
  if (a?.startsWith('kv.') || a?.startsWith('sql.')) return 'db'
  if (a?.startsWith('wait.') || a?.startsWith('rabbitmq.')) return 'bolt'
  return 'bolt'
}

const successRate = computed(() => {
  const m = metrics.value
  if (!m || m.total_executions === 0) return '—'
  return ((m.success_count / m.total_executions) * 100).toFixed(1) + '%'
})

function onKey(e: KeyboardEvent) {
  if (!props.open) return
  if (e.key === 'Escape') emit('close')
}

onMounted(() => {
  window.addEventListener('keydown', onKey)
})
</script>

<template>
  <Teleport to="body">
    <aside
      v-if="open && stepId"
      class="fixed top-0 right-0 h-screen w-[520px] bg-g-2 border-l border-g-5 flex flex-col z-30 shadow-2xl anim-enter"
    >
      <!-- header -->
      <div class="px-4 pt-4 pb-3 border-b border-g-5">
        <div class="flex items-start justify-between gap-3 mb-2">
          <div class="flex items-center gap-2 min-w-0">
            <Icon :name="actionGlyph(action)" class-name="w-3.5 h-3.5 text-g-9" />
            <span class="font-mono text-[13px] font-semibold text-g-14 truncate">{{ stepId }}</span>
            <span v-if="action" class="font-mono text-[10px] px-1.5 py-0.5 rounded bg-g-4 text-g-9 shrink-0">{{ action }}</span>
          </div>
          <div class="flex items-center gap-1">
            <button @click="copyId" class="w-6 h-6 rounded hover:bg-g-4 flex items-center justify-center text-g-9 hover:text-g-12" title="Copy step id">
              <Icon name="copy" class-name="w-3.5 h-3.5" />
            </button>
            <button @click="emit('close')" class="w-6 h-6 rounded hover:bg-g-4 flex items-center justify-center text-g-9 hover:text-g-12" title="Close (Esc)">
              <Icon name="x" class-name="w-3.5 h-3.5" />
            </button>
          </div>
        </div>
        <div v-if="title" class="text-[12px] text-g-9 mb-3">{{ title }}</div>
        <div class="flex items-center gap-2 flex-wrap">
          <StatusBadge :status="status" />
          <span class="font-mono text-[11px] text-g-10">{{ lastDuration() }}</span>
          <span v-if="stage" class="font-mono text-[11px] text-g-8">stage · {{ stage }}</span>
          <div class="flex-1" />
          <span class="font-mono text-[10px] text-g-8 flex items-center gap-1">
            <Kbd>J</Kbd><Kbd>K</Kbd> nav
          </span>
        </div>
      </div>

      <!-- tabs -->
      <div class="flex border-b border-g-5 px-2 shrink-0">
        <button
          v-for="t in tabs"
          :key="t.id"
          :class="['px-3 py-2 text-[12px] font-medium transition-colors relative', tab === t.id ? 'text-g-14' : 'text-g-9 hover:text-g-12']"
          @click="tab = t.id"
        >
          {{ t.label }}
          <span v-if="tab === t.id" class="absolute bottom-0 left-2 right-2 h-[2px] bg-g-13" />
        </button>
      </div>

      <!-- body -->
      <div class="flex-1 overflow-y-auto">
        <!-- Config -->
        <div v-if="tab === 'config'" class="p-4 space-y-4">
          <div v-if="tplDiffs.length > 0">
            <div class="flex items-center justify-between mb-2">
              <h4 class="text-[11px] font-semibold uppercase tracking-wider text-g-9">Resolved (last run)</h4>
              <span class="text-[10px] text-g-8 font-mono">templates → values</span>
            </div>
            <div class="bg-g-1 border border-g-5 rounded p-3 space-y-2">
              <div v-for="d in tplDiffs" :key="d.path" class="font-mono text-[11.5px] leading-5">
                <div class="text-[10px] text-g-8 mb-0.5">{{ d.path }}</div>
                <div class="bg-red-400/10 text-red-400/90 px-2 py-0.5 rounded-t border-l-2 border-red-400/60 break-all">- {{ d.before }}</div>
                <div class="bg-emerald-400/10 text-emerald-400/90 px-2 py-0.5 rounded-b border-l-2 border-emerald-400/60 break-all">+ {{ d.after }}</div>
              </div>
            </div>
          </div>
          <div v-if="step">
            <h4 class="text-[11px] font-semibold uppercase tracking-wider text-g-9 mb-2">Step config</h4>
            <div class="bg-g-1 border border-g-5 rounded p-3 max-h-[420px] overflow-auto">
              <JsonView :data="step" :default-open="true" />
            </div>
          </div>
          <div v-if="(step as any)?.timeout || (step as any)?.retry">
            <h4 class="text-[11px] font-semibold uppercase tracking-wider text-g-9 mb-2">Retry policy</h4>
            <div class="grid grid-cols-2 gap-2">
              <div v-if="(step as any)?.retry?.max_attempts" class="bg-g-1 border border-g-5 rounded px-3 py-2">
                <div class="text-[10px] text-g-8 font-mono uppercase">max attempts</div>
                <div class="text-[14px] font-mono text-g-14">{{ (step as any).retry.max_attempts }}</div>
              </div>
              <div v-if="(step as any)?.retry?.delay" class="bg-g-1 border border-g-5 rounded px-3 py-2">
                <div class="text-[10px] text-g-8 font-mono uppercase">delay</div>
                <div class="text-[14px] font-mono text-g-14">{{ (step as any).retry.delay }}</div>
              </div>
              <div v-if="(step as any)?.timeout" class="bg-g-1 border border-g-5 rounded px-3 py-2">
                <div class="text-[10px] text-g-8 font-mono uppercase">timeout</div>
                <div class="text-[14px] font-mono text-g-14">{{ (step as any).timeout }}</div>
              </div>
            </div>
          </div>
        </div>

        <!-- Output -->
        <div v-if="tab === 'output'" class="p-4 space-y-4">
          <div v-if="status === 'running'" class="border border-amber-400/20 bg-amber-400/5 rounded px-3 py-2.5 flex items-center gap-2">
            <span class="w-1.5 h-1.5 bg-amber-400 rounded-full animate-pulse" />
            <span class="text-[12px] text-g-12">Step is running — output streaming below</span>
          </div>
          <div>
            <div class="flex items-center justify-between mb-2">
              <h4 class="text-[11px] font-semibold uppercase tracking-wider text-g-9">Output</h4>
              <div class="flex items-center gap-1">
                <button class="text-[10px] text-g-9 hover:text-g-13 px-2 py-1 rounded hover:bg-g-4 font-mono flex items-center gap-1">
                  <Icon name="copy" class-name="w-3 h-3" />copy
                </button>
              </div>
            </div>
            <div class="bg-g-1 border border-g-5 rounded p-3 max-h-[420px] overflow-auto">
              <JsonView v-if="history[0]?.output" :data="history[0].output" :default-open="true" />
              <div v-else class="text-[12px] text-g-8 font-mono">— no output yet —</div>
            </div>
          </div>
        </div>

        <!-- Logs -->
        <div v-if="tab === 'logs'" class="p-4 font-mono text-[11.5px] leading-relaxed">
          <div v-if="logsLoading" class="flex items-center gap-2 text-[12px] text-g-8">
            <span class="w-3 h-3 border-2 border-g-5 border-t-g-9 rounded-full animate-spin" />
            Loading log lines…
          </div>
          <div v-else-if="!history.length" class="text-[12px] text-g-8">No execution yet for this step.</div>
          <div v-else-if="!logLines.length" class="text-[12px] text-g-8">— no log lines for the latest run —</div>
          <template v-else>
            <div class="flex items-center justify-between mb-2 -mx-2 px-2">
              <span class="text-[10px] uppercase tracking-wider text-g-8">
                latest run · {{ logLines.length }} {{ logLines.length === 1 ? 'line' : 'lines' }}
              </span>
              <button
                class="text-[10px] text-g-9 hover:text-g-13 px-2 py-0.5 rounded hover:bg-g-4 flex items-center gap-1"
                @click="loadLogs"
                title="Refresh"
              >
                <Icon name="refresh" class-name="w-3 h-3" />
                refresh
              </button>
            </div>
            <div
              v-for="(line, i) in logLines"
              :key="i"
              class="grid grid-cols-[88px_44px_1fr] gap-2 py-1 hover:bg-g-3 -mx-2 px-2 rounded"
            >
              <span class="text-g-8 tabular-nums">{{ line.time }}</span>
              <span :class="levelClass(line.level)">{{ line.level }}</span>
              <span :class="messageClass(line.level)" class="break-words">{{ line.message }}</span>
            </div>
          </template>
        </div>

        <!-- Runs -->
        <div v-if="tab === 'runs'" class="p-4 space-y-2">
          <div class="text-[11px] text-g-9 mb-2">Last 30 invocations of <span class="font-mono text-g-12">{{ stepId }}</span></div>
          <div class="flex gap-[3px] flex-wrap mb-3">
            <div
              v-for="(h, i) in history.slice(0, 30)"
              :key="i"
              :class="[
                'w-2.5 h-5 rounded-sm',
                h.status === 'success' ? 'bg-emerald-400' :
                h.status === 'failed' ? 'bg-red-400' :
                h.status === 'running' ? 'bg-amber-400 animate-pulse' :
                'bg-g-6'
              ]"
              :title="`${h.execution_id} · ${h.status}`"
            />
          </div>
          <div v-if="metrics" class="grid grid-cols-3 gap-2">
            <div class="bg-g-1 border border-g-5 rounded px-3 py-2">
              <div class="text-[10px] text-g-8 font-mono uppercase">success rate</div>
              <div class="text-[15px] font-mono text-emerald-400">{{ successRate }}</div>
            </div>
            <div class="bg-g-1 border border-g-5 rounded px-3 py-2">
              <div class="text-[10px] text-g-8 font-mono uppercase">total runs</div>
              <div class="text-[15px] font-mono text-g-14">{{ metrics.total_executions }}</div>
            </div>
            <div class="bg-g-1 border border-g-5 rounded px-3 py-2">
              <div class="text-[10px] text-g-8 font-mono uppercase">avg</div>
              <div class="text-[15px] font-mono text-g-14">{{ metrics.avg_duration_ms ? fmtDur(metrics.avg_duration_ms) : '—' }}</div>
            </div>
          </div>
        </div>
      </div>

      <div class="border-t border-g-5 px-4 py-2 flex items-center justify-between gap-2 shrink-0">
        <div class="text-[10px] text-g-8 font-mono flex items-center gap-1.5">
          <Kbd>R</Kbd> rerun · <Kbd>C</Kbd> copy · <Kbd>O</Kbd> output
        </div>
      </div>
    </aside>
  </Teleport>
</template>
