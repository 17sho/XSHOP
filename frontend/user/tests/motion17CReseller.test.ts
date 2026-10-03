import assert from 'node:assert/strict'
import test from 'node:test'
import { createRequire } from 'node:module'
import { dom, vue, loadSource, settle } from './helpers/motion17CHarness.ts'
const require = createRequire(import.meta.url)
const { createRouter, createMemoryHistory } = require('vue-router')

test('actual reseller shell retains chrome and query drafts across lazy child navigation', async () => {
  dom.window.matchMedia = (() => ({ matches: false, addEventListener() {}, removeEventListener() {} })) as any
  const events: any[] = []
  dom.window.HTMLElement.prototype.animate = function() { const record = { target: this, cancelled: false }; events.push(record); return { cancel: () => { record.cancelled = true } } as any }
  let loads = 0
  const wrapper = { setup: (_: any, { slots }: any) => () => vue.h('div', slots.default?.()) }
  const Empty = { setup: () => () => null }
  const Shell = loadSource('views/reseller/ResellerConsoleLayout.vue', {
    'vue-i18n': { useI18n: () => ({ t: (key: string) => key }) }, '@/components/ui/button': { Button: wrapper }, '@/components/ui/card': { Card: wrapper },
    '../../components/reseller-console/ResellerConsoleTopbar.vue': { default: { setup: () => () => vue.h('header', 'Reseller topbar') } },
    '../../components/reseller-console/ResellerPageState.vue': { default: Empty },
    '../../composables/reseller/useResellerProfile': { useResellerProfile: () => ({ loading: vue.ref(false), state: vue.ref({ modules: { orders: { enabled: true }, site: { enabled: true } } }), load: async () => { loads++ } }) },
  }).default
  const mocks: Record<string, any> = { './templates/registry': { getActiveTemplate: () => 'classic' }, './components/ErrorBoundary.vue': { default: wrapper } }
  for (const name of ['Navbar', 'Toast', 'ConfirmDialog', 'BackToTop', 'MobileBottomNav', 'CustomFooterLinks']) mocks[`./components/${name}.vue`] = { default: Empty }
  const App = loadSource('App.vue', mocks).default
  const page = (title: string) => ({ setup: () => () => vue.h('section', [title, vue.h('input', { value: 'persistent draft' })]) })
  let release!: (value: any) => void
  const delayed = new Promise(resolve => { release = resolve })
  const router = createRouter({ history: createMemoryHistory(), routes: [
    { path: '/', component: page('Public') },
    { path: '/reseller', component: Shell, meta: { resellerConsole: true }, children: [
      { path: '', component: page('Dashboard') }, { path: 'orders', component: () => delayed }, { path: 'site', component: page('Site') },
      ...['finance', 'ledger', 'withdraws', 'domains', 'products', 'apply'].map(path => ({ path, component: page(path) })),
    ] },
  ] })
  await router.push('/'); await router.isReady()
  const host = document.createElement('div'); document.body.append(host)
  const app = vue.createApp(App); app.use(router); app.mount(host)
  try {
    await settle(); assert.equal(events.length, 0)
    await router.push('/reseller'); await settle(); await new Promise(resolve => setTimeout(resolve, 30)); await settle()
    assert.equal(events.length, 0)
    const header = host.querySelector('header'), aside = host.querySelector('aside')
    const navigation = router.push('/reseller/orders'); await settle()
    assert.equal(events.length, 0, 'unresolved lazy child cannot trigger another entrance')
    release(page('Orders')); await navigation; await settle()
    assert.equal(events.length, 0)
    assert.match(host.textContent!, /Orders/)
    assert.equal(host.querySelector('header'), header); assert.equal(host.querySelector('aside'), aside)
    const input = host.querySelector('input')
    await router.replace('/reseller/orders?status=all'); await settle()
    assert.equal(events.length, 0); assert.equal(host.querySelector('input'), input)
    await router.push('/reseller/site'); await settle()
    assert.match(host.textContent!, /Site/)
    assert.equal(host.querySelector('header'), header); assert.equal(host.querySelector('aside'), aside)
    assert.equal(events.length, 0)
    assert.equal(loads, 1)
  } finally { app.unmount(); host.remove() }
})
