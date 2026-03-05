<script setup lang="ts">
import { computed, watch } from 'vue'
import { VueFlow, type Node, type Edge } from '@vue-flow/core'
import dagre from 'dagre'
import type { Graph } from '@/composables/useWorkflowApi'
import type { StepCounts } from '@/composables/useGlobalEvents'
import StepNode from './StepNode.vue'

const props = defineProps<{
  graph: Graph
  stepStatuses?: Record<string, string>
  stepCounts?: Record<string, StepCounts>
  stepIterations?: Record<string, number>
  recentStatuses?: Record<string, string>
  stepExecCounts?: Record<string, number>
}>()

const emit = defineEmits<{
  (e: 'nodeClick', stepId: string): void
  (e: 'layoutReady', payload: { maxY: number }): void
}>()

const nodeTypes: Record<string, any> = {
  step: StepNode,
}

function layoutGraph(g: Graph): { nodes: Node[]; edges: Edge[] } {
  const dagreGraph = new dagre.graphlib.Graph()
  dagreGraph.setDefaultEdgeLabel(() => ({}))
  dagreGraph.setGraph({ rankdir: 'TB', ranksep: 100, nodesep: 80 })

  // Build a set of nodes that have conditional outputs
  // (i.e., at least one outgoing edge with type "when")
  const conditionalSources = new Set<string>()
  const gEdges = g.edges || []
  gEdges.forEach((e) => {
    if (e.type === 'when') {
      conditionalSources.add(e.source)
    }
  })

  const gNodes = g.nodes || []
  gNodes.forEach((n) => {
    const hasWhen = !!n.when
    const hasCondOutputs = conditionalSources.has(n.id)
    // py-7 = 56px padding + ~32px content row + 4px border = 92px base
    const baseH = 92
    const condExtra = (hasWhen || hasCondOutputs) ? 24 : 0
    const pipeExtra = (n.pipeline && n.pipeline.length > 0)
      ? 12 + n.pipeline.length * 26
      : 0
    const h = baseH + condExtra + pipeExtra
    dagreGraph.setNode(n.id, { width: 320, height: h })
  })
  gEdges.forEach((e) => {
    dagreGraph.setEdge(e.source, e.target)
  })

  dagre.layout(dagreGraph)

  const nodes: Node[] = gNodes.map((n) => {
    const pos = dagreGraph.node(n.id)

    // Determine node status: running/waiting > recent terminal > stepStatuses > pending
    let status = 'pending'
    if (props.stepStatuses) {
      status = props.stepStatuses[n.id] || 'pending'
    }
    if (props.stepCounts?.[n.id]) {
      const sc = props.stepCounts[n.id]
      if (sc.waiting.length > 0) status = 'waiting'
      else if (sc.running.length > 0) status = 'running'
    }
    // Recent terminal statuses override pending (but not running/waiting)
    if (status === 'pending' && props.recentStatuses?.[n.id]) {
      status = props.recentStatuses[n.id]
    }

    // Active execution count for this step
    const sc = props.stepCounts?.[n.id]
    const activeCount = sc ? sc.running.length + sc.waiting.length : 0

    // Goto iteration count
    const iteration = props.stepIterations?.[n.id]

    const hasCondOutputs = conditionalSources.has(n.id)
    const hasWhen = !!n.when

    const baseH = 92
    const condExtra = (hasWhen || hasCondOutputs) ? 24 : 0
    const pipeExtra = (n.pipeline && n.pipeline.length > 0)
      ? 12 + n.pipeline.length * 26
      : 0
    const height = baseH + condExtra + pipeExtra

    return {
      id: n.id,
      type: 'step',
      position: { x: pos.x - 160, y: pos.y - height / 2 },
      data: {
        label: n.label || n.id,
        action: n.action,
        status,
        activeCount: activeCount > 0 ? activeCount : undefined,
        iteration,
        pipeline: n.pipeline,
        execCount: props.stepExecCounts?.[n.id] || 0,
        when: n.when,
        hasConditionalOutputs: hasCondOutputs,
      },
    }
  })

  const edges: Edge[] = gEdges.map((e, i) => {
    const isGoto = e.type === 'goto'
    const isWhen = e.type === 'when'
    const sourceStatus = props.stepStatuses?.[e.source]
    const sourceCounts = props.stepCounts?.[e.source]
    const sourceRecent = props.recentStatuses?.[e.source]
    const hasActivity = sourceCounts && (sourceCounts.running.length > 0 || sourceCounts.waiting.length > 0)
    const isRunning = sourceStatus === 'running' || (sourceCounts?.running?.length ?? 0) > 0
    const isActive = isRunning || sourceStatus === 'success' || hasActivity || sourceRecent === 'success'

    if (isGoto) {
      return {
        id: `e-${i}`,
        source: e.source,
        target: e.target,
        type: 'smoothstep',
        animated: isRunning,
        style: {
          stroke: '#f59e0b',
          strokeWidth: 2,
          strokeDasharray: '6 3',
        },
      }
    }

    // Conditional edge: connect from the "when-true" handle
    if (isWhen) {
      const sourceHasCondOutputs = conditionalSources.has(e.source)
      return {
        id: `e-${i}`,
        source: e.source,
        sourceHandle: sourceHasCondOutputs ? 'when-true' : 'default',
        target: e.target,
        animated: isRunning || sourceRecent === 'success',
        label: '✓',
        labelStyle: { fill: '#34d399', fontSize: '11px', fontWeight: 600 },
        labelBgStyle: { fill: '#1a1a1a', fillOpacity: 0.9 },
        labelBgPadding: [4, 6] as [number, number],
        labelBgBorderRadius: 4,
        style: {
          stroke: isActive ? '#34d399' : '#1e4d3a',
          strokeWidth: isActive ? 2 : 1.5,
        },
      }
    }

    // Non-conditional edge from a source that has conditional outputs → "when-false" handle
    const sourceHasCondOutputs = conditionalSources.has(e.source)
    return {
      id: `e-${i}`,
      source: e.source,
      sourceHandle: sourceHasCondOutputs ? 'when-false' : 'default',
      target: e.target,
      animated: isRunning || sourceRecent === 'success',
      style: {
        stroke: isActive ? '#a3a3a3' : '#2e2e2e',
        strokeWidth: isActive ? 2 : 1.5,
      },
    }
  })

  return { nodes, edges }
}

const layout = computed(() => layoutGraph(props.graph))

watch(layout, (l) => {
  if (l.nodes.length === 0) return
  let maxY = 0
  for (const n of l.nodes) {
    const h = (n.style && typeof n.style === 'object' && 'height' in n.style) ? Number(n.style.height) : 92
    const bottom = n.position.y + h
    if (bottom > maxY) maxY = bottom
  }
  emit('layoutReady', { maxY })
}, { immediate: true })

function onNodeClick({ node }: any) {
  emit('nodeClick', node.id)
}
</script>

<template>
  <VueFlow
    :nodes="layout.nodes"
    :edges="layout.edges"
    :node-types="nodeTypes"
    :fit-view-on-init="true"
    :nodes-draggable="false"
    :nodes-connectable="false"
    class="w-full h-full"
    @node-click="onNodeClick"
  />
</template>
