import test from 'node:test'
import assert from 'node:assert/strict'
import { existsSync, mkdirSync, readFileSync, rmdirSync, symlinkSync, unlinkSync } from 'node:fs'
import path from 'node:path'
import { checkSkills, syncSkills, validateSource } from '../skills.mjs'
import { fixture } from './fixtures.mjs'

const skill = '---\nname: example\ndescription: Exercise a real workflow.\n---\n\n# Example\n'

function skillFixture(t) {
  const result = fixture(t)
  result.write('.agents/skills/example/SKILL.md', skill)
  result.write('scripts/skills-lock.json', '{"schemaVersion":1,"files":{}}\n')
  return result
}

test('sync includes supporting scripts and binary assets, and becomes idempotent', t => {
  const { root, write } = skillFixture(t)
  write('.agents/skills/example/scripts/run.mjs', 'export const verdict = "before"\n')
  write('.agents/skills/example/assets/data.bin', Buffer.from([0, 255, 10]))
  assert.equal(syncSkills(root).updated, 3)
  assert.equal(checkSkills(root).files, 3)
  write('.agents/skills/example/scripts/run.mjs', 'export const verdict = "after"\n')
  assert.throws(() => checkSkills(root), /stale/)
  assert.equal(syncSkills(root).updated, 1)
  assert.match(readFileSync(path.join(root, '.claude/skills/example/scripts/run.mjs'), 'utf8'), /after/)
  assert.deepEqual(readFileSync(path.join(root, '.claude/skills/example/assets/data.bin')), Buffer.from([0, 255, 10]))
  assert.equal(syncSkills(root).updated, 0)
})

test('independent mirror edits abort sync before any other file is written', t => {
  const { root, write } = skillFixture(t)
  write('.agents/skills/example/reference.md', 'before\n')
  syncSkills(root)
  write('.agents/skills/example/reference.md', 'after\n')
  write('.claude/skills/example/SKILL.md', `${skill}Independent decision\n`)
  assert.throws(() => syncSkills(root), /Independent mirror edit/)
  assert.equal(readFileSync(path.join(root, '.claude/skills/example/reference.md'), 'utf8'), 'before\n')
  assert.match(readFileSync(path.join(root, '.claude/skills/example/SKILL.md'), 'utf8'), /Independent decision/)
})

test('sync removes unchanged owned files and preserves unowned mirror files', t => {
  const { root, write } = skillFixture(t)
  write('.agents/skills/example/obsolete.txt', 'generated\n')
  syncSkills(root)
  write('.claude/skills/example/local.txt', 'local material\n')
  unlinkSync(path.join(root, '.agents/skills/example/obsolete.txt'))
  assert.equal(syncSkills(root).removed, 1)
  assert.equal(existsSync(path.join(root, '.claude/skills/example/obsolete.txt')), false)
  assert.equal(readFileSync(path.join(root, '.claude/skills/example/local.txt'), 'utf8'), 'local material\n')
  assert.equal(checkSkills(root).unownedMirrorFiles, 1)
})

test('text line endings are portable across checkouts', t => {
  const { root, write } = skillFixture(t)
  syncSkills(root)
  write('.claude/skills/example/SKILL.md', skill.replace(/\n/g, '\r\n'))
  assert.equal(checkSkills(root).skills, 1)
  write('.agents/skills/example/scripts/helper.py', 'print("example")\n')
  syncSkills(root)
  write('.claude/skills/example/scripts/helper.py', 'print("example")\r\n')
  assert.equal(checkSkills(root).skills, 1)
})

test('owned assets can transition between files and directories', t => {
  const { root, write } = skillFixture(t)
  write('.agents/skills/example/assets/template', 'old asset\n')
  syncSkills(root)
  unlinkSync(path.join(root, '.agents/skills/example/assets/template'))
  write('.agents/skills/example/assets/template/config.json', '{}\n')
  assert.equal(syncSkills(root).removed, 1)
  assert.equal(checkSkills(root).files, 2)
  unlinkSync(path.join(root, '.agents/skills/example/assets/template/config.json'))
  rmdirSync(path.join(root, '.agents/skills/example/assets/template'))
  write('.agents/skills/example/assets/template', 'new asset\n')
  assert.equal(syncSkills(root).removed, 1)
  assert.equal(readFileSync(path.join(root, '.claude/skills/example/assets/template'), 'utf8'), 'new asset\n')
  assert.equal(checkSkills(root).files, 2)
})

