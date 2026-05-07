<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import dagre from 'dagre'
import type { Graph, GraphNode, GraphEdge } from '@/composables/useWorkflowApi'
import { useSettings } from '@/composables/useSettings'

const props = defineProps<{
  graph: Graph
  stepStatuses?: Record<string, string>
  stepDurations?: Record<string, number>
  stepIterations?: Record<string, number>
  selectedId?: string | null
  height?: number
  fillHeight?: boolean
  showStages?: boolean
  editable?: boolean
}>()

const emit = defineEmits<{
  (e: 'nodeClick', stepId: string): void
  (e: 'addBetween', source: string, target: string): void
  (e: 'dropAction', action: string): void
}>()

const { settings } = useSettings()

const NODE_W = 196
const NODE_H = 64

interface LaidOutNode extends GraphNode {
  x: number
  y: number
  height: number
}

interface LaidOutEdge {
  source: string
  target: string
  type?: string
  label?: string
  d: string
  midX: number
  midY: number
  x1: number
  y1: number
  x2: number
  y2: number
}

interface Layout {
  nodes: LaidOutNode[]
  edges: LaidOutEdge[]
  width: number
  height: number
  minX: number
  minY: number
}

const layout = computed<Layout>(() => {
  const g = new dagre.graphlib.Graph()
  g.setDefaultEdgeLabel(() => ({}))
  g.setGraph({ rankdir: 'LR', ranksep: 100, nodesep: 30, marginx: 20, marginy: 24 })

  const nodes = props.graph.nodes || []
  const edges = props.graph.edges || []

  for (const n of nodes) {
    const pipeExtra = n.pipeline && n.pipeline.length > 0 ? n.pipeline.length * 22 + 8 : 0
    const condExtra = n.when ? 18 : 0
    const h = NODE_H + pipeExtra + condExtra
    g.setNode(n.id, { width: NODE_W, height: h })
  }
  for (const e of edges) g.setEdge(e.source, e.target)
  dagre.layout(g)

  const laidNodes: LaidOutNode[] = nodes.map(n => {
    const p = g.node(n.id)
    const pipeExtra = n.pipeline && n.pipeline.length > 0 ? n.pipeline.length * 22 + 8 : 0
    const condExtra = n.when ? 18 : 0
    const h = NODE_H + pipeExtra + condExtra
    return {
      ...n,
      x: p.x - NODE_W / 2,
      y: p.y - h / 2,
      height: h,
    }
  })

  const byId: Record<string, LaidOutNode> = Object.fromEntries(laidNodes.map(n => [n.id, n]))

  const laidEdges: LaidOutEdge[] = edges.map((e: GraphEdge) => {
    const a = byId[e.source]
    const b = byId[e.target]
    if (!a || !b) {
      return { source: e.source, target: e.target, type: e.type, label: e.label, d: '', midX: 0, midY: 0, x1: 0, y1: 0, x2: 0, y2: 0 }
    }
    if (e.type === 'goto') {
      const x1 = a.x + 12
      const y1 = a.y + a.height
      const x2 = b.x + 12
      const y2 = b.y
      const minX = Math.min(x1, x2) - 60
      return { source: e.source, target: e.target, type: e.type, label: e.label, d: `M ${x1} ${y1} C ${minX} ${y1}, ${minX} ${y2}, ${x2} ${y2}`, midX: minX, midY: (y1 + y2) / 2, x1, y1, x2, y2 }
    }
    const sameCol = Math.abs(a.x - b.x) < 10
    let x1: number, y1: number, x2: number, y2: number, d: string
    if (sameCol) {
      x1 = a.x + NODE_W / 2; y1 = a.y + a.height
      x2 = b.x + NODE_W / 2; y2 = b.y
      const my = (y1 + y2) / 2
      d = `M ${x1} ${y1} C ${x1} ${my}, ${x2} ${my}, ${x2} ${y2}`
    } else {
      x1 = a.x + NODE_W; y1 = a.y + a.height / 2
      x2 = b.x; y2 = b.y + b.height / 2
      const mx = (x1 + x2) / 2
      d = `M ${x1} ${y1} C ${mx} ${y1}, ${mx} ${y2}, ${x2} ${y2}`
    }
    return { source: e.source, target: e.target, type: e.type, label: e.label, d, midX: (x1 + x2) / 2, midY: (y1 + y2) / 2, x1, y1, x2, y2 }
  })

  let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity
  for (const n of laidNodes) {
    if (n.x < minX) minX = n.x
    if (n.y < minY) minY = n.y
    const right = n.x + NODE_W
    const bottom = n.y + n.height
    if (right > maxX) maxX = right
    if (bottom > maxY) maxY = bottom
  }
  if (!isFinite(minX)) { minX = 0; minY = 0; maxX = 100; maxY = 100 }
  minX -= 24; minY -= 24; maxX += 24; maxY += 24

  return {
    nodes: laidNodes,
    edges: laidEdges,
    minX,
    minY,
    width: maxX - minX,
    height: maxY - minY,
  }
})

