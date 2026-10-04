import test from 'node:test'
import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { readFileSync } from 'node:fs'
import postcss from 'postcss'
import tailwind from '../tailwind.config.js'

test('HarmonyOS Sans faces use local, unmodified fonts with swap rendering', () => {
  const expected = new Map([
    ['400 500', ['Regular', '297b088424be212207df2ce8b98e335468b782aa6b96832af0b8b773d711e2b1']],
    ['600 900', ['Bold', '43a424b85e47fb53a17b3b32026a71801f86f8e022ca6798d186b47d39fa5f01']],
  ])
  let count = 0
  postcss.parse(readFileSync(new URL('../src/fonts.css', import.meta.url), 'utf8')).walkAtRules('font-face', rule => {
    const declarations = Object.fromEntries(rule.nodes.filter(node => node.type === 'decl').map(node => [node.prop, node.value]))
    assert.equal(declarations['font-family'], "'HarmonyOS Sans'")
    assert.equal(declarations['font-display'], 'swap')
    const [weight, hash] = expected.get(declarations['font-weight'])
    const filename = `HarmonyOS_Sans_SC_${weight}.ttf`
    assert.ok(declarations.src.includes(`/fonts/harmonyos-sans/${filename}`))
    assert.ok(declarations.src.includes(`?v=${hash}`))
    const bytes = readFileSync(new URL(`../public/fonts/harmonyos-sans/${filename}`, import.meta.url))
    assert.equal(createHash('sha256').update(bytes).digest('hex'), hash)
    count++
  })
  assert.equal(count, 2)
})

test('both UI font stacks use HarmonyOS Sans and the original font license is bundled', () => {
  for (const family of ['sans', 'mono']) assert.equal(tailwind.theme.extend.fontFamily[family][0], '"HarmonyOS Sans"')
  const html = readFileSync(new URL('../index.html', import.meta.url), 'utf8')
  assert.ok(html.includes('HarmonyOS_Sans_SC_Regular.ttf'))
  assert.ok(html.includes('?v=297b088424be212207df2ce8b98e335468b782aa6b96832af0b8b773d711e2b1'))
  assert.ok(!/fonts\.googleapis|fonts\.gstatic|Fira\+Code|family=Inter/.test(html))
  const license = readFileSync(new URL('../public/fonts/harmonyos-sans/LICENSE.txt', import.meta.url), 'utf8')
  assert.ok(license.includes('Copyright 2021 Huawei Device Co., Ltd.'))
  assert.ok(license.includes('HarmonyOS Sans Fonts License Agreement'))
})
