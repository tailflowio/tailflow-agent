import { ref } from 'vue'

const API_BASE = '/api'

export interface WorkflowInfo {
  name: string
  description: string
  tags: string[]
  author: string
  has_trigger: boolean
  trigger_type: string
}

export interface PipelineAction {
  action: string
  title?: string
}

export interface GraphNode {
  id: string
  label: string
  type: string
  action?: string
  pipeline?: PipelineAction[]
  when?: string
}

export interface GraphEdge {
  source: string
  target: string
  type?: string
  label?: string
}

export interface Graph {
  nodes: GraphNode[]
  edges: GraphEdge[]
}

export interface Execution {
  id: string
  workflow_name: string
  status: string
  started_at: string
  finished_at?: string
  params?: Record<string, unknown>
  steps?: Record<string, StepResult>
  error?: string
}

export interface StepResult {
  status: string
  started_at?: string
  finished_at?: string
  input?: unknown
  output?: unknown
  error?: string | { message: string }
}

export interface StepDetailHistory {
  execution_id: string
  status: string
  started_at: string
  finished_at?: string
  duration_ms: number
  output?: unknown
  error?: string
}

export interface StepDetailMetrics {
  total_executions: number
  success_count: number
  failure_count: number
  avg_duration_ms: number
  last_execution: string
}

export interface StepDetail {
  step: Record<string, unknown>
  history: StepDetailHistory[] | null
  metrics: StepDetailMetrics
}

export interface ProcessMetrics {
  cpu_percent: number
  memory_bytes: number
  goroutines: number
  net_rx_bytes: number
  net_tx_bytes: number
  uptime_s: number
}

export interface ListExecutionsParams {
  status?: string[]
  sort?: 'date' | 'duration'
  order?: 'asc' | 'desc'
  limit?: number
  offset?: number
}

export interface PaginatedResponse<T> {
  items: T[]
  total: number
}

export function useWorkflowApi() {
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function fetchJSON<T>(url: string, options?: RequestInit): Promise<T> {
    loading.value = true
    error.value = null
    try {
      const res = await fetch(url, { cache: 'no-store', ...options })
      if (!res.ok) {
        const body = await res.json().catch(() => ({ error: res.statusText }))
        throw new Error(body.error || res.statusText)
      }
      return await res.json()
    } catch (e) {
      error.value = (e as Error).message
      throw e
    } finally {
      loading.value = false
    }
  }

  function getWorkflow() {
    return fetchJSON<Record<string, unknown>>(`${API_BASE}/workflow`)
  }

  function getWorkflowGraph() {
    return fetchJSON<Graph>(`${API_BASE}/workflow/graph`)
  }

  function validateWorkflow() {
    return fetchJSON<{ valid: boolean; errors?: string[] }>(
      `${API_BASE}/workflow/validate`,
      { method: 'POST' }
    )
  }

  function runWorkflow(params?: Record<string, unknown>) {
    return fetchJSON<{ execution_id: string; status: string }>(
      `${API_BASE}/workflow/run`,
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ params }),
      }
    )
  }

  function listExecutions(params?: ListExecutionsParams) {
    const query = new URLSearchParams()
    if (params?.status?.length) query.set('status', params.status.join(','))
    if (params?.sort) query.set('sort', params.sort)
    if (params?.order) query.set('order', params.order)
    if (params?.limit != null) query.set('limit', String(params.limit))
    if (params?.offset != null) query.set('offset', String(params.offset))
    const qs = query.toString()
    return fetchJSON<PaginatedResponse<Execution>>(`${API_BASE}/executions${qs ? '?' + qs : ''}`)
  }

  function getExecution(id: string) {
    return fetchJSON<Execution>(`${API_BASE}/executions/${id}`)
  }

  function cancelExecution(id: string) {
    return fetchJSON<{ cancelled: boolean }>(`${API_BASE}/executions/${id}/cancel`, { method: 'POST' })
  }

  function getWorkflowActivity() {
    return fetchJSON<{ steps: Record<string, { running: string[]; waiting: string[] }> }>(
      `${API_BASE}/workflow/activity`
    )
  }

  function getStepDetail(id: string) {
    return fetchJSON<StepDetail>(`${API_BASE}/workflow/steps/${id}`)
  }

  function getMetrics() {
    return fetchJSON<ProcessMetrics>(`${API_BASE}/metrics`)
  }

  return {
    loading,
    error,
    getWorkflow,
    getWorkflowGraph,
    getWorkflowActivity,
    validateWorkflow,
    runWorkflow,
    listExecutions,
    getExecution,
    cancelExecution,
    getStepDetail,
    getMetrics,
  }
}
