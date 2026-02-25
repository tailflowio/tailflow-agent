<script setup lang="ts">
import { ref, computed } from 'vue'
import { RouterView, RouterLink, useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useTheme } from '@/composables/useTheme'
import { useRunTrigger } from '@/composables/useRunTrigger'
import { useWorkflowApi } from '@/composables/useWorkflowApi'
import ParamForm from '@/components/ParamForm.vue'

const route = useRoute()
const router = useRouter()
const { t, locale } = useI18n()
const { theme, toggle } = useTheme()
const { showRunDialog, workflowReady, workflowName, workflowParams } = useRunTrigger()

const currentLocale = computed(() => locale.value)

function toggleLocale() {
  const next = locale.value === 'en' ? 'fr' : 'en'
  locale.value = next
  localStorage.setItem('locale', next)
}
const api = useWorkflowApi()
const running = ref(false)

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

function isActive(path: string) {
  if (path === '/') return route.path === '/'
  return route.path.startsWith(path)
}

const navItems = [
  { path: '/', key: 'nav.overview', icon: 'overview' },
  { path: '/steps', key: 'nav.steps', icon: 'steps' },
  { path: '/executions', key: 'nav.executions', icon: 'executions' },
]
</script>

<template>
  <div class="flex h-screen overflow-hidden">
    <!-- Sidebar -->
    <aside class="w-56 flex-shrink-0 bg-g-2 border-r border-g-5 flex flex-col">
      <!-- Logo -->
      <div class="p-5 border-b border-g-5">
        <div class="flex items-center gap-2">
          <RouterLink to="/" class="text-base font-semibold text-g-14 tracking-tight">.TailFlow</RouterLink>
          <span class="text-[10px] font-mono px-1.5 py-0.5 bg-g-4 text-g-9 rounded">v2.0</span>
        </div>
        <p v-if="workflowName" class="text-xs text-g-9 mt-0.5 truncate">{{ workflowName }}</p>
      </div>

      <!-- Nav items -->
      <nav class="flex-1 py-3 px-3 space-y-0.5">
        <RouterLink
          v-for="item in navItems"
          :key="item.path"
          :to="item.path"
          :class="[
            'flex items-center gap-2.5 px-3 py-2 rounded-md text-sm transition-colors',
            isActive(item.path)
              ? 'bg-g-4 text-g-14'
              : 'text-g-11 hover:text-g-14 hover:bg-g-3'
          ]"
        >
          <!-- Overview icon -->
          <svg v-if="item.icon === 'overview'" class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M4 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2V6zm10 0a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2V6zM4 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2v-2zm10 0a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2v-2z" />
          </svg>
          <!-- Steps icon -->
          <svg v-else-if="item.icon === 'steps'" class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M3.75 6.75h16.5M3.75 12h16.5m-16.5 5.25h16.5" />
          </svg>
          <!-- Executions icon -->
          <svg v-else-if="item.icon === 'executions'" class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M13 2L3 14h9l-1 8 10-12h-9l1-8z" />
          </svg>
          {{ t(item.key) }}
        </RouterLink>
      </nav>

      <!-- Bottom section -->
      <div class="p-3 border-t border-g-5 space-y-2">
        <!-- Theme toggle -->
        <div class="flex items-center justify-between px-2 py-1.5">
          <span class="text-xs text-g-9">{{ theme === 'dark' ? t('nav.darkMode') : t('nav.lightMode') }}</span>
          <button
            @click="toggle"
            class="relative w-9 h-5 rounded-full transition-colors duration-300 focus:outline-none"
            :class="theme === 'light' ? 'bg-g-6' : 'bg-g-9'"
            :title="theme === 'dark' ? t('nav.lightMode') : t('nav.darkMode')"
          >
            <span
              class="absolute top-0.5 left-0.5 w-4 h-4 rounded-full transition-all duration-300 flex items-center justify-center"
              :class="theme === 'light' ? 'translate-x-4 bg-white' : 'translate-x-0 bg-g-15'"
            >
              <svg v-if="theme === 'light'" class="w-2.5 h-2.5 text-g-10" fill="currentColor" viewBox="0 0 20 20">
                <path fill-rule="evenodd" d="M10 2a1 1 0 011 1v1a1 1 0 11-2 0V3a1 1 0 011-1zm4 8a4 4 0 11-8 0 4 4 0 018 0zm-.464 4.95l.707.707a1 1 0 001.414-1.414l-.707-.707a1 1 0 00-1.414 1.414zm2.12-10.607a1 1 0 010 1.414l-.706.707a1 1 0 11-1.414-1.414l.707-.707a1 1 0 011.414 0zM17 11a1 1 0 100-2h-1a1 1 0 100 2h1zm-7 4a1 1 0 011 1v1a1 1 0 11-2 0v-1a1 1 0 011-1zM5.05 6.464A1 1 0 106.465 5.05l-.708-.707a1 1 0 00-1.414 1.414l.707.707zm1.414 8.486l-.707.707a1 1 0 01-1.414-1.414l.707-.707a1 1 0 011.414 1.414zM4 11a1 1 0 100-2H3a1 1 0 000 2h1z" clip-rule="evenodd" />
              </svg>
              <svg v-else class="w-2.5 h-2.5 text-g-6" fill="currentColor" viewBox="0 0 20 20">
                <path d="M17.293 13.293A8 8 0 016.707 2.707a8.001 8.001 0 1010.586 10.586z" />
              </svg>
            </span>
          </button>
        </div>

        <!-- Locale toggle -->
        <div class="flex items-center justify-between px-2 py-1.5">
          <span class="text-xs text-g-9">{{ t('nav.language') }}</span>
          <button
            @click="toggleLocale"
            class="text-[12px] font-mono font-medium text-g-11 hover:text-g-14 px-2 py-0.5 rounded-md bg-g-4 hover:bg-g-5 transition-colors"
          >
            {{ currentLocale === 'en' ? 'FR' : 'EN' }}
          </button>
        </div>

        <!-- Run button -->
        <button
          v-if="workflowReady"
          @click="showRunDialog = true"
          class="w-full flex items-center justify-center gap-2 px-4 py-2 rounded-md text-sm font-medium bg-g-14 text-g-1 hover:bg-g-12 transition-colors"
        >
          <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M3.75 13.5l10.5-11.25L12 10.5h8.25L9.75 21.75 12 13.5H3.75z" />
          </svg>
          {{ t('nav.run') }}
        </button>
      </div>
    </aside>

    <!-- Main content -->
    <main class="flex-1 overflow-y-auto bg-g-1">
      <div class="max-w-7xl mx-auto p-6">
        <RouterView />
      </div>
    </main>

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
