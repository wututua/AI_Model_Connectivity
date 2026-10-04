import test from 'node:test'
import assert from 'node:assert/strict'
import { loadUtility } from './helpers.mjs'

const { mergeModels, parseModels } = loadUtility('models')

test('model selections trim, remove blanks, deduplicate and preserve order and case', () => {
  const current = [' custom/model ', 'model-a', 'Model-A']
  const result = mergeModels(current, ['model-a', '', 'model-b'])
  assert.deepEqual(Array.from(result), ['custom/model', 'model-a', 'Model-A', 'model-b'])
  assert.deepEqual(current, [' custom/model ', 'model-a', 'Model-A'])
})

test('manual model entry supports individual IDs and multiline or delimited paste', () => {
  assert.deepEqual(Array.from(parseModels('openai/gpt:custom-v2')), ['openai/gpt:custom-v2'])
  assert.deepEqual(Array.from(parseModels(' model-a,\r\nmodel-b;model-a，model-c；model-d ')), ['model-a', 'model-b', 'model-c', 'model-d'])
  assert.deepEqual(Array.from(parseModels(' \r\n ,；')), [])
})

test('syncing cannot discard manually added or previously selected models', () => {
  assert.deepEqual(Array.from(mergeModels(['custom', 'removed-upstream'], ['new', 'custom'])), ['custom', 'removed-upstream', 'new'])
  assert.deepEqual(Array.from(mergeModels(['custom'], [])), ['custom'])
})
