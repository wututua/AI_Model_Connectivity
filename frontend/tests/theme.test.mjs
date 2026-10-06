import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import { loadUtility } from './helpers.mjs'

for (const failure of ['getter', 'read', 'write']) {
  test(`theme works in memory when storage ${failure} throws`, () => {
    const window = {}
    Object.defineProperty(window, 'localStorage', {
      get() {
        if (failure === 'getter') throw new Error('blocked getter')
        return {
          getItem() { if (failure === 'read') throw new Error('blocked read'); return null },
          setItem() { throw new Error('blocked write') },
        }
      },
    })
    const theme = loadUtility('theme', { window })
    assert.equal(theme.readTheme(), 'dark')
    for (const next of ['light', 'auto', 'dark']) {
      assert.equal(theme.nextTheme(theme.readTheme()), next)
      theme.saveTheme(next)
      assert.equal(theme.readTheme(), next)
    }
  })
}

test('only valid saved themes are accepted', () => {
  for (const value of [null, '', 'unknown', 'DARK', 'dark', 'light', 'auto']) {
    const theme = loadUtility('theme', { window: { localStorage: { getItem: () => value } } })
    assert.equal(theme.readTheme(), ['dark', 'light', 'auto'].includes(value) ? value : 'dark')
  }
})

test('auto resolves current system preference and explicit modes ignore it', () => {
  let dark = false
  let applied
  const theme = loadUtility('theme', {
    window: { matchMedia: () => ({ matches: dark }) },
    document: { body: { setAttribute: (name, value) => { assert.equal(name, 'data-theme'); applied = value } } },
  })
  theme.applyTheme('auto')
  assert.equal(applied, 'light')
  dark = true
  theme.applyTheme('auto')
  assert.equal(applied, 'dark')
  theme.applyTheme('light')
  assert.equal(applied, 'light')
})

test('startup HTML theme tolerates denied storage and invalid values', () => {
  const html = readFileSync(new URL('../index.html', import.meta.url), 'utf8')
  const script = html.match(/<script>([\s\S]*?)<\/script>/)[1]
  for (const saved of ['blocked', 'invalid', 'light', 'auto']) {
    let applied
    const globals = {
      window: { matchMedia: () => ({ matches: false }) },
      document: { body: { setAttribute: (_, value) => { applied = value } } },
    }
    Object.defineProperty(globals, 'localStorage', {
      get() {
        if (saved === 'blocked') throw new Error('storage denied')
        return { getItem: () => saved }
      },
    })
    runInNewContext(script, globals)
    assert.equal(applied, saved === 'light' || saved === 'auto' ? 'light' : 'dark')
  }
})