const stagesLayout = computed(() => {
  if (!props.showStages || !props.graph.stages) return []
  const byStep: Record<string, string> = {}
  for (const s of props.graph.stages) for (const id of s.steps) byStep[id] = s.name

  return props.graph.stages.map((s, i) => {
    const stepNodes = layout.value.nodes.filter(n => byStep[n.id] === s.name)
    if (stepNodes.length === 0) return null
    let sx = Infinity, sy = Infinity, ex = -Infinity, ey = -Infinity
    for (const n of stepNodes) {
      if (n.x < sx) sx = n.x
      if (n.y < sy) sy = n.y
      const r = n.x + NODE_W
      const b = n.y + n.height
      if (r > ex) ex = r
      if (b > ey) ey = b
    }
    return { name: s.name, index: i, x: sx - 18, y: sy - 26, w: ex - sx + 36, h: ey - sy + 36 }
  }).filter(Boolean) as { name: string; index: number; x: number; y: number; w: number; h: number }[]
})

function statusFill(status: string): string {
  if (status === 'success') return 'fill-emerald-400'
  if (status === 'completed_with_errors') return 'fill-amber-400'
  if (status === 'running') return 'fill-amber-400'
  if (status === 'waiting') return 'fill-violet-400'
  if (status === 'failed') return 'fill-red-400'
  if (status === 'cancelled') return 'fill-orange-400'
  if (status === 'scheduled') return 'fill-violet-400'
  if (status === 'skipped') return 'fill-g-7'
  return 'fill-g-6'
}

function statusStroke(status: string): string {
  if (status === 'success') return 'stroke-emerald-400'
  if (status === 'completed_with_errors') return 'stroke-amber-400'
  if (status === 'running') return 'stroke-amber-400'
  if (status === 'waiting') return 'stroke-violet-400'
  if (status === 'failed') return 'stroke-red-400'
  if (status === 'cancelled') return 'stroke-orange-400'
  if (status === 'scheduled') return 'stroke-violet-400'
  if (status === 'skipped') return 'stroke-g-7'
  return 'stroke-g-7'
}

function isCompletedEdge(e: LaidOutEdge): boolean {
  const sa = props.stepStatuses?.[e.source]
  const sb = props.stepStatuses?.[e.target]
  return sa === 'success' && sb === 'success'
}

function isActiveEdge(e: LaidOutEdge): boolean {
  const sa = props.stepStatuses?.[e.source]
  const sb = props.stepStatuses?.[e.target]
  if (e.type === 'goto') return sa === 'success' && sb === 'running'
  return sa === 'success' && (sb === 'running' || sb === 'waiting')
}

function nodeStatus(id: string): string {
  return props.stepStatuses?.[id] || 'pending'
}

const hoverEdge = ref<string | null>(null)
const dragOver = ref(false)

function edgeKey(e: LaidOutEdge): string { return e.source + '->' + e.target }

function onDrop(e: DragEvent) {
  e.preventDefault()
  dragOver.value = false
  const action = e.dataTransfer?.getData('action')
  if (action) emit('dropAction', action)
}

function onDragOver(e: DragEvent) {
  e.preventDefault()
  dragOver.value = true
}

function onDragLeave() { dragOver.value = false }

const containerHeight = computed(() => props.height ?? layout.value.height)

watch(() => props.graph, () => { /* recompute */ }, { deep: true })

// ---------------- Pan & zoom ----------------
const svgRef = ref<SVGSVGElement>()
const zoom = ref(1)
const panX = ref(0)
const panY = ref(0)
const isPanning = ref(false)
const panStart = ref<{ x: number; y: number; px: number; py: number } | null>(null)

