import test from 'node:test'
import assert from 'node:assert/strict'
import { reactive, nextTick } from 'vue'
import { runtime, deferred } from './helpers/motion17BRuntime.ts'
import { debounceAsync } from '../src/utils/debounce.ts'
const intervals = new Map<number, Function>()
const listeners = new Map<string, Function>()
let timerId = 0
const startInterval = (fn: Function) => { intervals.set(++timerId, fn); return timerId }
const stopInterval = (id: number) => { intervals.delete(id) }
Object.assign(globalThis, { setInterval: startInterval, clearInterval: stopInterval, window: { setTimeout, clearTimeout, setInterval: startInterval, clearInterval: stopInterval, open: () => ({}), removeEventListener: (type: string) => listeners.delete(type), addEventListener: (type: string, fn: Function) => listeners.set(type, fn) } })
const wait = () => new Promise(resolve => setTimeout(resolve, 240))
function setup(name: 'Login' | 'Register' | 'Forgot', configGate?: Promise<any>) {
  const miniApp = reactive({ isMiniApp: false, isReady: false, initData: '' })
  const calls: any[] = [], pushes: any[] = [], accepted: any[] = []
  const request = (payload: any) => { const d = deferred(); calls.push({ ...d, payload }); return d.promise }
  const auth: any = reactive({ sessionGeneration: 0, token: '', user: null, loading: false, challengeToken: 'challenge', challengeExpiresAt: '',
    clearChallenge() { auth.challengeToken = ''; auth.challengeExpiresAt = '' },
    acceptOAuthLogin(data: any) { accepted.push(data); if (data.token) { auth.token = data.token; auth.sessionGeneration++ }; if (data.requires_totp) { auth.challengeToken = data.challenge_token; auth.challengeExpiresAt = data.challenge_expires_at } },
    // Existing store semantics are deliberately reproduced to expose off-page session writes.
    async login(p: any) { const r = await request(p); auth.acceptOAuthLogin(r.data.data); return { requiresTotp: !!r.data.data.requires_totp } },
    async register(p: any) { const r = await request(p); auth.acceptOAuthLogin(r.data.data); return true },
    async verify2FA(p: any) { const r = await request(p); auth.acceptOAuthLogin(r.data.data); return true },
    sendVerifyCode: request, forgotPassword: request,
  })
  auth.googleLogin = auth.login; auth.telegramLogin = auth.login; auth.telegramMiniAppLogin = auth.login
  const r = runtime(`src/composables/use${name}.ts`, [`use${name}`], {
    useUserAuthStore: () => auth, userAuthAPI: { telegramOidcStart: request, googleRedirectIntent: request, login: request, register: request, verify2FA: request, sendVerifyCode: request, forgotPassword: request, googleLogin: request, telegramLogin: request, telegramMiniAppLogin: request },
    useRouter: () => ({ push: (p: any) => pushes.push(p), replace() {} }), useRoute: () => ({ query: {} }),
    useAppStore: () => ({ config: { email_verification_enabled: false }, loadConfig: () => configGate || Promise.resolve() }), useTelegramMiniAppStore: () => miniApp,
    useI18n: () => ({ t: (s: string) => s }), debounceAsync,
    useFormValidation: () => ({ addRule() {}, requiredRule() {}, emailRule() {}, minLengthRule() {}, validateAll: () => true, touchField() {}, hasError: () => false }),
    createGoogleRedirectPreparedIntent: () => ({ issuedAt: 1 }), createGoogleRedirectIntent: () => ({}), getGoogleRedirectSessionStorage: () => ({}), storeGoogleRedirectIntent: () => accepted.push('redirect-intent'),
    getPasswordStrength: () => '', shouldResumeGoogleRedirect2FA: () => false, detectGoogleIdentityUXMode: () => 'popup', isTelegramUrlEnvironment: () => false,
  })
  const g = r.run(() => r[`use${name}`]())
  g.email.value = 'a@example.invalid'
  if (g.password) g.password.value = 'fixture'
  if (g.agreed) g.agreed.value = true
  if (g.code) g.code.value = 'fixture'
  if (g.newPassword) g.newPassword.value = 'fixture'
  if (g.totpCode) g.totpCode.value = 'fixture'
  return { g, auth, calls, pushes, accepted, miniApp, mount: r.mount, dispose: r.dispose }
}
for (const action of ['queued', 'inflight']) test(`2FA cancellation invalidates ${action} verification without a late error`, async () => {
  const s = setup('Login'); const pending = s.g.handleVerify2FA()
  if (action === 'inflight') await wait()
  s.g.cancel2FA(); s.g.error.value = 'current'; await wait()
  s.calls.forEach(d => d.resolve(success)); await pending
  assert.equal(s.g.error.value, 'current'); assert.equal(s.accepted.length, 0); assert.equal(s.auth.loading, false); s.dispose()
})
test('2FA-required response never installs a session before verification', async () => {
  intervals.clear(); const s = setup('Login'); const pending = s.g.handleLogin(); await wait()
  s.calls[0].resolve({ data: { data: { token: 'must-not-install', user: { id: 1 }, requires_totp: true, challenge_token: 'fixture', challenge_expires_at: '2099-01-01T00:00:00Z' } } }); await pending
  assert.equal(s.auth.token, ''); assert.equal(s.g.step.value, 'totp'); assert.equal(s.pushes.length, 0)
  s.dispose(); assert.equal(intervals.size, 0)
})
test('expired 2FA challenge does not create a replacement countdown interval', async () => {
  intervals.clear(); const s = setup('Login'); const pending = s.g.handleLogin(); await wait()
  s.calls[0].resolve({ data: { data: { requires_totp: true, challenge_token: 'fixture', challenge_expires_at: '2000-01-01T00:00:00Z' } } }); await pending
  assert.equal(s.g.step.value, 'password'); assert.equal(intervals.size, 0); s.dispose()
})
for (const method of ['startTelegramOidc', 'prepareGoogleRedirectLogin']) test(`${method}: disposed continuation cannot redirect or save intent`, async () => {
  Object.assign(globalThis, { sessionStorage: { removeItem() {}, setItem() {} } })
  ;(window as any).location = { href: 'original' }
  const s = setup('Login'); const pending = s.g[method]().catch(() => undefined); s.dispose()
  s.calls[0].resolve({ data: { data: { auth_url: '/fixture' } } }); await pending
  assert.equal((window as any).location.href, 'original'); assert.equal(s.accepted.length, 0)
})
for (const leave of ['none', 'account', 'dispose']) test(`GitHub popup completion respects ${leave} ownership`, async () => {
  ;(window as any).location = { origin: 'https://fixture.invalid' }
  const s = setup('Login'); await s.mount(); s.g.startGitHubLogin()
  const callback = listeners.get('message')!
  if (leave === 'account') s.auth.sessionGeneration++
  if (leave === 'dispose') s.dispose()
  callback({ origin: 'https://fixture.invalid', data: { type: 'dujiao:github-oauth', data: success.data.data } })
  assert.equal(s.accepted.length, leave === 'none' ? 1 : 0); s.dispose()
})
test('late Google widget error is ignored after page disposal', () => {
  const s = setup('Login'); s.dispose(); s.g.handleGoogleScriptError(); assert.equal(s.g.error.value, '')
})
test('changing email in 2FA discards the old account challenge', () => {
  const s = setup('Login'); s.g.step.value = 'totp'; s.g.email.value = 'new@example.invalid'
  assert.equal(s.auth.challengeToken, ''); assert.equal(s.g.step.value, 'password'); s.dispose()
})
test('mini-app identity replacement cannot install the previous identity response', async () => {
  const s = setup('Login'); await s.mount()
  Object.assign(s.miniApp, { isMiniApp: true, isReady: true, initData: 'old' }); await nextTick()
  s.miniApp.initData = 'new'; await nextTick()
  s.calls[0].resolve(success); for (let i = 0; i < 5; i++) await Promise.resolve()
  assert.equal(s.accepted.length, 0); assert.equal(s.calls.length, 2)
  s.calls[1].resolve(success); for (let i = 0; i < 5; i++) await Promise.resolve()
  assert.equal(s.accepted.length, 1); s.dispose()
})
for (const provider of ['google', 'telegram', 'miniApp']) {
  for (const leave of [false, true]) test(`login ${provider}: ${leave ? 'stale' : 'same-page'} response is owned`, async () => {
    const s = setup('Login'); await s.mount()
    let pending: any
    if (provider === 'google') pending = s.g.handleGoogleCredential('fixture')
    if (provider === 'telegram') pending = (window as any).__dujiaoUserTelegramLogin({ id: 1, auth_date: 1, hash: 'fixture' })
    if (provider === 'miniApp') { s.miniApp.isMiniApp = true; s.miniApp.isReady = true; s.miniApp.initData = 'fixture'; await nextTick() }
    assert.equal(s.calls.length, 1)
    if (leave) s.dispose()
    s.calls[0].resolve(success); await pending; for (let i = 0; i < 5; i++) await Promise.resolve()
    assert.equal(s.accepted.length, leave ? 0 : 1); assert.equal(s.pushes.length, leave ? 0 : 1)
    s.dispose()
  })
}
test('typing while initial config loads does not suppress same-page provider setup', async () => {
  const config = deferred(); const s = setup('Login', config.promise)
  const mount = s.mount(); s.g.email.value = 'typed@example.invalid'; config.resolve(true); await mount
  assert.equal(typeof (window as any).__dujiaoUserTelegramLogin, 'function'); s.dispose()
})
test('login mounted config continuation cannot re-register callbacks after disposal', async () => {
  const config = deferred(); const s = setup('Login', config.promise)
  const mount = s.mount(); s.dispose(); config.resolve(true); await mount
  assert.equal((window as any).__dujiaoUserTelegramLogin, undefined)
})
for (const name of ['Login', 'Register', 'Forgot'] as const) test(`${name}: stale captcha-config refresh cannot clear current inputs`, async () => {
  const config = deferred(); const s = setup(name, config.promise)
  const pending = s.g.handleCaptchaConfigStale(); s.dispose()
  s.g.turnstileToken.value = 'new state'; config.resolve(true); await pending
  assert.equal(s.g.turnstileToken.value, 'new state')
})
for (const name of ['Register', 'Forgot'] as const) test(`${name}: disposal clears completed send-code countdown`, async () => {
  intervals.clear()
  const s = setup(name); const pending = s.g.handleSendCode(); await wait()
  s.calls[0].resolve(success); await pending
  assert.equal(intervals.size, 1); s.dispose(); assert.equal(intervals.size, 0)
})
const success = { data: { data: { token: 'fixture-token', user: { id: 1 } } } }
for (const [name, method] of [['Login', 'handleLogin'], ['Login', 'handleVerify2FA'], ['Register', 'handleRegister'], ['Register', 'handleSendCode'], ['Forgot', 'handleReset'], ['Forgot', 'handleSendCode']] as const) {
  for (const leave of ['dispose', 'account', 'email']) test(`${name}.${method}: ${leave} blocks in-flight UI/session completion`, async () => {
    intervals.clear()
    const s = setup(name); const pending = s.g[method](); await wait()
    assert.equal(s.calls.length, 1)
    if (leave === 'dispose') s.dispose()
    if (leave === 'account') { s.auth.sessionGeneration++; s.auth.token = 'other-account' }
    if (leave === 'email') s.g.email.value = 'b@example.invalid'
    s.calls[0].resolve(success); await pending
    assert.equal(s.accepted.length, 0, 'stale server response cannot establish session')
    assert.equal(s.pushes.length, 0); assert.equal(intervals.size, 0)
    assert.equal(s.g.error.value, ''); s.dispose()
  })
  test(`${name}.${method}: current failure remains visible and releases loading`, async () => {
    const s = setup(name); const pending = s.g[method](); await wait(); s.calls[0].reject(new Error('current failure')); await pending
    assert.equal(s.g.error.value, 'current failure'); assert.equal(s.auth.loading, false)
    assert.equal(s.accepted.length, 0); assert.equal(s.pushes.length, 0); s.dispose()
  })
  test(`${name}.${method}: same-page successful completion still works`, async () => {
    intervals.clear()
    const s = setup(name); const pending = s.g[method](); await wait(); s.calls[0].resolve(success); await pending
    if (method === 'handleSendCode') assert.equal(s.g.countdown.value, 60)
    else assert.equal(s.pushes.length, 1)
    assert.equal(s.auth.loading, false); s.dispose()
  })
  test(`${name}.${method}: stale rejection cannot reset current error/loading`, async () => {
    const s = setup(name); const pending = s.g[method](); await wait(); s.dispose()
    s.g.error.value = 'new state'; s.auth.loading = true
    s.calls[0].reject(new Error('stale')); await pending
    assert.equal(s.g.error.value, 'new state'); assert.equal(s.auth.loading, true)
  })
  for (const change of ['email', 'account']) test(`${name}.${method}: ${change} cancels queued submission and releases obsolete busy state`, async () => {
    const s = setup(name); const queued = s.g[method]()
    if (change === 'email') s.g.email.value = 'new@example.invalid'
    else s.auth.sessionGeneration++
    await wait(); s.calls.forEach(d => d.resolve(success)); await queued
    assert.equal(s.calls.length, 0); assert.equal(s.auth.loading, false); s.dispose()
  })
  test(`${name}.${method}: changing identity during a request releases only obsolete loading`, async () => {
    const s = setup(name); const old = s.g[method](); await wait()
    s.g.email.value = 'other@example.invalid'
    assert.equal(s.auth.loading, false)
    const fresh = s.g[method](); await wait()
    s.calls[0].reject(new Error('stale')); await old
    assert.equal(s.auth.loading, true)
    s.calls[1].resolve(success); await fresh; assert.equal(s.auth.loading, false); s.dispose()
  })
  test(`${name}.${method}: disposal cancels queued submission`, async () => {
    const s = setup(name); const pending = s.g[method](); s.dispose(); await wait()
    s.calls.forEach(d => d.resolve(success)); await pending
    assert.equal(s.calls.length, 0)
  })
}
