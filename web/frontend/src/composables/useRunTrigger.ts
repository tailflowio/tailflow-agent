import { ref } from 'vue'

const showRunDialog = ref(false)
const workflowReady = ref(false)
const workflowName = ref('')
const workflowParams = ref<{ name: string; type: string; required?: boolean; default?: unknown }[]>([])

export function useRunTrigger() {
  return { showRunDialog, workflowReady, workflowName, workflowParams }
}
