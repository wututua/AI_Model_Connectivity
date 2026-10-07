const fs = require('node:fs')
const path = require('node:path')
const { createHash } = require('node:crypto')

const packageName = '@radix-ui/react-dismissable-layer'
const files = [
  ['index.js', '041f442853dfb1c6ff9255fc5f0e8c2fa4e50bef906ef8f4fe9b4a1f581f4f6a', '35668101dc6ea090d8e49e9ae0e052b2880e4c0e576d345f7ffc1aff66219c91'],
  ['index.mjs', '01df449a23d65bab1f2f2215a953cba74741419496ebd9a7932533f267cc873d', '71e9a8d4755f422fac46098c33c713664895f9dde86e4a1ddfdbe92062ba2c1c'],
]
const hash = value => createHash('sha256').update(value).digest('hex')
const guard = [
  '      const currentLayers = Array.from(context.layers);',
  '      if (!node || node !== currentLayers[currentLayers.length - 1]) {',
  '        return;',
  '      }',
  '',
].join('\n')

function applyPatch(packageDir) {
  const pkg = JSON.parse(fs.readFileSync(path.join(packageDir, 'package.json'), 'utf8'))
  if (pkg.name !== packageName || pkg.version !== '1.1.19') {
    throw new Error('Review the Radix Escape patch before changing the dependency version')
  }
  // Validate both entry points before writing either. See Radix issue #4143.
  const updates = files.map(([name, before, after]) => {
    const target = path.join(packageDir, 'dist', name)
    const original = fs.readFileSync(target)
    if (hash(original) === after) return null
    if (hash(original) !== before) throw new Error(`Unexpected Radix source: ${name}`)
    const anchor = '      onEscapeKeyDown?.(event);\n'
    const patched = original.toString('utf8').replace(anchor, guard + anchor)
    if (hash(patched) !== after) throw new Error(`Unexpected Radix patch result: ${name}`)
    return { target, patched }
  })
  for (const update of updates) {
    if (update) fs.writeFileSync(update.target, update.patched)
  }
}

module.exports = { applyPatch }
if (require.main === module) {
  applyPatch(path.resolve(__dirname, '../node_modules', packageName))
  console.log('Verified Radix Escape patch (CommonJS and ESM)')
}
