import { createRequire } from 'node:module'
import { readFileSync } from 'node:fs'
import { resolve, dirname } from 'node:path'
import { JSDOM } from 'jsdom'
const require = createRequire(import.meta.url)
export const dom = new JSDOM('<!doctype html><html><body><button id="trigger">Open</button><div id="app"></div></body></html>', { url: 'https://fixture.example.invalid' })
for (const key of ['window', 'document', 'HTMLElement', 'Element', 'Node', 'SVGElement', 'Event', 'KeyboardEvent', 'MouseEvent']) Object.defineProperty(globalThis, key, { configurable: true, value: dom.window[key] })
globalThis.requestAnimationFrame = (fn: FrameRequestCallback) => setTimeout(fn, 0) as any
globalThis.cancelAnimationFrame = (id: number) => clearTimeout(id)
dom.window.scrollTo = () => {}
globalThis.getComputedStyle = dom.window.getComputedStyle.bind(dom.window)
export const vue = require('vue')
const ts = require('typescript')
const { parse, compileScript } = require('vue/compiler-sfc')
export const root = resolve(import.meta.dirname, '../../src')
export function loader(mocks: Record<string, any> = {}) {
  const cache = new Map<string, any>()
  const load = (file: string): any => {
    file = resolve(root, file)
    if (cache.has(file)) return cache.get(file)
    let source = readFileSync(file, 'utf8')
    if (file.endsWith('.vue')) source = compileScript(parse(source, { filename: file }).descriptor, { id: 'motion17d', inlineTemplate: true }).content
    const output = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS, esModuleInterop: true } }).outputText
    const mod = { exports: {} }
    cache.set(file, mod.exports)
    const localRequire = (name: string) => {
      if (name in mocks) return mocks[name]?.default ? { __esModule: true, ...mocks[name] } : mocks[name]
      if (!name.startsWith('.') && !name.startsWith('@/')) return require(name)
      let target = name.startsWith('@/') ? resolve(root, name.slice(2)) : resolve(dirname(file), name)
      if (!/\.(ts|vue)$/.test(target)) target += '.ts'
      return load(target)
    }
    new Function('require', 'module', 'exports', output)(localRequire, mod, mod.exports)
    return mod.exports
  }
  return load
}
export const deferred = () => {
  let resolve!: (value: any) => void
  let reject!: (value: any) => void
  const promise = new Promise((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
export const settle = async () => { await Promise.resolve(); await vue.nextTick(); await Promise.resolve(); await vue.nextTick() }
export function mountSetup(setup: () => any) {
  let state: any
  const host = document.createElement('div'); document.body.append(host)
  const app = vue.createApp({ setup() { state = setup(); return () => vue.h('div') } })
  app.mount(host)
  return { state, unmount() { app.unmount(); host.remove() } }
}
