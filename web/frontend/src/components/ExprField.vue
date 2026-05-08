<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import Kbd from './primitives/Kbd.vue'

export interface Suggestion {
  path: string
  kind: 'param' | 'var' | 'step' | 'env' | 'loop' | 'fn'
  type: string
  preview?: string
}

const props = defineProps<{
  modelValue: string
  label?: string
  suggestions?: Suggestion[]
  placeholder?: string
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', v: string): void
}>()

const value = ref(props.modelValue || '')
const showAuto = ref(false)
const sel = ref(0)
const inputEl = ref<HTMLInputElement | null>(null)
const popoverPos = ref<{ top: number; left: number; width: number }>({ top: 0, left: 0, width: 320 })

function recomputePos() {
  const wrap = inputEl.value?.parentElement
  if (!wrap) return
  const r = wrap.getBoundingClientRect()
  popoverPos.value = { top: r.bottom + 4, left: r.left, width: r.width }
}

watch(() => props.modelValue, (v) => { value.value = v || '' })
watch(value, (v) => emit('update:modelValue', v))

const filtered = computed<Suggestion[]>(() => {
  const q = value.value.replace(/[{}]/g, '').trim().toLowerCase()
  const all = props.suggestions || []
  if (!q) return all
  return all.filter(s => s.path.toLowerCase().includes(q) || s.kind.includes(q))
})

watch(value, () => { sel.value = 0 })

function pick(s: Suggestion) {
  value.value = s.path
  showAuto.value = false
}

function onFocus() {
  recomputePos()
  showAuto.value = true
}

function onBlur() {
  setTimeout(() => { showAuto.value = false }, 150)
}

function onKey(e: KeyboardEvent) {
  if (!showAuto.value) return
  if (e.key === 'ArrowDown') { e.preventDefault(); sel.value = Math.min(filtered.value.length - 1, sel.value + 1) }
  else if (e.key === 'ArrowUp') { e.preventDefault(); sel.value = Math.max(0, sel.value - 1) }
  else if (e.key === 'Enter') { e.preventDefault(); const s = filtered.value[sel.value]; if (s) pick(s) }
  else if (e.key === 'Escape') { e.preventDefault(); showAuto.value = false; nextTick(() => inputEl.value?.blur()) }
}

const TPL_OPEN = '{{'
const TPL_CLOSE = '}}'

const kindStyle = (k: Suggestion['kind']) => {
  if (k === 'param') return 'bg-violet-400/15 text-violet-400'
  if (k === 'step') return 'bg-emerald-400/15 text-emerald-400'
  if (k === 'var') return 'bg-amber-400/15 text-amber-400'
  if (k === 'env') return 'bg-orange-400/15 text-orange-400'
  if (k === 'loop') return 'bg-emerald-400/15 text-emerald-400'
  return 'bg-g-4 text-g-9'
}
</script>

<template>
  <div>
    <label v-if="label" class="text-[10px] font-mono uppercase tracking-wider text-g-8 mb-1 block">{{ label }}</label>
    <div class="relative">
      <div class="bg-g-1 border border-g-5 rounded px-2.5 py-1.5 font-mono text-[12px] focus-within:border-g-7 flex items-center">
        <span class="text-g-7 select-none">{{ TPL_OPEN }}&nbsp;</span>
        <input
          ref="inputEl"
          v-model="value"
          @focus="onFocus"
          @blur="onBlur"
          @keydown="onKey"
          :placeholder="placeholder || 'expression…'"
          class="flex-1 bg-transparent outline-none text-violet-400 placeholder:text-g-7"
        />
        <span class="text-g-7 select-none">&nbsp;{{ TPL_CLOSE }}</span>
      </div>
    </div>
  </div>
  <Teleport to="body">
    <div
      v-if="showAuto && filtered.length > 0"
      class="fixed bg-g-2 border border-g-6 rounded shadow-xl z-[80] anim-enter max-h-[280px] overflow-y-auto"
      :style="{ top: popoverPos.top + 'px', left: popoverPos.left + 'px', width: popoverPos.width + 'px' }"
    >
      <div class="px-2 py-1 text-[10px] uppercase tracking-wider font-mono text-g-8 border-b border-g-5 sticky top-0 bg-g-2">
        expr-lang · {{ filtered.length }} suggestion{{ filtered.length > 1 ? 's' : '' }}
      </div>
      <div
        v-for="(s, i) in filtered"
        :key="s.path"
        @mousedown.prevent="pick(s)"
        @mouseenter="sel = i"
        :class="['px-2 py-1.5 cursor-pointer', i === sel ? 'bg-g-3' : 'hover:bg-g-3']"
      >
        <div class="flex items-center gap-2">
          <span :class="['font-mono text-[10px] px-1 rounded', kindStyle(s.kind)]">{{ s.kind }}</span>
          <span class="font-mono text-[12px] text-g-13">{{ s.path }}</span>
          <span class="font-mono text-[10px] text-g-8 ml-auto">{{ s.type }}</span>
        </div>
        <div v-if="s.preview" class="font-mono text-[10px] text-g-9 mt-0.5 ml-9 truncate">→ {{ s.preview }}</div>
      </div>
      <div class="px-2 py-1 border-t border-g-5 flex items-center gap-2 text-[10px] font-mono text-g-8 sticky bottom-0 bg-g-2">
        <Kbd>↑</Kbd><Kbd>↓</Kbd> nav · <Kbd>↵</Kbd> insert · <Kbd>esc</Kbd> close
      </div>
    </div>
  </Teleport>
</template>
