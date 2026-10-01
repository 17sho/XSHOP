import { debounceAsync } from '../src/utils/debounce.ts'
import test from 'node:test'
import assert from 'node:assert/strict'
import { harness, settle, response, deferred } from './helpers/motion17AHarness.ts'
for (const kind of ['detail', 'latest', 'wallet', 'channels']) for (const exit of ['replace', 'unmount']) test(`payment stale ${kind} after ${exit} cannot own next state`, async () => {
  const h = harness('usePayment', {}, { order_no: 'A' }); h.mount()
  try {
    if (kind === 'latest' || kind === 'channels') {
      details(h)[0].resolve(response({ order_no: 'A', status: 'pending_payment', total_amount: '10.00', payable_amount: '10.00' })); await settle()
    }
    const old = h.calls.find((c: any) => c.kind === kind)
    assert.ok(old, `fixture must dispatch ${kind}`)
    if (exit === 'replace') h.route.query = { order_no: 'B', guest: '1' }
    else h.unmount()
    await settle()
    const before = h.state.order.value
    old.resolve(response(kind === 'detail' ? { order_no: 'A', status: 'paid' } : kind === 'latest' ? { payment_id: 1, pay_url: 'https://synthetic.invalid/old', interaction_mode: 'redirect' } : kind === 'wallet' ? { balance: '999' } : [{ id: 777 }]))
    await settle()
    assert.equal(h.state.order.value, before)
    assert.equal(h.state.paymentResult.value, null)
    assert.equal(h.state.walletBalance.value, '0')
    assert.equal(h.state.channels.value.some((c: any) => c.id === 777), false)
    assert.equal(h.redirects.length, 0)
    if (exit === 'replace') assert.equal(h.state.loading.value, true)
  } finally { h.unmount() }
})
for (const exit of ['replace', 'unmount']) test(`payment poll capture continuation after ${exit} must not issue another read`, async () => {
  const h = harness('usePayment', {}, { order_no: 'A' }); h.mount()
  try {
    details(h)[0].resolve(response({ order_no: 'A', status: 'pending_payment' })); await settle()
    h.calls.find((c: any) => c.kind === 'latest').resolve(response({ payment_id: 1, provider_type: 'official', channel_type: 'wechat', qr_code: 'synthetic' })); await settle()
    const capture = h.calls.find((c: any) => c.kind === 'capture')
    capture.resolve(response({})); await settle()
    void [...h.timers.values()][0](); await settle()
    const pollCapture = h.calls.filter((c: any) => c.kind === 'capture').at(-1)
    if (exit === 'replace') h.route.query = { order_no: 'B' }
    else h.unmount()
    await settle(); const before = details(h).length
    pollCapture.resolve(response({})); await settle()
    assert.equal(details(h).length, before)
  } finally { h.unmount() }
})
for (const exit of ['replace', 'unmount']) test(`payment router push rejection after ${exit} cannot fallback navigate`, async () => {
  const push = deferred()
  const h = harness('usePayment', { useRouter: () => ({ resolve: () => ({ matched: [{}] }), push: () => push.promise }) }, { order_no: 'A' }); h.mount()
  try {
    details(h)[0].resolve(response({ order_no: 'A', status: 'paid' })); await settle()
    const timer = [...h.timers.values()][0]; assert.ok(timer); void timer(); await settle()
    if (exit === 'replace') h.route.query = { order_no: 'B' }
    else h.unmount()
    await settle(); push.reject(new Error('synthetic failed navigation')); await settle()
    assert.equal(h.redirects.length, 0)
  } finally { h.unmount() }
})
for (const channel of ['paypal', 'stripe']) for (const exit of ['replace', 'unmount']) test(`${channel} return capture after ${exit} cannot clean a newer route`, async () => {
  const h = harness('usePayment', {}, { order_no: 'A' }); h.mount()
  try {
    details(h)[0].resolve(response({ order_no: 'A', status: 'pending_payment' })); await settle()
    h.calls.find((c: any) => c.kind === 'latest').resolve(response({ payment_id: 9, provider_type: 'official', channel_type: channel, pay_url: 'https://synthetic.invalid/pay' })); await settle()
    h.route.query = { order_no: 'A', [channel === 'paypal' ? 'pp_return' : 'stripe_return']: '1' }; h.route.fullPath = '/pay?return=1'; await settle()
    const capture = h.calls.find((c: any) => c.kind === 'capture'); assert.ok(capture)
    const oldReads = details(h).slice(1)
    if (exit === 'replace') h.route.query = { order_no: 'B', tracking: 'preserve' }
    else h.unmount()
    await settle(); const before = details(h).length
    capture.resolve(response({ committed: true })); oldReads.forEach((c: any) => c.resolve(response({ order_no: 'A', status: 'paid' }))); await settle()
    assert.equal(details(h).length, before, 'old capture cannot reload the new identity')
    assert.equal(h.redirects.length, 0, 'old capture/sync cannot erase new query markers')
    assert.equal(h.calls.filter((c: any) => c.kind === 'capture').length, 1)
  } finally { h.unmount() }
})
for (const exit of ['replace', 'unmount']) test(`payment creation after ${exit} preserves server outcome without display/redirect or duplicate dispatch`, async () => {
  const h = harness('usePayment', {}, { order_no: 'A', guest: '1' }); h.mount()
  try {
    details(h)[0].resolve(response({ order_no: 'A', status: 'pending_payment', total_amount: '10.00' })); await settle()
    h.state.selectedChannelId.value = 7
    void h.state.handlePayment(); void h.state.handlePayment(); await settle()
    const creates = h.calls.filter((c: any) => c.kind === 'create'); assert.equal(creates.length, 1)
    if (exit === 'replace') h.route.query = { order_no: 'B', guest: '1' }
    else h.unmount()
    await settle(); creates[0].resolve(response({ payment_id: 9, pay_url: 'https://synthetic.invalid/committed', interaction_mode: 'redirect' })); await settle()
    assert.equal(h.state.paymentResult.value, null); assert.equal(h.redirects.length, 0)
    assert.equal(h.calls.filter((c: any) => c.kind === 'cancel').length, 0)
  } finally { h.unmount() }
})
test('return markers survive until provider capture completes, with no duplicate capture on A -> B -> A', async () => {
  const h = harness('usePayment', {}, { order_no: 'A' }); h.mount()
  try {
    details(h)[0].resolve(response({ order_no: 'A', status: 'pending_payment' })); await settle()
    h.calls.find((c: any) => c.kind === 'latest').resolve(response({ payment_id: 9, provider_type: 'official', channel_type: 'paypal', pay_url: 'https://synthetic.invalid/pay' })); await settle()
    h.route.query = { order_no: 'A', pp_return: '1' }; h.route.fullPath = '/pay?A-return'; await settle()
    const capture = h.calls.find((c: any) => c.kind === 'capture'); assert.ok(capture)
    h.route.query = { order_no: 'B' }; await settle()
    h.route.query = { order_no: 'A', pp_return: '1' }; h.route.fullPath = '/pay?A-return-again'; await settle()
    const a = details(h).filter((c: any) => c.args[0] === 'A').at(-1)
    a.resolve(response({ order_no: 'A', status: 'pending_payment' })); await settle()
    h.calls.filter((c: any) => c.kind === 'latest').at(-1).resolve(response({ payment_id: 9, provider_type: 'official', channel_type: 'paypal', pay_url: 'https://synthetic.invalid/pay' })); await settle()
    assert.equal(h.calls.filter((c: any) => c.kind === 'capture').length, 1)
    assert.equal(h.route.query.pp_return, '1')
    capture.resolve(response({ committed: true })); await settle()
    details(h).at(-1).resolve(response({ order_no: 'A', status: 'pending_payment' })); await settle()
    assert.equal(h.route.query.pp_return, undefined)
    assert.equal(h.state.order.value.order_no, 'A'); assert.equal(h.state.paymentResult.value.payment_id, 9)
  } finally { h.unmount() }
})
test('provider return arriving before latest-payment response cannot be consumed by generic sync', async () => {
  const h = harness('usePayment', {}, { order_no: 'A' }); h.mount()
  try {
    details(h)[0].resolve(response({ order_no: 'A', status: 'pending_payment' })); await settle()
    h.route.query = { order_no: 'A', stripe_return: '1' }; h.route.fullPath = '/pay?stripe_return=1'; await settle()
    details(h).slice(1).forEach((c: any) => c.resolve(response({ order_no: 'A', status: 'pending_payment' }))); await settle()
    const latest = h.calls.filter((c: any) => c.kind === 'latest')
    latest.forEach((c: any) => c.resolve(response({ payment_id: 2, provider_type: 'official', channel_type: 'stripe', pay_url: 'https://synthetic.invalid/pay' }))); await settle()
    assert.equal(h.route.query.stripe_return, '1', 'capture has not completed')
    assert.equal(h.calls.filter((c: any) => c.kind === 'capture').length, 1)
  } finally { h.unmount() }
})
test('payment restore uses shared reduced-motion-aware scroll policy', () => {
  let scrolls = 0
  const h = harness('usePayment', { scrollPageToTop: () => { scrolls++ } }, { order_no: 'A' })
  try {
    h.state.cachedPayment.value = { payment_id: 1, pay_url: 'https://synthetic.invalid' }
    h.state.restoreCachedPayment()
    assert.equal(scrolls, 1)
  } finally { h.unmount() }
})
test('manual refresh continuation cannot read replacement identity', async () => {
  const h = harness('usePayment', {}, { order_no: 'A' }); h.mount()
  try {
    details(h)[0].resolve(response({ order_no: 'A', status: 'pending_payment' })); await settle()
    h.state.paymentResult.value = { payment_id: 1, provider_type: 'official', channel_type: 'wechat' }; await settle()
    void h.state.handleRefresh(); await settle()
    const capture = h.calls.find((c: any) => c.kind === 'capture'); assert.ok(capture)
    h.route.query = { order_no: 'B' }; await settle(); const before = h.calls.length
    capture.resolve(response({ committed: true })); await settle()
    assert.equal(h.calls.length, before)
  } finally { h.unmount() }
})
for (const name of ['usePayment', 'useRechargeOrderDetail']) test(`${name} discarded QR generation cannot update after disposal`, async () => {
  const qr = deferred()
  const h = harness(name, { QRCode: { toDataURL: () => qr.promise } }, { order_no: 'A' }); h.mount()
  try {
    const state = name === 'usePayment' ? h.state.paymentResult : h.state.payment
    state.value = { qr_code: 'synthetic', interaction_mode: 'qr' }; await settle(); h.unmount()
    qr.resolve('data:image/stale'); await settle(); assert.equal(h.state.qrImageUrl.value, '')
  } finally { h.unmount() }
})
test('identity change cancels old queued reads using real debounce', async () => {
  const h = harness('usePayment', { debounceAsync }, { order_no: 'A' }); h.mount()
  try {
    void h.state.handleRefresh(); await settle()
    h.route.query = { order_no: 'B' }; await settle()
    const before = details(h).length
    for (const [id, fn] of [...h.timers]) if ((fn as any).delay === 250) { h.timers.delete(id); void fn() }
    await settle()
    assert.equal(details(h).length, before, 'old debounced read must not execute against B')
  } finally { h.unmount() }
})
for (const action of ['change-method', 'new-payment']) test(`late latest-payment response cannot undo ${action}`, async () => {
  const h = harness('usePayment', {}, { order_no: 'A', guest: '1' }); h.mount()
  try {
    details(h)[0].resolve(response({ order_no: 'A', status: 'pending_payment', total_amount: '10.00' })); await settle()
    const latest = h.calls.find((c: any) => c.kind === 'latest')
    if (action === 'change-method') h.state.handleChangePaymentMethod()
    else {
      h.state.selectedChannelId.value = 7; void h.state.handlePayment(); await settle()
      h.calls.find((c: any) => c.kind === 'create').resolve(response({ payment_id: 22, qr_code: 'new' })); await settle()
    }
    latest.resolve(response({ payment_id: 11, qr_code: 'old' })); await settle()
    assert.equal(h.state.paymentResult.value?.payment_id || null, action === 'change-method' ? null : 22)
  } finally { h.unmount() }
})
const details = (h: any) => h.calls.filter((c: any) => c.kind === 'detail')
for (const start of ['empty', 'member']) test(`payment identity ${start} -> guest A reloads without query-only reset`, async () => {
  const h = harness('usePayment', {}, start === 'empty' ? {} : { order_no: 'A' }); h.mount()
  try {
    if (start === 'member') { details(h)[0].resolve(response({ order_no: 'A', status: 'pending_payment' })); await settle() }
    const before = details(h).length
    h.route.query = { order_no: 'A', guest: '1' }; await settle()
    assert.equal(details(h).length, before + 1, 'guest/member + orderNo is identity, including absent old order')
    assert.equal(h.state.order.value, null)
    details(h).at(-1).resolve(response({ order_no: 'A', status: 'pending_payment' })); await settle()
    h.state.useBalance.value = true
    h.route.query = { order_no: 'A', guest: '1', tracking: 'same' }; await settle()
    assert.equal(details(h).length, before + 1); assert.equal(h.state.useBalance.value, true)
  } finally { h.unmount() }
})
