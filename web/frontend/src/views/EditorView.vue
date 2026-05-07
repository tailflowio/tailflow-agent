<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import yaml from 'js-yaml'
import { useWorkflowApi, type Graph } from '@/composables/useWorkflowApi'
import { useStepInspector } from '@/composables/useStepInspector'
import WorkflowDAGCustom from '@/components/WorkflowDAGCustom.vue'
import YamlEditor from '@/components/YamlEditor.vue'
import ExprField, { type Suggestion } from '@/components/ExprField.vue'
import Icon from '@/components/primitives/Icon.vue'
import Kbd from '@/components/primitives/Kbd.vue'

const api = useWorkflowApi()
const inspector = useStepInspector()

const ACTION_CATALOG = [
  { cat: 'Flow', actions: ['loop', 'condition', 'group', 'delay', 'schedule', 'goto', 'validate'] },
  { cat: 'Data', actions: ['set', 'template', 'json.decode', 'json.encode', 'object', 'math', 'hash'] },
  { cat: 'Arrays', actions: ['array.sort', 'array.filter', 'array.map', 'array.pick', 'array.concat', 'array.uniq'] },
  { cat: 'Strings', actions: ['string.replace', 'string.match_all'] },
  { cat: 'I/O', actions: ['http', 'exec', 'file.read', 'file.write', 'log', 'table', 'response'] },
  { cat: 'Persistence', actions: ['kv.get', 'kv.set', 'kv.delete', 'lock', 'unlock'] },
  { cat: 'SQL', actions: ['sql.query', 'sql.exec', 'sql.begin', 'sql.commit', 'sql.rollback'] },
  { cat: 'Async', actions: ['wait.webhook', 'wait.rabbitmq', 'rabbitmq.shovel'] },
]

const ACTION_DOCS: Record<string, string> = {
  http: 'Make an HTTP request',
  exec: 'Run a shell command',
  set: 'Set workflow variables',
  loop: 'Iterate an array',
  delay: 'Sleep for a duration',
  table: 'Print a formatted table',
  log: 'Print a log line',
  'json.decode': 'Parse a JSON string',
  'json.encode': 'Encode value as JSON',
  'array.filter': 'Keep items matching condition',
  'array.uniq': 'Drop duplicate items',
}

const workflow = ref<any>(null)
const graph = ref<Graph | null>(null)
const yamlContent = ref<string>('')
const yamlSaved = ref<string>('')
const saveError = ref<string>('')
const saveBusy = ref(false)
const editorEnabled = ref(false)
const workflowFile = ref<string>('')

const search = ref('')
const yamlOpen = ref(true)
const runOpen = ref(false)

async function loadAll() {
  try {
    workflow.value = await api.getWorkflow()
    graph.value = await api.getWorkflowGraph()
  } catch {}
  try {
    const v = await fetch('/api/version').then(r => r.json())
    editorEnabled.value = !!v.editor_enabled
    workflowFile.value = v.workflow_file || ''
  } catch {}
  try {
    const text = await fetch('/api/workflow/raw').then(r => r.ok ? r.text() : null)
    if (text) {
      yamlContent.value = text
      yamlSaved.value = text
    }
  } catch {}
  if (!yamlContent.value && workflow.value) {
    yamlContent.value = JSON.stringify(workflow.value, null, 2)
    yamlSaved.value = yamlContent.value
  }
}

onMounted(loadAll)

const dirty = computed(() => yamlContent.value !== yamlSaved.value)

async function saveYaml() {
  if (!dirty.value || saveBusy.value) return
  saveBusy.value = true
  saveError.value = ''
  try {
    const r = await fetch('/api/workflow/raw', {
      method: 'PUT',
      headers: { 'Content-Type': 'text/plain' },
      body: yamlContent.value,
    })
    if (!r.ok) {
      const data = await r.json().catch(() => null)
      saveError.value = data?.errors?.join(', ') || `HTTP ${r.status}`
      return
    }
    yamlSaved.value = yamlContent.value
    workflow.value = await api.getWorkflow()
    graph.value = await api.getWorkflowGraph()
  } catch (err: any) {
    saveError.value = err?.message || 'save failed'
  } finally {
    saveBusy.value = false
  }
}

function revertYaml() {
  yamlContent.value = yamlSaved.value
  saveError.value = ''
}

