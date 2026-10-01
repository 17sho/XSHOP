import test from 'node:test'
import assert from 'node:assert/strict'
import { vue, dom, loadSource, settle } from './helpers/motion17CHarness.ts'

const auth = { email: 'fixture@example.invalid', order_password: 'fixture-password' }
const response = (orderNo = 'ORDER-1', page = 1, totalPage = 3) => ({ data: {
  data: [{ order_no: orderNo }], pagination: { page, page_size: 20, total: 41, total_page: totalPage },
} })
function setup(saved = false) {
  dom.window.sessionStorage.clear(); dom.window.localStorage.clear()
  if (saved) dom.window.sessionStorage.setItem('guest_order_auth', JSON.stringify(auth))
  const calls: any[] = []
  const guestOrderAPI = loadSource('api/order.ts', { './client': { userApi: {
    get: (url: string, options: any) => new Promise((resolve, reject) => calls.push({ url, options, resolve, reject })),
  } } }).guestOrderAPI
  const { useGuestOrders } = loadSource('composables/useGuestOrders.ts', {
    'vue-i18n': { useI18n: () => ({ t: (key: string) => key }) }, '../api': { guestOrderAPI },
  })
  // Exercise the actual debounce implementation without wall-clock sleeps.
  const timers = new Map<number, Function>(); let timerId = 0
  const oldSet = window.setTimeout; const oldClear = window.clearTimeout
  window.setTimeout = ((fn: Function) => { timers.set(++timerId, fn); return timerId }) as any
  window.clearTimeout = ((id: number) => timers.delete(id)) as any
  let g: any
  const app = vue.createApp({ setup() { g = useGuestOrders(); return () => null } })
  const host = document.createElement('div'); document.body.append(host); app.mount(host)
  const flush = () => { const work = [...timers.values()]; timers.clear(); work.forEach(fn => void fn()) }
  return { g, calls, guestOrderAPI, flush, timers, close: () => {
    app.unmount(); host.remove(); window.setTimeout = oldSet; window.clearTimeout = oldClear
  } }
}

test('email lookup sends only pagination and Guest authorization, never an order-number filter', async () => {
  const h = setup()
  try {
    h.g.setActiveTab('credentials'); h.g.email.value = auth.email; h.g.orderPassword.value = auth.order_password
    const search = h.g.searchByCredentials(); h.flush()
    const call = h.calls[1]
    assert.equal(call.url, '/guest/orders')
    assert.deepEqual(call.options.params, { page: 1, page_size: 20 })
    assert.equal(call.options.headers.Authorization, `Guest ${Buffer.from(auth.email + '\n' + auth.order_password).toString('base64url')}`)
    call.resolve(response()); await search
    assert.equal(h.g.orders.value[0].order_no, 'ORDER-1')
    assert.deepEqual(JSON.parse(dom.window.sessionStorage.getItem('guest_order_auth')!), auth)
    assert.equal('orderNo' in h.g, false)
    assert.equal('searchByOrderNo' in h.g, false)
  } finally { h.close() }
})

test('browser lookup includes cookies, loads saved credentials, and paginates without credentials', async () => {
  const h = setup(true)
  try {
    assert.equal(h.g.activeTab.value, 'browser')
    assert.equal(h.g.email.value, auth.email); assert.equal(h.g.orderPassword.value, auth.order_password)
    assert.equal(h.g.hasSavedAuth.value, true)
    assert.deepEqual(h.calls[0].options, { params: { page: 1, page_size: 20 }, credentials: 'include' })
    assert.equal(h.calls[0].url, '/guest/orders/browser')
    h.calls[0].resolve(response()); await settle()
    assert.equal(h.g.orders.value[0].order_no, 'ORDER-1'); assert.equal(h.g.loading.value, false)
    h.g.changePage(0); h.g.changePage(4); assert.equal(h.calls.length, 1)
    h.g.changePage(2)
    assert.deepEqual(h.calls[1].options, { params: { page: 2, page_size: 20 }, credentials: 'include' })
    h.calls[1].resolve(response('ORDER-2', 2)); await settle()
    assert.equal(h.g.pagination.value.page, 2); assert.equal(h.g.orders.value[0].order_no, 'ORDER-2')
  } finally { h.close() }
})

