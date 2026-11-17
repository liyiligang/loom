import type { ConfiguredModel } from '../types'

// Match Go's strings.TrimSpace, including NEL and preserving BOM.
function trimModelValue(value: string) { return value.replace(/^\p{White_Space}+|\p{White_Space}+$/gu, '') }

export function modelCatalogErrors(rows: ConfiguredModel[]) {
  const counts = new Map<string, number>()
  for (const row of rows) {
    const key = `${row.provider}:${trimModelValue(row.id)}`
    counts.set(key, (counts.get(key) ?? 0) + 1)
  }
  return rows.map(row => {
    const name = trimModelValue(row.name)
    const id = trimModelValue(row.id)
    return {
      name: !name ? 'modelCatalog.nameRequired' : Array.from(name).length > 120 ? 'modelCatalog.nameTooLong' : name.includes('\0') ? 'modelCatalog.nameInvalid' : undefined,
      id: !id ? 'modelCatalog.idRequired' : new TextEncoder().encode(id).length > 120 ? 'modelCatalog.idTooLong' : /[\p{White_Space}\p{Cc}]/u.test(id) ? 'modelCatalog.idInvalid' : (counts.get(`${row.provider}:${id}`) ?? 0) > 1 ? 'modelCatalog.idDuplicate' : undefined,
    } as const
  })
}
