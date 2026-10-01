import test from 'node:test'
import assert from 'node:assert/strict'
import { vue, dom, loadSource, settle } from './helpers/motion17CHarness.ts'

test('actual OrdersPanel local tabs preserve navigation identity without custom animation', async () => {
  let reduce = false
  const listeners = new Set<() => void>()
  Object.defineProperty(window, 'matchMedia', { configurable: true, value: () => ({ matches: reduce, addEventListener: (_: string, fn: () => void) => listeners.add(fn), removeEventListener: (_: string, fn: () => void) => listeners.delete(fn) }) })
  const plays: HTMLElement[] = []; let cancels = 0
  ;(dom.window.HTMLElement.prototype as any).animate = function () { plays.push(this); return { cancel() { cancels++ } } }
  const pass = vue.defineComponent({ setup(_: any, { slots }: any) { return () => vue.h('div', slots.default?.()) } })
  const controls = Object.fromEntries(['Badge', 'Button', 'Label', 'Input', 'Select', 'SelectContent', 'SelectItem', 'SelectTrigger', 'SelectValue'].map(k => [k, pass]))
  const request = async () => ({ data: { data: [], pagination: { page: 1, total: 0, total_page: 1, page_size: 20 } } })
  const mocks: Record<string, any> = {
    'vue-i18n': { useI18n: () => ({ t: (s: string) => s }) },
    '../../stores/userAuth': { useUserAuthStore: () => vue.reactive({ sessionGeneration: 0 }) },
    '../../api': { userOrderAPI: { list: request, stats: request } }, '../../api/wallet': { walletAPI: { rechargeOrders: request, rechargeStats: request } },
  }
  for (const path of ['badge', 'button', 'label', 'input', 'select']) mocks['@/components/ui/' + path] = controls
  for (const path of ['EmptyState', 'PaginationNav', 'shared/PanelHeading', 'shared/StatCard']) mocks['../../components/' + path + '.vue'] = { default: pass }
  const Panel = loadSource('views/personal/OrdersPanel.vue', mocks).default
  const host = document.createElement('div'); document.body.append(host)
  const app = vue.createApp(Panel); app.component('router-link', pass); app.mount(host); await settle()
  try {
    assert.equal(plays.length, 0, 'initial route entry must remain owned by App')
    const tabs = [...host.querySelectorAll('button')]
    const product = tabs.find(el => el.textContent === 'orders.tabs.product')!
    const recharge = tabs.find(el => el.textContent === 'orders.tabs.recharge')!
    recharge.click(); await settle()
    assert.equal(plays.length, 0)
    assert.equal(host.contains(product), true); assert.equal(host.contains(recharge), true)
    product.click(); await settle(); assert.equal(plays.length, 0)
    product.click(); await settle(); assert.equal(plays.length, 0)
    reduce = true; listeners.forEach(fn => fn())
    recharge.click(); await settle(); assert.equal(plays.length, 0); assert.equal(cancels, 0)
  } finally { app.unmount(); host.remove() }
  assert.equal(listeners.size, 0)
})
