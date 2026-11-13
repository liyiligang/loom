import { computed, ref, watchEffect } from 'vue'
import { isThemePreference, readThemePreference, resolveTheme, saveThemePreference, themeStorageKey } from './themePreference.ts'
import type { ThemePreference } from './themePreference.ts'

function browserStorage(): Storage | undefined {
  try { return typeof window === 'undefined' ? undefined : window.localStorage } catch { return undefined }
}

const preference = ref<ThemePreference>(readThemePreference(browserStorage()))
const systemDark = ref(false)
const theme = computed(() => resolveTheme(preference.value, systemDark.value))

function setTheme(value: ThemePreference) {
  if (!isThemePreference(value)) return
  preference.value = value
  saveThemePreference(value, browserStorage())
}

export function useTheme() { return { theme, preference, setTheme } }

export function synchronizeDocumentTheme() {
  const media = window.matchMedia('(prefers-color-scheme: dark)')
  preference.value = readThemePreference(browserStorage())
  systemDark.value = media.matches
  const stop = watchEffect(() => {
    document.documentElement.dataset.theme = theme.value
    document.documentElement.classList.toggle('dark', theme.value === 'dark')
    document.querySelector('meta[name="theme-color"]')?.setAttribute('content', theme.value === 'dark' ? '#25282e' : '#ffffff')
  })
  const onSystemChange = () => { systemDark.value = media.matches }
  const onStorage = (event: StorageEvent) => {
    if (event.key === themeStorageKey || event.key === null) preference.value = readThemePreference(browserStorage())
  }
  media.addEventListener('change', onSystemChange)
  window.addEventListener('storage', onStorage)
  return () => {
    stop()
    media.removeEventListener('change', onSystemChange)
    window.removeEventListener('storage', onStorage)
  }
}
