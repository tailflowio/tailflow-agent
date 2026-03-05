import { ref, onUnmounted } from 'vue'
import type { WorkflowEvent } from '@tailflow/shared'

export type { WorkflowEvent }

export interface StepVolume {
  input?: unknown
  output?: unknown
  waiting?: { type: string; since: string; details: unknown }
}

export interface StepOutputEntry {
  iteration: number
  output: unknown
  timestamp: string
}

export interface PipelineEntry {
  iteration: number
  totalIterations: number
  stepIndex: number
  totalSteps: number
  action: string
  status: 'running' | 'ok' | 'failed'
  durationMs?: number
}

export interface SSEOptions {
  onDisconnect?: () => void
}

export function useSSE(executionId: string, options?: SSEOptions) {
  const events = ref<WorkflowEvent[]>([])
  const connected = ref(false)
  const finished = ref(false)
  const stepStatuses = ref<Record<string, string>>({})
  const stepVolumes = ref<Record<string, StepVolume>>({})
  const stepIterations = ref<Record<string, number>>({})
  const stepOutputHistory = ref<Record<string, StepOutputEntry[]>>({})
  const stepPipelineProgress = ref<Record<string, PipelineEntry[]>>({})

  let eventSource: EventSource | null = null

  // --- rAF batching buffers (non-reactive) ---
  let pendingEvents: WorkflowEvent[] = []
  let pendingStatuses: Record<string, string> = {}
  let pendingVolumes: Record<string, Partial<StepVolume>> = {}
  let pendingIterations: Record<string, number> = {}
  let pendingOutputEntries: Record<string, StepOutputEntry[]> = {}
  let pendingPipelineUpdates: Record<string, PipelineEntry[]> = {}
  let pendingFinished = false
  let rafId: number | null = null

  function scheduleFlush() {
    if (rafId !== null) return
    rafId = requestAnimationFrame(flushBatch)
  }

  function flushBatch() {
    rafId = null

    // Events
    if (pendingEvents.length > 0) {
      events.value.push(...pendingEvents)
      pendingEvents = []
    }

    // Step statuses
    const statusKeys = Object.keys(pendingStatuses)
    if (statusKeys.length > 0) {
      stepStatuses.value = { ...stepStatuses.value, ...pendingStatuses }
      pendingStatuses = {}
    }

    // Volumes
    const volKeys = Object.keys(pendingVolumes)
    if (volKeys.length > 0) {
      const current = { ...stepVolumes.value }
      for (const stepId of volKeys) {
        current[stepId] = { ...(current[stepId] || {}), ...pendingVolumes[stepId] }
      }
      stepVolumes.value = current
      pendingVolumes = {}
    }

    // Iterations
    const iterKeys = Object.keys(pendingIterations)
    if (iterKeys.length > 0) {
      stepIterations.value = { ...stepIterations.value, ...pendingIterations }
      pendingIterations = {}
    }

    // Output history
    const outKeys = Object.keys(pendingOutputEntries)
    if (outKeys.length > 0) {
      const current = { ...stepOutputHistory.value }
      for (const stepId of outKeys) {
        if (!current[stepId]) current[stepId] = []
        current[stepId] = [...current[stepId], ...pendingOutputEntries[stepId]]
      }
      stepOutputHistory.value = current
      pendingOutputEntries = {}
    }

    // Pipeline progress
    const pipeKeys = Object.keys(pendingPipelineUpdates)
    if (pipeKeys.length > 0) {
      const current = { ...stepPipelineProgress.value }
      for (const stepId of pipeKeys) {
        if (!current[stepId]) current[stepId] = []
        const arr = [...current[stepId]]
        for (const entry of pendingPipelineUpdates[stepId]) {
          const existingIdx = arr.findIndex(
            (p) => p.iteration === entry.iteration && p.stepIndex === entry.stepIndex
          )
          if (existingIdx >= 0) {
            arr[existingIdx] = entry
          } else {
            arr.push(entry)
          }
        }
        current[stepId] = arr
      }
      stepPipelineProgress.value = current
      pendingPipelineUpdates = {}
    }

    // Finished flag (must be last so UI sees all events before closing)
    if (pendingFinished) {
      finished.value = true
      pendingFinished = false
      disconnect()
    }
  }

  function connect() {
    eventSource = new EventSource(`/api/executions/${executionId}/events`)

    eventSource.addEventListener('connected', () => {
      // Clear all state on (re)connect — server replays stored events
      events.value = []
      pendingEvents = []
      pendingStatuses = {}
      pendingVolumes = {}
      pendingIterations = {}
      pendingOutputEntries = {}
      pendingPipelineUpdates = {}
      stepStatuses.value = {}
      stepVolumes.value = {}
      stepIterations.value = {}
      stepOutputHistory.value = {}
      stepPipelineProgress.value = {}
      pendingFinished = false
      connected.value = true
    })

    eventSource.addEventListener('step.started', (e) => {
      const event: WorkflowEvent = JSON.parse(e.data)
      pendingEvents.push(event)
      if (event.step_id) {
        pendingStatuses[event.step_id] = 'running'
      }
      scheduleFlush()
    })

    eventSource.addEventListener('step.completed', (e) => {
      const event: WorkflowEvent = JSON.parse(e.data)
      pendingEvents.push(event)
      if (event.step_id) {
        pendingStatuses[event.step_id] = 'success'
      }
      scheduleFlush()
    })

    eventSource.addEventListener('step.failed', (e) => {
      const event: WorkflowEvent = JSON.parse(e.data)
      pendingEvents.push(event)
      if (event.step_id) {
        pendingStatuses[event.step_id] = 'failed'
      }
      scheduleFlush()
    })

    eventSource.addEventListener('step.skipped', (e) => {
      const event: WorkflowEvent = JSON.parse(e.data)
      pendingEvents.push(event)
      if (event.step_id) {
        pendingStatuses[event.step_id] = 'skipped'
      }
      scheduleFlush()
    })

    eventSource.addEventListener('step.waiting', (e) => {
      const event: WorkflowEvent = JSON.parse(e.data)
      pendingEvents.push(event)
      if (event.step_id) {
        pendingStatuses[event.step_id] = 'waiting'
        pendingVolumes[event.step_id] = {
          ...(pendingVolumes[event.step_id] || {}),
          waiting: {
            type: (event.data?.wait_type as string) || 'webhook',
            since: event.timestamp,
            details: event.data,
          },
        }
      }
      scheduleFlush()
    })

    eventSource.addEventListener('step.input', (e) => {
      const event: WorkflowEvent = JSON.parse(e.data)
      pendingEvents.push(event)
      if (event.step_id) {
        pendingVolumes[event.step_id] = {
          ...(pendingVolumes[event.step_id] || {}),
          input: event.data,
        }
      }
      scheduleFlush()
    })

    eventSource.addEventListener('step.output', (e) => {
      const event: WorkflowEvent = JSON.parse(e.data)
      pendingEvents.push(event)
      if (event.step_id) {
        pendingVolumes[event.step_id] = {
          ...(pendingVolumes[event.step_id] || {}),
          output: event.data?.output,
        }
        // Track output history per iteration
        if (!pendingOutputEntries[event.step_id]) {
          pendingOutputEntries[event.step_id] = []
        }
        pendingOutputEntries[event.step_id].push({
          iteration: pendingIterations[event.step_id] ?? stepIterations.value[event.step_id] ?? 1,
          output: event.data?.output,
          timestamp: event.timestamp,
        })
      }
      scheduleFlush()
    })

    eventSource.addEventListener('step.log', (e) => {
      const event: WorkflowEvent = JSON.parse(e.data)
      pendingEvents.push(event)

      // Parse pipeline progress from log messages
      if (event.step_id && event.message) {
        const match = event.message.match(/^\[(\d+)\/(\d+)\] step (\d+)\/(\d+) (\S+)(?:\s+(OK|FAILED)\s+\((\d+)ms\))?$/)
        if (match) {
          const entry: PipelineEntry = {
            iteration: parseInt(match[1]),
            totalIterations: parseInt(match[2]),
            stepIndex: parseInt(match[3]),
            totalSteps: parseInt(match[4]),
            action: match[5],
            status: match[6] === 'OK' ? 'ok' : match[6] === 'FAILED' ? 'failed' : 'running',
            durationMs: match[7] ? parseInt(match[7]) : undefined,
          }
          if (!pendingPipelineUpdates[event.step_id]) {
            pendingPipelineUpdates[event.step_id] = []
          }
          pendingPipelineUpdates[event.step_id].push(entry)
        }
      }
      scheduleFlush()
    })

    eventSource.addEventListener('step.goto', (e) => {
      const event: WorkflowEvent = JSON.parse(e.data)
      pendingEvents.push(event)
      if (event.step_id) {
        const iteration = (event.data?.iteration as number) || 2
        // Set iteration for ALL body steps (not just the goto step)
        const body = (event.data?.body as string[]) || []
        for (const bid of body) {
          pendingIterations[bid] = iteration
          pendingStatuses[bid] = 'pending'
        }
      }
      scheduleFlush()
    })

    eventSource.addEventListener('workflow.started', (e) => {
      const event: WorkflowEvent = JSON.parse(e.data)
      pendingEvents.push(event)
      scheduleFlush()
    })

    eventSource.addEventListener('workflow.completed', (e) => {
      const event: WorkflowEvent = JSON.parse(e.data)
      pendingEvents.push(event)
      pendingFinished = true
      scheduleFlush()
    })

    eventSource.onerror = () => {
      connected.value = false
      // If the connection closed before we got workflow.completed,
      // flush any pending events and mark finished to stop auto-reconnect.
      if (!finished.value && pendingFinished) {
        if (rafId !== null) {
          cancelAnimationFrame(rafId)
          rafId = null
        }
        flushBatch()
      }
      // Notify caller so it can re-fetch execution status
      if (!finished.value && options?.onDisconnect) {
        options.onDisconnect()
      }
    }
  }

  function disconnect() {
    if (eventSource) {
      eventSource.close()
      eventSource = null
    }
    connected.value = false

    // Cancel pending rAF and flush synchronously
    if (rafId !== null) {
      cancelAnimationFrame(rafId)
      rafId = null
      flushBatch()
    }
  }

  onUnmounted(disconnect)

  return {
    events,
    connected,
    finished,
    stepStatuses,
    stepVolumes,
    stepIterations,
    stepOutputHistory,
    stepPipelineProgress,
    connect,
    disconnect,
  }
}
