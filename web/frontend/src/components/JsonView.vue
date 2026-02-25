<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

const props = defineProps<{
  data: unknown
}>()

const tab = ref<'list' | 'json'>('list')
const search = ref('')

// ── List mode (flat key-value pairs) ──

interface FlatEntry {
  key: string
  value: string
  type: string
}

function flatten(value: unknown, prefix: string = ''): FlatEntry[] {
  if (value === null || value === undefined) {
    return [{ key: prefix || '(root)', value: 'null', type: 'null' }]
  }
  if (typeof value === 'string') {
    return [{ key: prefix || '(root)', value, type: 'string' }]
  }
  if (typeof value === 'number') {
    return [{ key: prefix || '(root)', value: String(value), type: 'number' }]
  }
  if (typeof value === 'boolean') {
    return [{ key: prefix || '(root)', value: String(value), type: 'boolean' }]
  }
  if (Array.isArray(value)) {
    if (value.length === 0) return [{ key: prefix || '(root)', value: '[]', type: 'null' }]
    const entries: FlatEntry[] = []
    value.forEach((v, i) => {
      const k = prefix ? `${prefix}[${i}]` : `[${i}]`
      entries.push(...flatten(v, k))
    })
    return entries
  }
  if (typeof value === 'object') {
    const obj = value as Record<string, unknown>
    const keys = Object.keys(obj)
    if (keys.length === 0) return [{ key: prefix || '(root)', value: '{}', type: 'null' }]
    const entries: FlatEntry[] = []
    for (const k of keys) {
      const path = prefix ? `${prefix}.${k}` : k
      entries.push(...flatten(obj[k], path))
    }
    return entries
  }
  return [{ key: prefix || '(root)', value: String(value), type: 'string' }]
}

const flatEntries = computed(() => {
  if (props.data == null) return []
  return flatten(props.data)
})

const filteredEntries = computed(() => {
  const q = search.value.toLowerCase().trim()
  if (!q) return flatEntries.value
  return flatEntries.value.filter(e =>
    e.key.toLowerCase().includes(q) || e.value.toLowerCase().includes(q)
  )
})

function valClass(type: string): string {
  if (type === 'string') return 'jv-str'
  if (type === 'number') return 'jv-num'
  if (type === 'boolean') return 'jv-bool'
  if (type === 'null') return 'jv-null'
  return ''
}

// ── JSON mode (raw highlighted) ──

