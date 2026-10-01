import test, { afterEach } from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import { createRequire } from 'node:module'
import { resolve, dirname } from 'node:path'
import ts from 'typescript'
import { JSDOM, VirtualConsole } from 'jsdom'
import { parse, compileScript, compileTemplate } from 'vue/compiler-sfc'
const browserErrors: string[] = []
const consoleBoundary = new VirtualConsole().on('jsdomError', (e: Error) => browserErrors.push(e.message))
const dom = new JSDOM('<!doctype html><body></body>', { url: 'https://fixture.example.invalid', virtualConsole: consoleBoundary })
// No real transports or external SDK resources are enabled in this harness.
globalThis.fetch = async () => { throw new Error('Real fetch is forbidden') }
for (const key of ['window', 'document', 'Element', 'HTMLElement', 'SVGElement', 'Node']) Object.defineProperty(globalThis, key, { configurable: true, value: (dom.window as any)[key] })
const require = createRequire(import.meta.url)
const vue = require('vue')
const pinia = require('pinia')
const { evaluate, deferred } = await import('./helpers/sourceRuntime.ts')
const root = resolve(import.meta.dirname, '../src')
const flush = async () => { await new Promise(r => setImmediate(r)); await vue.nextTick() }
const wrap = (tag: string) => ({ setup: (_: any, { slots }: any) => () => vue.h(tag, null, slots.default?.()) })
const Input = { props: ['modelValue'], emits: ['update:modelValue'], setup: (p: any, { emit }: any) => () => vue.h('input', { value: p.modelValue, onInput: (e: any) => emit('update:modelValue', e.target.value) }) }
let app: any
let host: HTMLElement
let warnings: string[] = []
afterEach(() => { app?.unmount(); app = undefined; host?.remove(); pinia.setActivePinia(undefined); assert.deepEqual(warnings, []); warnings = [] })
function loadComponent(file: string, mocks: Record<string, any>, cache = new Map<string, any>()): any {
  file = resolve(root, file)
  if (cache.has(file)) return cache.get(file)
  const descriptor = parse(fs.readFileSync(file, 'utf8'), { filename: file }).descriptor
  const script = compileScript(descriptor, { id: 'security-restoration' })
  const template = compileTemplate({ source: descriptor.template!.content, filename: file, id: 'security-restoration', compilerOptions: { bindingMetadata: script.bindings } })
  assert.deepEqual(template.errors, [])
  const run = (code: string) => {
    const output = ts.transpileModule(code, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS, esModuleInterop: true } }).outputText
    const mod = { exports: {} as any }
    Function('require', 'module', 'exports', output)((name: string) => {
      if (name in mocks) return mocks[name].default ? { __esModule: true, ...mocks[name] } : mocks[name]
      if (name.endsWith('.vue')) return { __esModule: true, default: loadComponent(resolve(dirname(file), name), mocks, cache) }
      if (name.startsWith('.') || name.startsWith('@/')) throw new Error(`Unmocked boundary ${name}`)
      return require(name)
    }, mod, mod.exports)
    return mod.exports
  }
  const component = run(script.content).default
  component.render = run(template.code).render
  cache.set(file, component)
  return component
}
async function mount(options: { bindOnly?: boolean; enabled?: boolean; setPassword?: boolean; mode?: string; query?: any; configGate?: any; features?: any; unknownConfig?: boolean; actualTwoFactor?: boolean; qrGate?: any } = {}) {
  const calls: { name: string; payload: any; gate: any }[] = [], navigations: any[] = [], sdk: any[] = [], commits: any[] = []
  pinia.setActivePinia(pinia.createPinia())
  const values = new Map<string, string>([['user_token', 'fixture-session']])
  const realAuth = evaluate('src/stores/userAuth.ts', ['useUserAuthStore'], {
    defineStore: pinia.defineStore, useRouter: () => ({ push: (p: any) => navigations.push(p) }), userAuthAPI: {},
    readAuthStorage: (key: string) => values.get(key) || null,
    writeAuthStorage: (key: string, value: string) => { values.set(key, value); if (key === 'user_token') commits.push({ token: value }) },
    removeAuthStorage: (key: string) => values.delete(key),
  })
  const auth = options.actualTwoFactor ? realAuth.useUserAuthStore() : vue.reactive({ token: 'fixture-session', sessionGeneration: 0, syncUserProfile(data: any) { commits.push(data) }, logout(path: string) { navigations.push(path) } })
  const request = (name: string, payload?: any) => {
    const gate = deferred(); calls.push({ name, payload, gate }); return gate.promise
  }
  const network: any[] = []
  const redirects = await import('../src/utils/googleRedirect.ts')
  const methods: Record<string, string> = {
    'POST /me/email/change': 'changeEmail', 'POST /me/email/send-verify-code': 'sendChangeEmailCode', 'PUT /me/password': 'changePassword',
    'POST /me/telegram/bind': 'bindTelegram', 'POST /me/telegram/miniapp/bind': 'bindTelegramMiniApp', 'POST /me/google/bind': 'bindGoogle',
    'DELETE /me/telegram/unbind': 'unbindTelegram', 'DELETE /me/google/unbind': 'unbindGoogle',
    'GET /me/telegram/oidc/start': 'telegramOidcBindStart', 'POST /me/google/redirect/intent': 'googleRedirectBindIntent',
  }
  const transport = (method: string, url: string, payload?: any, opts?: any) => {
    network.push({ method, url, payload, opts })
    if (method === 'GET' && url === '/me/login-logs') return Promise.resolve(response([]))
    if (method === 'GET' && ['/me/telegram', '/me/google'].includes(url)) return Promise.resolve(response({ bound: false, can_unbind: false }))
    const name = methods[`${method} ${url}`]
    assert.ok(name, `Unexpected synthetic request ${method} ${url}`)
    return request(name, payload)
  }
  const userApi = Object.fromEntries(['get', 'put', 'post', 'delete'].map(method => [method, (url: string, p: any, opts: any) => transport(method.toUpperCase(), url, p, opts)]))
  const api = evaluate('src/api/user.ts', ['userProfileAPI'], { userApi, GOOGLE_REDIRECT_API_PATHS: redirects.GOOGLE_REDIRECT_API_PATHS }).userProfileAPI
  const store = evaluate('src/stores/userProfile.ts', ['useUserProfileStore'], { defineStore: pinia.defineStore, useUserAuthStore: () => auth, userProfileAPI: api, normalizePersonalCenterVisibility: () => ({}) })
  const profile = store.useUserProfileStore()
  profile.profile = { id: 1, email: 'ordinary@example.invalid', email_change_mode: options.bindOnly ? 'bind_only' : 'change_with_old_and_new', password_change_mode: options.setPassword ? 'set_without_old' : 'change_with_old' }
  const config = vue.reactive({ config: options.unknownConfig ? null : { ...(options.features !== undefined ? { security_center_config: options.features } : {}), telegram_auth: { enabled: !!options.enabled, bot_username: 'fixture_bot', mode: options.mode || 'widget' }, google_auth: { enabled: !!options.enabled, client_id: 'fixture-client' } }, locale: 'en', loadConfig: () => options.configGate?.promise || Promise.resolve() })
  const mini = vue.reactive({ isMiniApp: false, isReady: false, initData: '' })
  const route = vue.reactive({ path: '/me/security', query: options.query || {} })
  const identity = { detectGoogleIdentityUXMode: () => 'popup', initializeGoogleIdentity: async (opts: any) => { sdk.push(opts); return { cancel() {} } }, renderGoogleIdentityButton() {}, resolveGoogleButtonWidth: () => 300 }
  const mocks: any = {
    'vue-i18n': { useI18n: () => ({ t: (s: string) => s }) },
    'vue-router': { useRoute: () => route, useRouter: () => ({ replace: (p: any) => navigations.push(p) }) },
    '../../stores/userAuth': { useUserAuthStore: () => auth }, '../../stores/userProfile': store,
    '../../stores/app': { useAppStore: () => config }, '../../stores/telegramMiniApp': { useTelegramMiniAppStore: () => mini },
    '../../api/user': { userProfileAPI: api }, '../../utils/googleIdentity': identity,
    '../../utils/googleRedirect': { ...redirects, tryBuildGoogleRedirectCredentialCallbackURL: () => '' },
    '../../utils/securityCenterConfig': await import('../src/utils/securityCenterConfig.ts'),
    '../../utils/externalIdentity': await import('../src/utils/externalIdentity.ts'),
    '../../utils/telegramMiniApp': { buildTelegramMiniAppEntryLink: () => '', isTelegramUrlEnvironment: () => false, openTelegramCompatibleLink: (p: any) => navigations.push(p) },
    '../../utils/alerts': { pageAlertVariant: () => 'default', pageAlertToneClass: () => '' },
    '../../components/shared/PanelHeading.vue': { default: { props: ['title', 'description'], setup: (p: any) => () => vue.h('header', [p.title, p.description]) } },
    '@/components/ui/alert': { Alert: wrap('aside'), AlertDescription: wrap('p') }, '@/components/ui/badge': { Badge: wrap('span') },
    '@/components/ui/button': { Button: wrap('button') }, '@/components/ui/input': { Input }, '@/components/ui/label': { Label: wrap('label') },
    '@/components/ui/table': Object.fromEntries(['Table', 'TableBody', 'TableCell', 'TableHead', 'TableHeader', 'TableRow'].map(n => [n, wrap('div')])),

    '../../components/security/TwoFactorSection.vue': { default: { render: () => vue.h('section', 'two-factor-fixture') } },
  }
  if (options.actualTwoFactor) {
    delete mocks['../../components/security/TwoFactorSection.vue']
    mocks['../../api/auth'] = { userTotpAPI: {
      status: async () => response({ enabled: false }), setup: () => request('setup'),
      enable: (payload: any) => request('enable', payload),
    } }
    mocks.qrcode = { default: { toDataURL: () => options.qrGate?.promise || Promise.resolve('data:image/png;base64,fixture') } }
  }
  host = document.createElement('div'); document.body.append(host)
  app = vue.createApp(loadComponent('views/personal/SecurityPanel.vue', mocks))
  app.config.warnHandler = (m: string) => warnings.push(m)
  const vm = app.mount(host)
  await flush()
  const twoFactor = options.actualTwoFactor ? vm.$.subTree.children.find((v: any) => v.type?.__name === 'TwoFactorSection')?.component.setupState : undefined
  if (options.actualTwoFactor) assert.ok(twoFactor, 'actual compiled TwoFactorSection mounted')
  return { twoFactor, values, state: vm.$.setupState, auth, profile, calls, network, navigations, sdk, commits, config, mini, route, dispose: () => { app.unmount(); app = undefined }, request }
}
const response = (data: any = {}) => ({ data: { data } })
const button = (key: string) => Array.from(host.querySelectorAll('button')).find(b => b.textContent?.trim() === `personalCenter.security.${key}`)
const type = (input: HTMLInputElement, value: string) => { input.value = value; input.dispatchEvent(new window.Event('input', { bubbles: true })) }

