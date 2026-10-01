import test from 'node:test'
import assert from 'node:assert/strict'
import { loader, vue, settle } from './helpers/motion17DHarness.ts'

Object.defineProperty(globalThis, 'localStorage', { configurable: true, value: window.localStorage })
const flushOverlay = async () => { await settle(); await new Promise(resolve => setTimeout(resolve, 40)); await settle() }

async function mountApiConfirmation() {
  localStorage.setItem('api_secret_viewed', '901')
  const credential = vue.reactive({ id: 901, api_key: 'SYNTHETIC-NONUSABLE', api_secret_masked: 'masked', status: 'approved', is_active: true })
  let mutations = 0
  const forbidden = () => { mutations++; throw new Error('No business writes in fixture') }
  const load = loader({
    'vue-i18n': { useI18n: () => ({ t: (key: string) => key }) },
    '../../api': { apiCredentialAPI: { getMy: async () => ({ data: { data: credential } }), regenerate: forbidden, apply: forbidden, update: forbidden } },
    '@/components/ui/button': { Button: 'button' },
    '@/components/ui/alert': { Alert: 'div', AlertDescription: 'div' },
    '@/components/ui/badge': { Badge: 'span' },
  })
  const component = load('views/personal/ApiPanel.vue').default
  const host = document.createElement('div')
  document.body.append(host)
  const app = vue.createApp(component)
  app.mount(host)
  await settle()
  const trigger = [...host.querySelectorAll('button')].find(button => button.textContent?.trim() === 'personalCenter.apiPanel.regenerate')!
  assert.ok(trigger, 'approved viewed credential offers its existing regenerate confirmation')
  trigger.focus()
  trigger.click()
  await flushOverlay()
  return { credential, trigger, mutations: () => mutations, unmount() { app.unmount(); host.remove(); localStorage.clear() } }
}

test('API confirmation joins overlay focus/scroll ownership and Escape cancels without rotating', async () => {
  const f = await mountApiConfirmation()
  try {
    assert.equal(document.body.style.overflow, 'hidden')
    assert.equal(document.documentElement.style.overflow, 'hidden')
    const dialog = document.querySelector('[data-api-credential-confirm]') as HTMLElement
    assert.ok(dialog)
    assert.equal(dialog.getAttribute('role'), 'alertdialog')
    assert.ok(dialog.contains(document.activeElement))
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }))
    await flushOverlay()
    assert.equal(document.querySelector('[data-api-credential-confirm]'), null)
    assert.equal(document.body.style.overflow, '')
    assert.equal(document.activeElement, f.trigger)
    assert.equal(f.mutations(), 0)
  } finally { f.unmount() }
})

test('credential revocation removes API confirmation and releases only its ownership', async () => {
  const f = await mountApiConfirmation()
  try {
    assert.equal(document.body.style.overflow, 'hidden')
    f.credential.status = 'disabled'
    await flushOverlay()
    assert.equal(document.querySelector('[data-api-credential-confirm]'), null)
    assert.equal(document.body.style.overflow, '')
    assert.equal(document.documentElement.style.overflow, '')
    assert.equal(f.mutations(), 0)
  } finally { f.unmount() }
})

test('API panel unmount releases the live confirmation without a business write', async () => {
  const f = await mountApiConfirmation()
  assert.equal(document.body.style.overflow, 'hidden')
  f.unmount()
  await flushOverlay()
  assert.equal(document.querySelector('[data-api-credential-confirm]'), null)
  assert.equal(document.body.style.overflow, '')
  assert.equal(document.documentElement.style.overflow, '')
  assert.equal(f.mutations(), 0)
})
