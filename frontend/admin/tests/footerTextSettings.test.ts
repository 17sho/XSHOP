import test, { afterEach } from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import { createRequire } from 'node:module'
import ts from 'typescript'
import { JSDOM } from 'jsdom'
import { parse, compileScript, compileTemplate } from 'vue/compiler-sfc'
const dom = new JSDOM('<!doctype html><body></body>', { url: 'https://fixture.example.invalid' })
for (const key of ['window', 'document', 'Element', 'HTMLElement', 'SVGElement', 'Node']) Object.defineProperty(globalThis, key, { configurable: true, value: (dom.window as any)[key] })
const require = createRequire(import.meta.url), vue = require('vue')
const keys = ['telegram_binding', 'google_binding', 'email_change', 'password_change', 'two_factor', 'login_history']
const flush = async () => { await new Promise(r => setImmediate(r)); await vue.nextTick() }
const deferred = () => { let resolve: any, reject: any; const promise = new Promise((a, b) => { resolve = a; reject = b }); return { promise, resolve, reject } }
let app: any, host: HTMLElement
const warnings: string[] = []
afterEach(() => { app?.unmount(); app = undefined; host?.remove(); assert.deepEqual(warnings.splice(0), []) })
function run(code: string, mocks: any) {
  const output = ts.transpileModule(code, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS, esModuleInterop: true } }).outputText
  const mod = { exports: {} as any }
  Function('require', 'module', 'exports', output)((name: string) => {
    if (name in mocks) return mocks[name]
    if (name === '@/utils/securityCenterConfig') return run(fs.readFileSync(new URL('../src/utils/securityCenterConfig.ts', import.meta.url), 'utf8'), {})
    if (name.startsWith('@/')) throw new Error(`Unmocked boundary ${name}`)
    return require(name)
  }, mod, mod.exports)
  return mod.exports
}
test('actual Settings loads and saves independent footer text without replacing adjacent settings', async () => {
  const source = fs.readFileSync(new URL('../src/views/admin/Settings.vue', import.meta.url), 'utf8')
  const descriptor = parse(source, { filename: 'Settings.vue' }).descriptor
  const script = compileScript(descriptor, { id: 'settings-parent' })
  const response = (data: any) => ({ data: { data } })
  const saves: any[] = []
  const mocks: any = {
    vue, 'vue-i18n': { useI18n: () => ({ t: (key: string) => key }) },
    '@/api/admin': { adminAPI: { getSettings: async (p: any) => response(p.key === 'site_config' ? { footer_text: '© {year} 雪糕数卡', footer_links: [{name: '支持', url: 'https://example.invalid'}], product_search_enabled: true } : {}), updateSettings: async (p: any) => saves.push(p) } },
    '@/utils/notify': { notifyError() {}, notifySuccess() {} }, '@/utils/favicon': { applySiteIcon() {} }, '@/utils/image': { getImageUrl: (s: string) => s },
    '@/composables/useAdminBrand': { useAdminBrand: () => ({ beginLoad: () => () => true }) },
    '@/utils/orderEmailTemplates': { orderEmailSceneKeys: [] },
    '@/utils/registrationEmailTemplates': { verificationScenes: ['registration', 'reset', 'telegram_bind', 'change_email_old', 'change_email_new'] },
  }
  for (const statement of ts.createSourceFile('parent.ts', script.content, ts.ScriptTarget.Latest, true).statements) {
    if (!ts.isImportDeclaration(statement)) continue
    const name = (statement.moduleSpecifier as any).text
    if (!name.includes('components/') && !name.startsWith('@/components')) continue
    const empty = { render: () => null }
    const entry: any = { __esModule: true, default: empty }
    const bindings = statement.importClause?.namedBindings
    if (bindings && ts.isNamedImports(bindings)) for (const element of bindings.elements) entry[element.name.text] = empty
    mocks[name] = entry
  }
  const component = run(script.content, mocks).default
  component.render = () => vue.h('div')
  host = document.createElement('div'); document.body.append(host)
  app = vue.createApp(component); app.config.warnHandler = (m: string) => warnings.push(m)
  const vm = app.mount(host); await flush(); await flush()
  const state = vm.$.setupState
  assert.equal(state.form.footer_text, '© {year} 雪糕数卡')
  state.form.footer_text = '新版权 {year}'
  await state.saveSiteSettings()
  assert.equal(saves[0].value.footer_text, '新版权 {year}')
  assert.deepEqual(saves[0].value.footer_links, [{ name: '支持', url: 'https://example.invalid' }])
  assert.equal(saves[0].value.product_search_enabled, true)
  state.form.footer_text = ''
  await state.saveSiteSettings()
  assert.equal(saves[1].value.footer_text, '')
})
