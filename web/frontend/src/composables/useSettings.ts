import { ref, watch } from 'vue'

export type AnimStyle = 'token' | 'particle' | 'pulse'
export type Density = 'comfortable' | 'compact'

export interface Settings {
  animStyle: AnimStyle
  density: Density
  autoOpenInspector: boolean
}

const STORAGE_KEY = 'tailflow-settings'

function load(): Settings {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw) {
      const v = JSON.parse(raw)
      return {
        animStyle: (v.animStyle === 'particle' || v.animStyle === 'pulse') ? v.animStyle : 'token',
        density: v.density === 'compact' ? 'compact' : 'comfortable',
        autoOpenInspector: !!v.autoOpenInspector,
      }
    }
  } catch {}
  return { animStyle: 'token', density: 'comfortable', autoOpenInspector: false }
}

const settings = ref<Settings>(load())
const open = ref(false)

watch(settings, (v) => {
  try { localStorage.setItem(STORAGE_KEY, JSON.stringify(v)) } catch {}
}, { deep: true })

export function useSettings() {
  function set<K extends keyof Settings>(key: K, value: Settings[K]) {
    settings.value = { ...settings.value, [key]: value }
  }
  function toggleOpen() { open.value = !open.value }
  function close() { open.value = false }
  return { settings, set, open, toggleOpen, close }
}
