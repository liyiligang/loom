import test from 'node:test'
import assert from 'node:assert/strict'
import { StreamText } from '../src/composables/streamText.ts'

test('loaded history is immediately readable, including a running response', () => {
  for (const status of ['completed', 'running']) {
    const stream = new StreamText('An existing answer.', status)
    assert.equal(stream.text, 'An existing answer.')
    assert.equal(stream.pending, false)
  }
})

test('live append reveals monotonically and catches up within 180 milliseconds', () => {
  const stream = new StreamText('Hello', 'running')
  const final = 'Hello, this answer arrives in a snapshot.'
  stream.update(final, 'running', 0)
  assert.equal(stream.text, 'Hello')
  let previous = stream.text
  for (const now of [32, 64, 96, 128, 160, 180]) {
    const text = stream.advance(now)
    assert.ok(text.startsWith(previous))
    assert.ok(final.startsWith(text))
    previous = text
  }
  assert.equal(stream.text, final)
  assert.equal(stream.pending, false)
})

test('continuous snapshots do not postpone the catch-up deadline', () => {
  const stream = new StreamText('', 'running')
  stream.update('a'.repeat(100), 'running', 0)
  stream.advance(32)
  stream.update('a'.repeat(200), 'running', 100)
  stream.advance(180)
  assert.equal(stream.text, 'a'.repeat(200))
  stream.update('a'.repeat(300), 'running', 200)
  assert.equal(stream.pending, true)
  assert.equal(stream.advance(380), 'a'.repeat(300))
})

test('completion drains the exact final text promptly; stopped output flushes immediately', () => {
  const stream = new StreamText('Answer', 'running')
  stream.update('Answer, finally complete.', 'completed', 0)
  assert.equal(stream.pending, true)
  assert.equal(stream.advance(120), 'Answer, finally complete.')
  for (const status of ['failed', 'unknown', 'interrupted', 'cancelled']) {
    const stopped = new StreamText('', 'running')
    stopped.update('Partial output', 'running', 0)
    assert.equal(stopped.update('Partial output retained.', status, 40), 'Partial output retained.')
    assert.equal(stopped.pending, false)
  }
})

test('corrections, recovery backlogs, reduced motion, and long text bypass animation', () => {
  const stream = new StreamText('Earlier text', 'running')
  assert.equal(stream.update('Corrected text', 'running', 0), 'Corrected text')
  const backlog = 'Corrected text' + 'x'.repeat(2_049)
  assert.equal(stream.update(backlog, 'running', 10), backlog)
  assert.equal(stream.update(backlog + ' No animation.', 'running', 20, false), backlog + ' No animation.')
  const long = new StreamText('a'.repeat(48_000), 'running')
  assert.equal(long.update('a'.repeat(48_001), 'running', 0), 'a'.repeat(48_001))
  assert.equal(long.pending, false)
})

test('Unicode reveal boundaries preserve CJK, combining marks, flags, and joined emoji', () => {
  const final = '中文 e\u0301 👨‍👩‍👧‍👦 🇨🇳 👩🏽‍💻 done'
  const boundaries = new Set([0, ...Array.from(new Intl.Segmenter(undefined, { granularity: 'grapheme' }).segment(final), part => part.index + part.segment.length)])
  const stream = new StreamText('', 'running')
  stream.update(final, 'running', 0)
  for (let now = 0; now <= 180; now += 10) {
    const text = stream.advance(now)
    assert.ok(boundaries.has(text.length), `split grapheme at ${text.length}`)
  }
  assert.equal(stream.text, final)
  const extended = new StreamText('👩', 'running')
  extended.update('👩🏽‍💻 hello', 'running', 0)
  assert.equal(extended.advance(32), '👩🏽‍💻')
})
