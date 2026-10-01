import test, { afterEach } from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import ts from 'typescript'
import { JSDOM } from 'jsdom'
import { parse, compileScript, compileTemplate } from 'vue/compiler-sfc'

const dom = new JSDOM('<!doctype html><html><body></body></html>', { url: 'https://store.example.test/', pretendToBeVisual: true })
for (const key of ['window', 'document', 'Element', 'HTMLElement', 'SVGElement', 'Node', 'localStorage', 'requestAnimationFrame', 'cancelAnimationFrame', 'getComputedStyle']) {
  Object.defineProperty(globalThis, key, { configurable: true, value: (dom.window as any)[key] })
}
const vue = await import('vue')
const i18n = await import('vue-i18n')
const messages = Object.fromEntries(['zh-CN', 'zh-TW', 'en-US'].map(locale => [locale,
  JSON.parse(fs.readFileSync(new URL(`../src/i18n/locales/${locale}.json`, import.meta.url), 'utf8')),
]))
const filename = 'ApiPanel.vue'
const source = fs.readFileSync(new URL('../src/views/personal/ApiPanel.vue', import.meta.url), 'utf8')
const { descriptor } = parse(source, { filename })
const script = compileScript(descriptor, { id: 'auth03' })
const template = compileTemplate({ source: descriptor.template!.content, filename, id: 'auth03', compilerOptions: { bindingMetadata: script.bindings } })
assert.deepEqual(template.errors, [])
function evaluate(code: string, imports: Record<string, any>) {
  const output = ts.transpileModule(code, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS } }).outputText
  const module = { exports: {} as any }
  Function('require', 'module', 'exports', output)((name: string) => {
    assert.ok(name in imports, `unmocked import: ${name}`)
    return imports[name]
  }, module, module.exports)
  return module.exports
}
const overlayLifecycle = evaluate(fs.readFileSync(new URL('../src/composables/overlayMotionLifecycle.ts', import.meta.url), 'utf8'), { vue })
const wrap = (tag: string) => ({ setup: (_props: any, { slots }: any) => () => vue.h(tag, null, slots.default?.()) })
const icon = wrap('span')
let app: ReturnType<typeof vue.createApp> | undefined
let container: HTMLDivElement
const flush = async () => { await new Promise(resolve => setTimeout(resolve, 60)); await vue.nextTick() }
const button = (key: string) => Array.from(document.querySelectorAll('button')).find(b => b.textContent?.trim() === messages['zh-CN'].personalCenter.apiPanel[key])
async function mount(status: string, active = false, locale = 'zh-CN') {
  localStorage.clear()
  let record = { id: 31, api_key: 'fixture-api-key', api_secret_tail: '1234', status, is_active: active }
  const calls: any[] = []
  const apiCredentialAPI = {
    getMy: async () => ({ data: { data: { ...record } } }),
    apply: async () => { calls.push(['apply']); return { data: {} } },
    regenerate: async () => { calls.push(['regenerate']); return { data: { data: { api_secret: 'fixture-new-secret' } } } },
    updateStatus: async (body: any) => { calls.push(['updateStatus', body]); record = { ...record, ...body }; return { data: {} } },
  }
  // Compile the real SFC script and render function; replace only API and visual primitives.
  const component = evaluate(script.content, {
    vue, 'vue-i18n': i18n, '../../api': { apiCredentialAPI },
    '../../composables/overlayMotionLifecycle': overlayLifecycle,
    '../../utils/alerts': { pageAlertVariant: () => 'default', pageAlertToneClass: () => '' },
    'lucide-vue-next': { AlertTriangle: icon, XCircle: icon, Info: icon, Check: icon, Key: icon },
    '../../components/shared/PanelHeading.vue': { default: wrap('header') },
    '@/components/ui/alert': { Alert: wrap('aside'), AlertDescription: wrap('p') },
    '@/components/ui/badge': { Badge: wrap('span') },
    '@/components/ui/button': { Button: wrap('button') },
  }).default
  component.render = evaluate(template.code, { vue }).render
  container = document.createElement('div')
  document.body.append(container)
  app = vue.createApp(component)
  app.use(i18n.createI18n({ legacy: false, locale, messages }))
  const vm = app.mount(container)
  await flush()
  return { state: (vm as any).$.setupState, calls, setRecord: (patch: any) => { record = { ...record, ...patch } } }
}
afterEach(() => { app?.unmount(); document.body.innerHTML = '' })

for (const locale of ['zh-TW', 'en-US']) {
  test(`AUTH-03: suspension and unavailable messages are localized in ${locale}`, async () => {
    const { state, setRecord } = await mount('disabled', false, locale)
    const panel = messages[locale].personalCenter.apiPanel
    for (const key of ['suspendedTitle', 'suspendedDesc', 'unavailableTitle', 'unavailableDesc']) {
      assert.equal(typeof panel[key], 'string')
      assert.ok(panel[key].length > 0)
    }
    assert.ok(container.textContent?.includes(panel.suspendedTitle))
    assert.ok(container.textContent?.includes(panel.suspendedDesc))
    setRecord({ status: 'future-status' })
    await state.loadCredential(); await flush()
    assert.ok(container.textContent?.includes(panel.unavailableTitle))
    assert.ok(container.textContent?.includes(panel.unavailableDesc))
  })
}