test('a directory transition preserves unowned mirror descendants', t => {
  const { root, write } = skillFixture(t)
  write('.agents/skills/example/assets/template/config.json', '{}\n')
  syncSkills(root)
  write('.claude/skills/example/assets/template/local.txt', 'local content\n')
  unlinkSync(path.join(root, '.agents/skills/example/assets/template/config.json'))
  rmdirSync(path.join(root, '.agents/skills/example/assets/template'))
  write('.agents/skills/example/assets/template', 'replacement\n')
  assert.throws(() => syncSkills(root), /Unowned mirror directory/)
  assert.equal(readFileSync(path.join(root, '.claude/skills/example/assets/template/local.txt'), 'utf8'), 'local content\n')
  assert.equal(existsSync(path.join(root, '.claude/skills/example/assets/template/config.json')), true)
})

test('a directory transition succeeds after the owned mirror child was already deleted', t => {
  const { root, write } = skillFixture(t)
  write('.agents/skills/example/assets/template/config.json', '{}\n')
  syncSkills(root)
  unlinkSync(path.join(root, '.claude/skills/example/assets/template/config.json'))
  unlinkSync(path.join(root, '.agents/skills/example/assets/template/config.json'))
  rmdirSync(path.join(root, '.agents/skills/example/assets/template'))
  write('.agents/skills/example/assets/template', 'replacement\n')
  assert.equal(syncSkills(root).updated, 1)
  assert.equal(readFileSync(path.join(root, '.claude/skills/example/assets/template'), 'utf8'), 'replacement\n')
  assert.equal(checkSkills(root).files, 2)
})

test('an unowned empty mirror directory blocks a transition before mutation', t => {
  const { root, write } = skillFixture(t)
  write('.agents/skills/example/assets/template/config.json', '{}\n')
  syncSkills(root)
  mkdirSync(path.join(root, '.claude/skills/example/assets/template/local'))
  const lock = readFileSync(path.join(root, 'scripts/skills-lock.json'))
  unlinkSync(path.join(root, '.agents/skills/example/assets/template/config.json'))
  rmdirSync(path.join(root, '.agents/skills/example/assets/template'))
  write('.agents/skills/example/assets/template', 'replacement\n')
  assert.throws(() => syncSkills(root), /Unowned mirror directory/)
  assert.equal(existsSync(path.join(root, '.claude/skills/example/assets/template/local')), true)
  assert.equal(readFileSync(path.join(root, '.claude/skills/example/assets/template/config.json'), 'utf8'), '{}\n')
  assert.deepEqual(readFileSync(path.join(root, 'scripts/skills-lock.json')), lock)
})

test('native invocation policy and real reference links are checked', t => {
  const { root, write } = skillFixture(t)
  write('.agents/skills/example/SKILL.md', skill.replace('description:', 'disable-model-invocation: true\ndescription:'))
  assert.throws(() => validateSource(root), /policies disagree/)
  write('.agents/skills/example/agents/openai.yaml', 'policy:\n  allow_implicit_invocation: false\n')
  assert.equal(validateSource(root).skills, 1)
  write('.agents/skills/example/SKILL.md', `${skill.replace('description:', 'disable-model-invocation: true\ndescription:')}\n[Reference](missing.md)\n`)
  assert.throws(() => validateSource(root), /Missing reference/)
  write('.agents/skills/example/missing.md', 'Actual reference\n')
  assert.equal(validateSource(root).skills, 1)
})

test('invalid YAML and duplicate metadata are rejected', t => {
  const { root, write } = skillFixture(t)
  write('.agents/skills/example/SKILL.md', '---\nname: example\nname: another\ndescription: Duplicate\n---\n')
  assert.throws(() => validateSource(root), /unique/i)
})

test('sync refuses a linked mirror ancestor without touching its target', t => {
  const { root, write } = skillFixture(t)
  write('external/skills/keep.txt', 'external content\n')
  symlinkSync(path.join(root, 'external'), path.join(root, '.claude'), process.platform === 'win32' ? 'junction' : 'dir')
  assert.throws(() => syncSkills(root), /Linked skill paths/)
  assert.equal(readFileSync(path.join(root, 'external/skills/keep.txt'), 'utf8'), 'external content\n')
})
