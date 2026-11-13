import test from 'node:test'
import assert from 'node:assert/strict'
import { mergeDetail, refreshScope } from '../src/composables/sessionSnapshot.ts'

function snapshot(cursor, entries, overrides = {}) {
  return { session: { id: 'session-a', state: 'running' }, cursor, entries, turns: [], interactions: [], ...overrides }
}

test('incremental output preserves history and replaces only changed entries', () => {
  const current = snapshot(10, [{ id: 'old', text: 'Earlier task' }, { id: 'live', text: 'Hello' }])
  const update = snapshot(12, [{ id: 'live', text: 'Hello world' }, { id: 'next', text: 'Tool result' }], { partial: true })
  const result = mergeDetail(current, update)
  assert.deepEqual(result.entries, [{ id: 'old', text: 'Earlier task' }, { id: 'live', text: 'Hello world' }, { id: 'next', text: 'Tool result' }])
  assert.equal(result.cursor, 12)
  assert.equal(current.entries[1].text, 'Hello')
})

test('a delayed update cannot roll back terminal output or the cursor', () => {
  const completed = snapshot(20, [{ id: 'live', text: 'Final answer', status: 'completed' }], { session: { id: 'session-a', state: 'idle' } })
  const delayed = snapshot(19, [{ id: 'live', text: 'Final', status: 'running' }], { partial: true })
  assert.equal(mergeDetail(completed, delayed), completed)
})

test('incremental updates cannot import another session history', () => {
  const current = snapshot(10, [{ id: 'old', text: 'Session A' }])
  const other = snapshot(11, [], { partial: true, session: { id: 'session-b' } })
  assert.throws(() => mergeDetail(current, other), /Reload the session/)
})

test('an unrelated session event refreshes the sidebar without loading selected history', () => {
  assert.deepEqual(refreshScope({ id: 15, type: 'session.updated', resource: 'session-b' }, 'session-a'), { bootstrap: true, detail: false, full: false })
  assert.deepEqual(refreshScope({ id: 16, type: 'session.updated', resource: 'session-a' }, 'session-a'), { bootstrap: false, detail: true, full: false })
  assert.deepEqual(refreshScope({ id: 17, type: 'state.updated', resource: '' }, 'session-a'), { bootstrap: true, detail: true, full: false })
})