function escapeHtml(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

function highlightJson(value: unknown, indent: number = 0): string {
  const pad = '  '.repeat(indent)
  const padInner = '  '.repeat(indent + 1)

  if (value === null || value === undefined) {
    return `<span class="jv-null">null</span>`
  }
  if (typeof value === 'boolean') {
    return `<span class="jv-bool">${value}</span>`
  }
  if (typeof value === 'number') {
    return `<span class="jv-num">${value}</span>`
  }
  if (typeof value === 'string') {
    return `<span class="jv-str">"${escapeHtml(value)}"</span>`
  }
  if (Array.isArray(value)) {
    if (value.length === 0) return `<span class="jv-brace">[]</span>`
    const items = value.map(v => `${padInner}${highlightJson(v, indent + 1)}`)
    return `<span class="jv-brace">[</span>\n${items.join(',\n')}\n${pad}<span class="jv-brace">]</span>`
  }
  if (typeof value === 'object') {
    const entries = Object.entries(value as Record<string, unknown>)
    if (entries.length === 0) return `<span class="jv-brace">{}</span>`
    const lines = entries.map(([k, v]) => {
      return `${padInner}<span class="jv-key">"${escapeHtml(k)}"</span><span class="jv-colon">:</span> ${highlightJson(v, indent + 1)}`
    })
    return `<span class="jv-brace">{</span>\n${lines.join(',\n')}\n${pad}<span class="jv-brace">}</span>`
  }
  return escapeHtml(String(value))
}

const jsonHtml = computed(() => {
  if (props.data == null) return '<span class="jv-null">-</span>'
  return highlightJson(props.data)
})
</script>

<template>
  <div class="jv-container">
    <!-- Tabs -->
    <div class="jv-tabs">
      <button
        :class="['jv-tab', tab === 'list' && 'jv-tab-active']"
        @click="tab = 'list'"
      >{{ t('common.list') }}</button>
      <button
        :class="['jv-tab', tab === 'json' && 'jv-tab-active']"
        @click="tab = 'json'"
      >{{ t('common.json') }}</button>
    </div>

    <!-- List mode (flat) -->
    <div v-if="tab === 'list'" class="jv-list">
      <input
        v-model="search"
        type="text"
        :placeholder="t('search.placeholder')"
        class="jv-search"
      />
      <div class="jv-flat">
        <div
          v-for="(entry, i) in filteredEntries"
          :key="i"
          class="jv-flat-row"
        >
          <span class="jv-flat-key">{{ entry.key }}</span>
          <span class="jv-flat-sep"> : </span>
          <span :class="['jv-flat-val', valClass(entry.type)]">{{ entry.value }}</span>
        </div>
        <div v-if="filteredEntries.length === 0" class="jv-empty">{{ t('search.noResults') }}</div>
      </div>
    </div>

    <!-- JSON mode -->
    <pre v-else class="jv-raw" v-html="jsonHtml" />
  </div>
</template>

<style>
.jv-container {
  font-family: ui-monospace, 'SF Mono', 'Cascadia Code', Menlo, monospace;
  font-size: 12px;
  line-height: 1.5;
}

/* Tabs */
.jv-tabs {
  display: flex;
  gap: 0;
  border-bottom: 1px solid var(--g-6, #2a2b36);
  margin-bottom: 8px;
}
.jv-tab {
  padding: 4px 12px;
  font-size: 12px;
  color: var(--g-9, #8c8ea0);
  border: none;
  background: none;
  cursor: pointer;
  border-bottom: 2px solid transparent;
  transition: all 0.15s;
}
.jv-tab:hover {
  color: var(--g-13, #d4d5e2);
}
.jv-tab-active {
  color: var(--g-14, #e4e5ed);
  border-bottom-color: var(--g-14, #e4e5ed);
}

/* Search */
.jv-search {
  width: 100%;
  padding: 5px 10px;
  font-size: 12px;
  font-family: inherit;
  border: 1px solid var(--g-6, #2a2b36);
  border-radius: 6px;
  background: var(--g-3, #16171f);
  color: var(--g-12, #b0b2c3);
  margin-bottom: 6px;
  outline: none;
}
.jv-search::placeholder {
  color: var(--g-7, #4a4b5c);
}
.jv-search:focus {
  border-color: var(--g-8, #5c5e70);
}

/* Flat list */
.jv-flat {
  max-height: 400px;
  overflow-y: auto;
}
.jv-flat-row {
  padding: 2px 0;
  display: flex;
  align-items: baseline;
  gap: 0;
}
.jv-flat-row:hover {
  background: var(--g-4, #1e1f2a);
  border-radius: 3px;
}
.jv-flat-key {
  color: #7b8af7;
  flex-shrink: 0;
}
.jv-flat-sep {
  color: var(--g-7, #4a4b5c);
  flex-shrink: 0;
}
.jv-flat-val {
  word-break: break-all;
}
.jv-flat-val.jv-str { color: #e8a057; }
.jv-flat-val.jv-num { color: #6ee7b7; }
.jv-flat-val.jv-bool { color: #c4b5fd; }
.jv-flat-val.jv-null { color: #6b7280; font-style: italic; }

.jv-empty {
  color: var(--g-7, #4a4b5c);
  padding: 8px 0;
  font-style: italic;
}

/* JSON raw */
.jv-raw {
  white-space: pre-wrap;
  word-break: break-all;
  margin: 0;
  max-height: 400px;
  overflow-y: auto;
}
.jv-raw .jv-key { color: #7b8af7; }
.jv-raw .jv-str { color: #e8a057; }
.jv-raw .jv-num { color: #6ee7b7; }
.jv-raw .jv-bool { color: #c4b5fd; }
.jv-raw .jv-null { color: #6b7280; font-style: italic; }
.jv-raw .jv-brace { color: var(--g-9, #8c8ea0); }
.jv-raw .jv-colon { color: var(--g-7, #4a4b5c); }
</style>