test('SC-FE-01: actual parent source off blocks retained child enrollment before render', async () => {
  const h = await mount({ actualTwoFactor: true, features: {} })
  h.config.config.security_center_config.two_factor = false
  const pending = h.twoFactor.startSetup()
  h.calls.forEach(c => c.gate.resolve(response({ secret: 'obsolete', otpauth_url: 'otpauth://obsolete' })))
  await pending
  h.twoFactor.enableCode = '123456'
  await h.twoFactor.submitEnable()
  assert.equal(h.calls.length, 0)
})
for (const action of ['setup', 'enable', 'qr']) test(`SC-FE-01: actual parent same-task ABA fences ${action} before sensitive commits`, async () => {
  const qr = deferred()
  const h = await mount({ actualTwoFactor: true, features: {}, ...(action === 'qr' ? { qrGate: qr } : {}) })
  h.twoFactor.enableCode = '123456'
  const pending = action === 'enable' ? h.twoFactor.submitEnable() : h.twoFactor.startSetup()
  assert.equal(h.calls.length, 1)
  if (action === 'qr') {
    h.calls[0].gate.resolve(response({ secret: 'obsolete', otpauth_url: 'otpauth://obsolete' }))
    await flush()
  }
  h.config.config.security_center_config.two_factor = false
  h.config.config.security_center_config.two_factor = true
  h.calls[0].gate.resolve(response({ secret: 'obsolete', otpauth_url: 'otpauth://obsolete', token: 'obsolete-token', recovery_codes: ['obsolete'] }))
  qr.resolve('data:image/png;base64,obsolete')
  await pending; await flush()
  assert.equal(h.twoFactor.setupResult, null)
  assert.equal(h.twoFactor.qrcodeDataUrl, '')
  assert.deepEqual(h.twoFactor.recoveryCodes, [])
  assert.deepEqual(h.commits, [])
  assert.equal(h.auth.token, 'fixture-session')
  assert.equal(h.values.get('user_token'), 'fixture-session')
  assert.equal(h.twoFactor.loading, false)
})

