import { sharedEn } from '@tailflow/shared'

export default {
  ...sharedEn,
  nav: {
    overview: 'Dashboard',
    executions: 'History',
    liveRuns: 'Live runs',
    run: 'Run',
  },
  run: {
    title: 'Run {name}',
    noParams: 'This workflow has no parameters.',
    default: 'Default: {value}',
    cancel: 'Cancel',
    submit: 'Run',
  },
  dashboard: {
    total: 'Total',
    success: 'Success',
    failed: 'Failed',
    running: 'Running',
    avgDuration: 'Avg duration',
    recentExecs: 'Recent executions',
    viewAll: 'View all',
    noExecs: 'No executions yet.',
    nextIn: 'next in {time}',
    nextNow: 'next now',
    systemMetrics: 'System',
    workflowMetrics: 'Workflow',
  },
  execution: {
    inProgress: 'in progress...',
    cancelling: 'Cancelling...',
    cancel: 'Cancel',
  },
  executions: {
    title: 'Executions',
    none: 'No executions yet.',
    loading: 'Loading...',
    connectionLost: 'Unable to reach the agent',
    connectionLostDesc: 'The workflow agent is not responding. Make sure it is running and try again.',
    retry: 'Retry',
  },
}
