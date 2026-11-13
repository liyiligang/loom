<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { Folder, ArrowUp } from '@lucide/vue'
import { request } from '../api'
import { useI18n } from '../i18n'
import type { DirectoryListing } from '../types'
const props = defineProps<{ open: boolean; path: string }>()
const emit = defineEmits<{ choose: [path: string]; close: [] }>()
const { t, errorMessage } = useI18n()
const listing = ref<DirectoryListing>()
const loading = ref(false), errorCause = ref<unknown>()
const error = computed(() => errorCause.value ? errorMessage(errorCause.value, 'local.directoryError') : '')
let revision = 0
async function load(path: string) {
  const current = ++revision
  loading.value = true; errorCause.value = undefined
  try { const result = await request<DirectoryListing>(`/local/directories?path=${encodeURIComponent(path)}`); if (current === revision) listing.value = result }
  catch (cause) { if (current === revision) errorCause.value = cause }
  finally { if (current === revision) loading.value = false }
}
watch(() => props.open, value => { if (value) { listing.value = undefined; void load(props.path) } else revision++ }, { immediate: true })
onBeforeUnmount(() => { ++revision })
</script>
<template>
  <section v-if="open" class="folder-browser" :aria-label="t('local.chooseFolder')"><div class="form-heading"><strong>{{ t('local.chooseFolder') }}</strong><button type="button" class="text-button" @click="emit('close')">{{ t('common.cancel') }}</button></div>
    <p v-if="error" role="alert">{{ error }}</p>
    <p v-if="loading" role="status">{{ t('local.loadingFolders') }}</p>
    <template v-if="listing"><code class="folder-path">{{ listing.path }}</code><button type="button" class="button" :disabled="loading || listing.parent === listing.path" @click="load(listing.parent)"><ArrowUp :size="15" />{{ t('local.parentFolder') }}</button><div class="folder-list"><button type="button" v-for="directory in listing.directories" :key="directory.path" :disabled="loading" @click="load(directory.path)"><Folder :size="17" />{{ directory.name }}</button><p v-if="!listing.directories.length" class="subtle">{{ t('local.noFolders') }}</p></div><p v-if="listing.limited" class="subtle">{{ t('local.folderLimit') }}</p><button type="button" class="button primary" :disabled="loading" @click="emit('choose', listing.path)">{{ t('local.useFolder') }}</button></template>
  </section>
</template>
