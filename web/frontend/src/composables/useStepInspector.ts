import { ref } from 'vue'

const stepId = ref<string | null>(null)
const open = ref(false)

export function useStepInspector() {
  function inspect(id: string) {
    stepId.value = id
    open.value = true
  }
  function close() {
    open.value = false
  }
  function toggle() {
    if (open.value) {
      open.value = false
    } else if (stepId.value) {
      open.value = true
    }
  }
  return { stepId, open, inspect, close, toggle }
}
