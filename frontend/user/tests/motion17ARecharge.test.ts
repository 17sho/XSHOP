import test from 'node:test'
import assert from 'node:assert/strict'
import { harness, settle, response } from './helpers/motion17AHarness.ts'
for (const operation of ['poll', 'capture']) test(`recharge ${operation} completion cannot populate next identity`, async () => {
  const h = harness('useRechargeOrderDetail'); h.mount()
  try {
    h.calls[0].resolve(response({ recharge: { recharge_no: 'A', status: 'pending' }, payment: { id: 1 } })); await settle()
    if (operation === 'poll') void [...h.timers.values()][0]()
    else { void h.state.checkPayment(); void h.state.checkPayment() }
    assert.equal(h.calls.length, 2, 'capture must have one in-flight dispatch')
    h.route.params.recharge_no = 'B'; await settle()
    h.calls[1].resolve(response({ recharge: { recharge_no: 'A', status: 'success' }, payment: { id: 1 } })); await settle()
    assert.equal(h.state.recharge.value, null)
    assert.equal(h.calls.length, 3, 'old capture must not trigger a read for the next order')
  } finally { h.unmount() }
})
for (const exit of ['replace', 'unmount']) test(`recharge ${exit} fences read/auto-redirect/poll completion`, async () => {
  const h = harness('useRechargeOrderDetail'); h.mount()
  try {
    if (exit === 'replace') h.route.params.recharge_no = 'B'
    else h.unmount()
    await settle()
    if (exit === 'replace') assert.equal(h.calls.length, 2)
    h.calls[0].resolve(response({ recharge: { recharge_no: 'A', status: 'pending' }, payment: { id: 1, interaction_mode: 'redirect', pay_url: 'https://synthetic.invalid/A' } })); await settle()
    assert.equal(h.state.recharge.value, null); assert.equal(h.state.payment.value, null)
    assert.equal(h.redirects.length, 0); assert.equal(h.timers.size, 0)
    if (exit === 'replace') {
      h.calls[1].resolve(response({ recharge: { recharge_no: 'B', status: 'pending' }, payment: { id: 2 } })); await settle()
      assert.equal(h.state.recharge.value.recharge_no, 'B'); assert.equal(h.timers.size, 1)
      h.route.query = { tracking: 'safe' }; await settle(); assert.equal(h.calls.length, 2)
    }
  } finally { h.unmount() }
})