test('SC-FE-01 control: current actual parent enrollment commits through real auth store', async () => {
  const h = await mount({ actualTwoFactor: true, features: {} })
  const setup = h.twoFactor.startSetup()
  h.calls[0].gate.resolve(response({ secret: 'current', otpauth_url: 'otpauth://current' }))
  await setup
  assert.equal(h.twoFactor.setupResult.secret, 'current')
  h.twoFactor.enableCode = '123456'
  const enable = h.twoFactor.submitEnable()
  h.calls[1].gate.resolve(response({ token: 'current-token', recovery_codes: ['current-code'] }))
  await enable
  assert.deepEqual(h.commits, [{ token: 'current-token' }])
  assert.equal(h.auth.token, 'current-token')
  assert.equal(h.values.get('user_token'), 'current-token')
  assert.deepEqual(h.twoFactor.recoveryCodes, ['current-code'])
})
test('SC-FE-01 control: source ABA releases only obsolete enrollment owner before render', async () => {
  const h = await mount({ actualTwoFactor: true, features: {} })
  const old = h.twoFactor.startSetup()
  h.config.config.security_center_config.two_factor = false
  assert.equal(h.twoFactor.loading, false)
  h.config.config.security_center_config.two_factor = true
  const current = h.twoFactor.startSetup()
  assert.equal(h.calls.length, 2)
  h.calls[0].gate.resolve(response({ secret: 'obsolete', otpauth_url: 'otpauth://obsolete' }))
  await old
  assert.equal(h.twoFactor.loading, true)
  assert.equal(h.twoFactor.setupResult, null)
  h.calls[1].gate.resolve(response({ secret: 'current', otpauth_url: 'otpauth://current' }))
  await current
  assert.equal(h.twoFactor.setupResult.secret, 'current')
  assert.equal(h.twoFactor.loading, false)
  assert.equal(h.profile.profile.email, 'ordinary@example.invalid')
})

test('original sections render in original order with ordinary dual-code email change and no top logout', async () => {
  const h = await mount({ enabled: true })
  const text = host.textContent!
  const keys = ['telegramTitle', 'googleTitle', 'currentEmailLabel', 'loginLogsTitle', 'passwordTitle']
  let last = -1
  for (const key of keys) { const index = text.indexOf(`personalCenter.security.${key}`); assert.ok(index > last, `missing/order ${key}`); last = index }
  assert.ok(text.indexOf('two-factor-fixture') > last)
  assert.ok(!text.includes('navbar.logout'))
  assert.ok(text.includes('oldCodeLabel'))
  const inputs = host.querySelector('form')!.querySelectorAll('input')
  assert.equal(inputs[0].value, 'ordinary@example.invalid')
  type(inputs[1], 'new@example.invalid'); type(inputs[2], 'old-code'); type(inputs[3], 'new-code')
  host.querySelector('form')!.dispatchEvent(new window.Event('submit', { bubbles: true, cancelable: true }))
  assert.deepEqual(h.calls.at(-1)?.payload, { new_email: 'new@example.invalid', old_code: 'old-code', new_code: 'new-code' })
  assert.equal(h.network.at(-1).method, 'POST')
  assert.equal(h.network.at(-1).url, '/me/email/change')
  h.calls.at(-1)!.gate.resolve(response({ id: 1, email: 'new@example.invalid', email_change_mode: 'change_with_old_and_new' })); await flush()
  assert.equal(h.profile.profile.email, 'new@example.invalid')
  assert.equal(h.commits.length, 1)
})