const ZOOM_MIN = 0.2
const ZOOM_MAX = 4

const viewBox = computed(() => {
  const w = layout.value.width / zoom.value
  const h = layout.value.height / zoom.value
  const x = layout.value.minX + panX.value
  const y = layout.value.minY + panY.value
  return `${x} ${y} ${w} ${h}`
})

// Convert a screen-space point (clientX/clientY) into world coordinates,
// using the SVG's actual screen CTM. This handles preserveAspectRatio,
// viewBox, and CSS scaling without manual math — it's the only formula
// that stays correct when the content has a different aspect ratio than
// the container (where letterboxing kicks in).
function clientToSvg(clientX: number, clientY: number): { x: number; y: number } | null {
  const svg = svgRef.value
  if (!svg) return null
  const pt = svg.createSVGPoint()
  pt.x = clientX
  pt.y = clientY
  const ctm = svg.getScreenCTM()
  if (!ctm) return null
  const t = pt.matrixTransform(ctm.inverse())
  return { x: t.x, y: t.y }
}

function onWheel(e: WheelEvent) {
  e.preventDefault()
  const delta = -e.deltaY
  const factor = Math.exp(delta * 0.0015)
  const next = Math.min(ZOOM_MAX, Math.max(ZOOM_MIN, zoom.value * factor))
  if (next === zoom.value) return
  // Keep cursor's world coordinate stable
  const before = clientToSvg(e.clientX, e.clientY)
  const oldZoom = zoom.value
  zoom.value = next
  if (before) {
    const after = clientToSvg(e.clientX, e.clientY)
    if (after) {
      panX.value += before.x - after.x
      panY.value += before.y - after.y
    } else {
      // Should not happen, but guard anyway
      void oldZoom
    }
  }
}

function onMouseDown(e: MouseEvent) {
  if (e.button !== 0) return
  // Don't start pan when clicking on a node (let node click handler take over)
  const target = e.target as Element
  if (target.closest('[data-dag-node]')) return
  e.preventDefault()
  // Manual drag breaks follow mode
  followActive.value = false
  isPanning.value = true
  panStart.value = { x: e.clientX, y: e.clientY, px: panX.value, py: panY.value }
  // Attach to window so the drag survives when the cursor leaves the SVG —
  // otherwise mousemove stops firing and the pan feels stuck/laggy.
  window.addEventListener('mousemove', onWindowMouseMove)
  window.addEventListener('mouseup', onWindowMouseUp)
}

function onWindowMouseMove(e: MouseEvent) {
  if (!isPanning.value || !panStart.value || !svgRef.value) return
  const rect = svgRef.value.getBoundingClientRect()
  const w = layout.value.width / zoom.value
  const h = layout.value.height / zoom.value
  // preserveAspectRatio="xMidYMid meet" applies a single uniform scale to
  // both axes — the smaller of the two ratios. Using rect.width/rect.height
  // independently gave a different conversion for X vs Y when the content's
  // aspect ratio didn't match the container, which made vertical drag feel
  // sluggish (or, in the other direction, hyper-sensitive).
  const scale = Math.min(rect.width / w, rect.height / h)
  const dx = (e.clientX - panStart.value.x) / scale
  const dy = (e.clientY - panStart.value.y) / scale
  panX.value = panStart.value.px - dx
  panY.value = panStart.value.py - dy
}

function onWindowMouseUp() {
  isPanning.value = false
  panStart.value = null
  window.removeEventListener('mousemove', onWindowMouseMove)
  window.removeEventListener('mouseup', onWindowMouseUp)
}

onUnmounted(() => {
  // Defensive: if user navigates away mid-drag, leave no orphan listeners.
  window.removeEventListener('mousemove', onWindowMouseMove)
  window.removeEventListener('mouseup', onWindowMouseUp)
})

function fit() {
  followActive.value = false
  zoom.value = 1
  panX.value = 0
  panY.value = 0
}

function zoomIn() { zoom.value = Math.min(ZOOM_MAX, zoom.value * 1.2) }
function zoomOut() { zoom.value = Math.max(ZOOM_MIN, zoom.value / 1.2) }
function reset100() {
  followActive.value = false
  zoom.value = 1
}

