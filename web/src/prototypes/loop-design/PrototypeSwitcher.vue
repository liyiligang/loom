<script setup lang="ts">
import { onBeforeUnmount, onMounted } from 'vue'
import { ChevronLeft, ChevronRight, Layers3 } from '@lucide/vue'
import { useI18n } from '../../i18n'
const props = defineProps<{ variant: string }>()
const emit = defineEmits<{ change: [value: string] }>()
const { t } = useI18n()
const development = import.meta.env.DEV
const variants = ['canvas', 'lanes', 'focus'] as const
function cycle(amount: number) { const index = variants.indexOf(props.variant as typeof variants[number]); emit('change', variants[(index + amount + variants.length) % variants.length]!) }
function keydown(event: KeyboardEvent) {
  if ((event.target as Element)?.closest('input, textarea, select, [contenteditable], [role="tab"]') || event.ctrlKey || event.metaKey || event.altKey) return
  if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') { event.preventDefault(); cycle(event.key === 'ArrowLeft' ? -1 : 1) }
}
onMounted(() => window.addEventListener('keydown', keydown))
onBeforeUnmount(() => window.removeEventListener('keydown', keydown))
</script>

<template>
  <div v-if="development" class="loop-prototype-switcher" :aria-label="t('loopPrototype.switcher')"><span><Layers3 :size="14" />{{ t('loopPrototype.prototype') }}</span><button :aria-label="t('loopPrototype.previousVariant')" @click="cycle(-1)"><ChevronLeft :size="17" /></button><strong>{{ t(`loopPrototype.variant.${variant as 'canvas' | 'lanes' | 'focus'}`) }}</strong><button :aria-label="t('loopPrototype.nextVariant')" @click="cycle(1)"><ChevronRight :size="17" /></button><small>{{ variants.indexOf(variant as typeof variants[number]) + 1 }} / 3</small></div>
</template>
