<script setup lang="ts">
import { onMounted, onBeforeUnmount, ref } from 'vue'
import Icon from '@/components/primitives/Icon.vue'

interface DocItem { id: string; l: string }
interface DocSection { section: string; items: DocItem[] }

const DOCS_NAV: DocSection[] = [
  { section: 'Getting started', items: [
    { id: 'overview', l: 'Overview' },
    { id: 'install', l: 'Install the agent' },
    { id: 'first-workflow', l: 'Your first workflow' },
    { id: 'cli', l: 'CLI reference' },
  ]},
  { section: 'Workflow model', items: [
    { id: 'yaml', l: 'YAML schema' },
    { id: 'params', l: 'Params & env' },
    { id: 'stages', l: 'Stages' },
    { id: 'steps', l: 'Steps & deps' },
    { id: 'goto', l: 'goto & loops' },
    { id: 'recovery', l: 'Recovery' },
  ]},
  { section: 'Templating', items: [
    { id: 'expr', l: 'expr-lang basics' },
    { id: 'scope', l: 'Available scope' },
    { id: 'operators', l: 'Operators & helpers' },
  ]},
  { section: 'Actions', items: [
    { id: 'actions', l: 'All 34 actions' },
    { id: 'http', l: 'http' },
    { id: 'exec', l: 'exec' },
    { id: 'loop', l: 'loop' },
    { id: 'kv', l: 'kv.*' },
  ]},
  { section: 'Operating', items: [
    { id: 'monitoring', l: 'Monitoring & metrics' },
    { id: 'sse', l: 'SSE event stream' },
    { id: 'security', l: 'Security model' },
  ]},
]

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

const active = ref('overview')
const containerRef = ref<HTMLElement | null>(null)
let suppressScrollUntil = 0

const onThisPage = DOCS_NAV.flatMap(g => g.items)

function jumpTo(id: string) {
  active.value = id
  const el = document.getElementById('doc-' + id)
  if (el && containerRef.value) {
    suppressScrollUntil = Date.now() + 700
    containerRef.value.scrollTo({ top: el.offsetTop - 24, behavior: 'smooth' })
  }
}

function onScroll() {
  if (Date.now() < suppressScrollUntil) return
  const c = containerRef.value
  if (!c) return
  const all = onThisPage
  let cur = all[0].id
  for (const it of all) {
    const el = document.getElementById('doc-' + it.id)
    if (el && el.offsetTop - c.scrollTop < 80) cur = it.id
  }
  active.value = cur
}

onMounted(() => {
  containerRef.value?.addEventListener('scroll', onScroll, { passive: true })
})
onBeforeUnmount(() => {
  containerRef.value?.removeEventListener('scroll', onScroll)
})

function copy(text: string) {
  try { navigator.clipboard?.writeText(text) } catch {}
}

const TPL_STEPS = '{{ steps.* }}'
const TPL_BRACES = '{{ }}'

const codeInstall = `# macOS / Linux
curl -fsSL https://tailflow.dev/install.sh | sh

# or via brew
brew install tailflow/tap/tailflow

# verify
tailflow --version`

const codeFirst = `version: "2.0"
name: "hello"
description: "ping a service and log the result"

params:
  - name: target
    type: string
    default: "https://api.tailflow.dev"

steps:
  - id: ping
    action: http
    config:
      method: GET
      url: "{{ params.target }}/health"

  - id: notify
    action: log
    depends_on: [ping]
    config:
      message: "got {{ steps.ping.output.status }} from {{ params.target }}"`

const codeRun = `tailflow run hello.yaml --param target=https://example.com
# →  http://localhost:8080  (live UI, auto-opens)`

const codeStages = `stages:
  - name: discover
    description: "Fetch project list from GitLab"
  - name: prepare
    description: "Filter and dedupe"
  - name: sync
    description: "Clone or pull each repo"
  - name: report`

const codeSteps = `steps:
  - id: scan
    stage: analyze
    action: exec
    title: "Scan with Claude"
    timeout: "600s"
    retry: { max_attempts: 3, delay: "2s" }
    on_recovery: skip
    config:
      command: ["sh", "-c", "claude -p ./.claude/audit"]

  - id: parse
    action: json.decode
    depends_on: [scan]
    config:
      input: "{{ steps.scan.output.stdout }}"`

const codeLoop = `# loop action — parallel
- id: clone-or-pull
  action: loop
  config:
    array: "{{ steps.dedupe.output }}"
    as: repo
    concurrency: 5
    error_policy: continue
    actions:
      - action: exec
        config:
          command: ["sh", "-c", "git clone {{ loop.repo.ssh_url }}"]

# goto — conditional re-entry (pagination, polling)
- id: page-cursor
  action: set
  goto:
    target: list-page
    when: "len(steps.list-page.output.body) >= 100"
    max_iterations: 50`

const codeExpr = `{{ params.target }}
{{ steps.ping.output.status == 200 }}
{{ vars.attempt + 1 }}
{{ env.GITLAB_TOKEN ?? "" }}
{{ len(steps.list-page.output.body) >= 100 }}
{{ lower(loop.repo.path) }}`

