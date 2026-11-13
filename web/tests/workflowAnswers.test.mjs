import test from 'node:test'
import assert from 'node:assert/strict'
import { useWorkflowAnswers } from '../src/composables/workflowAnswers.ts'

test('conversation and Grill me share the same draft across tab switches', () => {
  const storage = new Map()
  globalThis.localStorage = { getItem: key => storage.get(key) ?? null, setItem: (key, value) => storage.set(key, value), removeItem: key => storage.delete(key) }
  const conversation = useWorkflowAnswers(() => 'shared-flow')
  const grill = useWorkflowAnswers(() => 'shared-flow')
  conversation.answers.value.Q1 = '30 minutes'
  conversation.save()
  assert.equal(grill.answers.value.Q1, '30 minutes')
  grill.answers.value.Q1 = '60 minutes'
  assert.equal(conversation.answers.value.Q1, '60 minutes')
  assert.equal(storage.get('loom.workflow.answers.shared-flow'), '{"Q1":"30 minutes"}')
  grill.save()
  assert.equal(JSON.parse(storage.get('loom.workflow.answers.shared-flow')).Q1, '60 minutes')
  grill.clear()
  assert.equal(storage.has('loom.workflow.answers.shared-flow'), false)
})

test('malformed storage and reserved object names cannot break question answers', () => {
  globalThis.localStorage = { getItem: () => 'null', setItem() {}, removeItem() {} }
  const answers = useWorkflowAnswers(() => 'malformed-flow').answers
  assert.equal(answers.value.constructor, undefined)
  answers.value.constructor = 'An explicit answer'
  assert.equal(answers.value.constructor, 'An explicit answer')
  globalThis.localStorage.getItem = () => '{"Q1":42,"Q2":"Retained answer"}'
  const restored = useWorkflowAnswers(() => 'restored-flow').answers
  assert.equal(restored.value.Q1, undefined)
  assert.equal(restored.value.Q2, 'Retained answer')
})
