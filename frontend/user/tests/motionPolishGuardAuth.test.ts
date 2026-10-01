import test from 'node:test'
import assert from 'node:assert/strict'
import { fixture, settle, vue, pass, empty, dom } from './helpers/motionPolishRouteHarness.ts'

for (const theme of ['classic', 'vault']) test(`${theme}: actual Register + actual delayed Turnstile has no transformed ancestors and keeps query drafts`, async () => {
  delete dom.window.turnstile
  document.querySelectorAll('script[data-turnstile]').forEach(s => s.remove())
  const prefix = theme === 'classic' ? '../..' : '../../..'
  const file = theme === 'classic' ? 'views/auth/Register.vue' : 'templates/vault/auth/Register.vue'
  const state: any = {
    userAuthStore: vue.reactive({ loading: false }), brandSiteName: 'Fixture', registrationEnabled: true,
    registerCaptchaEnabled: vue.ref(true), captchaProvider: vue.ref('turnstile'), turnstileSiteKey: vue.ref('synthetic-public-site-key'), registerTurnstileToken: vue.ref(''),
    email: vue.ref('draft@example.invalid'), password: vue.ref(''), agreed: vue.ref(false), passwordStrength: vue.ref(''),
    formValidation: { getError: () => '', hasError: () => false, touchField() {} },
  }
  const Input = { props: ['modelValue'], emits: ['update:modelValue'], setup: (p: any, { emit }: any) => () => vue.h('input', { value: p.modelValue, onInput: (e: any) => emit('update:modelValue', e.target.value) }) }
  const h = await fixture(theme, {
    [`${prefix}/composables/useRegister`]: { useRegister: () => state },
    [`${prefix}/components/captcha/ImageCaptcha.vue`]: { default: empty },
    '@/components/ui/card': { Card: pass }, '@/components/ui/input': { Input },
    '@/components/ui/select': { Select: pass, SelectContent: pass, SelectItem: pass, SelectTrigger: pass, SelectValue: pass },
  }, load => [{ path: '/', component: empty }, { path: '/auth/login', component: empty }, { path: '/auth/register', component: load(file).default }, { path: '/privacy', component: empty }, { path: '/terms', component: empty }])
  try {
    await h.go('/auth/register')
    assert.equal(h.events.length, 0)
    assert.equal(h.host.querySelector('iframe'), null)
    const script = document.querySelector('script[data-turnstile="1"]')!
    assert.ok(script, 'actual Turnstile has queued script load')
    dom.window.turnstile = { render(container: HTMLElement) { container.append(document.createElement('iframe')); return 'synthetic-widget' }, reset() {}, remove() {} }
    script.dispatchEvent(new dom.window.Event('load')); await settle(); await settle()
    assert.equal(h.events.length, 0)
    const iframe = h.host.querySelector('iframe')!
    assert.ok(iframe, 'actual delayed Turnstile renders')
    for (let ancestor = iframe.parentElement; ancestor; ancestor = ancestor.parentElement) {
      assert.ok(['', 'none'].includes(window.getComputedStyle(ancestor).transform))
    }
    const input = h.host.querySelector('input')!
    input.value = 'changed@example.invalid'; input.dispatchEvent(new dom.window.Event('input', { bubbles: true })); await settle()
    await h.go('/auth/register?source=same')
    assert.equal(h.events.length, 0); assert.equal(h.host.querySelector('input'), input)
    assert.equal(state.email.value, 'changed@example.invalid')
  } finally { h.unmount(); delete dom.window.turnstile; document.querySelectorAll('script[data-turnstile]').forEach(s => s.remove()) }
})

for (const theme of ['classic', 'vault']) test(`${theme}: actual Login step has no custom local motion and preserves query/form drafts`, async () => {
  const prefix = theme === 'classic' ? '../..' : '../../..'
  const file = theme === 'classic' ? 'views/auth/Login.vue' : 'templates/vault/auth/Login.vue'
  const state: any = {
    step: vue.ref('totp'), totpMode: vue.ref('code'), totpCode: vue.ref(''), recoveryCode: vue.ref(''), challengeRemainingSeconds: vue.ref(120),
    email: vue.ref('draft@example.invalid'), password: vue.ref(''), showPassword: vue.ref(false), rememberMe: vue.ref(false), error: vue.ref(''), info: vue.ref(''),
    userAuthStore: vue.reactive({ loading: false }), brandSiteName: 'Fixture', formValidation: { getError: () => '' },
  }
  const Input = { props: ['modelValue'], emits: ['update:modelValue'], setup: (p: any, { emit }: any) => () => vue.h('input', { value: p.modelValue, onInput: (e: any) => emit('update:modelValue', e.target.value) }) }
  const h = await fixture(theme, {
    [`${prefix}/composables/useLogin`]: { useLogin: () => state },
    [`${prefix}/components/FormField.vue`]: { default: { setup: (_: any, { slots }: any) => () => vue.h('label', slots.default?.({ id: 'fixture' })) } },
    [`${prefix}/components/captcha/ImageCaptcha.vue`]: { default: empty }, [`${prefix}/components/captcha/TurnstileCaptcha.vue`]: { default: empty }, [`${prefix}/components/auth/GoogleIdentityButton.vue`]: { default: empty },
    '@/components/ui/card': { Card: pass }, '@/components/ui/input': { Input },
  }, load => [{ path: '/', component: empty }, { path: '/auth/login', component: load(file).default }])
  try {
    await h.go('/auth/login'); assert.equal(h.events.length, 0, 'original route opacity has no custom WAAPI owner')
    const form = h.host.querySelector('form')!
    state.totpMode.value = 'recovery'; await settle()
    assert.equal(h.events.length, 0)
    assert.equal(h.host.querySelector('form'), form)
    const input = form.querySelector('input')!
    input.value = 'synthetic draft'; input.dispatchEvent(new dom.window.Event('input', { bubbles: true })); await settle()
    state.challengeRemainingSeconds.value--; await settle()
    await h.go('/auth/login?redirect=/products/A')
    assert.equal(h.events.length, 0)
    assert.equal(h.host.querySelector('input'), input)
    assert.equal(state.recoveryCode.value, 'synthetic draft')
    state.totpMode.value = 'code'; await settle()
    h.reduce(true)
    state.totpMode.value = 'recovery'; await settle(); assert.equal(h.events.length, 0)
    h.reduce(false); state.totpMode.value = 'code'; await settle()
  } finally { h.unmount() }
  assert.equal(h.listeners.size, 0)
})
