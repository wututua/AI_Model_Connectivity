import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import ts from 'typescript'

export function loadUtility(name, globals = {}) {
  const source = readFileSync(new URL(`../src/utils/${name}.ts`, import.meta.url), 'utf8')
  const { outputText } = ts.transpileModule(source, {
    compilerOptions: { target: ts.ScriptTarget.ES2020, module: ts.ModuleKind.CommonJS },
  })
  const exports = {}
  runInNewContext(outputText, { exports, ...globals }, { filename: `${name}.ts` })
  return exports
}

export async function settle() {
  for (let i = 0; i < 8; i++) await Promise.resolve()
}

export function fakeClock() {
  let now = 0
  let nextID = 0
  const timers = new Map()
  return {
    globals: {
      setTimeout(fn, delay) {
        const id = ++nextID
        timers.set(id, { at: now + delay, fn })
        return id
      },
      clearTimeout(id) { timers.delete(id) },
    },
    get pending() { return timers.size },
    async advance(ms) {
      await settle()
      const end = now + ms
      while (true) {
        const next = [...timers].sort((a, b) => a[1].at - b[1].at)[0]
        if (!next || next[1].at > end) break
        now = next[1].at
        timers.delete(next[0])
        next[1].fn()
        await settle()
      }
      now = end
      await settle()
    },
  }
}
