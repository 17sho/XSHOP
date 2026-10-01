import test, { afterEach } from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import ts from 'typescript'
import { JSDOM } from 'jsdom'
import { parse, compileScript, compileTemplate } from 'vue/compiler-sfc'

// runtime-dom must see a document before Vue (including Pinia/helpers) imports.
const dom = new JSDOM('<!doctype html><html><body></body></html>', { url: 'https://store.example.test/' })
for (const key of ['window', 'document', 'Element', 'HTMLElement', 'SVGElement', 'Node']) {
  Object.defineProperty(globalThis, key, { configurable: true, value: (dom.window as any)[key] })
}
const vue = await import('vue')
const pinia = await import('pinia')
const browserStorage = await import('../src/utils/browserStorage.ts')
const { evaluate, deferred } = await import('./helpers/sourceRuntime.ts')
const filename = 'TwoFactorSection.vue'
const source = fs.readFileSync(new URL('../src/components/security/TwoFactorSection.vue', import.meta.url), 'utf8')
const { descriptor } = parse(source, { filename })
const script = compileScript(descriptor, { id: 'fe05-rotation' })
const template = compileTemplate({ source: descriptor.template!.content, filename, id: 'fe05-rotation', compilerOptions: { bindingMetadata: script.bindings } })
assert.deepEqual(template.errors, [])
function moduleExports(code: string, imports: Record<string, any>) {
  const output = ts.transpileModule(code, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS } }).outputText
  const module = { exports: {} as any }
  Function('require', 'module', 'exports', output)((name: string) => {
    assert.ok(name in imports, `unexpected import: ${name}`)
    return imports[name]
  }, module, module.exports)
  return module.exports
}
const wrap = (tag: string) => ({ setup: (_props: any, { slots }: any) => () => vue.h(tag, null, slots.default?.()) })
const Input = {
  props: ['modelValue'], emits: ['update:modelValue'],
  setup: (props: any, { emit }: any) => () => vue.h('input', {
    value: props.modelValue, onInput: (event: Event) => emit('update:modelValue', (event.target as HTMLInputElement).value),
  }),
}
let app: ReturnType<typeof vue.createApp> | undefined
const flush = async () => { await new Promise(resolve => setImmediate(resolve)); await vue.nextTick() }
const button = (key: string) => Array.from(document.querySelectorAll('button')).find(b => b.textContent?.trim() === `personalCenter.security.twofa.${key}`)
afterEach(() => { app?.unmount(); app = undefined; document.body.innerHTML = ''; pinia.setActivePinia(undefined) })