const codeKv = `- action: kv.set
  config: { key: "last_seen", value: "{{ now() }}" }

- action: kv.get
  config: { key: "last_seen" }

- action: kv.delete
  config: { key: "last_seen" }`

const codeSse = `curl -N localhost:8080/events
# event: step.start
# data: {"run":"exe_8f3c","step":"list-page","at":"2026-04-30T12:04:23Z"}

# event: step.success
# data: {"run":"exe_8f3c","step":"list-page","duration_ms":1240}`

const codeHealth = `curl localhost:8080/healthz | jq
# {
#   "status": "healthy",
#   "uptime_s": 14823,
#   "goroutines": 87,
#   "mem_alloc_bytes": 142000000,
#   "active_runs": 2
# }`

const codeHttp = `- id: list-page
  action: http
  retry: { max_attempts: 5, delay: "2s" }
  config:
    method: GET
    url: "https://{{ env.GITLAB_HOST }}/api/v4/groups/{{ params.group }}/projects"
    headers:
      PRIVATE-TOKEN: "{{ env.GITLAB_TOKEN }}"
    query:
      page: "{{ vars.page ?? 1 }}"`
</script>

<template>
  <div class="flex h-full">
    <!-- TOC -->
    <aside class="w-[220px] shrink-0 border-r border-g-5 bg-g-1 overflow-y-auto py-6 px-3">
      <div class="px-2 mb-4">
        <div class="text-[11px] font-semibold text-g-13">Documentation</div>
        <div class="text-[10px] font-mono text-g-8 mt-0.5">agent</div>
      </div>
      <div v-for="g in DOCS_NAV" :key="g.section" class="mb-5">
        <div class="text-[10px] font-mono uppercase tracking-wider text-g-8 px-2 mb-1.5">{{ g.section }}</div>
        <button
          v-for="it in g.items"
          :key="it.id"
          @click="jumpTo(it.id)"
          :class="['w-full text-left px-2 py-1 rounded text-[12px]', active === it.id ? 'bg-g-3 text-g-14' : 'text-g-10 hover:text-g-13 hover:bg-g-2']"
        >{{ it.l }}</button>
      </div>
    </aside>

    <!-- Content -->
    <div ref="containerRef" class="flex-1 overflow-y-auto">
      <div class="max-w-[820px] mx-auto px-10 py-10">
        <!-- Header -->
        <div class="mb-8 pb-6 border-b border-g-5">
          <div class="flex items-center gap-2 mb-2">
            <span class="text-[10px] font-mono uppercase tracking-wider text-g-8">getting started</span>
            <span class="text-g-7 text-[10px]">·</span>
            <span class="text-[10px] font-mono text-g-8">read time: 6 min</span>
          </div>
          <h1 class="text-[28px] font-semibold text-g-14 tracking-tight mb-2">Tailflow agent</h1>
          <p class="text-[14px] text-g-10 leading-relaxed max-w-[640px]">
            Define your backend logic in YAML. The agent runs it. One binary, zero infrastructure, 34 built-in actions, recovery, templating with expr-lang.
          </p>
          <div class="flex items-center gap-2 mt-4">
            <button @click="jumpTo('install')" class="h-8 px-3 rounded text-[12px] font-medium bg-g-14 text-g-1 hover:bg-g-15">Install</button>
            <button @click="jumpTo('first-workflow')" class="h-8 px-3 rounded text-[12px] font-medium bg-g-3 border border-g-5 text-g-12 hover:bg-g-4">First workflow →</button>
          </div>
        </div>

        <!-- Overview -->
        <section id="doc-overview" class="scroll-mt-16 mb-12">
          <div class="flex items-baseline gap-3 mb-2">
            <h2 class="text-[20px] font-semibold text-g-14 tracking-tight">Overview</h2>
            <a href="#doc-overview" class="text-g-8 hover:text-g-12 font-mono text-[11px]">#</a>
          </div>
          <p class="text-[13px] text-g-10 leading-relaxed mb-4 max-w-[640px]">Tailflow turns a YAML file into an executable workflow with stages, dependencies, retries, templating, and live observability.</p>
          <div class="grid grid-cols-3 gap-3">
            <div class="bg-g-2 border border-g-5 rounded p-3">
              <Icon name="cpu" class-name="w-4 h-4 text-g-9 mb-2" />
              <div class="text-[13px] font-semibold text-g-13">Single binary</div>
              <div class="text-[11px] text-g-9 font-mono mt-0.5">~14 MB · zero deps</div>
            </div>
            <div class="bg-g-2 border border-g-5 rounded p-3">
              <Icon name="bolt" class-name="w-4 h-4 text-g-9 mb-2" />
              <div class="text-[13px] font-semibold text-g-13">34 actions</div>
              <div class="text-[11px] text-g-9 font-mono mt-0.5">http, exec, sql, kv…</div>
            </div>
            <div class="bg-g-2 border border-g-5 rounded p-3">
              <Icon name="refresh" class-name="w-4 h-4 text-g-9 mb-2" />
              <div class="text-[13px] font-semibold text-g-13">Recovery</div>
              <div class="text-[11px] text-g-9 font-mono mt-0.5">crash-safe runs</div>
            </div>
          </div>
        </section>

        <!-- Install -->
        <section id="doc-install" class="scroll-mt-16 mb-12">
          <div class="flex items-baseline gap-3 mb-2">
            <h2 class="text-[20px] font-semibold text-g-14 tracking-tight">Install the agent</h2>
            <a href="#doc-install" class="text-g-8 hover:text-g-12 font-mono text-[11px]">#</a>
          </div>
          <p class="text-[13px] text-g-10 leading-relaxed mb-4 max-w-[640px]">Download a single binary for macOS, Linux or Windows. No background daemon, no database, nothing to provision.</p>
          <pre class="bg-g-2 border border-g-5 rounded p-3 font-mono text-[12px] leading-relaxed overflow-x-auto">
            <div class="flex items-center justify-between mb-2 -mt-1">
              <span class="text-[10px] font-mono text-g-8 uppercase tracking-wider">bash</span>
              <button @click="copy(codeInstall)" class="text-[10px] font-mono text-g-8 hover:text-g-12 flex items-center gap-1"><Icon name="copy" class-name="w-3 h-3" />copy</button>
            </div><code class="text-g-12">{{ codeInstall }}</code>
          </pre>
          <div class="bg-g-2 border border-g-5 rounded p-3 flex gap-2.5 items-start mt-3">
            <span class="bg-g-9 w-1.5 h-1.5 rounded-full mt-1.5 shrink-0" />
            <div class="flex-1">
              <div class="text-[12px] font-mono uppercase tracking-wider mb-1 text-g-12">binary location</div>
              <div class="text-[13px] text-g-11 leading-relaxed">The installer drops <code class="font-mono text-g-13 bg-g-3 px-1 rounded">tailflow</code> in <code class="font-mono text-g-13 bg-g-3 px-1 rounded">/usr/local/bin</code>.</div>
            </div>
          </div>
        </section>

        <!-- First workflow -->
        <section id="doc-first-workflow" class="scroll-mt-16 mb-12">
          <div class="flex items-baseline gap-3 mb-2">
            <h2 class="text-[20px] font-semibold text-g-14 tracking-tight">Your first workflow</h2>
          </div>
          <p class="text-[13px] text-g-10 leading-relaxed mb-4 max-w-[640px]">A workflow is a YAML file. The agent reads it, builds the DAG, then serves a web UI for the run.</p>
          <pre class="bg-g-2 border border-g-5 rounded p-3 font-mono text-[12px] leading-relaxed overflow-x-auto">
            <div class="flex items-center justify-between mb-2 -mt-1">
              <span class="text-[10px] font-mono text-g-8 uppercase tracking-wider">yaml</span>
              <button @click="copy(codeFirst)" class="text-[10px] font-mono text-g-8 hover:text-g-12 flex items-center gap-1"><Icon name="copy" class-name="w-3 h-3" />copy</button>
            </div><code class="text-g-12">{{ codeFirst }}</code>
          </pre>
          <p class="text-[13px] text-g-11 leading-relaxed mt-4">Run it:</p>
          <pre class="bg-g-2 border border-g-5 rounded p-3 font-mono text-[12px] leading-relaxed overflow-x-auto mt-2">
            <div class="flex items-center justify-between mb-2 -mt-1">
              <span class="text-[10px] font-mono text-g-8 uppercase tracking-wider">bash</span>
              <button @click="copy(codeRun)" class="text-[10px] font-mono text-g-8 hover:text-g-12 flex items-center gap-1"><Icon name="copy" class-name="w-3 h-3" />copy</button>
            </div><code class="text-g-12">{{ codeRun }}</code>
          </pre>
        </section>

        <!-- CLI -->
        <section id="doc-cli" class="scroll-mt-16 mb-12">
          <div class="flex items-baseline gap-3 mb-2">
            <h2 class="text-[20px] font-semibold text-g-14 tracking-tight">CLI reference</h2>
          </div>
          <p class="text-[13px] text-g-10 leading-relaxed mb-4 max-w-[640px]">The agent is a single CLI.</p>
          <div class="bg-g-2 border border-g-5 rounded overflow-hidden">
            <div class="grid grid-cols-[200px_120px_60px_1fr] gap-3 px-3 py-2 text-[10px] uppercase tracking-wider text-g-8 font-mono">
              <div>name</div><div>type</div><div>req</div><div>description</div>
            </div>
            <div v-for="(r, i) in [
              { name: 'tailflow run <file>', type: 'command', req: true, desc: 'Run a workflow once. Opens the web UI on :8080.' },
              { name: 'tailflow validate <file>', type: 'command', req: false, desc: 'Static-validate the YAML against the schema.' },
              { name: 'tailflow serve <file>', type: 'command', req: false, desc: 'Serve the UI without running. Useful for editing.' },
              { name: '--port', type: 'flag', req: false, desc: 'Override the HTTP port (default :8080).' },
              { name: '--selfhosted', type: 'flag', req: false, desc: 'Enable exec, js, file.* actions for self-hosted deployments.' },
            ]" :key="i" class="grid grid-cols-[200px_120px_60px_1fr] gap-3 px-3 py-2 border-t border-g-4 items-start">
              <div class="font-mono text-[12px] text-g-13">{{ r.name }}</div>
              <div class="font-mono text-[11px] text-violet-400">{{ r.type }}</div>
              <div class="font-mono text-[11px] text-g-9">{{ r.req ? 'yes' : 'no' }}</div>
              <div class="text-[12px] text-g-10 leading-relaxed">{{ r.desc }}</div>
            </div>
          </div>
        </section>

        <!-- YAML schema -->
        <section id="doc-yaml" class="scroll-mt-16 mb-12">
          <div class="flex items-baseline gap-3 mb-2">
            <h2 class="text-[20px] font-semibold text-g-14 tracking-tight">YAML schema</h2>
          </div>
          <p class="text-[13px] text-g-10 leading-relaxed mb-4 max-w-[640px]">A workflow has 7 top-level keys. Only <code class="font-mono text-g-13 bg-g-3 px-1 rounded">version</code>, <code class="font-mono text-g-13 bg-g-3 px-1 rounded">name</code> and <code class="font-mono text-g-13 bg-g-3 px-1 rounded">steps</code> are required.</p>
          <div class="bg-g-2 border border-g-5 rounded overflow-hidden">
            <div class="grid grid-cols-[160px_180px_60px_1fr] gap-3 px-3 py-2 text-[10px] uppercase tracking-wider text-g-8 font-mono">
              <div>name</div><div>type</div><div>req</div><div>description</div>
            </div>
            <div v-for="(r, i) in [
              { name: 'version', type: 'string', req: true, desc: 'Schema version. Currently always 2.0.' },
              { name: 'name', type: 'string', req: true, desc: 'Workflow identifier. Used in logs, run IDs, and the UI.' },
              { name: 'description', type: 'string', req: false, desc: 'Free-form description shown in the dashboard.' },
              { name: 'tags', type: 'string[]', req: false, desc: 'Searchable labels.' },
              { name: 'recovery', type: 'bool', req: false, desc: 'Opt in to crash-safe persistence.' },
              { name: 'env', type: 'map<string,string>', req: false, desc: 'Imported environment variables.' },
              { name: 'params', type: 'Param[]', req: false, desc: 'Typed inputs collected before the run.' },
              { name: 'stages', type: 'Stage[]', req: false, desc: 'Logical groupings for visualization.' },
              { name: 'steps', type: 'Step[]', req: true, desc: 'The actual unit of work. Order does not matter.' },
            ]" :key="i" class="grid grid-cols-[160px_180px_60px_1fr] gap-3 px-3 py-2 border-t border-g-4 items-start">
              <div class="font-mono text-[12px] text-g-13">{{ r.name }}</div>
              <div class="font-mono text-[11px] text-violet-400">{{ r.type }}</div>
              <div class="font-mono text-[11px]" :class="r.req ? 'text-amber-400' : 'text-g-9'">{{ r.req ? 'yes' : 'no' }}</div>
              <div class="text-[12px] text-g-10 leading-relaxed">{{ r.desc }}</div>
            </div>
          </div>
        </section>

        <!-- Params -->
        <section id="doc-params" class="scroll-mt-16 mb-12">
          <div class="flex items-baseline gap-3 mb-2">
            <h2 class="text-[20px] font-semibold text-g-14 tracking-tight">Params & env</h2>
          </div>
          <p class="text-[13px] text-g-10 leading-relaxed mb-4 max-w-[640px]">Params are typed inputs. Env imports environment variables and supports templating.</p>
          <div class="bg-amber-400/5 border border-amber-400/30 rounded p-3 flex gap-2.5 items-start">
            <span class="bg-amber-400 w-1.5 h-1.5 rounded-full mt-1.5 shrink-0" />
            <div class="flex-1">
              <div class="text-[12px] font-mono uppercase tracking-wider mb-1 text-amber-400">careful</div>
              <div class="text-[13px] text-g-11 leading-relaxed">Env values are interpolated at workflow load. Params are interpolated per-step. Don't try to put <code class="font-mono text-g-13 bg-g-3 px-1 rounded">{{ TPL_STEPS }}</code> in env.</div>
            </div>
          </div>
        </section>

        <!-- Stages -->
        <section id="doc-stages" class="scroll-mt-16 mb-12">
          <div class="flex items-baseline gap-3 mb-2">
            <h2 class="text-[20px] font-semibold text-g-14 tracking-tight">Stages</h2>
          </div>
          <p class="text-[13px] text-g-10 leading-relaxed mb-4 max-w-[640px]">Stages are pure UX: they group steps in the dashboard. They have no runtime semantics.</p>
          <pre class="bg-g-2 border border-g-5 rounded p-3 font-mono text-[12px] leading-relaxed overflow-x-auto">
            <div class="flex items-center justify-between mb-2 -mt-1">
              <span class="text-[10px] font-mono text-g-8 uppercase tracking-wider">yaml</span>
              <button @click="copy(codeStages)" class="text-[10px] font-mono text-g-8 hover:text-g-12 flex items-center gap-1"><Icon name="copy" class-name="w-3 h-3" />copy</button>
            </div><code class="text-g-12">{{ codeStages }}</code>
          </pre>
        </section>

        <!-- Steps -->
        <section id="doc-steps" class="scroll-mt-16 mb-12">
          <div class="flex items-baseline gap-3 mb-2">
            <h2 class="text-[20px] font-semibold text-g-14 tracking-tight">Steps & dependencies</h2>
          </div>
          <p class="text-[13px] text-g-10 leading-relaxed mb-4 max-w-[640px]">A step has an id, an action, and a config block. Dependencies are explicit (depends_on) or inferred from template references.</p>
          <pre class="bg-g-2 border border-g-5 rounded p-3 font-mono text-[12px] leading-relaxed overflow-x-auto">
            <div class="flex items-center justify-between mb-2 -mt-1">
              <span class="text-[10px] font-mono text-g-8 uppercase tracking-wider">yaml</span>
              <button @click="copy(codeSteps)" class="text-[10px] font-mono text-g-8 hover:text-g-12 flex items-center gap-1"><Icon name="copy" class-name="w-3 h-3" />copy</button>
            </div><code class="text-g-12">{{ codeSteps }}</code>
          </pre>
        </section>

        <!-- Goto -->
        <section id="doc-goto" class="scroll-mt-16 mb-12">
          <div class="flex items-baseline gap-3 mb-2">
            <h2 class="text-[20px] font-semibold text-g-14 tracking-tight">goto & loops</h2>
          </div>
          <p class="text-[13px] text-g-10 leading-relaxed mb-4 max-w-[640px]">Two ways to repeat work: the <code class="font-mono text-g-13 bg-g-3 px-1 rounded">loop</code> action (parallel iteration) and the <code class="font-mono text-g-13 bg-g-3 px-1 rounded">goto</code> field (conditional re-entry).</p>
          <pre class="bg-g-2 border border-g-5 rounded p-3 font-mono text-[12px] leading-relaxed overflow-x-auto">
            <div class="flex items-center justify-between mb-2 -mt-1">
              <span class="text-[10px] font-mono text-g-8 uppercase tracking-wider">yaml</span>
              <button @click="copy(codeLoop)" class="text-[10px] font-mono text-g-8 hover:text-g-12 flex items-center gap-1"><Icon name="copy" class-name="w-3 h-3" />copy</button>
            </div><code class="text-g-12">{{ codeLoop }}</code>
          </pre>
          <div class="bg-amber-400/5 border border-amber-400/30 rounded p-3 flex gap-2.5 items-start mt-3">
            <span class="bg-amber-400 w-1.5 h-1.5 rounded-full mt-1.5 shrink-0" />
            <div class="flex-1">
              <div class="text-[12px] font-mono uppercase tracking-wider mb-1 text-amber-400">max_iterations</div>
              <div class="text-[13px] text-g-11 leading-relaxed"><code class="font-mono text-g-13 bg-g-3 px-1 rounded">goto</code> always carries a hard cap. Without it the runtime rejects the workflow at validation time.</div>
            </div>
          </div>
        </section>

        <!-- Recovery -->
        <section id="doc-recovery" class="scroll-mt-16 mb-12">
          <div class="flex items-baseline gap-3 mb-2">
            <h2 class="text-[20px] font-semibold text-g-14 tracking-tight">Recovery</h2>
          </div>
          <p class="text-[13px] text-g-10 leading-relaxed mb-4 max-w-[640px]">Opt in with <code class="font-mono text-g-13 bg-g-3 px-1 rounded">recovery: true</code>. The runtime persists every step transition. After a crash, the run resumes from the last checkpoint.</p>
        </section>

        <!-- expr-lang -->
        <section id="doc-expr" class="scroll-mt-16 mb-12">
          <div class="flex items-baseline gap-3 mb-2">
            <h2 class="text-[20px] font-semibold text-g-14 tracking-tight">expr-lang basics</h2>
          </div>
          <p class="text-[13px] text-g-10 leading-relaxed mb-4 max-w-[640px]">Templates use expr-lang inside <code class="font-mono text-g-13 bg-g-3 px-1 rounded">{{ TPL_BRACES }}</code>. It's typed, fast, side-effect-free.</p>
          <pre class="bg-g-2 border border-g-5 rounded p-3 font-mono text-[12px] leading-relaxed overflow-x-auto">
            <div class="flex items-center justify-between mb-2 -mt-1">
              <span class="text-[10px] font-mono text-g-8 uppercase tracking-wider">expr</span>
              <button @click="copy(codeExpr)" class="text-[10px] font-mono text-g-8 hover:text-g-12 flex items-center gap-1"><Icon name="copy" class-name="w-3 h-3" />copy</button>
            </div><code class="text-violet-400">{{ codeExpr }}</code>
          </pre>
        </section>

        <!-- Scope -->
        <section id="doc-scope" class="scroll-mt-16 mb-12">
          <div class="flex items-baseline gap-3 mb-2">
            <h2 class="text-[20px] font-semibold text-g-14 tracking-tight">Available scope</h2>
          </div>
          <p class="text-[13px] text-g-10 leading-relaxed mb-4 max-w-[640px]">Every template runs in a fixed scope.</p>
          <div class="bg-g-2 border border-g-5 rounded overflow-hidden">
            <div class="grid grid-cols-[200px_100px_1fr] gap-3 px-3 py-2 text-[10px] uppercase tracking-wider text-g-8 font-mono">
              <div>path</div><div>type</div><div>description</div>
            </div>
            <div v-for="(r, i) in [
              { name: 'params.<name>', type: 'value', desc: 'Workflow inputs as declared in params:.' },
              { name: 'env.<NAME>', type: 'string', desc: 'Environment variables imported via env:.' },
              { name: 'vars.<name>', type: 'value', desc: 'Variables set by the set action. Mutable.' },
              { name: 'steps.<id>.output', type: 'value', desc: 'Structured output of any upstream step.' },
              { name: 'loop.<as>', type: 'value', desc: 'Current item inside a loop body.' },
            ]" :key="i" class="grid grid-cols-[200px_100px_1fr] gap-3 px-3 py-2 border-t border-g-4 items-start">
              <div class="font-mono text-[12px] text-g-13">{{ r.name }}</div>
              <div class="font-mono text-[11px] text-violet-400">{{ r.type }}</div>
              <div class="text-[12px] text-g-10 leading-relaxed">{{ r.desc }}</div>
            </div>
          </div>
        </section>

        <!-- Operators -->
        <section id="doc-operators" class="scroll-mt-16 mb-12">
          <div class="flex items-baseline gap-3 mb-2">
            <h2 class="text-[20px] font-semibold text-g-14 tracking-tight">Operators & helpers</h2>
          </div>
          <div class="bg-g-2 border border-g-5 rounded overflow-hidden">
            <div class="grid grid-cols-[160px_100px_1fr] gap-3 px-3 py-2 text-[10px] uppercase tracking-wider text-g-8 font-mono">
              <div>operator</div><div>kind</div><div>description</div>
            </div>
            <div v-for="(r, i) in [
              { name: '??', type: 'operator', desc: 'Nil-coalescing. a ?? b returns b if a is null or empty.' },
              { name: '?:', type: 'operator', desc: 'Ternary. cond ? a : b.' },
              { name: 'len(x)', type: 'fn', desc: 'Length of a string, array or map.' },
              { name: 'lower(s)', type: 'fn', desc: 'Lowercase string.' },
              { name: 'split(s, sep)', type: 'fn', desc: 'Split string into array.' },
              { name: 'string(x)', type: 'fn', desc: 'Coerce to string.' },
            ]" :key="i" class="grid grid-cols-[160px_100px_1fr] gap-3 px-3 py-2 border-t border-g-4 items-start">
              <div class="font-mono text-[12px] text-g-13">{{ r.name }}</div>
              <div class="font-mono text-[11px] text-violet-400">{{ r.type }}</div>
              <div class="text-[12px] text-g-10 leading-relaxed">{{ r.desc }}</div>
            </div>
          </div>
        </section>

        <!-- Actions -->
        <section id="doc-actions" class="scroll-mt-16 mb-12">
          <div class="flex items-baseline gap-3 mb-2">
            <h2 class="text-[20px] font-semibold text-g-14 tracking-tight">All 34 actions</h2>
          </div>
          <p class="text-[13px] text-g-10 leading-relaxed mb-4 max-w-[640px]">The full catalog, by category.</p>
          <div class="space-y-3">
            <div v-for="c in ACTION_CATALOG" :key="c.cat" class="bg-g-2 border border-g-5 rounded p-3">
              <div class="flex items-baseline gap-2 mb-2">
                <div class="text-[12px] font-semibold text-g-13">{{ c.cat }}</div>
                <div class="text-[10px] font-mono text-g-8">{{ c.actions.length }} actions</div>
              </div>
              <div class="flex flex-wrap gap-1.5">
                <span v-for="a in c.actions" :key="a" class="font-mono text-[11px] text-g-12 bg-g-3 border border-g-5 rounded px-1.5 py-0.5 hover:border-g-7 cursor-pointer">{{ a }}</span>
              </div>
            </div>
          </div>
          <div class="bg-g-2 border border-g-5 rounded p-3 flex gap-2.5 items-start mt-3">
            <span class="bg-g-9 w-1.5 h-1.5 rounded-full mt-1.5 shrink-0" />
            <div class="flex-1">
              <div class="text-[12px] font-mono uppercase tracking-wider mb-1 text-g-12">saas-build tag</div>
              <div class="text-[13px] text-g-11 leading-relaxed"><code class="font-mono text-g-13 bg-g-3 px-1 rounded">exec</code>, <code class="font-mono text-g-13 bg-g-3 px-1 rounded">js</code> and <code class="font-mono text-g-13 bg-g-3 px-1 rounded">file.*</code> are gated behind the agent build. Not available in the SaaS runtime.</div>
            </div>
          </div>
        </section>

        <!-- HTTP -->
        <section id="doc-http" class="scroll-mt-16 mb-12">
          <div class="flex items-baseline gap-3 mb-2">
            <h2 class="text-[20px] font-semibold text-g-14 tracking-tight">http</h2>
          </div>
          <p class="text-[13px] text-g-10 leading-relaxed mb-4 max-w-[640px]">Make an HTTP request. Supports retries, timeouts, body templating.</p>
          <pre class="bg-g-2 border border-g-5 rounded p-3 font-mono text-[12px] leading-relaxed overflow-x-auto">
            <div class="flex items-center justify-between mb-2 -mt-1">
              <span class="text-[10px] font-mono text-g-8 uppercase tracking-wider">yaml</span>
              <button @click="copy(codeHttp)" class="text-[10px] font-mono text-g-8 hover:text-g-12 flex items-center gap-1"><Icon name="copy" class-name="w-3 h-3" />copy</button>
            </div><code class="text-g-12">{{ codeHttp }}</code>
          </pre>
        </section>

        <!-- exec -->
        <section id="doc-exec" class="scroll-mt-16 mb-12">
          <div class="flex items-baseline gap-3 mb-2">
            <h2 class="text-[20px] font-semibold text-g-14 tracking-tight">exec</h2>
          </div>
          <p class="text-[13px] text-g-10 leading-relaxed mb-4 max-w-[640px]">Run a shell command. Agent build only.</p>
          <div class="bg-red-400/5 border border-red-400/30 rounded p-3 flex gap-2.5 items-start">
            <span class="bg-red-400 w-1.5 h-1.5 rounded-full mt-1.5 shrink-0" />
            <div class="flex-1">
              <div class="text-[12px] font-mono uppercase tracking-wider mb-1 text-red-400">trust</div>
              <div class="text-[13px] text-g-11 leading-relaxed"><code class="font-mono text-g-13 bg-g-3 px-1 rounded">exec</code> runs as the agent user. Never feed untrusted templated input into a <code class="font-mono text-g-13 bg-g-3 px-1 rounded">sh -c</code> command without escaping.</div>
            </div>
          </div>
        </section>

        <!-- loop -->
        <section id="doc-loop" class="scroll-mt-16 mb-12">
          <div class="flex items-baseline gap-3 mb-2">
            <h2 class="text-[20px] font-semibold text-g-14 tracking-tight">loop</h2>
          </div>
          <p class="text-[13px] text-g-10 leading-relaxed mb-4 max-w-[640px]">Iterate an array, optionally in parallel, optionally tolerating failures.</p>
          <div class="bg-g-2 border border-g-5 rounded overflow-hidden">
            <div class="grid grid-cols-[160px_180px_60px_1fr] gap-3 px-3 py-2 text-[10px] uppercase tracking-wider text-g-8 font-mono">
              <div>name</div><div>type</div><div>req</div><div>description</div>
            </div>
            <div v-for="(r, i) in [
              { name: 'array', type: 'array | template', req: true, desc: 'The items to iterate.' },
              { name: 'as', type: 'string', req: true, desc: 'Bind name. Use as loop.<as> inside body.' },
              { name: 'concurrency', type: 'int', req: false, desc: 'Parallel workers. Default 1.' },
              { name: 'error_policy', type: 'enum', req: false, desc: 'fail (default) or continue.' },
              { name: 'actions', type: 'Action[]', req: true, desc: 'Sub-pipeline executed per item.' },
            ]" :key="i" class="grid grid-cols-[160px_180px_60px_1fr] gap-3 px-3 py-2 border-t border-g-4 items-start">
              <div class="font-mono text-[12px] text-g-13">{{ r.name }}</div>
              <div class="font-mono text-[11px] text-violet-400">{{ r.type }}</div>
              <div class="font-mono text-[11px]" :class="r.req ? 'text-amber-400' : 'text-g-9'">{{ r.req ? 'yes' : 'no' }}</div>
              <div class="text-[12px] text-g-10 leading-relaxed">{{ r.desc }}</div>
            </div>
          </div>
        </section>

        <!-- kv -->
        <section id="doc-kv" class="scroll-mt-16 mb-12">
          <div class="flex items-baseline gap-3 mb-2">
            <h2 class="text-[20px] font-semibold text-g-14 tracking-tight">kv.* — key/value persistence</h2>
          </div>
          <p class="text-[13px] text-g-10 leading-relaxed mb-4 max-w-[640px]">Process-local key-value store. Survives the workflow run.</p>
          <pre class="bg-g-2 border border-g-5 rounded p-3 font-mono text-[12px] leading-relaxed overflow-x-auto">
            <div class="flex items-center justify-between mb-2 -mt-1">
              <span class="text-[10px] font-mono text-g-8 uppercase tracking-wider">yaml</span>
              <button @click="copy(codeKv)" class="text-[10px] font-mono text-g-8 hover:text-g-12 flex items-center gap-1"><Icon name="copy" class-name="w-3 h-3" />copy</button>
            </div><code class="text-g-12">{{ codeKv }}</code>
          </pre>
        </section>

        <!-- Monitoring -->
        <section id="doc-monitoring" class="scroll-mt-16 mb-12">
          <div class="flex items-baseline gap-3 mb-2">
            <h2 class="text-[20px] font-semibold text-g-14 tracking-tight">Monitoring & metrics</h2>
          </div>
          <p class="text-[13px] text-g-10 leading-relaxed mb-4 max-w-[640px]">The agent serves Prometheus metrics on <code class="font-mono text-g-13 bg-g-3 px-1 rounded">/metrics</code> and a JSON status on <code class="font-mono text-g-13 bg-g-3 px-1 rounded">/healthz</code>.</p>
          <pre class="bg-g-2 border border-g-5 rounded p-3 font-mono text-[12px] leading-relaxed overflow-x-auto">
            <div class="flex items-center justify-between mb-2 -mt-1">
              <span class="text-[10px] font-mono text-g-8 uppercase tracking-wider">bash</span>
              <button @click="copy(codeHealth)" class="text-[10px] font-mono text-g-8 hover:text-g-12 flex items-center gap-1"><Icon name="copy" class-name="w-3 h-3" />copy</button>
            </div><code class="text-g-12">{{ codeHealth }}</code>
          </pre>
        </section>

        <!-- SSE -->
        <section id="doc-sse" class="scroll-mt-16 mb-12">
          <div class="flex items-baseline gap-3 mb-2">
            <h2 class="text-[20px] font-semibold text-g-14 tracking-tight">SSE event stream</h2>
          </div>
          <p class="text-[13px] text-g-10 leading-relaxed mb-4 max-w-[640px]">Subscribe to <code class="font-mono text-g-13 bg-g-3 px-1 rounded">/events</code> for a Server-Sent-Events stream of every step transition. The web UI uses this to render live runs without polling.</p>
          <pre class="bg-g-2 border border-g-5 rounded p-3 font-mono text-[12px] leading-relaxed overflow-x-auto">
            <div class="flex items-center justify-between mb-2 -mt-1">
              <span class="text-[10px] font-mono text-g-8 uppercase tracking-wider">bash</span>
              <button @click="copy(codeSse)" class="text-[10px] font-mono text-g-8 hover:text-g-12 flex items-center gap-1"><Icon name="copy" class-name="w-3 h-3" />copy</button>
            </div><code class="text-g-12">{{ codeSse }}</code>
          </pre>
        </section>

        <!-- Security -->
        <section id="doc-security" class="scroll-mt-16 mb-12">
          <div class="flex items-baseline gap-3 mb-2">
            <h2 class="text-[20px] font-semibold text-g-14 tracking-tight">Security model</h2>
          </div>
          <p class="text-[13px] text-g-10 leading-relaxed mb-4 max-w-[640px]">The agent is built for local dev workflows. It exposes a UI on localhost only by default, no auth.</p>
          <ul class="list-disc pl-5 space-y-1.5 text-g-11 text-[13px]">
            <li>Default bind is <code class="font-mono text-g-13 bg-g-3 px-1 rounded">127.0.0.1:8080</code>.</li>
            <li>Templates cannot read arbitrary files. <code class="font-mono text-g-13 bg-g-3 px-1 rounded">file.read</code> is gated behind the agent build.</li>
            <li>The SaaS runtime drops <code class="font-mono text-g-13 bg-g-3 px-1 rounded">exec</code>, <code class="font-mono text-g-13 bg-g-3 px-1 rounded">js</code> and <code class="font-mono text-g-13 bg-g-3 px-1 rounded">file.*</code> entirely.</li>
          </ul>
          <div class="mt-6 pt-6 border-t border-g-5 flex items-center justify-between">
            <div class="text-[11px] font-mono text-g-8">end of docs</div>
          </div>
        </section>
      </div>
    </div>

    <!-- On this page -->
    <aside class="w-[200px] shrink-0 py-10 px-4 border-l border-g-5 overflow-y-auto hidden xl:block">
      <div class="text-[10px] font-mono uppercase tracking-wider text-g-8 mb-2">On this page</div>
      <div class="flex flex-col gap-1">
        <button
          v-for="it in onThisPage"
          :key="it.id"
          @click="jumpTo(it.id)"
          :class="['text-left text-[11.5px] py-0.5', active === it.id ? 'text-g-14 border-l-2 border-g-13 pl-2 -ml-0.5' : 'text-g-9 hover:text-g-12 pl-2']"
        >{{ it.l }}</button>
      </div>
    </aside>
  </div>
</template>
