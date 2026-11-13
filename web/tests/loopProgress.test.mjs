import test from 'node:test'
import assert from 'node:assert/strict'
import { loopProgress } from '../src/composables/loopProgress.ts'

function workflow(values = {}) {
  return { state: 'running', stage: 'design', hashes: { 'documents/requirements.md': 'approved' }, ...values }
}

test('no workflow or queued turn never appears as an executing node', () => {
  const empty = loopProgress(null)
  assert.equal(empty.current, null)
  assert.equal(empty.flowing, false)
  assert.equal(empty.completed, 0)
  const queued = loopProgress(workflow({ state: 'pending' }), 'running')
  assert.equal(queued.status, 'queued')
  assert.equal(queued.states.get('design'), 'queued')
  assert.equal(queued.flowing, false)
})

test('the native active stage alone owns motion and upstream completion uses accepted files', () => {
  const active = loopProgress(workflow(), 'running')
  assert.equal(active.current, 'design')
  assert.equal(active.states.get('requirements'), 'completed')
  assert.equal(active.states.get('design'), 'running')
  assert.equal(active.states.get('grill'), 'pending')
  assert.equal(active.flowing, true)
  const missing = loopProgress(workflow({ hashes: {} }), 'running')
  assert.equal(missing.states.get('requirements'), 'pending')
  assert.equal(missing.completed, 0)
})

test('native approvals, questions and uncertain synchronization stop route particles', () => {
  for (const [native, expected] of [['waiting_approval', 'approval'], ['waiting_input', 'answers'], ['syncing', 'unknown'], [undefined, 'unknown'], ['offline', 'unknown']]) {
    const progress = loopProgress(workflow(), native)
    assert.equal(progress.current, 'design')
    assert.equal(progress.status, expected)
    assert.equal(progress.flowing, false)
  }
  assert.equal(loopProgress(workflow(), 'idle').status, 'validating')
})

test('human confirmation and paused gates retain the correct location', () => {
  const review = workflow({ stage: 'alignment', state: 'review', hashes: { 'documents/requirements.md': 'r', 'design-documents/design.md': 'd', 'decisions/grill-me.md': 'g' } })
  const progress = loopProgress(review, 'idle')
  assert.equal(progress.current, 'confirmation')
  assert.equal(progress.status, 'confirmation')
  assert.equal(progress.flowing, false)
  const paused = loopProgress({ ...review, state: 'cancelled', pausedState: 'review' }, 'running')
  assert.equal(paused.current, 'confirmation')
  assert.equal(paused.states.get('confirmation'), 'paused')
  assert.equal(paused.flowing, false)
  const completingTurn = loopProgress(workflow({ state: 'cancelled', pausedState: 'running' }), 'running')
  assert.equal(completingTurn.status, 'pausing')
  assert.equal(completingTurn.flowing, false)
})

test('failed stages stop in place and preview readiness finishes preparation without running development', () => {
  const failed = loopProgress(workflow({ state: 'failed' }), 'running')
  assert.equal(failed.current, 'design')
  assert.equal(failed.states.get('design'), 'failed')
  assert.equal(failed.flowing, false)
  const ready = loopProgress(workflow({ state: 'preview', stage: 'loop' }), 'running')
  assert.equal(ready.status, 'ready')
  assert.equal(ready.completed, 6)
  assert.equal(ready.flowing, false)
  assert.ok([...ready.states.values()].every(state => state === 'completed'))
})
