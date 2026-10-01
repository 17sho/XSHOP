import test from 'node:test'
import assert from 'node:assert/strict'
import { fixture, settle, vue, pass, empty, dom } from './helpers/motionPolishRouteHarness.ts'

for (const theme of ['classic', 'vault']) test(`${theme}: actual Login preserves query/form drafts without custom local motion`, async () => {
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
    await h.go('/auth/login')
    assert.equal(h.events.length, 0)
    const form = h.host.querySelector('form')!
    state.totpMode.value = 'recovery'; await settle()
    assert.equal(h.host.querySelector('form'), form)
    const input = form.querySelector('input')!
    input.value = 'synthetic draft'; input.dispatchEvent(new dom.window.Event('input', { bubbles: true })); await settle()
    state.challengeRemainingSeconds.value--; await settle()
    await h.go('/auth/login?redirect=/products/A')
    assert.equal(h.host.querySelector('input'), input)
    assert.equal(state.recoveryCode.value, 'synthetic draft')
    state.totpMode.value = 'code'; await settle()
    h.reduce(true); state.totpMode.value = 'recovery'; await settle()
    assert.equal(h.events.length, 0)
    h.reduce(false); state.totpMode.value = 'code'; await settle()
  } finally { h.unmount() }
  assert.equal(h.listeners.size, 0)
})
