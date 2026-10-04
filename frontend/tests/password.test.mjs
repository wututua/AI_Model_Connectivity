import assert from 'node:assert/strict'
import test from 'node:test'
import { loadUtility } from './helpers.mjs'

const { passwordError } = loadUtility('password', { TextEncoder })
test('passwords require eight characters, upper/lower letters and digits, not symbols', () => {
  assert.equal(passwordError('Abcd1234'), '')
  assert.equal(passwordError('Abcd1234!'), '')
  for (const value of ['Abc1234', 'abcdefgh1', 'ABCDEFGH1', 'Abcdefgh', 'Aa1'.repeat(400)]) assert.notEqual(passwordError(value), '')
})