// Center the viewport on a specific node at a comfortable zoom level so the
// neighbours (parents/children) are visible too. Returns true if it found a
// node to focus on.
function focusNode(nodeId: string, targetZoom = 2.4): boolean {
  const node = layout.value.nodes.find(n => n.id === nodeId)
  if (!node) return false

  zoom.value = targetZoom

  const cx = node.x + NODE_W / 2
  const cy = node.y + node.height / 2
  panX.value = (cx - layout.value.minX) - layout.value.width / (2 * targetZoom)
  panY.value = (cy - layout.value.minY) - layout.value.height / (2 * targetZoom)

  return true
}

// Find the most relevant "active" step to follow:
// 1. Running step (preferred — that's what's happening right now)
// 2. Waiting step (paused on a wait_for / human gate)
// 3. Latest succeeded step (for recently-finished runs)
function findActiveStepId(): string | null {
  const statuses = props.stepStatuses
  if (!statuses) return null

  let running: string | null = null
  let waiting: string | null = null
  let lastSuccess: string | null = null

  for (const node of layout.value.nodes) {
    const s = statuses[node.id]
    if (s === 'running') running = node.id
    else if (s === 'waiting' && !waiting) waiting = node.id
    else if (s === 'success') lastSuccess = node.id
  }

  return running || waiting || lastSuccess
}

// Follow mode: when on, viewport recenters on the active step whenever it
// changes. Any manual pan disables it. Click the "focus" button to re-enable.
const followActive = ref(true)

function toggleFollow() {
  if (followActive.value) {
    // Re-center now even if already on (useful when user manually drifted)
    focusNode(findActiveStepId() ?? '')
    return
  }
  followActive.value = true
  focusNode(findActiveStepId() ?? '')
}

// React to active step changes while follow is enabled.
watch(
  [() => layout.value.nodes.length, () => findActiveStepId()],
  ([nodeCount, activeId]) => {
    if (!followActive.value) return
    if (!nodeCount || !activeId) return
    focusNode(activeId)
  },
  { immediate: true },
)
</script>

