import test from 'node:test'
import assert from 'node:assert/strict'
import { unlinkSync } from 'node:fs'
import path from 'node:path'
import { checkGoFormat } from '../go-format.mjs'
import { repository } from './fixtures.mjs'

test('Go formatting checks existing tracked and new files while allowing unstaged deletions', t => {
  const { root, write, git } = repository(t)
  write('removed.go', 'package example\n')
  git('add', 'removed.go')
  git('commit', '-q', '-m', 'Add old source')
  unlinkSync(path.join(root, 'removed.go'))
  write('new file.go', 'package example\n')
  assert.equal(checkGoFormat(root).files, 1)
  write('new file.go', 'package example\nfunc example( ){ }\n')
  assert.throws(() => checkGoFormat(root), /Unformatted Go files/)
})

test('invalid Go source fails the formatting gate', t => {
  const { root, write } = repository(t)
  write('invalid.go', 'package {\n')
  assert.throws(() => checkGoFormat(root))
})
