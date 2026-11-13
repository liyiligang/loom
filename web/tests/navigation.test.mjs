import test from 'node:test'
import assert from 'node:assert/strict'
import { readExpansion, toggleExpansion, navigationStorageKey } from '../src/composables/navigationExpansion.ts'
import { refreshScope } from '../src/composables/sessionSnapshot.ts'

test('expansion survives reload and malformed or blocked storage is ignored', () => {
  const value = { 'project:p1': true, 'requirement:r1': false }
  assert.deepEqual(readExpansion({ getItem: key => key === navigationStorageKey ? JSON.stringify(value) : null }), value)
  assert.deepEqual(readExpansion({ getItem: () => '{broken' }), {})
  assert.deepEqual(readExpansion({ getItem: () => '[true]' }), {})
  assert.deepEqual(readExpansion({ getItem: () => '{"project:p1":true,"requirement:r1":"true","__proto__":true}' }), { 'project:p1': true })
  assert.deepEqual(readExpansion({ getItem() { throw new Error('Blocked') } }), {})
})

test('project and requirement toggles are independent and preserve other projects', () => {
  const initial = { 'project:p1': true, 'requirement:r1': true, 'project:p2': false }
  const closed = toggleExpansion(initial, 'project:p1')
  assert.equal(closed['project:p1'], false)
  assert.equal(closed['requirement:r1'], true)
  const expanded = toggleExpansion(closed, 'requirement:r2')
  assert.equal(expanded['project:p1'], false)
  assert.equal(expanded['project:p2'], false)
  assert.equal(expanded['requirement:r2'], true)
  assert.equal(initial['project:p2'], false)
})

test('group changes refresh navigation and selected membership without fetching unrelated conversation history', () => {
  assert.deepEqual(refreshScope({ type: 'navigation.updated', resource: 's1' }, 's1'), { bootstrap: true, detail: true, full: false })
  assert.deepEqual(refreshScope({ type: 'navigation.updated', resource: 'r1' }, 's1'), { bootstrap: true, detail: false, full: false })
})
