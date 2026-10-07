const assert = require('node:assert/strict')
const fs = require('node:fs')
const os = require('node:os')
const path = require('node:path')
const { test } = require('node:test')
const { applyPatch } = require('../scripts/patch-radix.cjs')

const files = ['index.js', 'index.mjs']
const guard = [
  '      const currentLayers = Array.from(context.layers);',
  '      if (!node || node !== currentLayers[currentLayers.length - 1]) {',
  '        return;',
  '      }',
  '',
].join('\n')

function fixture(t) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'radix-patch-'))
  t.after(() => fs.rmSync(root, { recursive: true, force: true }))
  fs.mkdirSync(path.join(root, 'dist'))
  fs.writeFileSync(path.join(root, 'package.json'), JSON.stringify({ name: '@radix-ui/react-dismissable-layer', version: '1.1.19' }))
  for (const name of files) {
    const source = fs.readFileSync(path.resolve(__dirname, '../node_modules/@radix-ui/react-dismissable-layer/dist', name), 'utf8')
    fs.writeFileSync(path.join(root, 'dist', name), source.replace(guard, ''))
  }
  return root
}

test('Radix patch covers both entry points and is idempotent', t => {
  const root = fixture(t)
  applyPatch(root)
  const patched = files.map(name => fs.readFileSync(path.join(root, 'dist', name), 'utf8'))
  assert.ok(patched.every(source => source.includes(guard)))
  applyPatch(root)
  assert.deepEqual(files.map(name => fs.readFileSync(path.join(root, 'dist', name), 'utf8')), patched)
})

test('Radix patch rejects changed source before modifying either entry point', t => {
  const root = fixture(t)
  const first = fs.readFileSync(path.join(root, 'dist', files[0]), 'utf8')
  fs.appendFileSync(path.join(root, 'dist', files[1]), '\n// changed source\n')
  assert.throws(() => applyPatch(root), /Unexpected Radix source/)
  assert.equal(fs.readFileSync(path.join(root, 'dist', files[0]), 'utf8'), first)
})

test('Radix patch rejects an unreviewed dependency version', t => {
  const root = fixture(t)
  fs.writeFileSync(path.join(root, 'package.json'), JSON.stringify({ name: '@radix-ui/react-dismissable-layer', version: '1.1.20' }))
  assert.throws(() => applyPatch(root), /Review the Radix Escape patch/)
})
