import type { Detail, WorkspaceEvent } from '../types'

function upsert<T extends { id: string }>(current: T[], changed: T[]): T[] {
  const replacements = new Map(changed.map(value => [value.id, value]))
  const result = current.map(value => replacements.get(value.id) ?? value)
  const present = new Set(current.map(value => value.id))
  return [...result, ...changed.filter(value => !present.has(value.id))]
}

export function mergeDetail(current: Detail | undefined, update: Detail): Detail {
  if (!update.partial) return update
  if (!current || current.session.id !== update.session.id) throw new Error('Reload the session before applying incremental updates.')
  if (update.cursor < current.cursor) return current
  return {
    session: update.session,
    cursor: update.cursor,
    entries: upsert(current.entries, update.entries),
    turns: upsert(current.turns, update.turns),
    interactions: upsert(current.interactions, update.interactions),
  }
}

export function refreshScope(event: WorkspaceEvent, selectedId: string) {
  if (event.type === 'navigation.updated') {
    return { bootstrap: true, detail: event.resource === selectedId, full: false }
  }
  if (event.type === 'session.updated') {
    return { bootstrap: event.resource !== selectedId, detail: event.resource === selectedId, full: false }
  }
  return { bootstrap: true, detail: event.type === 'state.updated', full: false }
}
