import { createRequire } from 'node:module'
import { readFileSync } from 'node:fs'
import { resolve, dirname } from 'node:path'
import { JSDOM } from 'jsdom'
const require = createRequire(import.meta.url)
export const dom = new JSDOM('<!doctype html><html><body></body></html>', { url: 'https://motion.example.invalid' })
for (const key of ['window', 'document', 'history', 'location', 'HTMLElement', 'Element', 'Node', 'SVGElement', 'Event']) Object.defineProperty(globalThis, key, { configurable: true, value: (dom.window as any)[key] })
globalThis.requestAnimationFrame = (fn: FrameRequestCallback) => setTimeout(fn, 0) as any
globalThis.cancelAnimationFrame = (id: number) => clearTimeout(id)
globalThis.getComputedStyle = dom.window.getComputedStyle.bind(dom.window)
export const vue = require('vue')
const ts = require('typescript')
const { parse, compileScript } = require('vue/compiler-sfc')
const root = resolve(import.meta.dirname, '../../src')
export function loadSource(file: string, mocks: Record<string, any> = {}): any {
  const cache = new Map<string, any>()
  const load = (file: string): any => {
    file = resolve(root, file)
    if (cache.has(file)) return cache.get(file)
    let source = readFileSync(file, 'utf8')
    if (file.endsWith('.vue')) source = compileScript(parse(source, { filename: file }).descriptor, { id: 'motion17c', inlineTemplate: true }).content
    const code = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS, esModuleInterop: true } }).outputText
    const mod = { exports: {} }
    cache.set(file, mod.exports)
    const localRequire = (name: string) => {
      if (name in mocks) return mocks[name]?.default ? { __esModule: true, ...mocks[name] } : mocks[name]
      if (!name.startsWith('.') && !name.startsWith('@/')) return require(name)
      let target = name.startsWith('@/') ? resolve(root, name.slice(2)) : resolve(dirname(file), name)
      if (!/\.(ts|vue)$/.test(target)) target += '.ts'
      return load(target)
    }
    new Function('require', 'module', 'exports', code)(localRequire, mod, mod.exports)
    return mod.exports
  }
  return load(file)
}
export const settle = async () => { await vue.nextTick(); await Promise.resolve(); await vue.nextTick() }