for (const missing of ['email', 'orderPassword']) test(`email lookup requires ${missing} before any credential request or persistence`, async () => {
  const h = setup()
  try {
    h.g.setActiveTab('credentials'); h.g.email.value = auth.email; h.g.orderPassword.value = auth.order_password
    h.g[missing].value = ''
    await h.g.searchByCredentials(); h.flush()
    assert.equal(h.calls.length, 1); assert.equal(h.timers.size, 0)
    assert.equal(h.g.error.value, 'guestOrders.errors.missing'); assert.equal(h.g.loading.value, false)
    assert.equal(dom.window.sessionStorage.getItem('guest_order_auth'), null)
    h.calls[0].resolve(response('STALE-BROWSER')); await settle()
    assert.deepEqual(h.g.orders.value, []); assert.equal(h.g.error.value, 'guestOrders.errors.missing')
  } finally { h.close() }
})

test('credential pagination preserves auth and ignores stale page success and failure after switching to browser', async () => {
  const h = setup(true)
  try {
    h.g.setActiveTab('credentials')
    const search = h.g.searchByCredentials(); h.flush(); h.calls[1].resolve(response()); await search
    h.g.changePage(0); h.g.changePage(4); h.flush(); assert.equal(h.calls.length, 2)
    h.g.changePage(2); h.flush()
    assert.deepEqual(h.calls[2].options.params, { page: 2, page_size: 20 })
    assert.deepEqual(h.calls[2].options.headers, h.calls[1].options.headers)
    h.calls[2].resolve(response('PAGE-2', 2)); await settle()
    assert.equal(h.g.pagination.value.page, 2); assert.equal(h.g.orders.value[0].order_no, 'PAGE-2')
    h.g.changePage(3); h.flush(); h.g.changePage(1); h.flush()
    h.g.setActiveTab('browser')
    h.calls[3].resolve(response('STALE-PAGE', 3)); h.calls[4].reject(new Error('stale page failure')); await settle()
    assert.deepEqual(h.g.orders.value, []); assert.equal(h.g.error.value, ''); assert.equal(h.g.loading.value, true)
    h.calls[5].resolve(response('CURRENT-BROWSER')); await settle()
    assert.equal(h.g.orders.value[0].order_no, 'CURRENT-BROWSER'); assert.equal(h.g.loading.value, false)
  } finally { h.close() }
})

for (const mode of ['browser', 'credentials']) test(`${mode} lookup preserves empty state and request errors`, async () => {
  const h = setup(true)
  try {
    let index = 0
    if (mode === 'credentials') { h.g.setActiveTab(mode); void h.g.searchByCredentials(); h.flush(); index = 1 }
    h.calls[index].resolve({ data: { data: [] } }); await settle()
    assert.deepEqual(h.g.orders.value, []); assert.equal(h.g.error.value, '')
    assert.equal(h.g.emptyMessage.value, mode === 'browser' ? 'guestOrders.browserEmpty' : 'guestOrders.empty')
    h.g.changePage(1); h.flush()
    h.calls[index + 1].reject(new Error('request denied')); await settle()
    assert.deepEqual(h.g.orders.value, []); assert.equal(h.g.error.value, 'request denied'); assert.equal(h.g.loading.value, false)
    h.g.changePage(1); h.flush()
    h.calls[index + 2].reject({}); await settle()
    assert.equal(h.g.error.value, mode === 'browser' ? 'guestOrders.errors.browserFailed' : 'guestOrders.errors.searchFailed')
  } finally { h.close() }
})

test('generic guest API still accepts order filters and exact encoded detail/payment order identities', async () => {
  const h = setup()
  try {
    const orderNo = 'EXACT /?订单'
    const filtered = h.guestOrderAPI.list({ ...auth, order_no: orderNo, page: 2 })
    assert.deepEqual(h.calls[1].options.params, { order_no: orderNo, page: 2 })
    h.calls[1].resolve(response(orderNo)); await filtered
    const detail = h.guestOrderAPI.detail(orderNo, auth)
    assert.equal(h.calls[2].url, '/guest/orders/' + encodeURIComponent(orderNo))
    assert.deepEqual(h.calls[2].options.params, {})
    assert.deepEqual(h.calls[2].options.headers, h.calls[1].options.headers)
    h.calls[2].resolve(response(orderNo)); await detail
    const payment = h.guestOrderAPI.latestPayment({ ...auth, order_no: orderNo })
    assert.equal(h.calls[3].url, '/guest/payments/latest')
    assert.deepEqual(h.calls[3].options.params, { order_no: orderNo })
    assert.deepEqual(h.calls[3].options.headers, h.calls[1].options.headers)
    h.calls[3].resolve({ data: { data: null } }); await payment
  } finally { h.close() }
})