async function mount(storageMode: 'normal' | 'denied' | 'recovered', persisted: boolean, allowSetup = true, setupGate?: any, initiallyEnabled = false) {
  const values = new Map<string, string>(persisted ? [['user_token', 'fixture-persisted'], ['user_profile', JSON.stringify({ id: 1 })]] : [])
  let writable = storageMode === 'normal'
  const denied = () => { throw new DOMException('Denied', 'SecurityError') }
  const storage = {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => writable ? values.set(key, value) : denied(),
    removeItem: (key: string) => writable ? values.delete(key) : denied(),
  }
  for (const target of [globalThis, window]) Object.defineProperty(target, 'localStorage', { configurable: true, get: () => storage })
  const boundary = evaluate('src/utils/authStorage.ts', ['readAuthStorage', 'writeAuthStorage', 'removeAuthStorage'], browserStorage)
  const requests: { url: string; method: string; authorization?: string; body?: any }[] = []
  const enable = deferred()
  const disable = deferred()
  const regenerate = deferred()
  let enabled = initiallyEnabled
  const { userApi, api } = evaluate('src/api/client.ts', ['userApi', 'api'], {
    ...boundary, i18n: { global: { t: (s: string) => s, locale: 'en-US' } },
    isPublicAuthEndpoint: (path: string) => path.startsWith('/auth/'),
    fetch: async (url: string, opts: any) => {
      requests.push({ url, method: opts.method, authorization: opts.headers.Authorization, body: opts.body && JSON.parse(opts.body) })
      let data: any
      if (url.endsWith('/auth/login')) data = { token: 'fixture-before-2fa', user: { id: 7 } }
      else if (url.endsWith('/me/2fa/setup')) data = setupGate ? await setupGate.promise : { secret: 'fixture-secret', otpauth_url: 'otpauth://fixture', expires_at: '' }
      else if (url.endsWith('/me/2fa/enable')) { data = await enable.promise; enabled = true }
      else if (url.endsWith('/me/2fa/disable')) { data = await disable.promise; enabled = false }
      else if (url.endsWith('/me/2fa/recovery-codes/regenerate')) data = await regenerate.promise
      else if (url.endsWith('/me/2fa/status')) data = { enabled, recovery_codes_remaining: 1, recovery_codes_total: 1 }
      else if (url.endsWith('/me')) data = { id: 7 }
      else throw new Error(`Unexpected fixture request: ${url}`)
      return { ok: true, json: async () => ({ status_code: 0, data }) }
    },
  })
  const { userAuthAPI, userTotpAPI } = evaluate('src/api/auth.ts', ['userAuthAPI', 'userTotpAPI'], { api, userApi })
  const storeModule = evaluate('src/stores/userAuth.ts', ['useUserAuthStore'], {
    ...boundary, defineStore: pinia.defineStore, useRouter: () => ({ push() {} }), userAuthAPI,
  })
  const store = pinia.createPinia()
  pinia.setActivePinia(store)
  const auth = storeModule.useUserAuthStore()
  assert.equal(auth.token, persisted ? 'fixture-persisted' : '')
  await auth.login({ email: 'fixture@example.test', password: 'synthetic-only' })
  assert.equal(boundary.readAuthStorage('user_token'), 'fixture-before-2fa')
  const component = moduleExports(script.content, {
    vue, 'vue-i18n': { useI18n: () => ({ t: (s: string) => s }) },
    qrcode: { default: { toDataURL: async () => 'data:image/png;base64,fixture' } },
    '../../api/auth': { userTotpAPI }, '../../stores/userAuth': storeModule,
    '../../utils/alerts': { pageAlertVariant: () => 'default', pageAlertToneClass: () => '' },
    '@/components/ui/alert': { Alert: wrap('aside'), AlertDescription: wrap('p') },
    '@/components/ui/badge': { Badge: wrap('span') }, '@/components/ui/button': { Button: wrap('button') },
    '@/components/ui/input': { Input },
  }).default
  component.render = moduleExports(template.code, { vue }).render
  const container = document.createElement('div')
  document.body.append(container)
  app = vue.createApp(component, { allowSetup })
  app.use(store)
  const vm = app.mount(container)
  await flush()
  return { auth, boundary, values, requests, enable, disable, regenerate, userApi, state: (vm as any).$.setupState, props: (vm as any).$.props, recover: () => { writable = true } }
}

test('FE05: failed removal shadows persisted credentials after storage recovers', async () => {
  const h = await mount('denied', true)
  const generation = h.auth.sessionGeneration
  h.auth.logout()
  assert.equal(h.auth.sessionGeneration, generation + 1)
  assert.equal(h.auth.token, '')
  assert.equal(h.auth.user, null)
  assert.equal(h.values.get('user_token'), 'fixture-persisted')
  assert.equal(JSON.parse(h.values.get('user_profile')!).id, 1)
  h.recover()
  await h.userApi.get('/me')
  assert.equal(h.requests.at(-1)?.authorization, undefined)
  assert.equal(h.boundary.readAuthStorage('user_token'), null)
  assert.equal(h.boundary.readAuthStorage('user_profile'), null)
  // A fresh login must clear the tombstones and restore normal persistence.
  await h.auth.login({ email: 'fixture@example.test', password: 'synthetic-only' })
  await h.userApi.get('/me')
  assert.equal(h.requests.at(-1)?.authorization, 'Bearer fixture-before-2fa')
  assert.equal(h.values.get('user_token'), 'fixture-before-2fa')
  assert.equal(JSON.parse(h.values.get('user_profile')!).id, 7)
})

test('FE05: failed account replacement keeps shadowing old persisted credentials after recovery', async () => {
  const h = await mount('denied', true)
  const generation = h.auth.sessionGeneration
  h.auth.acceptOAuthLogin({ token: 'fixture-replacement', user: { id: 8 } })
  assert.equal(h.auth.sessionGeneration, generation + 1)
  assert.equal(h.values.get('user_token'), 'fixture-persisted')
  h.recover()
  await h.userApi.get('/me')
  assert.equal(h.requests.at(-1)?.authorization, 'Bearer fixture-replacement')
  assert.equal(h.boundary.readAuthStorage('user_token'), h.auth.token)
  assert.equal(JSON.parse(h.boundary.readAuthStorage('user_profile')).id, 8)
  // Successful later removal retires both persisted credentials and the fallback.
  h.auth.logout()
  await h.userApi.get('/me')
  assert.equal(h.requests.at(-1)?.authorization, undefined)
  assert.equal(h.values.has('user_token'), false)
  assert.equal(h.values.has('user_profile'), false)
  assert.equal(h.boundary.readAuthStorage('user_token'), null)
})