test('bind-only hides password/two-factor until verified email binding then offers initial password setup', async () => {
  const h = await mount({ bindOnly: true, setPassword: true })
  assert.ok(host.textContent!.includes('subtitleBindOnly'))
  assert.ok(!host.textContent!.includes('oldCodeLabel'))
  assert.ok(!host.textContent!.includes('two-factor-fixture'))
  assert.equal(host.querySelectorAll('form').length, 1)
  Object.assign(h.state.passwordForm, { newPassword: 'synthetic', confirmPassword: 'synthetic' })
  const blocked = h.state.handleChangePassword()
  h.calls.forEach(c => c.gate.resolve(response()))
  await blocked
  assert.equal(h.calls.length, 0)
  const inputs = host.querySelector('form')!.querySelectorAll('input')
  type(inputs[1], 'bound@example.invalid'); type(inputs[2], 'fixture-code')
  host.querySelector('form')!.dispatchEvent(new window.Event('submit', { bubbles: true, cancelable: true }))
  assert.deepEqual(h.calls.at(-1)?.payload, { new_email: 'bound@example.invalid', new_code: 'fixture-code' })
  h.calls.at(-1)!.gate.resolve(response({ id: 1, email: 'bound@example.invalid', email_change_mode: 'change_with_old_and_new', password_change_mode: 'set_without_old' })); await flush()
  assert.ok(host.textContent!.includes('setPasswordTitle'))
  assert.ok(host.textContent!.includes('two-factor-fixture'))
  assert.ok(!host.textContent!.includes('currentPasswordLabel'))
  Object.assign(h.state.passwordForm, { newPassword: 'synthetic-password', confirmPassword: 'synthetic-password' })
  const pending = h.state.handleChangePassword()
  assert.deepEqual(h.calls.at(-1)?.payload, { new_password: 'synthetic-password' })
  assert.equal(h.network.at(-1).method, 'PUT')
  assert.equal(h.network.at(-1).url, '/me/password')
  h.calls.at(-1)!.gate.resolve(response()); await pending
  assert.deepEqual(h.navigations, ['/auth/login?reason=password_changed'])
})
for (const leave of ['unmount', 'session', 'sessionABA']) {
  for (const action of ['handleChangeEmail', 'handleChangePassword', 'handleSendNewCode', 'handleGoogleBind']) {
    test(`${action}: ${leave} suppresses late commits, feedback, cooldown and navigation`, async () => {
      const h = await mount({ enabled: true })
      Object.assign(h.state.securityForm, { newEmail: 'new@example.invalid', oldCode: 'old', newCode: 'new' })
      Object.assign(h.state.passwordForm, { oldPassword: 'synthetic-old', newPassword: 'synthetic-new', confirmPassword: 'synthetic-new' })
      const pending = h.state[action]('synthetic-credential')
      assert.equal(h.calls.length, 1)
      if (leave === 'unmount') h.dispose()
      else { h.auth.sessionGeneration++; h.auth.token = 'other'; if (leave === 'sessionABA') { h.auth.sessionGeneration++; h.auth.token = 'fixture-session' } }
      h.state.securityAlert = { level: 'warning', message: 'replacement-state' }
      h.calls[0].gate.resolve(response({ id: 9, email: 'stale@example.invalid', bound: true })); await pending; await flush()
      assert.equal(h.commits.length, 0)
      assert.equal(h.profile.googleBinding?.bound, leave === 'unmount' ? false : undefined)
      assert.equal(h.state.securityAlert.message, 'replacement-state')
      assert.equal(h.state.newCodeCooldown, 0)
      assert.equal(h.navigations.length, 0)
    })
  }
}
for (const enabled of [false, true]) test(`third-party controls honor enabled=${enabled} without spinners`, async () => {
  const h = await mount({ enabled })
  assert.equal(h.sdk.length > 0, enabled)
  assert.equal(host.querySelectorAll('script[src^="https://telegram.org"]').length, enabled ? 1 : 0)
  assert.equal(!!host.querySelector('.social-google-button'), enabled)
  assert.equal(host.querySelectorAll('[class*="animate-spin"], [class*="loader"], [class*="spinner"]').length, 0)
  if (!enabled) {
    const blocked = h.state.handleGoogleBind('synthetic-credential')
    h.calls.forEach(c => c.gate.resolve(response()))
    await blocked
    const tg = h.state.handleTelegramBind({ id: 1, auth_date: 1, hash: 'synthetic-hash' })
    h.calls.forEach(c => c.gate.resolve(response()))
    await tg
    assert.equal(h.calls.length, 0)
  }
})
for (const leave of ['unmount', 'session', 'disable', 'providerABA']) test(`retained Telegram/Google SDK callbacks are revoked on ${leave}`, async () => {
  const h = await mount({ enabled: true })
  const telegram = (window as any).__dujiaoSecurityTelegramBind
  assert.equal(typeof telegram, 'function')
  const google = h.sdk.at(-1).onCredential
  const error = host.querySelector('script')!.onerror!
  if (leave === 'unmount') h.dispose()
  if (leave === 'session') h.auth.sessionGeneration++
  if (leave === 'disable') { h.config.config.telegram_auth.enabled = false; h.config.config.google_auth.enabled = false }
  if (leave === 'providerABA') { h.config.config.telegram_auth.bot_username = 'other_bot'; h.config.config.telegram_auth.bot_username = 'fixture_bot'; h.config.config.google_auth.client_id = 'other'; h.config.config.google_auth.client_id = 'fixture-client' }
  // Invoke before Vue's next render: detached handlers alone are not revocation.
  const late = telegram({ id: 1, auth_date: 1, hash: 'synthetic-hash' })
  google('synthetic-credential')
  h.calls.forEach(c => c.gate.resolve(response()))
  await late
  error.call(null, new window.Event('error'))
  assert.equal(h.calls.length, 0)
  assert.equal(h.state.securityAlert, null)
  await flush()
})
for (const action of ['startTelegramOidcBind', 'prepareGoogleRedirectBind']) {
  for (const leave of ['unmount', 'session']) test(`${action}: ${leave} prevents stale redirects/intents`, async () => {
    const h = await mount({ enabled: true, mode: 'oidc' })
    const writes: any[] = []
    const storage = { setItem: (...v: any[]) => writes.push(v), removeItem() {} }
    for (const target of [globalThis, window]) Object.defineProperty(target, 'sessionStorage', { configurable: true, value: storage })
    const pending = h.state[action]().catch(() => undefined)
    assert.equal(h.calls.length, 1)
    const before = writes.length
    if (leave === 'unmount') h.dispose(); else h.auth.sessionGeneration++
    h.calls[0].gate.resolve(response({ auth_url: 'https://identity.example.invalid', state: 'A'.repeat(43), issued_at: Date.now() })); await pending
    assert.equal(writes.length, before)
    assert.equal(window.location.href, 'https://fixture.example.invalid/')
    assert.deepEqual(browserErrors.splice(0), [], 'must not attempt jsdom navigation')
    assert.equal(h.navigations.length, 0)
  })
}
test('mount continuation cannot install Telegram callback after disposal during config load', async () => {
  const gate = deferred()
  const h = await mount({ enabled: true, configGate: gate })
  h.dispose(); gate.resolve(true); await flush()
  assert.equal((window as any).__dujiaoSecurityTelegramBind, undefined)
})

test('email response keeps a newer draft while still committing the authoritative account email', async () => {
  const h = await mount()
  Object.assign(h.state.securityForm, { newEmail: 'submitted@example.invalid', oldCode: 'old', newCode: 'new' })
  const pending = h.state.handleChangeEmail()
  h.state.securityForm.newEmail = 'next-draft@example.invalid'
  h.calls[0].gate.resolve(response({ id: 1, email: 'submitted@example.invalid', email_change_mode: 'change_with_old_and_new' })); await pending
  assert.equal(h.state.securityForm.newEmail, 'next-draft@example.invalid')
  assert.equal(h.profile.profile.email, 'submitted@example.invalid')
})
test('new-email code response cannot attach its cooldown to a replacement email draft', async () => {
  const h = await mount()
  h.state.securityForm.newEmail = 'old-draft@example.invalid'
  const pending = h.state.handleSendNewCode()
  h.state.securityForm.newEmail = 'new-draft@example.invalid'
  h.calls[0].gate.resolve(response()); await pending
  assert.equal(h.state.newCodeCooldown, 0)
  assert.equal(h.state.securityAlert, null)
})