test('AUTH-03: refreshed administrator suspension removes previously displayed secrets and confirmation', async () => {
  const { state, calls, setRecord } = await mount('approved', true)
  button('generateSecret')!.click(); await flush()
  button('regenerate')!.click(); await flush()
  assert.ok(button('regenerateConfirm'))
  setRecord({ status: 'disabled', is_active: false })
  await state.loadCredential(); await flush()
  assert.ok(container.textContent?.includes('API 已被管理员停用'))
  assert.ok(!container.textContent?.includes('fixture-new-secret'))
  assert.equal(button('regenerateConfirm'), undefined)
  assert.equal(container.querySelector('[role="switch"]'), null)
  assert.deepEqual(calls, [['regenerate']])
})

for (const status of ['none', 'pending_review', 'rejected']) {
  test(`AUTH-03: ordinary ${status} application behavior is unchanged`, async () => {
    const { calls } = await mount(status)
    assert.equal(container.querySelector('[role="switch"]'), null)
    assert.equal(button('regenerate'), undefined)
    if (status === 'pending_review') {
      assert.ok(container.textContent?.includes(messages['zh-CN'].personalCenter.apiPanel.pendingTitle))
      assert.equal(button('apply'), undefined)
      assert.equal(button('reapply'), undefined)
      assert.deepEqual(calls, [])
    } else {
      button(status === 'none' ? 'apply' : 'reapply')!.click(); await flush()
      assert.deepEqual(calls, [['apply']])
    }
  })
}

test('AUTH-03: admin-disabled credential renders suspension, not approval or owner recovery controls', async () => {
  const { calls } = await mount('disabled')
  assert.ok(container.textContent?.includes('API 已被管理员停用'), 'must explain administrator suspension')
  assert.ok(container.textContent?.includes('请联系管理员恢复'))
  assert.ok(!container.textContent?.includes(messages['zh-CN'].personalCenter.apiPanel.approvedNoticeTitle))
  assert.equal(container.querySelector('[role="switch"]'), null)
  for (const key of ['generateSecret', 'regenerate', 'apply', 'reapply']) assert.equal(button(key), undefined)
  assert.deepEqual(calls, [])
})

test('AUTH-03: unknown credential status fails closed', async () => {
  await mount('future-status', true)
  assert.ok(container.textContent?.includes('API 凭证暂不可用'))
  assert.equal(container.querySelector('[role="switch"]'), null)
  assert.equal(button('generateSecret'), undefined)
  assert.equal(button('regenerate'), undefined)
})

test('AUTH-03: owner-paused approved credentials retain enable, pause and confirmed rotation', async () => {
  const { calls } = await mount('approved')
  const toggle = container.querySelector<HTMLButtonElement>('[role="switch"]')!
  assert.equal(toggle.getAttribute('aria-checked'), 'false')
  toggle.click(); await flush()
  assert.equal(toggle.getAttribute('aria-checked'), 'true')
  toggle.click(); await flush()
  assert.equal(toggle.getAttribute('aria-checked'), 'false')
  button('regenerate')!.click(); await flush()
  assert.ok(button('regenerateConfirm'))
  assert.deepEqual(calls, [['updateStatus', { is_active: true }], ['updateStatus', { is_active: false }]])
  button('regenerateConfirm')!.click(); await flush()
  assert.deepEqual(calls[2], ['regenerate'])
  assert.ok(container.textContent?.includes('fixture-api-key'))
  assert.ok(container.textContent?.includes('fixture-new-secret'))
})

test('AUTH-03: approved credential still permits first secret generation', async () => {
  const { calls } = await mount('approved', true)
  button('generateSecret')!.click(); await flush()
  assert.deepEqual(calls, [['regenerate']])
  assert.ok(container.textContent?.includes('fixture-new-secret'))
})

for (const status of ['disabled', 'future-status', 'pending_review', 'rejected']) {
  test(`AUTH-03: action handlers guard non-approved ${status} even before the next render`, async () => {
    const { state, calls } = await mount('approved')
    button('regenerate')!.click(); await flush()
    assert.ok(button('regenerateConfirm'))
    state.credential.status = status
    // Exercise the already-rendered confirmation before Vue patches the DOM.
    button('regenerateConfirm')!.click()
    await flush()
    await state.handleFirstGenerate()
    state.handleRegenerate()
    await state.handleToggleStatus()
    await flush()
    assert.deepEqual(calls, [], 'non-approved credentials must not send owner mutation requests')
    assert.equal(button('regenerateConfirm'), undefined)
  })
}