test('FE05: blank 2FA code does not send a request or replace the session', async () => {
  const h = await mount('denied', true)
  await h.state.startSetup()
  const count = h.requests.length
  const generation = h.auth.sessionGeneration
  h.state.enableCode = '   '
  await h.state.submitEnable()
  assert.equal(h.requests.length, count)
  assert.equal(h.state.alert.level, 'warning')
  assert.equal(h.auth.token, 'fixture-before-2fa')
  assert.equal(h.auth.sessionGeneration, generation)
})

test('FE05: enable response without a token preserves the current session', async () => {
  const h = await mount('denied', true)
  const generation = h.auth.sessionGeneration
  h.state.enableCode = '123456'
  const pending = h.state.submitEnable()
  h.enable.resolve({ recovery_codes: ['fixture-recovery'] })
  await pending
  await h.userApi.get('/me')
  assert.equal(h.requests.at(-1)?.authorization, 'Bearer fixture-before-2fa')
  assert.equal(h.auth.token, 'fixture-before-2fa')
  assert.equal(h.auth.sessionGeneration, generation)
  assert.equal(h.state.alert.level, 'success')
})

for (const mode of ['denied', 'recovered', 'normal'] as const) {
  for (const persisted of [false, true]) {
    test(`FE05: rendered 2FA rotation updates API bearer and generation (${mode}, persisted=${persisted})`, async () => {
      const h = await mount(mode, persisted)
      button('startSetup')!.click(); await flush()
      const input = document.querySelector('input')!
      input.value = ' 123456 '
      input.dispatchEvent(new window.Event('input', { bubbles: true })); await flush()
      const generation = h.auth.sessionGeneration
      button('enableSubmit')!.click(); await flush()
      assert.deepEqual(h.requests.at(-1), {
        url: '/api/v1/me/2fa/enable', method: 'POST', authorization: 'Bearer fixture-before-2fa', body: { code: '123456' },
      })
      assert.equal(h.state.loading, true)
      if (mode === 'recovered') h.recover()
      h.enable.resolve({ token: 'fixture-after-2fa', recovery_codes: ['fixture-recovery'] })
      await flush()
      // Exercise the real client, not a replacement-token mock. This also catches
      // fallback shadowing when the component's direct persistent write succeeds.
      await h.userApi.get('/me')
      assert.equal(h.requests.at(-1)?.authorization, 'Bearer fixture-after-2fa')
      assert.equal(h.auth.token, 'fixture-after-2fa')
      assert.equal(h.boundary.readAuthStorage('user_token'), 'fixture-after-2fa')
      assert.equal(h.auth.sessionGeneration, generation + 1)
      assert.equal(h.auth.user.id, 7)
      assert.equal(h.auth.isAuthenticated, true)
      assert.equal(h.values.get('user_token'), mode === 'denied' ? (persisted ? 'fixture-persisted' : undefined) : 'fixture-after-2fa')
      assert.equal(h.requests.at(-2)?.url, '/api/v1/me/2fa/status')
      assert.equal(h.requests.at(-2)?.authorization, 'Bearer fixture-after-2fa')
      assert.equal(h.state.loading, false)
      assert.equal(h.state.setupResult, null)
      assert.equal(h.state.enableCode, '')
      assert.equal(h.state.status.enabled, true)
      assert.equal(h.state.alert.level, 'success')
      assert.ok(document.body.textContent?.includes('fixture-recovery'))
      button('acknowledge')!.click(); await flush()
      assert.ok(!document.body.textContent?.includes('fixture-recovery'))
    })
  }
}