test('ordinary password form submits the original old/new password payload then logs out once', async () => {
  const h = await mount()
  const form = host.querySelectorAll('form')[1]
  const inputs = form.querySelectorAll('input')
  type(inputs[0], 'synthetic-old'); type(inputs[1], 'synthetic-new'); type(inputs[2], 'synthetic-new')
  form.dispatchEvent(new window.Event('submit', { bubbles: true, cancelable: true }))
  assert.deepEqual(h.calls[0].payload, { old_password: 'synthetic-old', new_password: 'synthetic-new' })
  assert.equal(h.profile.changingPassword, true)
  assert.equal(form.querySelector('button')!.disabled, false) // Vue patches on the next tick.
  await flush(); assert.equal(form.querySelector('button')!.disabled, true)
  h.calls[0].gate.resolve(response()); await flush()
  assert.deepEqual(h.navigations, ['/auth/login?reason=password_changed'])
  assert.equal(h.profile.changingPassword, false)
})
test('rendered old/new email send controls use original endpoints and cooldown/duplicate guards', async () => {
  const h = await mount()
  button('sendOldCode')!.click()
  assert.deepEqual(h.calls[0].payload, { kind: 'old' })
  assert.equal(h.network.at(-1).url, '/me/email/send-verify-code')
  await flush(); assert.equal(button('sendOldCode')!.disabled, true)
  h.calls[0].gate.resolve(response()); await flush()
  assert.equal(h.state.oldCodeCooldown, 60)
  const count = h.calls.length
  await h.state.handleSendOldCode(); assert.equal(h.calls.length, count)
  const inputs = host.querySelector('form')!.querySelectorAll('input')
  type(inputs[1], 'new@example.invalid')
  button('sendNewCode')!.click()
  assert.deepEqual(h.calls.at(-1)!.payload, { kind: 'new', new_email: 'new@example.invalid' })
  h.calls.at(-1)!.gate.resolve(response()); await flush()
  assert.equal(h.state.newCodeCooldown, 60)
  h.dispose(); assert.equal(h.state.newCodeCooldown, 0); assert.equal(h.state.oldCodeCooldown, 0)
})
for (const provider of ['Google', 'Telegram']) test(`${provider}: unbinding requires explicit fresh backend capability`, async () => {
  const h = await mount({ enabled: true })
  const field = provider === 'Google' ? 'googleBinding' : 'telegramBinding'
  for (const capability of [undefined, false, 'true', 1]) {
    h.profile[field] = { bound: true, can_unbind: capability }
    await flush()
    assert.equal(button(`${provider.toLowerCase()}Unbind`)!.disabled, true)
    await h.state[`handleUnbind${provider}`]()
    assert.equal(h.calls.length, 0)
  }
  h.profile[field] = { bound: true, can_unbind: true }; await flush()
  const retained = button(`${provider.toLowerCase()}Unbind`)!
  assert.equal(retained.disabled, false)
  h.profile[field].can_unbind = false
  retained.click()
  assert.equal(h.calls.length, 0, 'rendered-enabled stale control must still recheck capability')
  h.profile[field].can_unbind = true; await flush()
  const pending = h.state[`handleUnbind${provider}`]()
  assert.equal(h.network.at(-1).method, 'DELETE')
  assert.equal(h.network.at(-1).url, `/me/${provider.toLowerCase()}/unbind`)
  h.calls[0].gate.resolve(response()); await pending
  assert.equal(h.profile[field].bound, false)
})
for (const provider of ['Google', 'Telegram']) test(`${provider}: current SDK credential still reaches the synthetic adapter`, async () => {
  const h = await mount({ enabled: true })
  const pending = provider === 'Google' ? h.sdk.at(-1).onCredential('synthetic-credential') : (window as any).__dujiaoSecurityTelegramBind({ id: 1, auth_date: 1, hash: 'synthetic-hash' })
  assert.equal(h.calls[0].name, `bind${provider}`)
  assert.equal(h.network.at(-1).method, 'POST')
  h.calls[0].gate.resolve(response({ bound: true })); await pending; await flush()
  assert.equal(h.state.securityAlert.level, 'success')
})
for (const leave of ['unmount', 'session']) test(`stale rejected email request after ${leave} cannot change feedback or owned busy state`, async () => {
  const h = await mount()
  Object.assign(h.state.securityForm, { newEmail: 'old@example.invalid', oldCode: 'old', newCode: 'new' })
  const pending = h.state.handleChangeEmail()
  if (leave === 'unmount') h.dispose(); else h.auth.sessionGeneration++
  h.state.securityAlert = { level: 'warning', message: 'replacement' }
  h.profile.securityError = 'replacement-error'
  h.profile.changingEmail = true
  h.calls[0].gate.reject(new Error('obsolete-error')); await pending
  assert.equal(h.state.securityAlert.message, 'replacement')
  assert.equal(h.profile.securityError, 'replacement-error')
  assert.equal(h.profile.changingEmail, true)
})
test('old email response cannot release a newer session request busy flag', async () => {
  const h = await mount()
  Object.assign(h.state.securityForm, { newEmail: 'old@example.invalid', oldCode: 'old', newCode: 'new' })
  const old = h.state.handleChangeEmail()
  h.auth.sessionGeneration++
  h.profile.profile = { id: 2, email_change_mode: 'change_with_old_and_new' }
  Object.assign(h.state.securityForm, { newEmail: 'fresh@example.invalid', oldCode: 'old', newCode: 'new' })
  const fresh = h.state.handleChangeEmail()
  h.calls[0].gate.resolve(response({ id: 1, email: 'obsolete@example.invalid' })); await old
  assert.equal(h.profile.changingEmail, true)
  assert.equal(h.commits.length, 0)
  h.calls[1].gate.resolve(response({ id: 2, email: 'fresh@example.invalid' })); await fresh
  assert.equal(h.profile.changingEmail, false)
  assert.equal(h.commits.length, 1)
})

