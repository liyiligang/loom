<script setup lang="ts">
import { nextTick, ref, useId } from 'vue'
import { useI18n } from '../i18n'

const props = defineProps<{ name: string; label: string; disabled: boolean; save: (name: string) => Promise<void> }>()
const { t, errorMessage } = useI18n()
const editing = ref(false)
const saving = ref(false)
const draft = ref('')
const initialName = ref('')
const error = ref('')
const input = ref<HTMLInputElement>()
const root = ref<HTMLElement>()
const errorId = useId()

function rename() {
  if (props.disabled || saving.value) return
  draft.value = props.name
  initialName.value = props.name
  error.value = ''
  editing.value = true
  void nextTick(() => { input.value?.focus(); input.value?.select() })
}
function cancel(focus = false) {
  if (saving.value) return
  editing.value = false
  error.value = ''
  if (focus) void nextTick(() => root.value?.querySelector<HTMLElement>('button, a')?.focus({ preventScroll: true }))
}
async function commit(focus = false) {
  if (!editing.value || saving.value || props.disabled) return
  const name = draft.value.trim()
  if (name === initialName.value.trim()) { cancel(focus); return }
  if (!name || name.length > 120) { error.value = t('errors.invalidName'); return }
  saving.value = true
  error.value = ''
  try {
    await props.save(name)
    editing.value = false
    if (focus) void nextTick(() => root.value?.querySelector<HTMLElement>('button, a')?.focus({ preventScroll: true }))
  }
  catch (cause) { error.value = errorMessage(cause, 'errors.action') }
  finally { saving.value = false }
}
function keydown(event: KeyboardEvent) {
  if (event.isComposing) return
  if (event.key === 'Enter') { event.preventDefault(); event.stopPropagation(); void commit(true) }
  else if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); cancel(true) }
}
</script>

<template>
  <div ref="root" class="tree-name-editor" :class="{ 'is-editing': editing }">
    <template v-if="editing">
      <input ref="input" v-model="draft" class="tree-name-input" :aria-label="label" :aria-invalid="Boolean(error)" :aria-describedby="error ? errorId : undefined" :aria-busy="saving" :readonly="saving" :disabled="disabled" maxlength="120" @keydown="keydown" @blur="commit()" @click.stop @dblclick.stop />
      <span v-if="error" :id="errorId" class="tree-name-error" role="alert">{{ error }}</span>
    </template>
    <slot v-else :rename="rename" />
  </div>
</template>
