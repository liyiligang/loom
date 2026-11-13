<script setup lang="ts">
import { ref, watch } from 'vue'
import { RefreshCw } from '@lucide/vue'
import { useI18n } from '../i18n'

const props = defineProps<{ src: string; title: string }>()
const { t } = useI18n()
const revision = ref(0)
const loading = ref(true)
watch(() => props.src, () => { loading.value = true })
function reload() { loading.value = true; revision.value++ }
</script>

<template>
  <section class="prototype-preview" :aria-label="t('requirement.prototypes')">
    <div class="prototype-preview-toolbar"><span>{{ t('prototype.interactive') }}</span><button type="button" class="icon-button" :aria-label="t('prototype.reload')" @click="reload"><RefreshCw :size="15" /></button></div>
    <p class="subtle prototype-preview-hint">{{ t('prototype.demoHint') }}</p>
    <p v-if="loading" class="subtle" role="status">{{ t('prototype.loading') }}</p>
    <iframe :key="`${src}:${revision}`" :src="src" :title="t('prototype.frameTitle', { name: title })" sandbox="allow-scripts allow-forms" referrerpolicy="no-referrer" @load="loading = false"></iframe>
  </section>
</template>

<style scoped>
.prototype-preview { margin-top: 14px; }
.prototype-preview-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 12px; font-size: 12px; color: var(--muted); }
.prototype-preview-hint { margin: 4px 0 12px; font-size: 11px; }
.prototype-preview iframe { display: block; width: 100%; height: 65vh; min-height: 420px; border: 1px solid var(--border); border-radius: 12px; background: #fff; }
@media (max-width: 700px) { .prototype-preview iframe { min-height: 360px; height: 60vh; } }
</style>