for (const capability of ['missing', 'unknown']) test(`${capability} email/password modes fail closed in DOM and direct handlers`, async () => {
  const h = await mount()
  h.profile.profile.email_change_mode = capability === 'missing' ? undefined : 'future-mode'
  h.profile.profile.password_change_mode = capability === 'missing' ? undefined : 'future-mode'
  await flush()
  assert.equal(host.querySelectorAll('form').length, 0)
  assert.ok(!host.textContent!.includes('two-factor-fixture'))
  Object.assign(h.state.securityForm, { newEmail: 'new@example.invalid', oldCode: 'old', newCode: 'new' })
  Object.assign(h.state.passwordForm, { oldPassword: 'synthetic-old', newPassword: 'synthetic-new', confirmPassword: 'synthetic-new' })
  const requests = [h.state.handleChangeEmail(), h.state.handleChangePassword(), h.state.handleSendOldCode(), h.state.handleSendNewCode()]
  h.calls.forEach(c => c.gate.resolve(response())); await Promise.all(requests)
  assert.equal(h.calls.length, 0)
})
for (const change of ['providerABA', 'sessionABA', 'remount']) test(`Telegram parsed data-onauth expression cannot borrow replacement ${change} authority`, async () => {
  let h = await mount({ enabled: true })
  const expression = host.querySelector('script')!.getAttribute('data-onauth')!
  // Telegram's SDK evaluates this expression inside a function, resolving the
  // callback name at invocation, rather than retaining the original function.
  const callback = Function('window', 'expression', 'with(window){return eval("(function(user){" + expression + "})")}')(window, expression)
  if (change === 'providerABA') { h.config.config.telegram_auth.bot_username = 'other'; h.config.config.telegram_auth.bot_username = 'fixture_bot' }
  if (change === 'sessionABA') { h.auth.sessionGeneration++; h.auth.sessionGeneration++ }
  if (change === 'remount') { h.dispose(); h = await mount({ enabled: true }) }
  await flush()
  callback({ id: 1, auth_date: 1, hash: 'synthetic-hash' })
  const count = h.calls.length
  h.calls.forEach(c => c.gate.resolve(response({ bound: true }))); await flush()
  assert.equal(count, 0)
  assert.equal(typeof (window as any).__dujiaoSecurityTelegramBind, 'function')
})





const featureKeys = ['telegram_binding', 'google_binding', 'email_change', 'password_change', 'two_factor', 'login_history'] as const
for (let mask = 0; mask < 64; mask++) test(`security feature independence mask=${mask}`, async () => {
  const features = Object.fromEntries(featureKeys.map((key, i) => [key, !!(mask & (1 << i))]))
  const h = await mount({ enabled: true, features })
  const text = host.textContent!
  for (const [key, label] of [['telegram_binding', 'telegramTitle'], ['google_binding', 'googleTitle'], ['email_change', 'newEmailLabel'], ['password_change', 'passwordTitle'], ['login_history', 'loginLogsTitle']]) {
    assert.equal(text.includes(`personalCenter.security.${label}`), features[key], key)
  }
  assert.equal(h.state.canSetupTwoFactor, features.two_factor)
  assert.equal(h.network.some(r => r.url === '/me/login-logs'), features.login_history)
  Object.assign(h.state.securityForm, { newEmail: 'new@example.invalid', oldCode: 'old', newCode: 'new' })
  Object.assign(h.state.passwordForm, { oldPassword: 'old', newPassword: 'new', confirmPassword: 'new' })
  for (const [key, handler, arg] of [['email_change', 'handleChangeEmail'], ['email_change', 'handleSendNewCode'], ['email_change', 'handleSendOldCode'], ['password_change', 'handleChangePassword'], ['google_binding', 'handleGoogleBind', 'synthetic'], ['telegram_binding', 'handleTelegramBind', { id: 1, auth_date: 1, hash: 'synthetic' }]]) {
    Object.assign(h.state.securityForm, { newEmail: 'new@example.invalid', oldCode: 'old', newCode: 'new' })
    Object.assign(h.state.passwordForm, { oldPassword: 'old', newPassword: 'new', confirmPassword: 'new' })
    const count = h.calls.length
    const pending = h.state[handler as string](arg)
    assert.equal(h.calls.length, count + (features[key as string] ? 1 : 0), `independent direct handler ${key}/${handler}`)
    if (features[key as string]) h.calls.at(-1)!.gate.resolve(response(handler === 'handleChangeEmail'
      ? { ...h.profile.profile, email: 'new@example.invalid' } : { bound: true }))
    await pending
    assert.equal(h.calls.length, count + (features[key as string] ? 1 : 0), `direct bypass ${key}`)
  }
})
test('unknown config fails closed and resolved legacy config restores features', async () => {
  const h = await mount({ unknownConfig: true })
  assert.equal(host.querySelectorAll('form').length, 0)
  assert.equal(h.network.some(r => r.url === '/me/login-logs'), false)
  h.config.config = {}; await flush()
  assert.equal(host.querySelectorAll('form').length, 2)
  assert.equal(h.state.canSetupTwoFactor, true)
})
test('disabled providers remain visible; feature off preserves identity records', async () => {
  const h = await mount()
  assert.ok(host.textContent!.includes('telegramTitle'))
  assert.ok(host.textContent!.includes('googleTitle'))
  h.profile.googleBinding = { bound: true, can_unbind: true }; await flush()
  assert.ok(host.textContent!.includes('googleTitle'))
  h.config.config.security_center_config = { google_binding: false }; await flush()
  assert.ok(!host.textContent!.includes('googleTitle'))
  await h.state.handleUnbindGoogle()
  assert.equal(h.calls.length, 0)
  assert.equal(h.profile.googleBinding.bound, true)
})
for (const provider of ['telegram', 'google']) test(`${provider} provider region follows only its independent security switch when provider is disabled`, async () => {
  const h = await mount({ enabled: false, features: { telegram_binding: true, google_binding: true } })
  const sibling = provider === 'telegram' ? 'google' : 'telegram'
  assert.equal(h.profile.profile.email_change_mode, 'change_with_old_and_new')
  assert.equal(h.profile.profile.password_change_mode, 'change_with_old')
  assert.equal(h.profile[`${provider}Binding`].bound, false)
  const visible = (name: string) => host.textContent!.includes(`personalCenter.security.${name}Title`)
  assert.equal(visible(provider), true)
  assert.ok(host.textContent!.includes(`personalCenter.security.${provider}DisabledTip`))
  assert.deepEqual(h.sdk, [])
  assert.equal(host.querySelectorAll('script[src^="https://telegram.org"], .social-google-button').length, 0)
  for (const key of ['telegramMiniAppBindAction', 'telegramOidcBindButton', 'telegramMiniAppEntryAction']) assert.equal(button(key), undefined)
  h.config.config.security_center_config[`${provider}_binding`] = false; await flush()
  assert.equal(visible(provider), false)
  assert.equal(visible(sibling), true)
  h.config.config.security_center_config[`${provider}_binding`] = true; await flush()
  assert.equal(visible(provider), true)
  assert.equal(visible(sibling), true)
  assert.ok(host.textContent!.includes(`personalCenter.security.${provider}DisabledTip`))
  assert.deepEqual(h.sdk, [])
  assert.equal(host.querySelectorAll('script[src^="https://telegram.org"], .social-google-button').length, 0)
})
test('visible disabled provider regions reject every direct bind entry without transport or state writes', async () => {
  const h = await mount({ enabled: false, features: { telegram_binding: true, google_binding: true } })
  const writes: any[] = []
  const storage = { setItem: (...args: any[]) => writes.push(args), removeItem: (...args: any[]) => writes.push(args) }
  for (const target of [globalThis, window]) Object.defineProperty(target, 'sessionStorage', { configurable: true, value: storage })
  h.state.securityAlert = { level: 'warning', message: 'retained-feedback' }
  h.profile.securityError = 'retained-error'
  const snapshot = () => JSON.stringify({ profile: h.profile.$state, alert: h.state.securityAlert, token: h.auth.token, values: [...h.values] })
  const before = snapshot(), networkBefore = h.network.length
  await h.state.handleTelegramBind({ id: 1, auth_date: 1, hash: 'synthetic' })
  await h.state.handleGoogleBind('synthetic')
  await h.state.startTelegramOidcBind()
  h.mini.isMiniApp = true; h.mini.isReady = true; h.mini.initData = 'synthetic'
  await h.state.handleTelegramMiniAppBind()
  h.state.openTelegramMiniAppEntry()
  await assert.rejects(h.state.prepareGoogleRedirectBind(), /unavailable/)
  assert.equal(h.calls.length, 0)
  assert.equal(h.network.length, networkBefore)
  assert.equal(snapshot(), before)
  assert.deepEqual(h.commits, [])
  assert.deepEqual(h.navigations, [])
  assert.deepEqual(writes, [])
  assert.deepEqual(browserErrors.splice(0), [])
})
for (const provider of ['Google', 'Telegram']) test(`${provider} bound disabled provider retains explicit canUnbind and session fencing`, async () => {
  const h = await mount({ enabled: false, features: { telegram_binding: true, google_binding: true } })
  const field = `${provider.toLowerCase()}Binding`
  h.profile[field] = { bound: true, can_unbind: false }; await flush()
  assert.ok(host.textContent!.includes(`personalCenter.security.${provider.toLowerCase()}Title`))
  assert.equal(button(`${provider.toLowerCase()}Unbind`)!.disabled, true)
  await h.state[`handleUnbind${provider}`]()
  assert.equal(h.calls.length, 0)
  assert.equal(h.profile[field].bound, true)
  h.profile[field].can_unbind = true; await flush()
  assert.equal(button(`${provider.toLowerCase()}Unbind`)!.disabled, false)
  const pending = h.state[`handleUnbind${provider}`]()
  assert.equal(h.network.at(-1).method, 'DELETE')
  assert.equal(h.network.at(-1).url, `/me/${provider.toLowerCase()}/unbind`)
  h.auth.sessionGeneration++
  h.profile[field] = { bound: true, can_unbind: true, provider_user_id: 'replacement' }
  h.state.securityAlert = { level: 'warning', message: 'replacement' }
  h.calls[0].gate.resolve(response()); await pending
  assert.equal(h.profile[field].provider_user_id, 'replacement')
  assert.equal(h.profile[field].bound, true)
  assert.equal(h.state.securityAlert.message, 'replacement')
  assert.deepEqual(h.commits, [])
})
for (const provider of ['Google', 'Telegram']) for (const aba of [false, true]) test(`${provider} feature revokes in-flight response and retained SDK ABA=${aba}`, async () => {
  const h = await mount({ enabled: true, features: {} })
  const key = `${provider.toLowerCase()}_binding`
  const retained = provider === 'Google' ? h.sdk.at(-1).onCredential : (window as any).__dujiaoSecurityTelegramBind
  const payload = provider === 'Google' ? 'synthetic' : { id: 1, auth_date: 1, hash: 'synthetic' }
  const pending = h.state[`handle${provider}Bind`](payload)
  h.config.config.security_center_config[key] = false
  if (aba) h.config.config.security_center_config[key] = true
  retained(payload)
  assert.equal(h.calls.length, 1)
  h.calls[0].gate.resolve(response({ bound: true })); await pending; await flush()
  assert.equal(h.profile[`${provider.toLowerCase()}Binding`].bound, false)
  assert.equal(h.state.securityAlert, null)
})
test('email off uses quiet readonly current email and neutral heading; password off does not remove 2FA', async () => {
  const h = await mount({ features: { email_change: false, password_change: false } })
  assert.ok(host.textContent!.includes('ordinary@example.invalid'))
  assert.ok(!host.textContent!.includes('subtitleBindOnly'))
  assert.equal(host.querySelector('header')!.textContent, 'personalCenter.security.titlepersonalCenter.security.subtitleReadOnly')
  assert.ok(host.textContent!.includes('subtitleReadOnly'))
  assert.ok(host.textContent!.includes('two-factor-fixture'))
})

