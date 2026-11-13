import { computed, ref, watchEffect } from 'vue'
import { isPreference, readPreference, resolveLocale, savePreference, translate, translateError, translateMessage, translateSessionTitle, translateState, languageStorageKey } from './locale.ts'
import type { Locale, Parameters } from './locale.ts'
import type { TranslationKey } from './catalog.ts'

function browserLanguages(): readonly string[] {
  return typeof navigator === 'undefined' ? [] : [...(navigator.languages ?? []), navigator.language]
}

function browserStorage(): Storage | undefined {
  try { return typeof window === 'undefined' ? undefined : window.localStorage } catch { return undefined }
}

const preference = ref(readPreference(browserStorage()))
const languages = ref(browserLanguages())
const locale = computed(() => preference.value ?? resolveLocale(languages.value))

function setLanguage(value: Locale) {
  if (!isPreference(value)) return
  preference.value = value
  savePreference(value, browserStorage())
}

function t(key: TranslationKey, parameters?: Parameters) { return translate(locale.value, key, parameters) }
function message(text: string | undefined, code?: string) { return translateMessage(locale.value, text ?? '', code) }
function errorMessage(cause: unknown, fallback: TranslationKey) { return translateError(locale.value, cause, fallback) }
function stateLabel(state: unknown) { return translateState(locale.value, state) }
function sessionTitle(session: { title: string; defaultTitle?: boolean }) { return translateSessionTitle(locale.value, session) }
function number(value: number) { return new Intl.NumberFormat(locale.value).format(value) }

export function useI18n() { return { locale, preference, setLanguage, t, message, errorMessage, stateLabel, sessionTitle, number } }

export function synchronizeDocumentLanguage() {
  const stop = watchEffect(() => {
    document.documentElement.lang = locale.value
    document.title = t('app.title')
    document.querySelector('meta[name="description"]')?.setAttribute('content', t('app.description'))
  })
  const onLanguageChange = () => { languages.value = browserLanguages() }
  const onStorage = (event: StorageEvent) => {
    if (event.key === languageStorageKey || event.key === null) preference.value = readPreference(browserStorage())
  }
  window.addEventListener('languagechange', onLanguageChange)
  window.addEventListener('storage', onStorage)
  return () => {
    stop()
    window.removeEventListener('languagechange', onLanguageChange)
    window.removeEventListener('storage', onStorage)
  }
}
