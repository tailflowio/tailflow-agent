<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import Icon from './primitives/Icon.vue'
import Kbd from './primitives/Kbd.vue'

const props = defineProps<{
  workflowName?: string
}>()

const emit = defineEmits<{
  (e: 'open-palette'): void
  (e: 'open-shortcuts'): void
  (e: 'toggle-theme'): void
  (e: 'toggle-locale'): void
}>()

const route = useRoute()

const breadcrumbs = computed<string[]>(() => {
  const wf = props.workflowName || ''
  const path = route.path
  if (path === '/') return wf ? [wf] : ['dashboard']
  if (route.name === 'execution') return [wf, 'runs', String(route.params.id || '').slice(0, 8)].filter(Boolean) as string[]
  if (route.name === 'step-detail') return [wf, 'steps', String(route.params.id || '')].filter(Boolean) as string[]
  if (path === '/executions') return [wf, 'history'].filter(Boolean) as string[]
  return [wf || 'tailflow']
})
</script>

<template>
  <header class="h-11 bg-g-1 border-b border-g-5 flex items-center px-4 gap-3 shrink-0">
    <div class="flex items-center gap-1.5 text-[12px] font-mono">
      <template v-for="(c, i) in breadcrumbs" :key="i">
        <span v-if="i > 0" class="text-g-7">/</span>
        <span :class="i === breadcrumbs.length - 1 ? 'text-g-13' : 'text-g-9'">{{ c }}</span>
      </template>
    </div>

    <div class="flex-1 flex justify-center">
      <button
        @click="emit('open-palette')"
        class="bg-g-2 border border-g-5 rounded h-7 w-[420px] flex items-center gap-2 px-2.5 hover:bg-g-3 hover:border-g-6 transition-colors"
      >
        <Icon name="search" class-name="w-3.5 h-3.5 text-g-8" />
        <span class="text-[11px] font-mono text-g-8 flex-1 text-left">Run · jump to step · open run · search…</span>
        <Kbd>⌘</Kbd><Kbd>K</Kbd>
      </button>
    </div>

    <div class="flex items-center gap-1.5">
      <button
        @click="emit('toggle-locale')"
        class="h-7 px-2 rounded hover:bg-g-2 flex items-center justify-center text-g-9 hover:text-g-13 font-mono text-[11px]"
        title="Toggle locale"
      >
        <span>FR/EN</span>
      </button>
      <button
        @click="emit('toggle-theme')"
        class="h-7 w-7 rounded hover:bg-g-2 flex items-center justify-center text-g-9 hover:text-g-13"
        title="Toggle theme"
      >
        <Icon name="eye" class-name="w-3.5 h-3.5" />
      </button>
      <button
        class="h-7 w-7 rounded hover:bg-g-2 flex items-center justify-center text-g-9 hover:text-g-13 relative"
        title="Notifications"
      >
        <Icon name="bell" class-name="w-3.5 h-3.5" />
        <span class="absolute top-1 right-1 w-1.5 h-1.5 bg-amber-400 rounded-full" />
      </button>
      <button
        @click="emit('open-shortcuts')"
        class="h-7 w-7 rounded hover:bg-g-2 flex items-center justify-center text-g-9 hover:text-g-13 font-mono text-[12px]"
        title="Keyboard shortcuts (?)"
      >?</button>
      <RouterLink
        to="/docs"
        :class="['h-7 px-2 rounded flex items-center gap-1.5 text-[11px] font-mono', $route.path === '/docs' ? 'bg-g-3 text-g-13' : 'text-g-9 hover:text-g-13 hover:bg-g-2']"
      >
        <Icon name="docs" class-name="w-3.5 h-3.5" />
        docs
      </RouterLink>
    </div>
  </header>
</template>
