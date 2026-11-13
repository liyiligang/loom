import test from 'node:test'
import assert from 'node:assert/strict'
import { parseDocument } from 'yaml'
import { withSkillName } from '../src/composables/skillSource.ts'

test('renaming a creation draft retains instructions, metadata, and comments', () => {
  const body = '\n# Workflow\n\nRead requirements before designing.\n'
  const source = `---\nname: old-name\n# Keep this comment\ndescription: Review changes.\ncustom:\n  enabled: true\n---\n${body}`
  const renamed = withSkillName(source, 'api-review')
  assert.equal(parseDocument(renamed.split('---')[1]).getIn(['metadata', 'display_name']), 'api-review')
  assert.ok(renamed.includes('# Keep this comment'))
  assert.ok(renamed.endsWith(body))
  const metadata = parseDocument(renamed.split('---')[1]).toJS()
  assert.equal(metadata.name, 'old-name')
  assert.equal(metadata.description, 'Review changes.')
  assert.deepEqual(metadata.custom, { enabled: true })
  assert.equal(withSkillName(renamed, 'api-review'), renamed)
})

test('copied multiline and flow-style names become the new skill name', () => {
  for (const source of [
    '---\nname: >-\n  External skill\n  workflow\ndescription: Preserve this description.\n---\nBody',
    '---\n{name: original, description: Preserve this description.}\n---\nBody',
  ]) {
    const renamed = withSkillName(source, 'workflow-copy')
    const metadata = parseDocument(renamed.split('---')[1]).toJS()
    assert.equal(metadata.metadata.display_name, 'workflow-copy')
    assert.equal(metadata.description, 'Preserve this description.')
    assert.ok(renamed.endsWith('Body'))
  }
})

test('plain instructions acquire a name while malformed metadata remains available for correction', () => {
  const plain = '# Instructions\n\nUse the project conventions.'
  const named = withSkillName(plain, 'project-workflow')
  assert.ok(named.endsWith(plain))
  assert.equal(parseDocument(named.split('---')[1]).getIn(['metadata', 'display_name']), 'project-workflow')
  for (const invalid of ['---\nname: [\n---\nBody', '---\nname: unfinished\n', '---\nname: one\nname: two\n---\nBody']) {
    assert.equal(withSkillName(invalid, 'corrected'), invalid)
  }
})

test('renaming retains BOM, CRLF, and an empty front matter document', () => {
  const source = '\uFEFF---\r\nname: old\r\n---\r\nBody\r\n'
  const named = withSkillName(source, 'new-name')
  assert.ok(named.startsWith('\uFEFF---\r\n'))
  assert.ok(named.endsWith('Body\r\n'))
  assert.equal(named.replace(/\r\n/g, '').includes('\n'), false)
  assert.equal(parseDocument(withSkillName('---\n---\nBody', 'new-name').split('---')[1]).getIn(['metadata', 'display_name']), 'new-name')
})

test('Unicode display names preserve the native name and existing metadata', () => {
  const name = '\u9700\u6c42\u5bf9\u9f50'
  for (const source of [
    '---\nname: native-skill\nmetadata:\n  category: planning\n---\nInstructions',
    '---\nname: native-skill\nmetadata: null\n---\nInstructions',
  ]) {
    const result = withSkillName(source, name)
    const metadata = parseDocument(result.split('---')[1]).toJS()
    assert.equal(metadata.name, 'native-skill')
    assert.equal(metadata.metadata.display_name, name)
    assert.ok(result.endsWith('Instructions'))
  }
})
