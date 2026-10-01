import test from 'node:test'
import assert from 'node:assert/strict'
import { fixture, settle, vue, pass, empty, dom } from './helpers/motionPolishRouteHarness.ts'

for (const theme of ['classic', 'vault']) test(`${theme}: Register upstream route opacity keeps input drafts on query-only navigation`, async () => {
  const prefix = theme === 'classic' ? '../..' : '../../..'
  const file = theme === 'classic' ? 'views/auth/Register.vue' : 'templates/vault/auth/Register.vue'
  const state: any = {
    userAuthStore: vue.reactive({ loading: false }), brandSiteName: 'Fixture', registrationEnabled: true,
    email: vue.ref('draft@example.invalid'), password: vue.ref(''), agreed: vue.ref(false), passwordStrength: vue.ref(''),
    formValidation: { getError: () => '', hasError: () => false, touchField() {} },
  }
  const Input = { props: ['modelValue'], emits: ['update:modelValue'], setup: (p: any, { emit }: any) => () => vue.h('input', { value: p.modelValue, onInput: (e: any) => emit('update:modelValue', e.target.value) }) }
  const h = await fixture(theme, {
    [`${prefix}/composables/useRegister`]: { useRegister: () => state },
    [`${prefix}/components/captcha/ImageCaptcha.vue`]: { default: empty }, [`${prefix}/components/captcha/TurnstileCaptcha.vue`]: { default: empty },
    '@/components/ui/card': { Card: pass }, '@/components/ui/input': { Input },
    '@/components/ui/select': { Select: pass, SelectContent: pass, SelectItem: pass, SelectTrigger: pass, SelectValue: pass },
  }, load => [{ path: '/', component: empty }, { path: '/auth/login', component: empty }, { path: '/auth/register', component: load(file).default }, { path: '/privacy', component: empty }, { path: '/terms', component: empty }])
  try {
    await h.go('/auth/register')
    assert.equal(h.events.length, 0)
    const input = h.host.querySelector('input')!
    input.value = 'changed@example.invalid'; input.dispatchEvent(new dom.window.Event('input', { bubbles: true })); await settle()
    await h.go('/auth/register?source=same')
    assert.equal(h.events.length, 0); assert.equal(h.host.querySelector('input'), input)
    assert.equal(state.email.value, 'changed@example.invalid')
  } finally { h.unmount() }
})
