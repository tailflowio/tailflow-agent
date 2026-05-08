<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { EventRow } from '@tailflow/shared'
import EventTimeline from '@tailflow/shared/components/EventTimeline.vue'

type DockTab = 'events' | 'logs' | 'io'

const props = withDefaults(defineProps<{
  events: EventRow[]
  total?: number
  hasMore?: boolean
  loading?: boolean
  serverSide?: boolean
  connected?: boolean
  finished?: boolean
  fillHeight?: boolean
}>(), {
  loading: false,
  hasMore: false,
  serverSide: false,
  connected: false,
  finished: false,
  fillHeight: false,
})

const emit = defineEmits<{
  'load-more': []
}>()

const tab = ref<DockTab>('events')

const LOG_TYPES = new Set(['step.log'])
const IO_TYPES = new Set(['step.input', 'step.output'])

const counts = computed(() => {
  let logs = 0, io = 0
  for (const e of props.events) {
    if (LOG_TYPES.has(e.event_type)) logs++
    else if (IO_TYPES.has(e.event_type)) io++
  }
  return { events: props.events.length, logs, io }
})

const filteredEvents = computed<EventRow[]>(() => {
  if (tab.value === 'events') return props.events
  const allow = tab.value === 'logs' ? LOG_TYPES : IO_TYPES
  return props.events.filter(e => allow.has(e.event_type))
})

const filteredTotal = computed(() => {
  if (tab.value === 'events') return props.total ?? props.events.length
  return filteredEvents.value.length
})

// In server-side mode, only "Events" tab can paginate. Other tabs filter loaded events client-side.
const effectiveServerSide = computed(() => props.serverSide && tab.value === 'events')
const effectiveHasMore = computed(() => effectiveServerSide.value && props.hasMore)

const tabDefs = computed(() => [
  { id: 'events' as DockTab, label: 'Events', count: counts.value.events },
  { id: 'logs' as DockTab, label: 'Logs', count: counts.value.logs },
  { id: 'io' as DockTab, label: 'I/O', count: counts.value.io },
])

const timelineRef = ref<InstanceType<typeof EventTimeline>>()

// When user switches tabs, the timeline's height calc may need a refresh
watch(tab, () => {
  // Next tick recalc
  setTimeout(() => timelineRef.value?.recalcHeight?.(), 0)
})
</script>

<template>
  <div
    class="bg-g-2 border border-g-5 rounded-lg overflow-hidden"
    :class="fillHeight ? 'flex flex-col flex-1 min-h-0' : ''"
  >
    <!-- Tab strip -->
    <div class="flex items-center border-b border-g-5 px-2 shrink-0">
      <button
        v-for="t in tabDefs"
        :key="t.id"
        @click="tab = t.id"
        class="px-3 py-2 text-[12px] font-medium relative transition-colors"
        :class="tab === t.id ? 'text-g-14' : 'text-g-9 hover:text-g-12'"
      >
        {{ t.label }}
        <span class="font-mono text-[10px] text-g-8 ml-1 tabular-nums">{{ t.count }}</span>
        <span
          v-if="tab === t.id"
          class="absolute bottom-0 left-2 right-2 h-[2px] bg-g-13"
        />
      </button>
      <div class="flex-1" />
      <div class="flex items-center gap-2 pr-2">
        <span
          v-if="connected && !finished"
          class="text-[10px] font-mono text-emerald-400 flex items-center gap-1"
        >
          <span class="w-1.5 h-1.5 bg-emerald-400 rounded-full animate-pulse" />
          streaming
        </span>
        <span
          v-else-if="finished"
          class="text-[10px] font-mono text-g-8 flex items-center gap-1"
        >
          <span class="w-1.5 h-1.5 bg-g-7 rounded-full" />
          finished
        </span>
      </div>
    </div>

    <!-- Embedded EventTimeline (keeps search, filter chips, columns, slide-over) -->
    <EventTimeline
      ref="timelineRef"
      :events="filteredEvents"
      :total="filteredTotal"
      :has-more="effectiveHasMore"
      :loading="loading && effectiveServerSide"
      :server-side="effectiveServerSide"
      :initial-sort-asc="false"
      :fill-height="fillHeight"
      class="!border-0 !rounded-none"
      @load-more="emit('load-more')"
    />
  </div>
</template>
