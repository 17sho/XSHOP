import test from 'node:test'
import assert from 'node:assert/strict'
import { fixture, settle, vue, empty, pass, dom } from './helpers/motionPolishRouteHarness.ts'

// The removed lift guard needed MutationObservers because ancestors transformed.
// Original route opacity creates no containing block, including child-only SDK inserts.
for (const hazard of ['fixed', 'dialog', 'iframe']) test(`App keeps dynamic ${hazard} outside transformed ancestors`, async () => {
  const shown = vue.ref(false)
  const Page = { setup: () => () => vue.h('section', shown.value ? [hazard === 'fixed' ? vue.h('div', { style: 'position:fixed' }) : vue.h(hazard)] : ['safe']) }
  const h = await fixture('classic', {}, () => [{ path: '/', component: empty }, { path: '/probe', component: Page }])
  try {
    await h.go('/probe'); shown.value = true; await settle()
    const child = h.host.querySelector('section')!.firstElementChild!
    assert.ok(child)
    for (let ancestor = child.parentElement; ancestor; ancestor = ancestor.parentElement) {
      assert.ok(['', 'none'].includes(window.getComputedStyle(ancestor).transform))
    }
    assert.equal(h.events.length, 0)
  } finally { h.unmount() }
})

for (const theme of ['classic', 'vault']) for (const owner of ['route', 'local']) test(`actual ${theme} Login + actual Turnstile: ${owner} never transforms widget ancestors before or after delayed SDK`, async () => {
  delete dom.window.turnstile
  document.querySelectorAll('script[data-turnstile]').forEach(s => s.remove())
  const prefix = theme === 'classic' ? '../..' : '../../..'
  const state = {
    step: vue.ref(owner === 'route' ? 'credentials' : 'totp'), totpMode: vue.ref('code'), totpCode: vue.ref(''), recoveryCode: vue.ref(''), challengeRemainingSeconds: vue.ref(120),
    email: vue.ref('draft@example.invalid'), password: vue.ref(''), showPassword: vue.ref(false), rememberMe: vue.ref(false), error: vue.ref(''), info: vue.ref(''),
    userAuthStore: vue.reactive({ loading: false }), brandSiteName: 'Fixture', formValidation: { getError: () => '' },
    loginCaptchaEnabled: vue.ref(true), captchaProvider: vue.ref('turnstile'), turnstileSiteKey: vue.ref('synthetic-public-site-key'), turnstileToken: vue.ref(''),
  }
  const h = await fixture(theme, {
    [`${prefix}/composables/useLogin`]: { useLogin: () => state },
    [`${prefix}/components/FormField.vue`]: { default: { setup: (_: any, { slots }: any) => () => vue.h('label', slots.default?.({ id: 'fixture' })) } },
    [`${prefix}/components/captcha/ImageCaptcha.vue`]: { default: empty },
    [`${prefix}/components/auth/GoogleIdentityButton.vue`]: { default: empty },
    '@/components/ui/card': { Card: pass }, '@/components/ui/input': { Input: pass },
  }, load => [{ path: '/', component: empty }, { path: '/auth/login', component: load(theme === 'classic' ? 'views/auth/Login.vue' : 'templates/vault/auth/Login.vue').default }])
  try {
    await h.go('/auth/login')
    if (owner === 'local') { state.step.value = 'credentials'; await settle() }
    assert.equal(h.host.querySelector('iframe'), null)
    assert.equal(h.events.length, 0, 'no custom route or local WAAPI owner')
    const before = h.events.length
    const script = document.querySelector('script[data-turnstile="1"]')!; assert.ok(script, 'actual Turnstile owns pending script')
    dom.window.turnstile = { render(container: HTMLElement) { container.append(document.createElement('iframe')); return 'synthetic-widget' }, reset() {}, remove() {} }
    script.dispatchEvent(new dom.window.Event('load')); await settle(); await settle()
    assert.ok(h.host.querySelector('iframe'), 'actual renderWidget continuation inserted SDK iframe')
    assert.equal(h.events.length, before, 'SDK completion must not replay the entrance')
    for (let ancestor = h.host.querySelector('iframe')!.parentElement; ancestor; ancestor = ancestor.parentElement) {
      assert.ok(['', 'none'].includes(window.getComputedStyle(ancestor).transform))
    }
  } finally { h.unmount(); delete dom.window.turnstile; document.querySelectorAll('script[data-turnstile]').forEach(s => s.remove()) }
})
