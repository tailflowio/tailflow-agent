<script setup lang="ts">
import { useSettings, type AnimStyle, type Density } from '@/composables/useSettings'
import Icon from './primitives/Icon.vue'

const { settings, set, open, close } = useSettings()

const animOptions: { value: AnimStyle; label: string; desc: string }[] = [
  { value: 'token', label: 'Token', desc: 'single dot rides each active edge' },
  { value: 'particle', label: 'Particle', desc: '3 staggered dots, suggests throughput' },
  { value: 'pulse', label: 'Pulse', desc: 'edges pulse with halos, calmer' },
]

const densityOptions: { value: Density; label: string }[] = [
  { value: 'comfortable', label: 'Comfortable' },
  { value: 'compact', label: 'Compact' },
]
</script>

<template>
  <Teleport to="body">
    <template v-if="open">
      <div class="fixed inset-0 z-[60]" style="background: rgba(0, 0, 0, 0.4)" @click="close" />
      <aside class="fixed top-0 right-0 h-screen w-[360px] bg-g-2 border-l border-g-5 z-[61] flex flex-col anim-enter shadow-2xl">
        <div class="flex items-center justify-between px-4 py-3 border-b border-g-5 shrink-0">
          <div class="flex items-center gap-2">
            <Icon name="settings" class-name="w-3.5 h-3.5 text-g-9" />
            <span class="text-[13px] font-semibold text-g-14">Settings</span>
          </div>
          <button @click="close" class="text-g-9 hover:text-g-12">
            <Icon name="x" class-name="w-4 h-4" />
          </button>
        </div>

        <div class="flex-1 overflow-y-auto p-4 space-y-5">
          <!-- Density -->
          <section>
            <div class="text-[10px] font-mono uppercase tracking-wider text-g-8 mb-2">Density</div>
            <div class="flex gap-1 bg-g-1 border border-g-5 rounded p-0.5">
              <button
                v-for="opt in densityOptions"
                :key="opt.value"
                @click="set('density', opt.value)"
                :class="['flex-1 px-2.5 py-1 text-[11px] font-mono rounded transition', settings.density === opt.value ? 'bg-g-3 text-g-14' : 'text-g-9 hover:text-g-12']"
              >{{ opt.label }}</button>
            </div>
          </section>

          <!-- DAG animation -->
          <section>
            <div class="text-[10px] font-mono uppercase tracking-wider text-g-8 mb-2">Live DAG animation</div>
            <div class="space-y-1.5">
              <button
                v-for="opt in animOptions"
                :key="opt.value"
                @click="set('animStyle', opt.value)"
                :class="['w-full text-left px-3 py-2 rounded border transition', settings.animStyle === opt.value ? 'bg-g-3 border-g-7' : 'bg-g-1 border-g-5 hover:border-g-6']"
              >
                <div class="flex items-center gap-2">
                  <span :class="['w-1.5 h-1.5 rounded-full', settings.animStyle === opt.value ? 'bg-amber-400' : 'bg-g-6']" />
                  <span :class="['text-[12px] font-mono', settings.animStyle === opt.value ? 'text-g-14' : 'text-g-11']">{{ opt.label }}</span>
                </div>
                <div class="text-[10px] font-mono text-g-8 mt-1 ml-3.5">{{ opt.desc }}</div>
              </button>
            </div>
          </section>

          <!-- Inspector -->
          <section>
            <div class="text-[10px] font-mono uppercase tracking-wider text-g-8 mb-2">Inspector</div>
            <button
              @click="set('autoOpenInspector', !settings.autoOpenInspector)"
              class="w-full flex items-center justify-between px-3 py-2 bg-g-1 border border-g-5 rounded hover:border-g-6"
            >
              <div class="text-left">
                <div class="text-[12px] text-g-12">Auto-open on running step</div>
                <div class="text-[10px] font-mono text-g-8 mt-0.5">expand the side panel automatically</div>
              </div>
              <span
                :class="[
                  'relative w-9 h-5 rounded-full transition',
                  settings.autoOpenInspector ? 'bg-emerald-400/30' : 'bg-g-5',
                ]"
              >
                <span
                  :class="[
                    'absolute top-0.5 left-0.5 w-4 h-4 rounded-full transition-all duration-200',
                    settings.autoOpenInspector ? 'translate-x-4 bg-emerald-400' : 'translate-x-0 bg-g-9',
                  ]"
                />
              </span>
            </button>
          </section>
        </div>

        <div class="border-t border-g-5 px-4 py-2 text-[10px] font-mono text-g-8">
          settings persist locally · per browser
        </div>
      </aside>
    </template>
  </Teleport>
</template>
