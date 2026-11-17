import test from 'node:test'
import assert from 'node:assert/strict'
import { captureReview } from '../../.agents/skills/code-review/scripts/review-scope.mjs'
import { repository } from './fixtures.mjs'

test('workspace review includes unstaged, staged, and new files with a stable snapshot', t => {
  const { root, write, git } = repository(t)
  write('tracked.txt', 'staged\n')
  git('add', 'tracked.txt')
  write('tracked.txt', 'working\n')
  write("new file 'two'.txt", 'new behavior\n')
  const review = captureReview({ cwd: root })
  assert.equal(review.mode, 'workspace')
  assert.equal(review.empty, false)
  assert.match(review.trackedDiff, /\+working/)
  assert.match(review.stagedDiff, /\+staged/)
  assert.match(review.unstagedDiff, /-staged\n\+working/)
  assert.equal(review.untracked[0].path, "new file 'two'.txt")
  assert.equal(review.untracked[0].content, 'new behavior\n')
  assert.equal(captureReview({ cwd: root }).snapshotHash, review.snapshotHash)
  write("new file 'two'.txt", 'changed behavior\n')
  assert.notEqual(captureReview({ cwd: root }).snapshotHash, review.snapshotHash)
})

test('a staged change remains reviewable when the working file is restored', t => {
  const { root, write, git } = repository(t)
  write('tracked.txt', 'planned\n')
  git('add', 'tracked.txt')
  write('tracked.txt', 'before\n')
  const review = captureReview({ cwd: root })
  assert.equal(review.trackedDiff, '')
  assert.equal(review.empty, false)
  assert.match(review.stagedDiff, /\+planned/)
  assert.match(review.unstagedDiff, /-planned/)
})

test('commit review pins the base and discloses excluded local changes', t => {
  const { root, write, git } = repository(t)
  const base = git('rev-parse', 'HEAD')
  write('tracked.txt', 'committed\n')
  git('add', 'tracked.txt')
  git('commit', '-q', '-m', 'Implement behavior')
  write('tracked.txt', 'local edit\n')
  write('new.txt', 'local file\n')
  const review = captureReview({ cwd: root, mode: 'commits', base })
  assert.equal(review.baseCommit, base)
  assert.equal(review.mergeBaseCommit, base)
  assert.match(review.trackedDiff, /\+committed/)
  assert.doesNotMatch(review.trackedDiff, /local edit/)
  assert.deepEqual(review.untracked, [])
  assert.equal(review.localChangesExcluded, true)
})

test('new binary files are represented by evidence without text decoding', t => {
  const { root, write } = repository(t)
  write('asset.bin', Buffer.from([0, 255, 17, 2]))
  const review = captureReview({ cwd: root })
  assert.equal(review.untracked[0].kind, 'binary')
  assert.equal(review.untracked[0].content, null)
  assert.equal(review.untracked[0].size, 4)
  assert.match(review.untracked[0].sha256, /^[a-f0-9]{64}$/)
  assert.equal(review.empty, false)
})

test('invalid scopes fail and an unchanged repository reports an empty scope', t => {
  const { root } = repository(t)
  assert.equal(captureReview({ cwd: root }).empty, true)
  assert.throws(() => captureReview({ cwd: root, mode: 'commits' }), /explicit base/)
  assert.throws(() => captureReview({ cwd: root, mode: 'unknown' }), /Mode/)
  assert.throws(() => captureReview({ cwd: root, base: '--not-a-reference' }))
})
