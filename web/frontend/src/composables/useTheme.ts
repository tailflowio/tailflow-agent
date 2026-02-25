import { ref, watch } from 'vue'

type Theme = 'dark' | 'light'

const STORAGE_KEY = 'tailflow-theme'

function getInitial(): Theme {
  const stored = localStorage.getItem(STORAGE_KEY)
  if (stored === 'light' || stored === 'dark') return stored
  return 'dark'
}

const theme = ref<Theme>(getInitial())

// Apply on load
document.documentElement.classList.toggle('light', theme.value === 'light')

watch(theme, (v) => {
  document.documentElement.classList.toggle('light', v === 'light')
  localStorage.setItem(STORAGE_KEY, v)
})

export function useTheme() {
  function toggle() {
    theme.value = theme.value === 'dark' ? 'light' : 'dark'
  }

  return { theme, toggle }
}
