import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { loader, vue, settle } from './helpers/motion17DHarness.ts'
const ui = {
  'vue-i18n': { useI18n: () => ({ t: (key: string) => key }) },
  '@/components/ui/alert': { Alert: 'div', AlertDescription: 'div' }, '@/components/ui/badge': { Badge: 'span' },
  '@/components/ui/button': { Button: 'button' }, '@/components/ui/input': { Input: 'input' }, '@/components/ui/label': { Label: 'label' },
}
const mount = (component: any) => {
  const host = document.createElement('div'); document.body.append(host)
  const app = vue.createApp(component); app.mount(host)
  return { host, unmount() { app.unmount(); host.remove() } }
}
test('GiftCard protects actual Turnstile host before delayed SDK fixed/iframe insertion, without animating it through a containing block', async () => {
  let writes = 0, target: HTMLElement | null = null
  const forbidden = () => { writes++; throw Error('Business operations forbidden') }
  window.turnstile = { render: (container: HTMLElement) => { target = container; return 'fixture' }, reset() {}, remove() {} }
  const config = vue.reactive({ captcha: { provider: 'turnstile', scenes: { gift_card_redeem: true }, turnstile: { site_key: 'fixture' } } })
  const load = loader({ ...ui,
    '../../api': { giftCardAPI: { redeem: forbidden } },
    '../../stores/app': { useAppStore: () => ({ config, loadConfig: async () => {} }) },
    '../../components/captcha/ImageCaptcha.vue': { default: { render: () => null } },
  })
  const f = mount(load('views/personal/GiftCardPanel.vue').default)
  try {
    await settle()
    const panel = f.host.querySelector('.gift-card-panel-enter')!
    assert.ok(panel.classList.contains('gift-card-panel-no-lift'), 'known asynchronous SDK host is protected from the first frame')
    assert.ok(target, 'real Turnstile component called inert SDK')
    const fixed = document.createElement('div'); fixed.style.position = 'fixed'
    fixed.append(document.createElement('iframe')); target!.append(fixed)
    await settle()
    assert.ok(panel.contains(fixed))
    assert.ok(panel.classList.contains('gift-card-panel-no-lift'), 'child-only DOM insertion cannot reactivate lift')
    const source = readFileSync(new URL('../src/views/personal/GiftCardPanel.vue', import.meta.url), 'utf8')
    assert.match(source, /\.gift-card-panel-no-lift\s*\{\s*animation-name: gift-card-panel-fade;/)
    const frames = source.match(/@keyframes gift-card-panel-fade\s*\{([\s\S]*?)\n\}/)![1]
    assert.doesNotMatch(frames, /transform:/)
    assert.match(frames, /opacity: 0/); assert.match(frames, /opacity: 1/)
    assert.equal(writes, 0)
  } finally { f.unmount(); delete window.turnstile }
})

test('Api confirmation is physically outside its animated root while entering; no credential operation', async () => {
  Object.defineProperty(globalThis, 'localStorage', { configurable: true, value: window.localStorage })
  localStorage.setItem('api_secret_viewed', '901')
  let writes = 0
  const forbidden = () => { writes++; throw Error('Business operations forbidden') }
  const load = loader({ ...ui, '../../api': { apiCredentialAPI: {
    getMy: async () => ({ data: { data: { id: 901, status: 'approved', api_key: 'fixture', is_active: true } } }),
    regenerate: forbidden, apply: forbidden, update: forbidden,
  } } })
  const f = mount(load('views/personal/ApiPanel.vue').default)
  try {
    await settle()
    const panel = f.host.querySelector('.api-panel-enter')!
    const trigger = [...panel.querySelectorAll('button')].find(b => b.textContent?.trim() === 'personalCenter.apiPanel.regenerate')!
    trigger.click(); await settle()
    const dialog = document.querySelector('[data-api-credential-confirm]')!
    assert.ok(dialog); assert.equal(panel.contains(dialog), false)
    assert.equal(document.querySelector('[data-overlay-root]')!.parentElement, document.body)
    assert.equal(writes, 0)
  } finally { f.unmount(); localStorage.clear() }
})