<template>
  <div
    :class="['relative w-full h-full', dragOver ? 'ring-1 ring-amber-400/40' : '']"
    @drop="editable ? onDrop($event) : undefined"
    @dragover="editable ? onDragOver($event) : undefined"
    @dragleave="editable ? onDragLeave() : undefined"
  >
    <!-- Zoom / pan controls (top-right) -->
    <div class="absolute top-3 right-3 z-10 flex items-center gap-1 px-1 py-1 rounded bg-g-2/90 border border-g-5 backdrop-blur select-none">
      <button
        @click="toggleFollow"
        :class="[
          'px-2 py-0.5 text-[10px] font-mono rounded flex items-center gap-1',
          followActive ? 'text-amber-400 bg-amber-400/10' : 'text-g-9 hover:text-g-13 hover:bg-g-3',
        ]"
        :title="followActive ? 'Following active step (click to recenter, drag to detach)' : 'Click to follow the active step'"
      >
        <span :class="['w-1.5 h-1.5 rounded-full', followActive ? 'bg-amber-400 animate-pulse' : 'bg-g-7']" />follow
      </button>
      <span class="text-g-7 text-[10px]">·</span>
      <button @click="fit" class="px-2 py-0.5 text-[10px] font-mono text-g-9 hover:text-g-13 hover:bg-g-3 rounded" title="Fit to view">fit</button>
      <button @click="reset100" class="px-2 py-0.5 text-[10px] font-mono text-g-9 hover:text-g-13 hover:bg-g-3 rounded" title="Zoom 100%">{{ Math.round(zoom * 100) }}%</button>
      <span class="text-g-7 text-[10px]">·</span>
      <button @click="zoomOut" class="px-2 py-0.5 text-[10px] font-mono text-g-9 hover:text-g-13 hover:bg-g-3 rounded" title="Zoom out">−</button>
      <button @click="zoomIn" class="px-2 py-0.5 text-[10px] font-mono text-g-9 hover:text-g-13 hover:bg-g-3 rounded" title="Zoom in">+</button>
    </div>
    <svg
      ref="svgRef"
      :viewBox="viewBox"
      :class="['w-full', fillHeight ? 'h-full block' : '']"
      :style="fillHeight ? { cursor: isPanning ? 'grabbing' : 'grab' } : { height: containerHeight + 'px', maxHeight: containerHeight + 'px', cursor: isPanning ? 'grabbing' : 'grab' }"
      preserveAspectRatio="xMidYMid meet"
      @wheel="onWheel"
      @mousedown="onMouseDown"
    >
      <defs>
        <marker id="dagArrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="5" markerHeight="5" orient="auto">
          <path d="M 0 0 L 10 5 L 0 10 z" class="fill-g-7" />
        </marker>
        <marker id="dagArrowActive" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="5" markerHeight="5" orient="auto">
          <path d="M 0 0 L 10 5 L 0 10 z" class="fill-amber-400" />
        </marker>
        <marker id="dagArrowGoto" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="5" markerHeight="5" orient="auto">
          <path d="M 0 0 L 10 5 L 0 10 z" class="fill-violet-400" />
        </marker>
      </defs>

      <!-- Stage backgrounds: only dashed border, no fill -->
      <g v-if="showStages">
        <g v-for="s in stagesLayout" :key="s.name">
          <rect :x="s.x" :y="s.y" :width="s.w" :height="s.h" rx="8" fill="none" class="stroke-g-5" stroke-width="1" stroke-dasharray="3 3" />
          <text :x="s.x + 10" :y="s.y + 14" class="fill-g-9 font-mono uppercase tracking-wider" font-size="9">{{ s.index + 1 }}. {{ s.name }}</text>
        </g>
      </g>

      <!-- Edges -->
      <g v-for="e in layout.edges" :key="edgeKey(e)">
        <template v-if="e.type === 'goto'">
          <path :d="e.d" class="stroke-violet-400/60 fill-none" stroke-width="1.2" stroke-dasharray="3 3" marker-end="url(#dagArrowGoto)" />
        </template>
        <template v-else>
          <path
            :d="e.d"
            :class="isActiveEdge(e) ? 'stroke-amber-400' : isCompletedEdge(e) ? 'stroke-emerald-400/40' : 'stroke-g-6'"
            :stroke-width="isActiveEdge(e) ? 1.4 : 1"
            fill="none"
            :marker-end="isActiveEdge(e) ? 'url(#dagArrowActive)' : 'url(#dagArrow)'"
          />
          <!-- token-on-edge -->
          <circle v-if="isActiveEdge(e) && settings.animStyle === 'token'" r="3.5" class="fill-amber-400">
            <animateMotion dur="1.4s" repeatCount="indefinite" :path="e.d" />
          </circle>
          <!-- particles -->
          <template v-if="isActiveEdge(e) && settings.animStyle === 'particle'">
            <circle r="1.6" class="fill-amber-400/80">
              <animateMotion dur="1.8s" repeatCount="indefinite" :path="e.d" begin="0s" />
            </circle>
            <circle r="1.6" class="fill-amber-400/80">
              <animateMotion dur="1.8s" repeatCount="indefinite" :path="e.d" begin="0.6s" />
            </circle>
            <circle r="1.6" class="fill-amber-400/80">
              <animateMotion dur="1.8s" repeatCount="indefinite" :path="e.d" begin="1.2s" />
            </circle>
          </template>
          <!-- pulse: halo path -->
          <path v-if="isActiveEdge(e) && settings.animStyle === 'pulse'" :d="e.d" class="stroke-amber-400/40 fill-none anim-edge-pulse" stroke-width="3" />

          <!-- editable: invisible larger hit area for hover (rendered first → behind) -->
          <path
            v-if="editable"
            :d="e.d"
            stroke="transparent"
            stroke-width="14"
            fill="none"
            class="cursor-pointer"
            @mouseenter="hoverEdge = edgeKey(e)"
            @mouseleave="hoverEdge = null"
            @click="emit('addBetween', e.source, e.target)"
          />
          <!-- editable: hover + button (rendered after → on top, but pointer-events:none so click goes through to hit area) -->
          <g
            v-if="editable"
            :opacity="hoverEdge === edgeKey(e) ? 1 : 0"
            style="pointer-events: none"
          >
            <circle :cx="e.midX" :cy="e.midY" r="9" class="fill-g-3 stroke-g-8" stroke-width="1" />
            <path :d="`M ${e.midX-4} ${e.midY} L ${e.midX+4} ${e.midY} M ${e.midX} ${e.midY-4} L ${e.midX} ${e.midY+4}`" class="stroke-g-12" stroke-width="1.4" stroke-linecap="round" />
          </g>
        </template>
      </g>

      <!-- Nodes -->
      <g v-for="n in layout.nodes" :key="n.id" :transform="`translate(${n.x},${n.y})`" class="cursor-pointer" data-dag-node @click="emit('nodeClick', n.id)">
        <!-- halo for running (pulse mode) -->
        <rect
          v-if="nodeStatus(n.id) === 'running' && settings.animStyle === 'pulse'"
          x="-3" y="-3"
          :width="NODE_W + 6"
          :height="n.height + 6"
          rx="9"
          class="fill-amber-400/0 stroke-amber-400/40 anim-halo"
          stroke-width="1"
        />

        <rect x="0" y="0" :width="NODE_W" :height="n.height" rx="6" :class="[selectedId === n.id ? 'fill-g-4' : 'fill-g-2', selectedId === n.id ? 'stroke-g-12' : statusStroke(nodeStatus(n.id))]" :stroke-width="selectedId === n.id ? 1.5 : 1" />

        <!-- left status stripe -->
        <rect x="0" y="0" width="3" :height="n.height" rx="2" :class="statusFill(nodeStatus(n.id))" />

        <!-- id + iter badge -->
        <text x="12" y="20" class="fill-g-13 font-mono" font-size="11.5" font-weight="600">{{ n.id }}</text>
        <g v-if="(stepIterations?.[n.id] || 0) > 1" :transform="`translate(${NODE_W - 36},9)`">
          <rect x="0" y="0" width="28" height="14" rx="3" class="fill-g-5" />
          <text x="14" y="10" text-anchor="middle" class="fill-g-11 font-mono" font-size="9">×{{ stepIterations?.[n.id] }}</text>
        </g>

        <!-- action chip -->
        <g transform="translate(12,28)">
          <rect x="0" y="0" :width="Math.max(48, (n.action?.length || 0) * 6 + 10)" height="14" rx="3" class="fill-g-6" />
          <text x="6" y="10" class="fill-g-11 font-mono" font-size="9">{{ n.action }}</text>
        </g>

        <!-- title + duration -->
        <text x="12" y="56" class="fill-g-9" font-size="10">{{ (n.label || n.id).length > 26 ? (n.label || n.id).slice(0, 25) + '…' : (n.label || n.id) }}</text>
        <text v-if="stepDurations?.[n.id] != null" :x="NODE_W - 10" y="56" text-anchor="end" class="fill-g-10 font-mono" font-size="10">
          {{ (stepDurations![n.id] || 0) < 1000 ? (stepDurations![n.id] || 0) + 'ms' : ((stepDurations![n.id] || 0) / 1000).toFixed(1) + 's' }}
        </text>

        <!-- when badge -->
        <g v-if="n.when" transform="translate(12,62)">
          <rect x="0" y="0" width="20" height="14" rx="3" class="fill-violet-400/15 stroke-violet-400/40" stroke-width="0.8" />
          <text x="10" y="10" text-anchor="middle" class="fill-violet-400 font-mono" font-size="9">if</text>
        </g>

        <!-- pipeline sub-actions -->
        <g v-if="n.pipeline && n.pipeline.length > 0" :transform="`translate(12, ${n.when ? 76 : 62})`">
          <g v-for="(p, i) in n.pipeline" :key="i" :transform="`translate(0, ${i * 22})`">
            <rect x="0" y="0" :width="NODE_W - 24" height="18" rx="3" class="fill-g-3 stroke-g-6" stroke-width="0.6" />
            <text x="6" y="12" class="fill-g-10 font-mono" font-size="9">└─ {{ i + 1 }}. {{ p.action }}</text>
          </g>
        </g>

        <!-- status pip top-right -->
        <circle :cx="NODE_W - 10" cy="10" r="3.5" :class="statusFill(nodeStatus(n.id))" :style="nodeStatus(n.id) === 'running' ? 'animation: pulse-dot 1.4s ease-in-out infinite' : ''" />
      </g>
    </svg>
  </div>
</template>
