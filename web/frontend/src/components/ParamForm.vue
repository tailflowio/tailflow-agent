<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

interface Param {
  name: string
  type: string
  required?: boolean
  default?: unknown
}

const props = defineProps<{
  params: Param[]
  loading?: boolean
}>()

const emit = defineEmits<{
  submit: [params: Record<string, unknown>]
  cancel: []
}>()

const values = ref<Record<string, string>>({})

props.params.forEach((p) => {
  if (p.default !== undefined) {
    values.value[p.name] = String(p.default)
  } else {
    values.value[p.name] = ''
  }
})

function submit() {
  const params: Record<string, unknown> = {}
  props.params.forEach((p) => {
    const val = values.value[p.name]
    if (val !== '' && val !== undefined) {
      switch (p.type) {
        case 'bool': params[p.name] = val === 'true'; break
        case 'int': params[p.name] = parseInt(val, 10); break
        case 'float': params[p.name] = parseFloat(val); break
        default: params[p.name] = val
      }
    }
  })
  emit('submit', params)
}
</script>

<template>
  <form @submit.prevent="submit">
    <div v-if="params.length === 0" class="text-g-9 text-[13px] mb-5">
      {{ t('run.noParams') }}
    </div>
    <div v-for="param in params" :key="param.name" class="mb-4">
      <label class="block text-[13px] font-medium text-g-11 mb-1.5">
        {{ param.name }}
        <span v-if="param.required" class="text-red-400">*</span>
        <span class="text-g-8 font-normal ml-1 text-[11px] font-mono">({{ param.type }})</span>
      </label>
      <input
        v-if="param.type !== 'bool'"
        v-model="values[param.name]"
        :type="param.type === 'int' || param.type === 'float' ? 'number' : 'text'"
        :required="param.required"
        class="w-full px-3 py-2 bg-g-3 border border-g-6 rounded-md text-[13px] text-g-13 placeholder:text-g-7 focus:border-g-8 focus:ring-1 focus:ring-g-8 outline-none transition-colors"
        :placeholder="param.default !== undefined ? t('run.default', { value: param.default }) : ''"
      />
      <select
        v-else
        v-model="values[param.name]"
        class="w-full px-3 py-2 bg-g-3 border border-g-6 rounded-md text-[13px] text-g-13 focus:border-g-8 focus:ring-1 focus:ring-g-8 outline-none transition-colors"
      >
        <option value="true">true</option>
        <option value="false">false</option>
      </select>
    </div>
    <div class="flex gap-2 justify-end pt-2">
      <button
        type="button"
        @click="emit('cancel')"
        class="px-3.5 py-2 text-[13px] font-medium rounded-md border border-g-6 text-g-10 hover:text-g-13 hover:bg-g-3 transition-colors"
      >
        {{ t('run.cancel') }}
      </button>
      <button
        type="submit"
        :disabled="loading"
        class="px-3.5 py-2 text-[13px] font-medium rounded-md bg-g-15 text-g-1 hover:bg-g-13 transition-colors disabled:opacity-40 disabled:cursor-not-allowed flex items-center gap-2"
      >
        <div v-if="loading" class="w-3 h-3 border-2 border-g-5 border-t-g-1 rounded-full animate-spin" />
        {{ t('run.submit') }}
      </button>
    </div>
  </form>
</template>
