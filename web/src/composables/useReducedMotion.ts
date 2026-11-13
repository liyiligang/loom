import { onBeforeUnmount, ref } from 'vue'

export function useReducedMotion() {
  const preference = window.matchMedia('(prefers-reduced-motion: reduce)')
  const reduced = ref(preference.matches)
  const change = () => { reduced.value = preference.matches }
  preference.addEventListener('change', change)
  onBeforeUnmount(() => preference.removeEventListener('change', change))
  return reduced
}