test('security enrollment off hides setup and prevents direct setup/confirm bypass without rotating tokens', async () => {
  const h = await mount('normal', false, false)
  assert.equal(button('startSetup'), undefined)
  const count = h.requests.length
  await h.state.startSetup()
  h.state.enableCode = '123456'
  await h.state.submitEnable()
  assert.equal(h.requests.length, count)
  assert.equal(h.auth.token, 'fixture-before-2fa')
})
test('enrollment toggle revokes a delayed setup response and confirm bypass', async () => {
  const gate = deferred()
  const h = await mount('normal', false, true, gate)
  const pending = h.state.startSetup()
  h.props.allowSetup = false; h.props.allowSetup = true
  gate.resolve({ secret: 'obsolete-secret', otpauth_url: 'otpauth://obsolete', expires_at: '' })
  await pending
  assert.equal(h.state.setupResult, null)
  assert.equal(h.state.qrcodeDataUrl, '')
  h.props.allowSetup = false
  h.state.enableCode = '123456'
  const count = h.requests.length
  await h.state.submitEnable()
  assert.equal(h.requests.length, count)
})
test('existing enabled 2FA management remains visible with enrollment off', async () => {
  const h = await mount('normal', false, false)
  h.state.status = { enabled: true, recovery_codes_remaining: 1, recovery_codes_total: 1 }; await flush()
  assert.ok(button('regenerateAction'))
  assert.ok(button('disableAction'))
  h.state.openDisable(); assert.equal(h.state.mode, 'disable')
  h.state.openRegenerate(); assert.equal(h.state.mode, 'regenerate')
})

test('obsolete enrollment finally cannot release a newer setup busy state', async () => {
  const gate = deferred()
  const h = await mount('normal', false, true, gate)
  const old = h.state.startSetup()
  h.props.allowSetup = false; h.props.allowSetup = true
  const newer = h.state.startSetup()
  gate.resolve({ secret: 'synthetic-secret', otpauth_url: 'otpauth://synthetic', expires_at: '' })
  await old
  // Both resolve from the same deferred boundary, but QR generation gives the newer request a continuation.
  assert.equal(h.state.loading, true)
  await newer
  assert.equal(h.state.loading, false)
})
for (const reason of ['toggle', 'unmount']) test(`late enrollment enable cannot rotate tokens after ${reason}`, async () => {
  const h = await mount('normal', false)
  h.state.enableCode = '123456'
  const pending = h.state.submitEnable()
  if (reason === 'toggle') h.props.allowSetup = false
  else { app!.unmount(); app = undefined }
  h.enable.resolve({ token: 'obsolete-token', recovery_codes: ['obsolete-recovery'] }); await pending
  assert.equal(h.auth.token, 'fixture-before-2fa')
  assert.deepEqual(h.state.recoveryCodes, [])
})

test('enrollment toggle never releases existing-factor management busy state or feedback', async () => {
  const h = await mount('normal', false, true)
  h.state.status = { enabled: true, recovery_codes_remaining: 1, recovery_codes_total: 1 }
  h.state.openDisable(); h.state.disableCode = '123456'
  const pending = h.state.submitDisable()
  assert.equal(h.state.loading, true)
  h.state.alert = { level: 'warning', message: 'management-feedback' }
  h.props.allowSetup = false
  assert.equal(h.state.loading, true)
  assert.equal(h.state.alert.message, 'management-feedback')
  assert.equal(h.requests.at(-1)?.url, '/api/v1/me/2fa/disable')
  h.disable.resolve({}); await pending
  assert.equal(h.state.loading, false)
})
test('already-enabled factor can be disabled with enrollment off using original challenge payload', async () => {
  const h = await mount('normal', false, false)
  h.state.status = { enabled: true, recovery_codes_remaining: 1, recovery_codes_total: 1 }
  h.state.openDisable(); h.state.disableCode = '123456'
  const pending = h.state.submitDisable()
  assert.deepEqual(h.requests.at(-1)?.body, { code: '123456' })
  h.disable.resolve({}); await pending
  assert.equal(h.state.alert.level, 'success')
})

test('already-enabled recovery-code management remains usable with enrollment off', async () => {
  const h = await mount('normal', false, false, undefined, true)
  assert.equal(h.state.status.enabled, true)
  h.state.openRegenerate(); h.state.regenerateCode = '123456'
  const pending = h.state.submitRegenerate()
  assert.equal(h.requests.at(-1)?.url, '/api/v1/me/2fa/recovery-codes/regenerate')
  assert.deepEqual(h.requests.at(-1)?.body, { code: '123456' })
  h.regenerate.resolve({ recovery_codes: ['synthetic-recovery'] }); await pending
  assert.equal(h.state.status.enabled, true)
  assert.deepEqual(h.state.recoveryCodes, ['synthetic-recovery'])
  assert.equal(h.state.alert.level, 'success')
})