for (const provider of ['Google', 'Telegram']) test(`${provider} independent positive binding does not report disabled sibling refresh failure`, async () => {
  const h = await mount({ enabled: true, features: { [provider === 'Google' ? 'telegram_binding' : 'google_binding']: false } })
  const pending = h.state[`handle${provider}Bind`](provider === 'Google' ? 'synthetic' : { id: 1, auth_date: 1, hash: 'synthetic' })
  assert.equal(h.calls[0].name, `bind${provider}`)
  h.calls[0].gate.resolve(response({ bound: true })); await pending
  assert.equal(h.state.securityAlert.level, 'success')
})
for (const mode of ['', 'unknown']) test(`Telegram unsupported mode ${JSON.stringify(mode)} fails closed for SDK and direct miniapp bind`, async () => {
  const h = await mount({ enabled: true })
  h.config.config.telegram_auth.mode = mode
  h.mini.isMiniApp = true; h.mini.isReady = true; h.mini.initData = 'synthetic'
  await flush()
  const pending = h.state.handleTelegramMiniAppBind()
  const count = h.calls.length
  h.calls.forEach(c => c.gate.resolve(response()))
  await pending
  assert.equal(count, 0)
  assert.equal(host.querySelectorAll('script[src^="https://telegram.org"]').length, 0)
})
test('shared classic and vault personal centers delegate to the same feature-gated SecurityPanel', () => {
  for (const path of ['views/PersonalCenter.vue', 'templates/vault/PersonalCenter.vue']) {
    const source = fs.readFileSync(resolve(root, path), 'utf8')
    assert.match(source, /import SecurityPanel from .*personal\/SecurityPanel\.vue/)
    assert.match(source, /<SecurityPanel/)
  }
})

