<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { RouterView, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useTheme } from '@/composables/useTheme'
// theme reactive ref is initialized via useTheme()
import { useRunTrigger } from '@/composables/useRunTrigger'
import { useWorkflowApi } from '@/composables/useWorkflowApi'
import { useShortcuts } from '@/composables/useShortcuts'
import AppSidebar from '@/components/AppSidebar.vue'
import AppTopBar from '@/components/AppTopBar.vue'
import CommandPalette from '@/components/CommandPalette.vue'
import ShortcutsHint from '@/components/ShortcutsHint.vue'
import StepInspector from '@/components/StepInspector.vue'
import SettingsPanel from '@/components/SettingsPanel.vue'
import ParamForm from '@/components/ParamForm.vue'
import { useStepInspector } from '@/composables/useStepInspector'

const router = useRouter()
const { t, locale } = useI18n()
const { toggle: toggleTheme } = useTheme()
const { showRunDialog, workflowReady, workflowName, workflowParams } = useRunTrigger()
const api = useWorkflowApi()

const paletteOpen = ref(false)
const inspector = useStepInspector()
const running = ref(false)

useShortcuts({ paletteOpen, inspectorOpen: inspector.open })

function toggleLocale() {
  const next = locale.value === 'en' ? 'fr' : 'en'
  locale.value = next
  localStorage.setItem('locale', next)
}

async function run(params: Record<string, unknown>) {
  running.value = true
  try {
    const result = await api.runWorkflow(params)
    showRunDialog.value = false
    if (result.execution_id) {
      router.push({ name: 'execution', params: { id: result.execution_id } })
    }
  } finally {
    running.value = false
  }
}

function handleKeydown(e: KeyboardEvent) {
  if ((e.metaKey || e.ctrlKey) && e.key === 'e') {
    e.preventDefault()
    if (workflowReady.value) showRunDialog.value = true
  }
}

function handleToggleTheme() { toggleTheme() }

function dispatchShortcuts() {
  window.dispatchEvent(new CustomEvent('toggle-shortcuts'))
}

onMounted(() => {
  window.addEventListener('keydown', handleKeydown)
  window.addEventListener('toggle-theme', handleToggleTheme)
})
onUnmounted(() => {
  window.removeEventListener('keydown', handleKeydown)
  window.removeEventListener('toggle-theme', handleToggleTheme)
})
</script>

<template>
  <div class="h-screen w-screen flex bg-g-1 text-g-13 overflow-hidden" :style="{ '--inspector-w': inspector.open.value ? '520px' : '0px' }">
    <AppSidebar />
    <div class="flex-1 flex flex-col min-w-0">
      <AppTopBar
        :workflow-name="workflowName"
        @open-palette="paletteOpen = true"
        @open-shortcuts="dispatchShortcuts"
        @toggle-theme="toggleTheme"
        @toggle-locale="toggleLocale"
      />
      <main class="flex-1 min-h-0 overflow-y-auto bg-g-1" :style="{ paddingRight: inspector.open.value ? '520px' : '0', transition: 'padding-right 180ms ease' }">
        <RouterView v-slot="{ Component, route }">
          <component :is="Component" :key="route.fullPath" />
        </RouterView>
      </main>
    </div>

    <CommandPalette :open="paletteOpen" @close="paletteOpen = false" />
    <ShortcutsHint />
    <SettingsPanel />
    <StepInspector :open="inspector.open.value" :step-id="inspector.stepId.value" @close="inspector.close()" />

    <!-- Run Dialog -->
    <Teleport to="body">
      <div
        v-if="showRunDialog"
        class="fixed inset-0 bg-black/70 flex items-center justify-center z-50"
        @click.self="showRunDialog = false"
      >
        <div class="bg-g-2 rounded-lg border border-g-6 p-6 w-full max-w-md anim-enter lm-card">
          <h2 class="text-[16px] font-semibold text-g-14 mb-5">{{ t('run.title', { name: workflowName }) }}</h2>
          <ParamForm
            :params="workflowParams"
            :loading="running"
            @submit="run"
            @cancel="showRunDialog = false"
          />
        </div>
      </div>
    </Teleport>
  </div>
</template>
