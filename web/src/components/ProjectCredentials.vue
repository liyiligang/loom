<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { KeyRound, RefreshCw } from '@lucide/vue'
import { request } from '../api'
import { useI18n } from '../i18n'
import type { StoredSecret } from '../types'

const props = defineProps<{ modelValue: string[]; projectId: string; local: boolean; refreshKey: number; disabled: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [ids: string[]]; loading: [value: boolean] }>()
const { t, errorMessage } = useI18n()
const records = ref<StoredSecret[]>([])
const loading = ref(false)
const error = ref<unknown>()
let revision = 0
async function load() {
  const version = ++revision
  loading.value = true
  emit('loading', true)
  error.value = undefined
  try {
    const result = await request<StoredSecret[]>('/secrets')
    if (version !== revision) return
    records.value = result
    emit('update:modelValue', props.modelValue.filter(id => result.some(value => value.id === id)))
  } catch (cause) { if (version === revision) error.value = cause }
  finally { if (version === revision) { loading.value = false; emit('loading', false) } }
}
function toggle(id: string) {
  emit('update:modelValue', props.modelValue.includes(id) ? props.modelValue.filter(value => value !== id) : [...props.modelValue, id])
}
watch(() => [props.projectId, props.refreshKey], () => { void load() }, { immediate: true, flush: 'sync' })
onBeforeUnmount(() => { ++revision; emit('loading', false) })
</script>

<template>
  <div class="project-credentials">
    <div class="project-credentials-heading"><span>{{ t('secrets.projectCredentials') }}</span><button type="button" class="icon-button" :disabled="disabled || loading" :aria-label="t('secrets.refresh')" @click="load"><RefreshCw :size="14" /></button></div>
    <p v-if="!local" class="subtle">{{ t('secrets.localOnly') }}</p>
    <p v-if="error" class="form-error" role="alert">{{ errorMessage(error, 'secrets.loadError') }}</p>
    <p v-else-if="loading" class="subtle" role="status">{{ t('secrets.loading') }}</p>
    <div v-else-if="records.length" class="project-credentials-list">
      <div v-for="record in records" :key="record.id" class="project-credential-row"><KeyRound :size="15" aria-hidden="true" /><span><strong>{{ record.name }}</strong></span><button type="button" class="credential-switch" role="switch" :aria-checked="modelValue.includes(record.id)" :disabled="disabled || !local" :aria-label="t('secrets.activateNamed', { name: record.name })" @click="toggle(record.id)"><span aria-hidden="true"></span></button></div>
    </div>
    <p v-else class="subtle">{{ t('secrets.noCredentials') }}</p>
    <p v-if="local && records.length" class="subtle">{{ t('secrets.projectHint') }}</p>
  </div>
</template>
