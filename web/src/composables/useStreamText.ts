import { onBeforeUnmount, ref, watch, type Ref } from 'vue'
import { StreamText } from './streamText'
import { useReducedMotion } from './useReducedMotion'

export function useStreamText(source: () => string, status: () => string, selected: Ref<boolean>) {
  const reducedMotion = useReducedMotion()
  const buffer = new StreamText(source(), status())
  const text = ref(buffer.text)
  let timer: ReturnType<typeof setTimeout> | undefined

  function cancel() {
    clearTimeout(timer)
    timer = undefined
  }
  function schedule() {
    if (timer || !buffer.pending || selected.value) return
    timer = setTimeout(() => {
      timer = undefined
      text.value = buffer.advance(performance.now())
      schedule()
    }, 32)
  }
  function sync() {
    cancel()
    if (selected.value) return
    text.value = buffer.update(source(), status(), performance.now(), !reducedMotion.value && !document.hidden)
    schedule()
  }
  watch([source, status, reducedMotion], () => {
    if (selected.value) { cancel(); return }
    sync()
  })
  watch(selected, value => { if (value) cancel(); else sync() })
  document.addEventListener('visibilitychange', sync)
  onBeforeUnmount(() => {
    cancel()
    document.removeEventListener('visibilitychange', sync)
  })
  return text
}
