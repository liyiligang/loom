import { execFileSync } from 'node:child_process'
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import assert from 'node:assert/strict'

export function fixture(t) {
  const base = path.resolve(os.tmpdir())
  const root = mkdtempSync(path.join(base, 'loom-tooling-test-'))
  t.after(() => {
    const target = path.resolve(root)
    assert.ok(target.startsWith(`${base}${path.sep}`) && target !== base)
    rmSync(target, { recursive: true, force: true })
  })
  const write = (name, data) => {
    const file = path.join(root, name)
    mkdirSync(path.dirname(file), { recursive: true })
    writeFileSync(file, data)
  }
  const git = (...args) => execFileSync('git', ['-c', 'core.hooksPath=disabled-hooks', '-c', 'commit.gpgSign=false', '-c', 'core.autocrlf=false', ...args], {
    cwd: root, encoding: 'utf8', env: { ...process.env, GIT_CONFIG_NOSYSTEM: '1' },
  }).trim()
  return { root, write, git }
}

export function repository(t) {
  const result = fixture(t)
  result.git('init', '-q', '-b', 'main')
  result.git('config', 'user.name', 'Tooling Test')
  result.git('config', 'user.email', 'tooling-test@example.invalid')
  result.git('config', 'core.autocrlf', 'false')
  result.write('tracked.txt', 'before\n')
  result.git('add', 'tracked.txt')
  result.git('commit', '-q', '-m', 'Baseline')
  return result
}