test('all disabled binding entry points reject direct bypass without resetting bound account records', async () => {
  const h = await mount({ enabled: true, features: Object.fromEntries(featureKeys.map(key => [key, false])) })
  h.profile.telegramBinding = { bound: true, can_unbind: true }
  h.profile.googleBinding = { bound: true, can_unbind: true }
  h.mini.isMiniApp = true; h.mini.isReady = true; h.mini.initData = 'synthetic'
  await Promise.all([h.state.handleUnbindTelegram(), h.state.handleUnbindGoogle(), h.state.handleTelegramMiniAppBind(), h.state.startTelegramOidcBind()])
  await assert.rejects(h.state.prepareGoogleRedirectBind(), /unavailable/)
  assert.equal(h.calls.length, 0)
  assert.equal(h.profile.telegramBinding.bound, true)
  assert.equal(h.profile.googleBinding.bound, true)
})
for (const provider of ['Google', 'Telegram']) test(`${provider} feature off during unbind preserves current identity record`, async () => {
  const h = await mount({ enabled: true, features: {} })
  const field = `${provider.toLowerCase()}Binding`
  h.profile[field] = { bound: true, can_unbind: true }
  const pending = h.state[`handleUnbind${provider}`]()
  assert.equal(h.calls.length, 1)
  h.config.config.security_center_config[`${provider.toLowerCase()}_binding`] = false
  h.calls[0].gate.resolve(response()); await pending
  assert.equal(h.profile[field].bound, true)
  assert.equal(h.state.securityAlert, null)
})
for (const [feature, action] of [['email_change', 'handleChangeEmail'], ['password_change', 'handleChangePassword'], ['email_change', 'handleSendNewCode']]) test(`${action} feature ABA fences stale commit, cooldown and navigation`, async () => {
  const h = await mount({ features: {} })
  Object.assign(h.state.securityForm, { newEmail: 'submitted@example.invalid', oldCode: 'old', newCode: 'new' })
  Object.assign(h.state.passwordForm, { oldPassword: 'old', newPassword: 'new', confirmPassword: 'new' })
  const pending = h.state[action]()
  assert.equal(h.calls.length, 1)
  h.config.config.security_center_config[feature] = false; h.config.config.security_center_config[feature] = true
  h.calls[0].gate.resolve(response({ id: 1, email: 'obsolete@example.invalid' })); await pending
  assert.equal(h.commits.length, 0)
  assert.equal(h.profile.profile.email, 'ordinary@example.invalid')
  assert.equal(h.state.securityAlert, null)
  assert.equal(h.state.newCodeCooldown, 0)
  assert.deepEqual(h.navigations, [])
})
for (const action of ['startTelegramOidcBind', 'prepareGoogleRedirectBind']) test(`${action} feature ABA prevents stale redirect state`, async () => {
  const h = await mount({ enabled: true, mode: 'oidc', features: {} })
  const writes: any[] = []
  const storage = { setItem: (...v: any[]) => writes.push(v), removeItem() {} }
  for (const target of [globalThis, window]) Object.defineProperty(target, 'sessionStorage', { configurable: true, value: storage })
  const pending = h.state[action]().catch(() => undefined)
  assert.equal(h.calls.length, 1)
  const key = action === 'startTelegramOidcBind' ? 'telegram_binding' : 'google_binding'
  h.config.config.security_center_config[key] = false; h.config.config.security_center_config[key] = true
  h.calls[0].gate.resolve(response({ auth_url: 'https://identity.example.invalid', state: 'A'.repeat(43), issued_at: Date.now() })); await pending
  assert.deepEqual(writes, [])
  assert.deepEqual(browserErrors.splice(0), [])
})
test('new enrollment is available independently of the password-change switch', async () => {
  const h = await mount({ features: { password_change: false, two_factor: true } })
  assert.equal(h.state.canManagePassword, false)
  assert.equal(h.state.canSetupTwoFactor, true)
  assert.ok(host.textContent!.includes('two-factor-fixture'))
})
test('resolved malformed security fields are closed while missing legacy fields default on', async () => {
  const policy = await import('../src/utils/securityCenterConfig.ts')
  for (const config of [null, undefined, false, [], { security_center_config: null }, { security_center_config: 'true' }]) assert.deepEqual(Object.values(policy.normalizeSecurityCenterConfig(config)), featureKeys.map(() => false))
  for (const raw of [undefined, {}]) assert.deepEqual(Object.values(policy.normalizeSecurityCenterConfig({ ...(raw === undefined ? {} : { security_center_config: raw }) })), featureKeys.map(() => true))
  for (const key of featureKeys) {
    const normalized = policy.normalizeSecurityCenterConfig({ security_center_config: { [key]: 'true' } })
    assert.equal(normalized[key], false)
    for (const sibling of featureKeys.filter(s => s !== key)) assert.equal(normalized[sibling], true)
  }
})

test('login-history disable hides and never fetches or deletes previously loaded records', async () => {
  const h = await mount({ features: { login_history: false } })
  h.profile.recentLoginLogs = [{ id: 123, ip: 'fixture-ip', created_at: 'fixture-date' }]
  const before = h.network.length
  await h.state.loadLoginHistory()
  assert.equal(h.network.length, before)
  assert.equal(h.profile.recentLoginLogs.length, 1)
  assert.ok(!host.textContent!.includes('loginLogsTitle'))
  h.config.config.security_center_config.login_history = true; await flush()
  assert.equal(h.network.filter(r => r.url === '/me/login-logs').length, 1)
  h.profile.recentLoginLogs = [{ id: 123, ip: 'fixture-ip', created_at: 'fixture-date' }]
  h.config.config.security_center_config.login_history = false; await flush()
  assert.equal(h.profile.recentLoginLogs.length, 1)
  assert.ok(!host.textContent!.includes('loginLogsTitle'))
})
test('Telegram miniapp binding uses the existing payload when its independent feature is on', async () => {
  const h = await mount({ enabled: true, features: { google_binding: false, email_change: false, password_change: false, two_factor: false, login_history: false } })
  h.mini.isMiniApp = true; h.mini.isReady = true; h.mini.initData = 'synthetic-init-data'
  const pending = h.state.handleTelegramMiniAppBind()
  assert.equal(h.calls[0].name, 'bindTelegramMiniApp')
  assert.deepEqual(h.calls[0].payload, { init_data: 'synthetic-init-data' })
  h.calls[0].gate.resolve(response({ bound: true })); await pending
  assert.equal(h.state.securityAlert.level, 'success')
})
