<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useGlobalEvents } from '@/composables/useGlobalEvents'
import { useSettings } from '@/composables/useSettings'
import { useWorkflowApi, type Execution, type WorkflowInfo } from '@/composables/useWorkflowApi'
import { fmtDur, statusDot } from '@/composables/useFormat'
import Icon from './primitives/Icon.vue'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const api = useWorkflowApi()

const workflow = ref<WorkflowInfo | null>(null)
const workflowFile = ref<string>('')
const appVersion = ref<string>('')
const recentRuns = ref<Execution[]>([])

const { stepCounts } = useGlobalEvents()
const settings = useSettings()

const liveCount = computed(() => {
  let c = 0
  for (const sc of Object.values(stepCounts.value)) c += sc.running.length + sc.waiting.length
  return c
})

onMounted(async () => {
  try {
    const wf = await api.getWorkflow() as unknown as WorkflowInfo
    workflow.value = wf
    workflowFile.value = (wf as any).file || (wf as any).path || (wf.name + '.yaml')
  } catch {}
  try {
    const v = await fetch('/api/version').then(r => r.json())
    appVersion.value = v.version || ''
  } catch {}
  try {
    const r = await api.listExecutions({ limit: 5 })
    recentRuns.value = r.items
  } catch {}
})

const items = computed(() => [
  { id: 'overview', l: t('nav.overview'), icon: 'home', kbd: 'g d', path: '/' },
  { id: 'execution', l: t('nav.liveRuns'), icon: 'play', kbd: 'g r', path: liveCount.value > 0 ? '/executions/latest' : '/executions', live: true, count: liveCount.value || null },
  { id: 'history', l: t('nav.executions'), icon: 'history', kbd: 'g h', path: '/executions' },
  { id: 'editor', l: 'Editor', icon: 'edit', kbd: 'g e', path: '/editor' },
])

function isActive(item: { id: string; path: string }): boolean {
  if (item.id === 'overview') return route.path === '/'
  if (item.id === 'execution') return route.path.startsWith('/executions/') && route.params.id !== undefined
  if (item.id === 'history') return route.path === '/executions'
  if (item.id === 'editor') return route.path === '/editor'
  return false
}

function go(path: string) {
  router.push(path)
}
</script>

<template>
  <aside class="w-[220px] shrink-0 bg-g-1 border-r border-g-5 flex flex-col">
    <!-- Logo -->
    <div class="px-4 pt-4 pb-3 flex items-center gap-2">
      <div class="w-7 h-7 rounded bg-g-13 flex items-center justify-center">
        <svg viewBox="0 0 16 16" class="w-4 h-4">
          <path d="M3 4 L8 4 L8 8 L13 8 L13 12 L8 12" stroke="#000" stroke-width="1.7" fill="none" stroke-linecap="square" />
        </svg>
      </div>
      <div class="flex flex-col min-w-0">
        <span class="text-[12px] font-semibold text-g-14 leading-tight">tailflow</span>
        <span v-if="appVersion" class="text-[9px] font-mono text-g-8 leading-tight">agent · v{{ appVersion }}</span>
      </div>
    </div>

    <!-- Workflow file selector -->
    <div v-if="workflow" class="px-3 mt-2">
      <div class="bg-g-2 border border-g-5 rounded px-2 py-1.5 flex items-center gap-2 cursor-pointer hover:bg-g-3 group">
        <Icon name="folder" class-name="w-3 h-3 text-g-8" />
        <span class="font-mono text-[11px] text-g-12 truncate flex-1">{{ workflowFile || (workflow.name + '.yaml') }}</span>
        <Icon name="chevronD" class-name="w-3 h-3 text-g-8" />
      </div>
      <div v-if="workflow.description" class="text-[9px] font-mono text-g-8 mt-1 px-1 truncate" :title="workflow.description">{{ workflow.description }}</div>
    </div>

    <!-- Run nav -->
    <nav class="px-2 mt-4 flex flex-col gap-0.5">
      <div class="text-[9px] font-mono uppercase tracking-wider text-g-8 px-2 mb-1">Run</div>
      <button
        v-for="it in items"
        :key="it.id"
        @click="go(it.path)"
        :class="[
          'flex items-center gap-2 px-2 py-1.5 rounded text-[12px]',
          isActive(it) ? 'bg-g-3 text-g-14' : 'text-g-10 hover:bg-g-2 hover:text-g-13',
        ]"
      >
        <Icon :name="it.icon" class-name="w-3.5 h-3.5" />
        <span class="font-medium">{{ it.l }}</span>
        <span v-if="it.live && liveCount > 0" class="w-1.5 h-1.5 bg-amber-400 rounded-full animate-pulse" />
        <span v-if="it.count" class="font-mono text-[10px] text-emerald-400 tabular-nums">{{ it.count }}</span>
        <span class="ml-auto font-mono text-[9px] text-g-7">{{ it.kbd }}</span>
      </button>
    </nav>

    <!-- Recent runs -->
    <div v-if="recentRuns.length > 0" class="px-2 mt-4 flex flex-col gap-0.5">
      <div class="text-[9px] font-mono uppercase tracking-wider text-g-8 px-2 mb-1">Recent runs</div>
      <RouterLink
        v-for="r in recentRuns"
        :key="r.id"
        :to="`/executions/${r.id}`"
        class="flex items-center gap-2 px-2 py-1 rounded hover:bg-g-2 cursor-pointer"
      >
        <span :class="['w-1.5 h-1.5 rounded-full', statusDot(r.status)]" />
        <span class="font-mono text-[10.5px] text-g-10 flex-1 truncate">{{ r.id.slice(0, 12) }}</span>
        <span v-if="r.finished_at && r.started_at" class="font-mono text-[9px] text-g-7 tabular-nums">{{ fmtDur(new Date(r.finished_at).getTime() - new Date(r.started_at).getTime()) }}</span>
        <span v-else class="font-mono text-[9px] text-g-7 tabular-nums">—</span>
      </RouterLink>
    </div>

    <div class="flex-1" />

    <!-- Agent footer -->
    <div class="border-t border-g-5 px-3 py-3">
      <div class="bg-g-2 border border-g-5 rounded p-2 mb-2">
        <div class="text-[9px] font-mono uppercase tracking-wider text-g-8 mb-1">Agent</div>
        <div class="flex items-center gap-1.5 mb-1">
          <span class="w-1.5 h-1.5 bg-emerald-400 rounded-full pulse-dot" />
          <span class="font-mono text-[10.5px] text-emerald-400">healthy</span>
          <span class="font-mono text-[9px] text-g-8 ml-auto">:8080</span>
        </div>
        <div class="font-mono text-[9px] text-g-8">{{ appVersion ? 'v' + appVersion : '' }}</div>
      </div>
      <div class="flex items-center justify-between">
        <span class="font-mono text-[10px] text-g-9">local</span>
        <button @click="settings.toggleOpen()" class="text-g-9 hover:text-g-13 cursor-pointer p-1 -m-1" title="Settings">
          <Icon name="settings" class-name="w-3.5 h-3.5" />
        </button>
      </div>
    </div>
  </aside>
</template>
