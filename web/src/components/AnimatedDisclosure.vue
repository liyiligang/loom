<script setup lang="ts">
import { ref, useId } from 'vue'
import { ChevronRight } from '@lucide/vue'

const open = ref(false)
const mounted = ref(false)
const panelId = useId()
function toggle() { mounted.value = true; open.value = !open.value }
</script>

<template>
  <section class="disclosure" :class="{ 'is-open': open }">
    <button type="button" class="disclosure-toggle" :aria-expanded="open" :aria-controls="panelId" @click="toggle">
      <slot name="summary" />
      <ChevronRight :size="15" class="disclosure-icon" aria-hidden="true" />
    </button>
    <div :id="panelId" class="disclosure-panel" :inert="!open" :aria-hidden="!open">
      <div class="disclosure-inner"><slot v-if="mounted" /></div>
    </div>
  </section>
</template>
