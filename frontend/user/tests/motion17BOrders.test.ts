import test from 'node:test'
import assert from 'node:assert/strict'
import { reactive } from 'vue'
import { runtime, deferred } from './helpers/motion17BRuntime.ts'
import { debounceAsync } from '../src/utils/debounce.ts'
Object.assign(globalThis, { window: { setTimeout, clearTimeout } })
function setup(kind: 'order' | 'recharge') {
  const list: any[] = [], stats: any[] = []
  const request = (queue: any[]) => (params: any) => { const d = deferred(); queue.push({ ...d, params }); return d.promise }
  const auth = reactive({ sessionGeneration: 0, token: 'A' })
  const names = ['loadOrders', 'loadRechargeOrders', 'orders', 'rechargeOrders', 'orderStats', 'rechargeStats', 'orderLoading', 'rechargeLoading', 'orderPagination', 'rechargePagination', 'orderFilters', 'rechargeFilters', 'handleOrderNoInput', 'handleRechargeNoInput', 'changeOrderPage', 'changeRechargePage', 'orderStatusProxy', 'rechargeStatusProxy', 'switchTab', 'applyOrderFilters', 'applyRechargeFilters']
  const r = runtime('src/views/personal/OrdersPanel.vue', names, {
    useI18n: () => ({ t: (s: string) => s }), debounceAsync,
    userOrderAPI: { list: request(list), stats: request(stats) },
    walletAPI: { rechargeOrders: request(list), rechargeStats: request(stats) },
    useUserAuthStore: () => auth,
  })
  const product = kind === 'order'
  return { r, list, stats, auth, load: product ? r.loadOrders : r.loadRechargeOrders,
    rows: product ? r.orders : r.rechargeOrders, counts: product ? r.orderStats : r.rechargeStats,
    loading: product ? r.orderLoading : r.rechargeLoading }
}
const response = (value: string) => ({ data: { data: [value], pagination: { page: 1, page_size: 20, total: 1, total_page: 5 } } })
const summary = (n: number) => ({ data: { data: { by_status: { pending: n } } } })
const flush = async () => { for (let i = 0; i < 6; i++) await Promise.resolve() }
for (const kind of ['order', 'recharge'] as const) {
  for (const action of ['page', 'status', 'apply', 'tab']) test(`${kind}: ${action} is immediate and cancels queued typing`, async () => {
    const s = setup(kind), r = s.r, product = kind === 'order'
    const pagination = product ? r.orderPagination : r.rechargePagination
    pagination.value.total_page = 5
    if (!product) r.switchTab('recharge')
    const before = s.list.length
    const queued = product ? r.handleOrderNoInput() : r.handleRechargeNoInput()
    if (action === 'page') (product ? r.changeOrderPage : r.changeRechargePage)(2)
    if (action === 'status') (product ? r.orderStatusProxy : r.rechargeStatusProxy).value = 'paid'
    if (action === 'apply') (product ? r.applyOrderFilters : r.applyRechargeFilters)()
    if (action === 'tab') r.switchTab(product ? 'recharge' : 'product')
    try {
      assert.equal(s.list.length, before + 1, 'explicit action starts immediately')
      await new Promise(resolve => setTimeout(resolve, 340))
      assert.equal(s.list.length, before + 1, 'queued search cannot replay')
    } finally { s.r.dispose(); s.list.forEach(d => d.resolve(response('cleanup'))); s.stats.forEach(d => d.resolve(summary(0))); await queued }
  })
  for (const action of ['dispose', 'account', 'typing']) test(`${kind}: ${action} invalidates pending list and stats`, async () => {
    const s = setup(kind); const pending = s.load()
    if (action === 'dispose') s.r.dispose()
    if (action === 'account') s.auth.sessionGeneration++
    if (action === 'typing') (kind === 'order' ? s.r.handleOrderNoInput : s.r.handleRechargeNoInput)()
    s.list[0].resolve(response('stale')); s.stats[0].resolve(summary(8)); await pending; await flush()
    assert.deepEqual(s.rows.value, []); assert.deepEqual(s.counts.value, {})
    s.r.dispose()
  })
}
for (const kind of ['order', 'recharge'] as const) {
  test(`${kind}: latest list and stats own results, stale errors and finally cannot reset newer state`, async () => {
    const s = setup(kind)
    const first = s.load(); s.list[0].resolve(response('first')); await first; await flush()
    const second = s.load(); s.list[1].resolve(response('second')); await second; await flush()
    s.stats[1].resolve(summary(2)); await flush(); s.stats[0].reject(new Error('old stats')); await flush()
    assert.deepEqual(s.counts.value, { pending: 2 })
    const third = s.load(); const fourth = s.load()
    s.list[2].reject(new Error('old list')); await third
    assert.equal(s.loading.value, true)
    assert.deepEqual(s.rows.value, ['second'])
    s.list[3].resolve(response('fourth')); await fourth
    assert.deepEqual(s.rows.value, ['fourth'])
    s.r.dispose()
  })
  test(`${kind}: returning to a tab reloads stats invalidated by tab departure`, async () => {
    const s = setup(kind)
    if (kind === 'recharge') s.r.switchTab('recharge')
    else s.load()
    s.list[0].resolve(response('rows')); await flush()
    s.r.switchTab(kind === 'order' ? 'recharge' : 'product')
    s.stats[0].resolve(summary(7)); await flush()
    s.r.switchTab(kind === 'order' ? 'product' : 'recharge')
    assert.equal(s.list.length, 3); assert.equal(s.stats.length, 3)
    s.list[2].resolve(response('returned')); s.stats[2].resolve(summary(9)); await flush()
    assert.deepEqual(s.counts.value, { pending: 9 }); s.r.dispose()
  })
  test(`${kind}: newest stats survive stale success and newest errors settle normally`, async () => {
    const s = setup(kind); const old = s.load(); const fresh = s.load()
    s.stats[1].resolve(summary(2)); await flush(); s.stats[0].resolve(summary(1)); await flush()
    assert.deepEqual(s.counts.value, { pending: 2 })
    s.list[1].reject(new Error('current')); await fresh
    assert.deepEqual(s.rows.value, []); assert.equal(s.loading.value, false)
    s.list[0].resolve(response('old')); await old
    assert.deepEqual(s.rows.value, []); s.r.dispose()
  })
  test(`${kind}: out-of-order success cannot replace the latest list`, async () => {
    const s = setup(kind); const old = s.load(); const fresh = s.load()
    s.list[1].resolve(response('fresh')); await fresh
    s.list[0].resolve(response('old')); await old
    assert.deepEqual(s.rows.value, ['fresh']); s.r.dispose()
  })
}
