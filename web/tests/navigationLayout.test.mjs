import test from 'node:test'
import assert from 'node:assert/strict'
import { clampNavigationWidth, navigationLayoutKey, readNavigationLayout } from '../src/composables/navigationLayout.ts'

test('navigation layout restores valid choices and confines saved widths', () => {
  const storage = value => ({ getItem: key => { assert.equal(key, navigationLayoutKey); return JSON.stringify(value) } })
  assert.deepEqual(readNavigationLayout(storage({ width: 340, collapsed: true })), { width: 340, collapsed: true })
  assert.deepEqual(readNavigationLayout(storage({ width: 9000, collapsed: false })), { width: 380, collapsed: false })
  assert.equal(clampNavigationWidth(-10), 240)
  assert.equal(clampNavigationWidth(280.6), 281)
})

test('invalid or blocked browser storage leaves navigation usable', () => {
  for (const storage of [undefined, { getItem() { throw new Error('blocked') } }, { getItem: () => 'invalid' }, { getItem: () => '[]' }, { getItem: () => '{"width":"10","collapsed":"true"}' }]) {
    assert.deepEqual(readNavigationLayout(storage), { width: 288, collapsed: false })
  }
  assert.equal(clampNavigationWidth(Infinity), 288)
})
