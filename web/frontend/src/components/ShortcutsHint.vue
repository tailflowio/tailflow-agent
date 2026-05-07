<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import Icon from './primitives/Icon.vue'
import Kbd from './primitives/Kbd.vue'

const open = ref(false)

function toggle() {
  open.value = !open.value
}

function close() {
  open.value = false
}

function onKey(e: KeyboardEvent) {
  const target = e.target as HTMLElement
  if (e.key === '?' && !(target?.matches?.('input,textarea'))) {
    open.value = !open.value
  }
  if (e.key === 'Escape') open.value = false
}

onMounted(() => {
  window.addEventListener('toggle-shortcuts', toggle)
  window.addEventListener('keydown', onKey)
})
onBeforeUnmount(() => {
  window.removeEventListener('toggle-shortcuts', toggle)
  window.removeEventListener('keydown', onKey)
})

const groups: { name: string; items: [string, string | null, string][] }[] = [
  { name: 'Global', items: [['⌘', 'K', 'command palette'], ['?', null, 'this menu'], ['esc', null, 'close panel']] },
  { name: 'Navigation', items: [['g', 'd', 'dashboard'], ['g', 'r', 'live runs'], ['g', 'h', 'history']] },
  { name: 'Run', items: [['R', null, 'rerun current'], ['⌘', '↵', 'run with params'], ['X', null, 'cancel run']] },
  { name: 'Inspector', items: [['I', null, 'toggle inspector'], ['J', null, 'next step'], ['K', null, 'prev step'], ['O', null, 'copy output']] },
]
</script>

<template>
  <Teleport to="body">
    <template v-if="open">
      <div class="fixed inset-0 z-[60]" style="background: rgba(0, 0, 0, 0.4)" @click="close" />
      <div
        class="fixed z-[61] bg-g-2 border border-g-6 rounded-lg shadow-2xl w-[640px] max-h-[80vh] overflow-auto anim-enter top-1/2 -translate-y-1/2"
        style="left: calc(220px + (100vw - 220px - var(--inspector-w, 0px) - 640px) / 2)"
      >
        <div class="flex items-center justify-between px-4 py-3 border-b border-g-5">
          <div class="flex items-center gap-2">
            <span class="text-[13px] font-semibold text-g-14">Keyboard shortcuts</span>
            <span class="text-[11px] font-mono text-g-8">tailflow</span>
          </div>
          <button @click="close" class="text-g-9 hover:text-g-12">
            <Icon name="x" class-name="w-4 h-4" />
          </button>
        </div>
        <div class="grid grid-cols-2 gap-x-6 gap-y-1 p-4">
          <div v-for="g in groups" :key="g.name" class="mb-2">
            <div class="text-[10px] font-mono uppercase tracking-wider text-g-8 mb-1.5">{{ g.name }}</div>
            <div v-for="(it, i) in g.items" :key="i" class="flex items-center justify-between py-1 text-[12px]">
              <span class="text-g-11">{{ it[2] }}</span>
              <div class="flex items-center gap-1 font-mono">
                <Kbd>{{ it[0] }}</Kbd>
                <Kbd v-if="it[1]">{{ it[1] }}</Kbd>
              </div>
            </div>
          </div>
        </div>
        <div class="border-t border-g-5 px-4 py-2 text-[10px] font-mono text-g-8 flex items-center justify-between">
          <span>press <Kbd>?</Kbd> to toggle</span>
          <span>or <Kbd>esc</Kbd> to close</span>
        </div>
      </div>
    </template>
  </Teleport>
</template>
