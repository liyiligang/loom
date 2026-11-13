export type Theme = 'light' | 'dark'
export type ThemePreference = Theme | 'system'
export const themeStorageKey = 'loom.theme'

export function isThemePreference(value: unknown): value is ThemePreference {
  return value === 'light' || value === 'dark' || value === 'system'
}

export function readThemePreference(storage?: Pick<Storage, 'getItem'>): ThemePreference {
  try {
    const saved = storage?.getItem(themeStorageKey)
    return isThemePreference(saved) ? saved : 'system'
  } catch { return 'system' }
}

export function saveThemePreference(preference: ThemePreference, storage?: Pick<Storage, 'setItem'>) {
  try { storage?.setItem(themeStorageKey, preference) } catch { /* Theme switching remains available without browser storage. */ }
}

export function resolveTheme(preference: ThemePreference, systemDark: boolean): Theme {
  return preference === 'system' ? systemDark ? 'dark' : 'light' : preference
}
