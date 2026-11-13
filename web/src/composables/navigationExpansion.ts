export const navigationStorageKey = 'loom.navigation.expansion.v1'
export type Expansion = Record<string, boolean>

export function readExpansion(storage?: Pick<Storage, 'getItem'>): Expansion {
  try {
    const value: unknown = JSON.parse(storage?.getItem(navigationStorageKey) ?? '{}')
    if (!value || typeof value !== 'object' || Array.isArray(value)) return {}
    return Object.fromEntries(Object.entries(value).filter(([key, open]) => /^(project|requirement):[a-zA-Z0-9-]+$/.test(key) && typeof open === 'boolean'))
  } catch { return {} }
}

export function toggleExpansion(expansion: Expansion, key: string): Expansion {
  return { ...expansion, [key]: !expansion[key] }
}
