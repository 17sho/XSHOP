import assert from 'node:assert/strict'
import test from 'node:test'
import { dom, vue, loadSource, settle } from './helpers/motion17CHarness.ts'
const wrapper = { setup: (_: any, { slots }: any) => () => vue.h('div', slots.default?.()) }
const Button = { setup: (_: any, { slots }: any) => () => vue.h('button', slots.default?.()) }
const Input = { props: ['modelValue'], emits: ['update:modelValue'], setup: (props: any, { emit }: any) => () => vue.h('input', { value: props.modelValue, onInput: (e: any) => emit('update:modelValue', e.target.value) }) }
const empty = { setup: () => () => null }
const ui = {
  'vue-i18n': { useI18n: () => ({ t: (key: string) => key }) },
  '@/components/ui/alert': { Alert: wrapper, AlertDescription: wrapper }, '@/components/ui/badge': { Badge: wrapper }, '@/components/ui/button': { Button }, '@/components/ui/card': { Card: wrapper }, '@/components/ui/input': { Input },
}
function capture() {
  const events: any[] = []
  dom.window.matchMedia = (() => ({ matches: false, addEventListener() {}, removeEventListener() {} })) as any
  dom.window.HTMLElement.prototype.animate = function() { const event = { target: this, cancelled: false }; events.push(event); return { cancel: () => { event.cancelled = true } } as any }
  return events
}
function mount(component: any) {
  const host = document.createElement('div'); document.body.append(host)
  const app = vue.createApp(component); app.component('RouterLink', wrapper); app.mount(host)
  return { host, unmount: () => { app.unmount(); host.remove() } }
}

for (const [path, prefix] of [['views/auth/Login.vue', '../..'], ['templates/vault/auth/Login.vue', '../../..']]) {
test(`${path}: 2FA mode changes preserve the form and do not replay on countdown/input updates`, async () => {
  const events = capture()
  const state: any = {
    step: vue.ref('totp'), totpMode: vue.ref('code'), totpCode: vue.ref(''), recoveryCode: vue.ref(''), challengeRemainingSeconds: vue.ref(120),
    email: vue.ref('draft@example.invalid'), password: vue.ref(''), showPassword: vue.ref(false), rememberMe: vue.ref(false), error: vue.ref(''), info: vue.ref(''),
    userAuthStore: vue.reactive({ loading: false }), brandSiteName: 'Fixture', formValidation: { getError: () => '' },
  }
  const Login = loadSource(path!, {
    ...ui, [`${prefix}/composables/useLogin`]: { useLogin: () => state },
    [`${prefix}/components/FormField.vue`]: { default: { setup: (_: any, { slots }: any) => () => vue.h('label', slots.default?.({ id: 'fixture-input' })) } },
    [`${prefix}/components/captcha/ImageCaptcha.vue`]: { default: empty }, [`${prefix}/components/captcha/TurnstileCaptcha.vue`]: { default: empty }, [`${prefix}/components/auth/GoogleIdentityButton.vue`]: { default: empty },
  }).default
  const { host, unmount } = mount(Login)
  try {
    await settle(); assert.equal(events.length, 0)
    const form = host.querySelector('form')!
    state.totpMode.value = 'recovery'; await settle()
    assert.equal(host.querySelector('form'), form)
    assert.equal(events.length, 0)
    const input = form.querySelector('input')!
    input.value = 'synthetic recovery draft'; input.dispatchEvent(new dom.window.Event('input', { bubbles: true })); await settle()
    state.challengeRemainingSeconds.value--; await settle()
    assert.equal(form.querySelector('input'), input)
    assert.equal(events.length, 0)
    assert.equal(state.recoveryCode.value, 'synthetic recovery draft')
    state.totpMode.value = 'code'; await settle()
    assert.equal(host.querySelector('form'), form)
  } finally { unmount() }
})
}

test('actual reseller v-show tabs preserve the form and draft without a custom animation owner', async () => {
  const events = capture()
  const Site = loadSource('components/reseller/ResellerSiteConfigPanel.vue', {
    ...ui,
    '../../api': { resellerAPI: { siteConfig: async () => ({ data: { data: { opened: true, can_edit: true } } }), updateSiteConfig: () => { throw new Error('Writes forbidden') } } },
    '../../stores/app': { useAppStore: () => ({ loadConfig: async () => {} }) }, '../../utils/richContent': { sanitizeRichHtml: (html: string) => html },
    '@/components/ui/select': { Select: wrapper, SelectContent: wrapper, SelectItem: wrapper, SelectTrigger: wrapper, SelectValue: wrapper }, '@/components/ui/switch': { Switch: empty }, '@/components/ui/textarea': { Textarea: Input },
    './ResellerImageField.vue': { default: empty }, './ResellerLocaleTabs.vue': { default: empty }, './ResellerRichText.vue': { default: empty },
  }).default
  const { host, unmount } = mount(Site)
  try {
    await settle(); await settle(); assert.equal(events.length, 0)
    const form = host.querySelector('form')!
    const input = form.querySelector('input')!
    input.value = 'Synthetic site draft'; input.dispatchEvent(new dom.window.Event('input', { bubbles: true })); await settle()
    const tabs = [...form.querySelectorAll('button')].filter(button => button.textContent?.includes('siteConfig.tabs.'))
    assert.equal(tabs.length, 4)
    tabs[1]!.click(); await settle(); tabs[2]!.click(); await settle(); tabs[0]!.click(); await settle()
    assert.equal(events.length, 0)
    assert.equal(host.querySelector('form'), form)
    assert.equal(form.querySelector('input'), input)
    assert.equal(input.value, 'Synthetic site draft')
  } finally { unmount() }
})
