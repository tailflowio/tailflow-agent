import { onBeforeUnmount, onMounted, type Ref } from 'vue'
import { useRouter } from 'vue-router'

interface Options {
  paletteOpen: Ref<boolean>
  inspectorOpen?: Ref<boolean>
}

export function useShortcuts(opts: Options) {
  const router = useRouter()
  let gPrefix = false
  let gTimer: ReturnType<typeof setTimeout> | null = null

  function clearG() {
    gPrefix = false
    if (gTimer) {
      clearTimeout(gTimer)
      gTimer = null
    }
  }

  function onKey(e: KeyboardEvent) {
    const target = e.target as HTMLElement | null
    if (target?.tagName === 'INPUT' || target?.tagName === 'TEXTAREA') return

    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
      e.preventDefault()
      opts.paletteOpen.value = true
      return
    }
    if (e.key === 'Escape') {
      opts.paletteOpen.value = false
      if (opts.inspectorOpen) opts.inspectorOpen.value = false
      return
    }
    if (e.key === 'i' && opts.inspectorOpen) {
      opts.inspectorOpen.value = !opts.inspectorOpen.value
      return
    }
    if (e.key === 'g') {
      gPrefix = true
      if (gTimer) clearTimeout(gTimer)
      gTimer = setTimeout(clearG, 800)
      return
    }
    if (gPrefix) {
      if (e.key === 'd') { router.push('/'); clearG() }
      else if (e.key === 'r') { router.push('/executions'); clearG() }
      else if (e.key === 'h') { router.push('/executions'); clearG() }
      else if (e.key === 'e') { router.push('/editor'); clearG() }
    }
  }

  onMounted(() => window.addEventListener('keydown', onKey))
  onBeforeUnmount(() => {
    window.removeEventListener('keydown', onKey)
    if (gTimer) clearTimeout(gTimer)
  })
}