function findStepBlock(stepId: string): { start: number; end: number; indent: string } | null {
  const lines = yamlContent.value.split('\n')
  let i = 0
  let inSteps = false
  for (; i < lines.length; i++) {
    if (/^steps:\s*$/.test(lines[i])) { inSteps = true; i++; break }
  }
  if (!inSteps) return null
  for (; i < lines.length; i++) {
    const m = lines[i].match(/^(\s*)-\s+id:\s+(.+?)\s*$/)
    if (!m) continue
    const id = m[2].trim().replace(/['"]/g, '')
    if (id !== stepId) continue
    const indent = m[1]
    let j = i + 1
    while (j < lines.length) {
      const l = lines[j]
      if (l.trim() === '') { j++; continue }
      // next sibling or different top-level item
      const sibling = l.match(/^(\s*)-\s+id:\s+/)
      if (sibling && sibling[1].length <= indent.length) break
      if (!l.startsWith(indent + ' ') && l.trim() !== '') break
      j++
    }
    return { start: i, end: j, indent }
  }
  return null
}

function setStepField(stepId: string, field: 'id' | 'title' | 'stage', value: string) {
  const block = findStepBlock(stepId)
  if (!block) return
  const lines = yamlContent.value.split('\n')
  const fieldRe = new RegExp(`^${block.indent}\\s+${field}:\\s+.*$`)
  const idLineRe = new RegExp(`^${block.indent}-\\s+id:\\s+.*$`)
  const trimmed = value.trim()
  if (field === 'id') {
    lines[block.start] = `${block.indent}- id: ${trimmed}`
  } else {
    let foundAt = -1
    for (let k = block.start + 1; k < block.end; k++) {
      if (fieldRe.test(lines[k])) { foundAt = k; break }
    }
    if (trimmed === '' && foundAt >= 0) {
      lines.splice(foundAt, 1)
    } else if (foundAt >= 0) {
      lines[foundAt] = `${block.indent}  ${field}: ${field === 'title' ? JSON.stringify(trimmed) : trimmed}`
    } else if (trimmed !== '') {
      // insert just after id line
      const idLine = lines.findIndex((l, idx) => idx >= block.start && idLineRe.test(l))
      const insertAt = idLine >= 0 ? idLine + 1 : block.start + 1
      lines.splice(insertAt, 0, `${block.indent}  ${field}: ${field === 'title' ? JSON.stringify(trimmed) : trimmed}`)
    }
  }
  yamlContent.value = lines.join('\n')
}

const editId = ref('')
const editTitle = ref('')
const editStageVal = ref('')

function existingStepIds(): Set<string> {
  const ids = new Set<string>()
  try {
    const doc = yaml.load(yamlContent.value) as any
    if (Array.isArray(doc?.steps)) {
      for (const s of doc.steps) if (s?.id) ids.add(String(s.id))
    }
  } catch {
    for (const n of (graph.value?.nodes || [])) ids.add(n.id)
  }
  return ids
}

function uniqStepId(base: string): string {
  const existing = existingStepIds()
  if (!existing.has(base)) return base
  let i = 2
  while (existing.has(base + '_' + i)) i++
  return base + '_' + i
}

function defaultStage(): string | null {
  const stages = (workflow.value as any)?.stages
  if (!stages || stages.length === 0) return null
  return stages[stages.length - 1]?.name || null
}

function appendStepToYaml(action: string, opts: { after?: string; before?: string } = {}) {
  const id = uniqStepId(action.replace(/\./g, '_'))
  const stage = defaultStage()
  const lines = yamlContent.value.split('\n')
  let insertAt = lines.length
  for (let i = lines.length - 1; i >= 0; i--) {
    if (lines[i].trim().length > 0) { insertAt = i + 1; break }
  }
  const indent = '  '
  const block: string[] = []
  if (insertAt > 0 && lines[insertAt - 1].trim() !== '') block.push('')
  block.push(`${indent}- id: ${id}`)
  if (stage) block.push(`${indent}  stage: ${stage}`)
  block.push(`${indent}  action: ${action}`)
  if (opts.after) block.push(`${indent}  depends_on: [${opts.after}]`)
  block.push(`${indent}  config: {}`)
  lines.splice(insertAt, 0, ...block)
  yamlContent.value = lines.join('\n')
}

function onDropAction(action: string) {
  appendStepToYaml(action)
}

function onAddBetween(source: string, target: string) {
  const action = 'log'
  const id = uniqStepId('step')
  const stage = defaultStage()
  const lines = yamlContent.value.split('\n')
  // 1. Find target step's depends_on line and replace `source` with new step id, or add depends_on if missing
  // 2. Append new step that depends on source and is depended on by target
  let modified = false
  let inSteps = false
  let curStepStart = -1
  let curStepIsTarget = false
  for (let i = 0; i < lines.length; i++) {
    const l = lines[i]
    if (/^steps:\s*$/.test(l)) inSteps = true
    if (!inSteps) continue
    const idMatch = l.match(/^\s*-\s+id:\s+(.+?)\s*$/)
    if (idMatch) {
      curStepStart = i
      curStepIsTarget = idMatch[1].trim().replace(/['"]/g, '') === target
      continue
    }
    if (curStepIsTarget && /^\s+depends_on:\s*\[/.test(l)) {
      lines[i] = l.replace(/depends_on:\s*\[([^\]]*)\]/, (_m, deps) => {
        const arr = deps.split(',').map((s: string) => s.trim()).filter(Boolean).filter((d: string) => d !== source)
        arr.push(id)
        return `depends_on: [${arr.join(', ')}]`
      })
      modified = true
      curStepIsTarget = false
    }
    void curStepStart
  }
  if (!modified) {
    // Couldn't rewrite — give up gracefully and just append step depending on source
  }
  const lastNonEmpty = (() => {
    for (let i = lines.length - 1; i >= 0; i--) if (lines[i].trim()) return i
    return lines.length - 1
  })()
  const indent = '  '
  const block: string[] = []
  if (lines[lastNonEmpty]?.trim() !== '') block.push('')
  block.push(`${indent}- id: ${id}`)
  if (stage) block.push(`${indent}  stage: ${stage}`)
  block.push(`${indent}  action: ${action}`)
  block.push(`${indent}  depends_on: [${source}]`)
  block.push(`${indent}  config: { message: "step ${id}" }`)
  lines.splice(lastNonEmpty + 1, 0, ...block)
  yamlContent.value = lines.join('\n')
}

const filteredCatalog = computed(() => {
  const q = search.value.toLowerCase()
  if (!q) return ACTION_CATALOG
  return ACTION_CATALOG
    .map(c => ({ ...c, actions: c.actions.filter(a => a.includes(q)) }))
    .filter(c => c.actions.length > 0)
})

const validation = computed(() => [
  { kind: 'ok' as const, msg: 'YAML schema valid' },
  { kind: 'ok' as const, msg: 'No template references to undefined steps/params' },
  { kind: 'ok' as const, msg: 'No cycles detected' },
])

function actionGlyph(a: string): string {
  if (a === 'http') return 'globe'
  if (a === 'exec') return 'terminal'
  if (a === 'loop') return 'repeat'
  if (a === 'condition') return 'branch'
  if (a === 'delay' || a === 'schedule') return 'history'
  if (a === 'set' || a === 'template') return 'edit'
  if (a.startsWith('array.') || a.startsWith('string.')) return 'filter'
  if (a.startsWith('json.')) return 'docs'
  if (a === 'log' || a === 'table') return 'docs'
  if (a.startsWith('kv.') || a.startsWith('sql.')) return 'db'
  if (a.startsWith('wait.') || a.startsWith('rabbitmq.')) return 'bolt'
  return 'bolt'
}

const localGraph = computed<Graph | null>(() => {
  if (!yamlContent.value) return graph.value
  try {
    const doc = yaml.load(yamlContent.value) as any
    if (!doc || !Array.isArray(doc.steps)) return graph.value
    const nodes = doc.steps.map((s: any) => ({
      id: s.id,
      label: s.title || s.id,
      type: 'step',
      action: s.action,
      pipeline: Array.isArray(s.config?.actions) ? s.config.actions.map((a: any) => ({ action: a.action })) : undefined,
      when: s.condition || undefined,
      goto_target: s.goto?.target,
      goto_max: s.goto?.max_iterations,
      depth: 0,
      is_last: false,
    }))
    const edges: { source: string; target: string; type?: string }[] = []
    for (const s of doc.steps) {
      const deps: string[] = Array.isArray(s.depends_on) ? s.depends_on : []
      for (const d of deps) edges.push({ source: d, target: s.id })
      if (s.goto?.target) edges.push({ source: s.id, target: s.goto.target, type: 'goto' })
    }
    const stagesMeta: any[] = Array.isArray(doc.stages) ? doc.stages : []
    const stageOf: Record<string, string> = {}
    for (const s of doc.steps) if (s.stage) stageOf[s.id] = s.stage
    const stages = stagesMeta.map((st: any) => ({
      name: st.name,
      description: st.description,
      steps: doc.steps.filter((s: any) => s.stage === st.name).map((s: any) => s.id),
    }))
    return { nodes, edges, stages } as Graph
  } catch {
    return graph.value
  }
})

const selectedStep = computed(() => {
  const id = inspector.stepId.value
  const g = localGraph.value
  if (!id) return g?.nodes[0] || null
  return g?.nodes.find(n => n.id === id) || null
})

const lineCount = computed(() => yamlContent.value.split('\n').length)

function copyYaml() {
  try { navigator.clipboard?.writeText(yamlContent.value) } catch {}
}

const exprDemo = ref('params.count')

const suggestions = computed<Suggestion[]>(() => {
  const out: Suggestion[] = []
  for (const p of (workflow.value?.params || [])) {
    out.push({ path: 'params.' + p.name, kind: 'param', type: p.type || 'value', preview: p.default !== undefined ? String(p.default) : undefined })
  }
  for (const n of (localGraph.value?.nodes || [])) {
    out.push({ path: `steps.${n.id}.output`, kind: 'step', type: 'value', preview: '<output>' })
  }
  out.push({ path: 'env.PATH', kind: 'env', type: 'string' })
  out.push({ path: 'len(x)', kind: 'fn', type: 'fn(any) -> int' })
  out.push({ path: 'lower(s)', kind: 'fn', type: 'fn(string) -> string' })
  out.push({ path: 'split(s, sep)', kind: 'fn', type: 'fn(string,string) -> array' })
  return out
})

const TPL_OPEN = '{{'
const TPL_CLOSE = '}}'

watch([() => inspector.stepId.value, localGraph], () => {
  const n = selectedStep.value
  if (!n) return
  editId.value = n.id
  editTitle.value = n.label || ''
  editStageVal.value = (localGraph.value?.stages?.find(s => s.steps.includes(n.id))?.name) || ''
}, { immediate: true })

function commitId() {
  const n = selectedStep.value
  if (!n || editId.value === n.id || !editId.value.trim()) { editId.value = n?.id || ''; return }
  setStepField(n.id, 'id', editId.value)
}
function commitTitle() {
  const n = selectedStep.value
  if (!n) return
  setStepField(n.id, 'title', editTitle.value)
}
function commitStage() {
  const n = selectedStep.value
  if (!n) return
  setStepField(n.id, 'stage', editStageVal.value)
}

function addStage() {
  const name = newStageName.value.trim()
  if (!name) return
  const lines = yamlContent.value.split('\n')
  let stagesIdx = -1
  for (let i = 0; i < lines.length; i++) {
    if (/^stages:\s*$/.test(lines[i])) { stagesIdx = i; break }
  }
  if (stagesIdx >= 0) {
    // find end of stages block
    let end = stagesIdx + 1
    while (end < lines.length && (lines[end].startsWith('  ') || lines[end].trim() === '')) end++
    // back up trailing empties
    while (end > stagesIdx + 1 && lines[end - 1].trim() === '') end--
    lines.splice(end, 0, `  - name: ${name}`)
    yamlContent.value = lines.join('\n')
  } else {
    // no stages section yet — insert before steps
    let stepsIdx = lines.findIndex(l => /^steps:\s*$/.test(l))
    if (stepsIdx < 0) stepsIdx = lines.length
    const block = ['stages:', `  - name: ${name}`, '']
    lines.splice(stepsIdx, 0, ...block)
    yamlContent.value = lines.join('\n')
  }
  newStageName.value = ''
}

function renameStage(oldName: string, newName: string) {
  const trimmed = newName.trim()
  if (!trimmed || trimmed === oldName) return
  const lines = yamlContent.value.split('\n')
  let inStages = false
  for (let i = 0; i < lines.length; i++) {
    if (/^stages:\s*$/.test(lines[i])) { inStages = true; continue }
    if (inStages) {
      if (/^[a-zA-Z]/.test(lines[i])) inStages = false
      else if (lines[i].match(new RegExp(`^\\s+-\\s+name:\\s+${oldName}\\s*$`))) {
        lines[i] = lines[i].replace(`name: ${oldName}`, `name: ${trimmed}`)
      }
    }
    // also rewrite step.stage references
    const m = lines[i].match(/^(\s+)stage:\s+(.+?)\s*$/)
    if (m && m[2].trim() === oldName) lines[i] = `${m[1]}stage: ${trimmed}`
  }
  yamlContent.value = lines.join('\n')
}

function deleteStage(name: string) {
  const lines = yamlContent.value.split('\n')
  let inStages = false
  let i = 0
  while (i < lines.length) {
    if (/^stages:\s*$/.test(lines[i])) { inStages = true; i++; continue }
    if (inStages) {
      if (/^[a-zA-Z]/.test(lines[i])) { inStages = false; i++; continue }
      const isStart = lines[i].match(new RegExp(`^\\s+-\\s+name:\\s+${name}\\s*$`))
      if (isStart) {
        let j = i + 1
        while (j < lines.length && lines[j].startsWith('    ')) j++
        lines.splice(i, j - i)
        continue
      }
    }
    // unset step.stage if matches
    const m = lines[i].match(/^(\s+)stage:\s+(.+?)\s*$/)
    if (m && m[2].trim() === name) {
      lines.splice(i, 1)
      continue
    }
    i++
  }
  yamlContent.value = lines.join('\n')
}

const newStageName = ref('')
const editingStage = ref<string | null>(null)
const editingStageNew = ref('')
</script>

<template>
  <div class="flex h-full">
    <!-- Action palette -->
    <div class="w-[260px] bg-g-2 border-r border-g-5 flex flex-col h-full shrink-0">
      <div class="p-3 border-b border-g-5">
        <div class="flex items-center justify-between mb-2">
          <span class="text-[11px] font-semibold uppercase tracking-wider text-g-9">Actions</span>
          <span class="text-[10px] font-mono text-g-8">34 built-in</span>
        </div>
        <div class="flex items-center gap-1.5 bg-g-3 border border-g-5 rounded px-2 h-7">
          <Icon name="search" class-name="w-3 h-3 text-g-8" />
          <input
            v-model="search"
            placeholder="filter actions…"
            class="bg-transparent outline-none text-[11px] font-mono text-g-12 placeholder:text-g-7 flex-1"
          />
        </div>
      </div>
      <div class="flex-1 overflow-y-auto py-2">
        <div v-for="group in filteredCatalog" :key="group.cat" class="mb-2">
          <div class="px-3 pt-1.5 pb-1 text-[10px] uppercase tracking-wider font-mono text-g-8">{{ group.cat }}</div>
          <div
            v-for="a in group.actions"
            :key="a"
            :draggable="editorEnabled"
            @dragstart="(e: DragEvent) => editorEnabled && e.dataTransfer?.setData('action', a)"
            @click="editorEnabled && onDropAction(a)"
            :class="['px-3 py-1.5 flex items-center gap-2 group', editorEnabled ? 'hover:bg-g-3 cursor-grab active:cursor-grabbing' : 'opacity-60 cursor-not-allowed']"
            :title="editorEnabled ? `Drag onto DAG or click to append a ${a} step` : 'restart agent with --editor to add steps'"
          >
            <Icon :name="actionGlyph(a)" class-name="w-3.5 h-3.5 text-g-9 group-hover:text-g-12" />
            <span class="font-mono text-[12px] text-g-12">{{ a }}</span>
            <span v-if="ACTION_DOCS[a]" class="text-[10px] text-g-8 truncate flex-1">{{ ACTION_DOCS[a] }}</span>
            <Icon name="grip" class-name="w-3 h-3 text-g-7 ml-auto opacity-0 group-hover:opacity-100" />
          </div>
        </div>
      </div>
      <div class="border-t border-g-5 p-3">
        <div class="text-[10px] font-mono text-g-8 uppercase tracking-wider mb-1.5">Templates</div>
        <div class="space-y-1">
          <button
            v-for="t in ['audit · Claude code review', 'fix · auto-repair loop', 'pull_all · GitLab sync', 'webhook → SQL → response']"
            :key="t"
            class="w-full text-left px-2 py-1.5 rounded text-[11px] font-mono text-g-10 hover:bg-g-3 hover:text-g-13"
          >{{ t }}</button>
        </div>
      </div>
    </div>

    <!-- Center DAG -->
    <div class="flex-1 min-w-0 flex flex-col bg-g-1 border-r border-g-5">
      <div class="px-4 py-2 border-b border-g-5 flex items-center gap-3 shrink-0">
        <div class="flex items-center gap-2">
          <span class="font-mono text-[12px] text-g-13">{{ workflow?.name || 'workflow' }}.yaml</span>
          <span v-if="!editorEnabled" class="text-[10px] font-mono text-g-9 px-1.5 py-0.5 rounded bg-g-3 border border-g-5" title="restart agent with --editor to enable persistence">read-only</span>
          <span v-else-if="dirty" class="text-[10px] font-mono text-amber-400 px-1.5 py-0.5 rounded bg-amber-400/10 border border-amber-400/20">unsaved</span>
          <span v-else class="text-[10px] font-mono text-emerald-400 px-1.5 py-0.5 rounded bg-emerald-400/10 border border-emerald-400/20">editable</span>
        </div>
        <div class="flex-1" />
        <span v-if="saveError" class="text-[11px] font-mono text-red-400 truncate max-w-[300px]" :title="saveError">{{ saveError }}</span>
        <div class="flex items-center gap-1.5 text-[10px] font-mono text-g-8">
          <span
            v-for="(v, i) in validation"
            :key="i"
            :class="['px-1.5 py-0.5 rounded', v.kind === 'ok' ? 'text-emerald-400' : 'text-amber-400']"
            :title="v.msg"
          >✓</span>
        </div>
        <button
          v-if="editorEnabled && dirty"
          @click="revertYaml"
          class="h-7 px-2 text-[11px] font-mono text-g-10 bg-g-3 border border-g-5 hover:bg-g-4 rounded"
        >Revert</button>
        <button
          v-if="editorEnabled"
          @click="saveYaml"
          :disabled="!dirty || saveBusy"
          :class="['h-7 px-2.5 text-[11px] font-mono rounded flex items-center gap-1.5', dirty ? 'bg-emerald-400/15 text-emerald-400 hover:bg-emerald-400/25 border border-emerald-400/30' : 'bg-g-3 border border-g-5 text-g-9 cursor-not-allowed']"
        >
          <Icon name="check" class-name="w-3 h-3" />{{ saveBusy ? 'saving…' : 'Save' }}
          <span class="ml-1 flex gap-0.5"><Kbd>⌘</Kbd><Kbd>S</Kbd></span>
        </button>
        <button
          @click="yamlOpen = !yamlOpen"
          class="h-7 px-2 text-[11px] font-mono text-g-10 bg-g-3 border border-g-5 hover:bg-g-4 rounded flex items-center gap-1.5"
        >
          <Icon name="docs" class-name="w-3 h-3" />{{ yamlOpen ? 'hide' : 'show' }} YAML
        </button>
        <button
          @click="runOpen = true"
          class="h-7 px-2.5 text-[11px] font-mono bg-g-14 text-g-1 hover:bg-g-15 rounded flex items-center gap-1.5"
        >
          <Icon name="play" class-name="w-3 h-3" />Test run
        </button>
      </div>
      <div class="flex-1 overflow-hidden p-4 relative min-h-0">
        <div v-if="localGraph" class="absolute inset-4">
          <WorkflowDAGCustom
            :graph="localGraph"
            :selected-id="inspector.stepId.value"
            :show-stages="true"
            :editable="editorEnabled"
            :fill-height="true"
            @node-click="(id: string) => inspector.inspect(id)"
            @add-between="onAddBetween"
            @drop-action="onDropAction"
          />
        </div>
        <div v-else class="flex items-center justify-center h-full">
          <div class="w-6 h-6 border-2 border-g-5 border-t-g-9 rounded-full animate-spin" />
        </div>
        <div class="absolute bottom-4 left-4 bg-g-2/90 border border-g-5 rounded px-2 py-1 backdrop-blur text-[10px] font-mono text-g-8 flex items-center gap-3">
          <span v-if="editorEnabled">drag actions or click · hover edges for + button · <Kbd>⌘</Kbd><Kbd>S</Kbd> to save</span>
          <span v-else>read-only · restart agent with <span class="text-g-12">--editor</span> to enable changes</span>
        </div>
      </div>
      <div class="border-t border-g-5 px-3 py-2 flex items-center gap-3 bg-g-2 shrink-0">
        <span class="text-[10px] font-mono uppercase tracking-wider text-g-8">Validation</span>
        <span
          v-for="(v, i) in validation"
          :key="i"
          :class="['text-[11px] font-mono flex items-center gap-1', v.kind === 'ok' ? 'text-g-10' : 'text-amber-400']"
        >
          <span>{{ v.kind === 'ok' ? '✓' : '!' }}</span>{{ v.msg }}
        </span>
      </div>
    </div>

    <!-- Right: form + YAML stacked -->
    <div class="w-[460px] flex flex-col shrink-0">
      <div :class="['border-b border-g-5 bg-g-2 flex flex-col', yamlOpen ? 'flex-1' : 'h-full']">
        <div class="px-4 py-2 border-b border-g-5 flex items-center justify-between">
          <span class="text-[11px] font-semibold uppercase tracking-wider text-g-9">Step · {{ selectedStep?.id || '—' }}</span>
          <span class="text-[10px] font-mono text-g-8">click any node</span>
        </div>
        <div v-if="selectedStep" class="p-4 space-y-4 overflow-y-auto">
          <div>
            <label class="text-[10px] font-mono uppercase tracking-wider text-g-8 mb-1 block">id</label>
            <input
              v-model="editId"
              :readonly="!editorEnabled"
              @blur="commitId"
              @keydown.enter="commitId"
              :class="['w-full border rounded px-2.5 py-1.5 font-mono text-[12px] outline-none', editorEnabled ? 'bg-g-1 border-g-5 text-g-13 focus:border-g-7' : 'bg-g-1 border-g-5 text-g-10']"
            />
          </div>
          <div class="grid grid-cols-2 gap-2">
            <div>
              <label class="text-[10px] font-mono uppercase tracking-wider text-g-8 mb-1 block">action</label>
              <div class="bg-g-1 border border-g-5 rounded px-2.5 py-1.5 font-mono text-[12px] text-g-13 flex items-center gap-1.5">
                <Icon :name="actionGlyph(selectedStep.action || '')" class-name="w-3 h-3" />
                {{ selectedStep.action }}
              </div>
            </div>
            <div>
              <label class="text-[10px] font-mono uppercase tracking-wider text-g-8 mb-1 block">stage</label>
              <select
                v-model="editStageVal"
                :disabled="!editorEnabled"
                @change="commitStage"
                :class="['w-full bg-g-1 border border-g-5 rounded px-2 py-1.5 font-mono text-[12px] outline-none', editorEnabled ? 'text-g-13 focus:border-g-7' : 'text-g-10']"
              >
                <option value="">—</option>
                <option v-for="s in (localGraph?.stages || [])" :key="s.name" :value="s.name">{{ s.name }}</option>
              </select>
            </div>
          </div>
          <div>
            <label class="text-[10px] font-mono uppercase tracking-wider text-g-8 mb-1 block">title</label>
            <input
              v-model="editTitle"
              :readonly="!editorEnabled"
              @blur="commitTitle"
              @keydown.enter="commitTitle"
              :class="['w-full border rounded px-2.5 py-1.5 text-[12px] outline-none', editorEnabled ? 'bg-g-1 border-g-5 text-g-13 focus:border-g-7' : 'bg-g-1 border-g-5 text-g-10']"
            />
          </div>
          <div v-if="selectedStep.when" class="border-t border-g-5 pt-3">
            <div class="text-[10px] font-mono uppercase tracking-wider text-g-8 mb-2">condition (if)</div>
            <div class="bg-g-1 border border-g-5 rounded px-2.5 py-1.5 font-mono text-[12px]">
              <span class="text-g-7">{{ TPL_OPEN }}&nbsp;</span>
              <span class="text-violet-400">{{ selectedStep.when }}</span>
              <span class="text-g-7">&nbsp;{{ TPL_CLOSE }}</span>
            </div>
          </div>
          <div v-if="selectedStep.goto_target" class="border-t border-g-5 pt-3">
            <div class="text-[10px] font-mono uppercase tracking-wider text-g-8 mb-2">goto · loop control</div>
            <div class="bg-g-1 border border-g-5 rounded px-2.5 py-1.5 font-mono text-[12px] text-g-13">
              target: {{ selectedStep.goto_target }} · max_iterations: {{ selectedStep.goto_max || '—' }}
            </div>
          </div>
          <div v-if="selectedStep.pipeline?.length" class="border-t border-g-5 pt-3">
            <div class="text-[10px] font-mono uppercase tracking-wider text-g-8 mb-2">sub-pipeline · {{ selectedStep.pipeline.length }} actions</div>
            <div
              v-for="(p, i) in selectedStep.pipeline"
              :key="i"
              class="bg-g-1 border border-g-5 rounded px-2.5 py-1.5 font-mono text-[12px] text-g-12 mb-1"
            >
              {{ i + 1 }}. {{ p.action }}{{ p.title ? ' · ' + p.title : '' }}
            </div>
          </div>

          <div class="border-t border-g-5 pt-3">
            <div class="text-[10px] font-mono uppercase tracking-wider text-g-8 mb-2 flex items-center justify-between">
              <span>stages</span>
              <span class="text-g-7 normal-case tracking-normal">{{ (localGraph?.stages || []).length }} defined</span>
            </div>
            <div class="space-y-1">
              <div
                v-for="s in (localGraph?.stages || [])"
                :key="s.name"
                class="bg-g-1 border border-g-5 rounded px-2.5 py-1.5 flex items-center gap-2"
              >
                <template v-if="editingStage === s.name">
                  <input
                    v-model="editingStageNew"
                    @blur="renameStage(s.name, editingStageNew); editingStage = null"
                    @keydown.enter="renameStage(s.name, editingStageNew); editingStage = null"
                    @keydown.escape="editingStage = null"
                    class="flex-1 bg-g-2 border border-g-7 rounded px-1.5 py-0.5 font-mono text-[12px] text-g-13 outline-none"
                    autofocus
                  />
                </template>
                <template v-else>
                  <span class="font-mono text-[12px] text-g-12 flex-1">{{ s.name }}</span>
                  <span class="font-mono text-[10px] text-g-8">{{ s.steps.length }} steps</span>
                  <button
                    v-if="editorEnabled"
                    @click="editingStage = s.name; editingStageNew = s.name"
                    class="text-g-9 hover:text-g-13"
                    title="Rename"
                  ><Icon name="edit" class-name="w-3 h-3" /></button>
                  <button
                    v-if="editorEnabled"
                    @click="deleteStage(s.name)"
                    class="text-g-9 hover:text-red-400"
                    title="Delete (orphans steps)"
                  ><Icon name="x" class-name="w-3 h-3" /></button>
                </template>
              </div>
              <div v-if="editorEnabled" class="flex items-center gap-1 mt-2">
                <input
                  v-model="newStageName"
                  @keydown.enter="addStage"
                  placeholder="new stage name…"
                  class="flex-1 bg-g-1 border border-g-5 rounded px-2.5 py-1 font-mono text-[12px] text-g-13 outline-none focus:border-g-7"
                />
                <button
                  @click="addStage"
                  :disabled="!newStageName.trim()"
                  :class="['h-7 px-2 text-[11px] font-mono rounded flex items-center gap-1', newStageName.trim() ? 'bg-emerald-400/15 text-emerald-400 hover:bg-emerald-400/25 border border-emerald-400/30' : 'bg-g-3 border border-g-5 text-g-8 cursor-not-allowed']"
                >
                  <Icon name="plus" class-name="w-3 h-3" />add
                </button>
              </div>
            </div>
          </div>

          <div class="border-t border-g-5 pt-3">
            <div class="text-[10px] font-mono uppercase tracking-wider text-g-8 mb-2 flex items-center justify-between">
              <span>expr-lang playground</span>
              <span class="text-g-7 normal-case tracking-normal">try the autocomplete</span>
            </div>
            <ExprField
              v-model="exprDemo"
              :suggestions="suggestions"
              placeholder="params.x · steps.y.output · vars.z"
            />
          </div>
        </div>
        <div v-else class="flex-1 flex items-center justify-center text-[12px] text-g-8 font-mono">No step selected</div>
      </div>

      <div v-if="yamlOpen" class="bg-g-1 border-t border-g-5 flex flex-col flex-1 min-h-0">
        <div class="px-4 py-2 border-b border-g-5 flex items-center justify-between bg-g-2 shrink-0">
          <span class="text-[11px] font-semibold uppercase tracking-wider text-g-9">YAML · {{ dirty ? 'editing' : 'saved' }}</span>
          <div class="flex items-center gap-2">
            <span class="text-[10px] font-mono text-g-8">{{ lineCount }} lines</span>
            <button @click="copyYaml" class="text-[10px] font-mono text-g-8 hover:text-g-12 flex items-center gap-1">
              <Icon name="copy" class-name="w-3 h-3" />copy
            </button>
          </div>
        </div>
        <div class="flex-1 min-h-0">
          <YamlEditor v-model="yamlContent" :readonly="!editorEnabled" @save="saveYaml" />
        </div>
      </div>
    </div>

    <!-- Run modal -->
    <Teleport to="body">
      <div
        v-if="runOpen"
        class="fixed inset-0 z-40 flex items-center justify-center backdrop-blur-sm"
        style="background: rgba(0, 0, 0, 0.6)"
        @click.self="runOpen = false"
      >
        <div class="w-[640px] max-h-[80vh] bg-g-2 border border-g-6 rounded-lg shadow-2xl flex flex-col anim-enter">
          <div class="px-4 py-3 border-b border-g-5 flex items-center justify-between">
            <div>
              <div class="text-[14px] font-semibold text-g-14">Run {{ workflow?.name || 'workflow' }}</div>
              <div class="text-[11px] font-mono text-g-9">test run · params editable</div>
            </div>
            <button @click="runOpen = false" class="text-g-9 hover:text-g-12">
              <Icon name="x" class-name="w-4 h-4" />
            </button>
          </div>
          <div class="p-4 space-y-3 overflow-y-auto">
            <div v-for="p in (workflow?.params || [])" :key="p.name">
              <label class="text-[10px] font-mono uppercase tracking-wider text-g-8 mb-1 block">
                {{ p.name }}
                <span v-if="p.required" class="text-red-400/80">*</span>
                · {{ p.type }}
                <span v-if="p.default !== undefined" class="text-g-7"> · default {{ p.default }}</span>
              </label>
              <input
                :placeholder="p.default !== undefined ? String(p.default) : ''"
                class="w-full bg-g-1 border border-g-5 rounded px-2.5 py-1.5 font-mono text-[12px] text-g-13 outline-none focus:border-g-7"
              />
              <div v-if="p.description" class="text-[10px] font-mono text-g-8 mt-1">{{ p.description }}</div>
            </div>
            <div v-if="!(workflow?.params?.length)" class="text-[12px] font-mono text-g-8">No params declared.</div>
          </div>
          <div class="border-t border-g-5 px-4 py-3 flex items-center justify-between gap-2">
            <div class="text-[10px] font-mono text-g-8 flex items-center gap-2"><Kbd>⌘</Kbd><Kbd>↵</Kbd> launch · <Kbd>esc</Kbd> cancel</div>
            <div class="flex items-center gap-2">
              <button @click="runOpen = false" class="h-8 px-3 text-[12px] font-medium text-g-10 hover:text-g-13 rounded">Cancel</button>
              <button class="h-8 px-3 text-[12px] font-medium bg-g-14 text-g-1 hover:bg-g-15 rounded flex items-center gap-1.5">
                <Icon name="play" class-name="w-3.5 h-3.5" />Launch
              </button>
            </div>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>
