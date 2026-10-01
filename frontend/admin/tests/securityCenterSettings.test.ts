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
async function mount() {
  const get = deferred(), put = deferred(), writes: any[] = [], reads: any[] = [], notices: any[] = []
  const file = new URL('../src/views/admin/components/SettingsSecurityCenterTab.vue', import.meta.url)
  const descriptor = parse(fs.readFileSync(file, 'utf8'), { filename: file.pathname }).descriptor
  const script = compileScript(descriptor, { id: 'security-settings' })
  const template = compileTemplate({ source: descriptor.template!.content, filename: file.pathname, id: 'security-settings', compilerOptions: { bindingMetadata: script.bindings } })
  assert.deepEqual(template.errors, [])
  const mocks = {
    vue, 'vue-i18n': { useI18n: () => ({ t: (key: string) => key }) },
    '@/api/admin': { adminAPI: { getSettings: (p: any) => { reads.push(p); return get.promise }, updateSettings: (p: any) => { writes.push(p); return put.promise } } },
    '@/utils/notify': { notifyError: (m: string) => notices.push(m), notifySuccess: (m: string) => notices.push(m) },
    '@/components/ui/switch': { Switch: { props: ['modelValue', 'disabled', 'id'], emits: ['update:modelValue'], setup: (p: any, { emit }: any) => () => vue.h('button', { id: p.id, role: 'switch', disabled: p.disabled, 'aria-checked': p.modelValue, onClick: () => emit('update:modelValue', !p.modelValue) }) } },
  }
  const component = run(script.content, mocks).default
  component.render = run(template.code, mocks).render
  host = document.createElement('div'); document.body.append(host)
  app = vue.createApp(component); app.config.warnHandler = (m: string) => warnings.push(m)
  const vm = app.mount(host); await flush()
  return { state: vm.$.setupState, exposed: vm, get, put, reads, writes, notices }
}
const response = (data: any) => ({ data: { data } })
test('security tab is loading-safe, uses the existing key API, and exposes ready to parent', async () => {
  const h = await mount()
  assert.deepEqual(h.reads, [{ key: 'security_center_config' }])
  await h.state.save(); assert.equal(h.writes.length, 0)
  assert.equal(h.state.ready, false)
  h.get.resolve(response({})); await flush()
  assert.equal(h.state.ready, true)
  assert.equal(host.querySelectorAll('[role=switch]').length, 6)
  assert.ok(host.textContent!.includes('providerHint'))
})
test('load failures disable controls and direct saves', async () => {
  const h = await mount()
  h.get.reject(new Error('synthetic')); await flush()
  await h.state.save()
  assert.equal(h.writes.length, 0)
  assert.equal(h.state.ready, false)
  assert.equal(h.state.loadFailed, true)
  assert.equal(host.querySelectorAll('[role=switch]:not([disabled])').length, 0)
})
for (let mask = 0; mask < 64; mask++) test(`admin persists independent booleans mask=${mask} without losing future siblings`, async () => {
  const h = await mount()
  h.get.resolve(response({ future_policy: { retained: true } })); await flush()
  keys.forEach((key, i) => { h.state.form[key] = !!(mask & (1 << i)) })
  const pending = h.state.save(); await flush()
  assert.equal(h.state.ready, false)
  assert.equal(h.state.submitting, true)
  assert.equal(host.querySelectorAll('[role=switch]:not([disabled])').length, 0)
  assert.deepEqual(h.writes, [{ key: 'security_center_config', value: { future_policy: { retained: true }, ...Object.fromEntries(keys.map((key, i) => [key, !!(mask & (1 << i))])) } }])
  await h.state.save(); assert.equal(h.writes.length, 1)
  h.put.resolve(response({})); await pending
  assert.equal(h.state.ready, true)
})
test('parent global save routes to this tab and consumes current ready/submitting state', () => {
  const source = fs.readFileSync(new URL('../src/views/admin/Settings.vue', import.meta.url), 'utf8')
  assert.match(source, /SettingsSecurityCenterTab/)
  assert.match(source, /currentTab\.value === 'security_center'/)
  assert.match(source, /securityCenterTabRef\.value\?\.save\(\)/)
  assert.match(source, /securityCenterTabRef\.value\?\.ready/)
})

