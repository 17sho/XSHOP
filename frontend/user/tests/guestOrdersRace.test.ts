import test from 'node:test'
import assert from 'node:assert/strict'
import { evaluate, deferred } from './helpers/sourceRuntime.ts'
import { debounceAsync } from '../src/utils/debounce.ts'

function setup() {
  const pending: ReturnType<typeof deferred>[] = []
  const calls: string[] = []
  const request = () => { const d = deferred(); pending.push(d); return d.promise }
  let unmount = () => {}
  const { useGuestOrders } = evaluate('src/composables/useGuestOrders.ts', ['useGuestOrders'], {
    useAppStore: () => ({ config: { captcha: { provider: 'none' } }, loadConfig: async () => {} }),
    useFormValidation: () => ({ emailRule: () => (value: string) => value.includes('@') ? null : 'error.email_invalid' }),
    useI18n: () => ({ t: (s: string) => s }), guestOrderAPI: {
      browserOrders: () => { calls.push('browser'); return request() },
      list: () => { calls.push('credentials'); return request() },
    }, debounceAsync,
    loadGuestOrderAuth: () => ({ email: '', order_password: '' }), saveGuestOrderAuth() {}, clearGuestOrderAuth() {},
    onUnmounted: (fn: () => void) => { unmount = fn },
  })
  return { g: useGuestOrders(), pending, calls, unmount: () => unmount() }
}
const response = (id: string) => ({ data: { data: [{ order_no: id }], pagination: { page: 1, total: 1, total_page: 1, page_size: 20 } } })
for (const action of ['mode', 'clear', 'unmount']) test(`FE04: ${action} invalidates pending guest responses and errors`, async () => {
  const { g, pending, unmount } = setup()
  const first = g.loadBrowserOrders()
  if (action === 'mode') g.setActiveTab('credentials')
  if (action === 'clear') g.clearSaved()
  if (action === 'unmount') unmount()
  pending[0]!.resolve(response('stale')); await first
  assert.deepEqual(g.orders.value, [])
  assert.equal(g.loading.value, false)
  assert.equal(g.error.value, '')
})
test('FE04: newest request owns results, errors and loading', async () => {
  const { g, pending } = setup()
  const old = g.loadBrowserOrders(); const fresh = g.loadBrowserOrders()
  pending[0]!.reject(new Error('stale error')); await old
  assert.equal(g.loading.value, true); assert.equal(g.error.value, '')
  pending[1]!.resolve(response('new')); await fresh
  assert.equal(g.orders.value[0].order_no, 'new'); assert.equal(g.loading.value, false)
})
test('FE04: a replacement credential search invalidates responses during its debounce window', async () => {
  Object.assign(globalThis, { window: { setTimeout, clearTimeout } })
  const { g, pending } = setup()
  g.setActiveTab('credentials'); g.email.value = 'old@example.invalid'; g.orderPassword.value = 'fixture'
  const old = g.searchByCredentials()
  await new Promise(resolve => setTimeout(resolve, 350))
  g.email.value = 'new@example.invalid'
  const fresh = g.searchByCredentials()
  pending[0]!.resolve(response('old-query')); await old
  try {
    assert.deepEqual(g.orders.value, [], 'old query must not appear under replacement credentials')
  } finally {
    g.clearSaved(); await fresh
  }
})
test('FE04: an invalid replacement credential query cannot revive an earlier response', async () => {
  Object.assign(globalThis, { window: { setTimeout, clearTimeout } })
  const { g, pending } = setup()
  g.setActiveTab('credentials'); g.email.value = 'fixture@example.invalid'; g.orderPassword.value = 'fixture'
  const old = g.searchByCredentials()
  await new Promise(resolve => setTimeout(resolve, 350))
  g.orderPassword.value = ''
  await g.searchByCredentials()
  pending[0]!.resolve(response('old-query')); await old
  assert.deepEqual(g.orders.value, [])
  assert.equal(g.error.value, 'guestOrders.errors.missing')
})
test('FE04: switching mode cancels scheduled credential work', async () => {
  Object.assign(globalThis, { window: { setTimeout, clearTimeout } })
  const { g, pending, calls } = setup()
  g.setActiveTab('credentials'); g.email.value = 'fixture@example.invalid'; g.orderPassword.value = 'fixture'
  const search = g.searchByCredentials(); g.setActiveTab('browser')
  await new Promise(resolve => setTimeout(resolve, 350))
  pending.forEach(d => d.resolve(response('unwanted')))
  await search
  assert.deepEqual(calls, ['browser'], 'only the new browser tab may start work')
})
