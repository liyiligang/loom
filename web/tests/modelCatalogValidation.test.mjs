import test from 'node:test'
import assert from 'node:assert/strict'
import { modelCatalogErrors } from '../src/composables/modelCatalogValidation.ts'

const row = (provider, id, name = 'Useful model') => ({ provider, id, name })

test('catalog errors identify both duplicates while allowing identifiers shared across agents', () => {
  const errors = modelCatalogErrors([row('codex', ' shared '), row('codex', 'shared'), row('claude', 'shared')])
  assert.equal(errors[0].id, 'modelCatalog.idDuplicate')
  assert.equal(errors[1].id, 'modelCatalog.idDuplicate')
  assert.equal(errors[2].id, undefined)
  assert.deepEqual(modelCatalogErrors([row('codex', 'one'), row('codex', 'two')]).map(value => value.id), [undefined, undefined])
  assert.deepEqual(modelCatalogErrors([row('codex', '\u0085shared\u0085'), row('codex', 'shared')]).map(value => value.id), ['modelCatalog.idDuplicate', 'modelCatalog.idDuplicate'])
  assert.deepEqual(modelCatalogErrors([row('codex', '\ufeffshared'), row('codex', 'shared')]).map(value => value.id), [undefined, undefined])
})

test('field validation matches name character limits and identifier byte and Unicode constraints', () => {
  const errors = modelCatalogErrors([row('codex', '', ''), row('codex', 'model\u0085name'), row('codex', '字'.repeat(41)), row('codex', 'valid', '字'.repeat(120)), row('codex', 'second', 'a'.repeat(121)), row('codex', 'third', 'a\0b')])
  assert.equal(errors[0].name, 'modelCatalog.nameRequired')
  assert.equal(errors[0].id, 'modelCatalog.idRequired')
  assert.equal(errors[1].id, 'modelCatalog.idInvalid')
  assert.equal(errors[2].id, 'modelCatalog.idTooLong')
  assert.equal(errors[3].name, undefined)
  assert.equal(errors[4].name, 'modelCatalog.nameTooLong')
  assert.equal(errors[5].name, 'modelCatalog.nameInvalid')
  assert.equal(modelCatalogErrors([row('codex', 'emoji', '\u{1f4f7}'.repeat(120))])[0].name, undefined)
  assert.equal(modelCatalogErrors([row('codex', 'empty-name', '\u0085')])[0].name, 'modelCatalog.nameRequired')
})
