<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { EditorState, Compartment } from '@codemirror/state'
import { EditorView, keymap, lineNumbers, highlightActiveLine } from '@codemirror/view'
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands'
import { yaml } from '@codemirror/lang-yaml'
import { oneDark } from '@codemirror/theme-one-dark'

const props = defineProps<{
  modelValue: string
  readonly?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', v: string): void
  (e: 'save'): void
}>()

const containerRef = ref<HTMLElement | null>(null)
let view: EditorView | null = null
const readonlyComp = new Compartment()

function buildState(initial: string) {
  return EditorState.create({
    doc: initial,
    extensions: [
      lineNumbers(),
      history(),
      highlightActiveLine(),
      keymap.of([...defaultKeymap, ...historyKeymap, indentWithTab, {
        key: 'Mod-s',
        preventDefault: true,
        run: () => { emit('save'); return true },
      }]),
      yaml(),
      oneDark,
      readonlyComp.of(EditorState.readOnly.of(!!props.readonly)),
      EditorView.updateListener.of((u) => {
        if (u.docChanged) {
          const v = u.state.doc.toString()
          if (v !== props.modelValue) emit('update:modelValue', v)
        }
      }),
      EditorView.theme({
        '&': { fontSize: '12px', height: '100%' },
        '.cm-scroller': { fontFamily: 'DM Mono, ui-monospace, monospace' },
        '.cm-gutters': { backgroundColor: 'var(--g-1)', borderRight: '1px solid var(--g-5)', color: 'var(--g-7)' },
        '.cm-content': { caretColor: 'var(--g-13)' },
        '&.cm-focused': { outline: 'none' },
      }),
    ],
  })
}

onMounted(() => {
  if (!containerRef.value) return
  view = new EditorView({
    state: buildState(props.modelValue || ''),
    parent: containerRef.value,
  })
})

onBeforeUnmount(() => {
  view?.destroy()
  view = null
})

watch(() => props.modelValue, (v) => {
  if (!view) return
  if (view.state.doc.toString() === v) return
  view.dispatch({
    changes: { from: 0, to: view.state.doc.length, insert: v || '' },
  })
})

watch(() => props.readonly, (ro) => {
  if (!view) return
  view.dispatch({ effects: readonlyComp.reconfigure(EditorState.readOnly.of(!!ro)) })
})
</script>

<template>
  <div ref="containerRef" class="h-full w-full overflow-hidden" />
</template>

<style>
.cm-editor { height: 100%; background: var(--g-1) !important; }
.cm-editor .cm-content { background: var(--g-1); }
</style>
