import { sharedFr } from '@tailflow/shared'

export default {
  ...sharedFr,
  nav: {
    overview: "Vue d'ensemble",
    executions: 'Historique',
    liveRuns: 'Live runs',
    run: 'Executer',
  },
  run: {
    title: 'Executer {name}',
    noParams: "Ce workflow n'a pas de parametres.",
    default: 'Defaut : {value}',
    cancel: 'Annuler',
    submit: 'Executer',
  },
  dashboard: {
    total: 'Total',
    success: 'Succes',
    failed: 'Echoue',
    running: 'En cours',
    avgDuration: 'Duree moy.',
    recentExecs: 'Executions recentes',
    viewAll: 'Voir tout',
    noExecs: 'Aucune execution.',
    nextIn: 'next dans {time}',
    nextNow: 'next now',
    systemMetrics: 'Systeme',
    workflowMetrics: 'Workflow',
  },
  execution: {
    inProgress: 'en cours...',
    cancelling: 'Annulation...',
    cancel: 'Annuler',
  },
  executions: {
    title: 'Executions',
    none: 'Aucune execution.',
    loading: 'Chargement...',
    connectionLost: "Impossible de joindre l'agent",
    connectionLostDesc: "L'agent ne repond pas. Verifiez qu'il est en cours d'execution et reessayez.",
    retry: 'Reessayer',
  },
}
