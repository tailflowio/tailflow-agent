import { ref, computed, onMounted, onUnmounted } from 'vue'
import type { ProcessMetrics } from './useWorkflowApi'

export interface StepCounts {
  running: string[]
  waiting: string[]
}

/**
 * Subscribes to the global SSE stream (/api/events) for real-time updates
 * and hydrates initial state from /api/workflow/activity endpoint.
 */
export function useGlobalEvents(onEvent?: () => void) {
  const connected = ref(false)
  let eventSource: EventSource | null = null
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null

  // executionId -> stepId -> 'running' | 'waiting'
  const execStepStates = ref<Record<string, Record<string, string>>>({})

  // Recent terminal statuses (success/failed/skipped) with auto-expiry
  const recentStatuses = ref<Record<string, string>>({})
  const statusTimers: Record<string, ReturnType<typeof setTimeout>> = {}

  function setRecentStatus(stepId: string, status: string, ttl = 4000) {
    recentStatuses.value = { ...recentStatuses.value, [stepId]: status }
    if (statusTimers[stepId]) clearTimeout(statusTimers[stepId])
    statusTimers[stepId] = setTimeout(() => {
      const copy = { ...recentStatuses.value }
      delete copy[stepId]
      recentStatuses.value = copy
      delete statusTimers[stepId]
    }, ttl)
  }

  function clearRecentStatus(stepId: string) {
    if (recentStatuses.value[stepId]) {
      const copy = { ...recentStatuses.value }
      delete copy[stepId]
      recentStatuses.value = copy
    }
    if (statusTimers[stepId]) {
      clearTimeout(statusTimers[stepId])
      delete statusTimers[stepId]
    }
  }

  // Debounce the callback: max 1 call/sec
  let debounceTimer: ReturnType<typeof setTimeout> | null = null
  function debouncedCallback() {
    if (!onEvent) return
    if (debounceTimer) return
    debounceTimer = setTimeout(() => {
      debounceTimer = null
      onEvent()
    }, 1000)
  }

  // Per step, list of execution IDs in running/waiting state
  const stepCounts = computed<Record<string, StepCounts>>(() => {
    const counts: Record<string, StepCounts> = {}
    for (const [execId, steps] of Object.entries(execStepStates.value)) {
      for (const [stepId, status] of Object.entries(steps)) {
        if (!counts[stepId]) counts[stepId] = { running: [], waiting: [] }
        if (status === 'running') counts[stepId].running.push(execId)
        else if (status === 'waiting') counts[stepId].waiting.push(execId)
      }
    }
    return counts
  })

  // Per-step total execution counts (from backend)
  const stepExecCounts = ref<Record<string, number>>({})

  // System metrics pushed via SSE
  const sysMetrics = ref<ProcessMetrics | null>(null)

  // Fetch activity from backend (source of truth)
  async function fetchActivity() {
    try {
      const resp = await fetch('/api/workflow/activity')
      if (!resp.ok) return
      const data = await resp.json()
      const steps: Record<string, { running?: string[]; waiting?: string[] }> = data.steps || {}

      // Rebuild execStepStates from API
      const newStates: Record<string, Record<string, string>> = {}
      for (const [stepId, activity] of Object.entries(steps)) {
        for (const execId of activity.running || []) {
          if (!newStates[execId]) newStates[execId] = {}
          newStates[execId][stepId] = 'running'
        }
        for (const execId of activity.waiting || []) {
          if (!newStates[execId]) newStates[execId] = {}
          newStates[execId][stepId] = 'waiting'
        }
      }
      execStepStates.value = newStates

      // Hydrate exec counts
      if (data.exec_counts) {
        stepExecCounts.value = data.exec_counts
      }
    } catch { /* ignore */ }
  }

  const eventTypes = [
    'workflow.started',
    'workflow.completed',
    'step.started',
    'step.completed',
    'step.failed',
    'step.skipped',
    'step.waiting',
  ]

  function connect() {
    eventSource = new EventSource('/api/events')

    eventSource.addEventListener('connected', () => {
      connected.value = true
    })

    eventSource.addEventListener('metrics', (e: MessageEvent) => {
      try {
        const data = JSON.parse(e.data)
        sysMetrics.value = data.data as ProcessMetrics
      } catch { /* ignore */ }
    })

    for (const type of eventTypes) {
      eventSource.addEventListener(type, (e: MessageEvent) => {
        try {
          const data = JSON.parse(e.data)
          const execId = data.execution_id as string
          const stepId = data.step_id as string

          if (type === 'workflow.started' && execId) {
            execStepStates.value[execId] = {}
          } else if (type === 'workflow.completed' && execId) {
            delete execStepStates.value[execId]
            // Immediately notify on terminal events (no debounce)
            if (onEvent) onEvent()
          } else if (execId && stepId) {
            if (!execStepStates.value[execId]) {
              execStepStates.value[execId] = {}
            }
            if (type === 'step.started') {
              execStepStates.value[execId][stepId] = 'running'
              clearRecentStatus(stepId)
              stepExecCounts.value = { ...stepExecCounts.value, [stepId]: (stepExecCounts.value[stepId] || 0) + 1 }
            } else if (type === 'step.waiting') {
              execStepStates.value[execId][stepId] = 'waiting'
              clearRecentStatus(stepId)
            } else {
              // step.completed, step.failed, step.skipped
              delete execStepStates.value[execId][stepId]
              if (type === 'step.completed') setRecentStatus(stepId, 'success')
              else if (type === 'step.failed') setRecentStatus(stepId, 'failed')
              else if (type === 'step.skipped') setRecentStatus(stepId, 'skipped', 2000)
            }
          }
        } catch { /* ignore parse errors */ }

        debouncedCallback()
      })
    }

    eventSource.onerror = () => {
      connected.value = false
      disconnect()
      reconnectTimer = setTimeout(connect, 3000)
    }
  }

  function disconnect() {
    if (eventSource) {
      eventSource.close()
      eventSource = null
    }
    connected.value = false
  }

  onMounted(() => {
    fetchActivity() // Hydrate from API on mount
    connect()
  })
  onUnmounted(() => {
    disconnect()
    if (reconnectTimer) clearTimeout(reconnectTimer)
    if (debounceTimer) clearTimeout(debounceTimer)
    for (const t of Object.values(statusTimers)) clearTimeout(t)
  })

  return { connected, stepCounts, recentStatuses, stepExecCounts, sysMetrics }
}
