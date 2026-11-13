import test from 'node:test'
import assert from 'node:assert/strict'
import { showLocalSetup } from '../src/composables/localSetup.ts'

test('first local visit requires confirmation and saved consent closes setup', () => {
  assert.equal(showLocalSetup({ confirmed: false }, [], false, false), true)
  assert.equal(showLocalSetup({ confirmed: true }, [{ local: true }], true, false), false)
  assert.equal(showLocalSetup(undefined, [], false, false), false)
})

test('remote users can skip local setup across refresh and explicitly return to it', () => {
  assert.equal(showLocalSetup({ confirmed: false }, [], false, true), false)
  assert.equal(showLocalSetup({ confirmed: false }, [{ local: false }], false, false), false)
  assert.equal(showLocalSetup({ confirmed: false }, [{ local: false }], true, false), true)
})
