export const navigationLayoutKey = 'loom.navigation.layout.v1'
export const navigationWidth = { default: 288, min: 240, max: 380 } as const
export interface NavigationLayout { width: number; collapsed: boolean }

export function clampNavigationWidth(width: number) {
  return Number.isFinite(width) ? Math.round(Math.max(navigationWidth.min, Math.min(navigationWidth.max, width))) : navigationWidth.default
}

export function readNavigationLayout(storage?: Pick<Storage, 'getItem'>): NavigationLayout {
  try {
    const value: unknown = JSON.parse(storage?.getItem(navigationLayoutKey) ?? 'null')
    if (value && typeof value === 'object' && !Array.isArray(value)) {
      const saved = value as Partial<NavigationLayout>
      return { width: typeof saved.width === 'number' ? clampNavigationWidth(saved.width) : navigationWidth.default, collapsed: saved.collapsed === true }
    }
  } catch { /* Layout preferences are optional. */ }
  return { width: navigationWidth.default, collapsed: false }
}