test('all three admin/user locales explain independent policy without promising email changes when off', () => {
  const source = fs.readFileSync(new URL('../src/i18n/index.ts', import.meta.url), 'utf8')
  const sourceFile = ts.createSourceFile('messages.ts', source, ts.ScriptTarget.Latest, true)
  const statement = sourceFile.statements.find(node => ts.isVariableStatement(node) && node.declarationList.declarations.some(d => d.name.getText(sourceFile) === 'messages'))!
  const initializer = (statement as any).declarationList.declarations[0].initializer.getText(sourceFile)
  const messages = Function('registrationEmailTemplateMessages', `return (${initializer})`)({ 'zh-CN': {}, 'zh-TW': {}, 'en-US': {} })
  for (const locale of ['zh-CN', 'zh-TW', 'en-US']) {
    const config = messages[locale].admin.settings.securityCenter
    assert.ok(config?.title)
    assert.equal(messages[locale].admin.settings.tabs.securityCenter, config.title)
    assert.deepEqual(Object.keys(config.sections).sort(), [...keys].sort())
    assert.deepEqual(Object.keys(config.hints).sort(), [...keys].sort())
    assert.ok(config.providerHint)
    const user = JSON.parse(fs.readFileSync(new URL(`../../user/src/i18n/locales/${locale}.json`, import.meta.url), 'utf8'))
    assert.ok(user.personalCenter.security.subtitleReadOnly)
  }
  assert.equal(messages['zh-CN'].admin.settings.securityCenter.title, '安全中心功能')
})

test('actual Settings parent routes global saves only when security child is ready and reports its saving state', async () => {
  const source = fs.readFileSync(new URL('../src/views/admin/Settings.vue', import.meta.url), 'utf8')
  const descriptor = parse(source, { filename: 'Settings.vue' }).descriptor
  const script = compileScript(descriptor, { id: 'settings-parent' })
  const saves: any[] = []
  const mocks: any = {
    vue, 'vue-i18n': { useI18n: () => ({ t: (key: string) => key }) },
    '@/api/admin': { adminAPI: { getSettings: async () => response({}), updateSettings: async (p: any) => saves.push(p) } },
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
  const vm = app.mount(host); await flush()
  const state = vm.$.setupState
  state.currentTab = 'security_center'
  assert.equal(state.currentOwnerReady, false)
  await state.saveSettings(); assert.deepEqual(saves, [])
  let calls = 0
  const gate = deferred()
  state.securityCenterTabRef = vue.reactive({ ready: true, submitting: false, save: async () => { calls++; state.securityCenterTabRef.submitting = true; state.securityCenterTabRef.ready = false; await gate.promise; state.securityCenterTabRef.submitting = false; state.securityCenterTabRef.ready = true } })
  assert.equal(state.currentOwnerReady, true)
  const pending = state.saveSettings()
  assert.equal(calls, 1)
  assert.equal(state.currentSaving, true)
  await state.saveSettings(); assert.equal(calls, 1)
  gate.resolve(true); await pending
  assert.equal(state.currentSaving, false)
  assert.equal(state.currentOwnerReady, true)
  assert.deepEqual(saves, [])
})

test('rendered switches default on and each click only changes its corresponding feature', async () => {
  const h = await mount()
  h.get.resolve(response({})); await flush()
  for (const key of keys) assert.equal(h.state.form[key], true)
  for (const key of keys) {
    host.querySelector<HTMLButtonElement>(`#security-center-${key}`)!.click(); await flush()
    assert.equal(h.state.form[key], false)
    for (const sibling of keys.filter(s => s !== key)) assert.equal(h.state.form[sibling], true)
    host.querySelector<HTMLButtonElement>(`#security-center-${key}`)!.click(); await flush()
    assert.equal(h.state.form[key], true)
  }
})
