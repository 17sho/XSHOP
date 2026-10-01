import test from 'node:test'
import assert from 'node:assert/strict'
import { harness, settle, response, deferred } from './helpers/motion17AHarness.ts'
for (const name of ['useOrderDetail', 'useGuestOrderDetail', 'usePayment']) test(`${name}: confirmation for A cannot cancel B after reuse`, async () => {
  const confirm = deferred()
  const h = harness(name, { useConfirmDialog: () => ({ confirm: () => confirm.promise }) }, { order_no: 'A' }); h.mount()
  try {
    h.calls.find((c: any) => c.kind === 'detail').resolve(response({ order_no: 'A', status: 'pending_payment' })); await settle()
    void h.state.cancelOrder(); await settle()
    if (name === 'usePayment') h.route.query = { order_no: 'B' }
    else h.route.params.order_no = 'B'
    await settle(); h.calls.filter((c: any) => c.kind === 'detail').at(-1).resolve(response({ order_no: 'B', status: 'pending_payment' })); await settle()
    confirm.resolve(true); await settle()
    assert.equal(h.calls.filter((c: any) => c.kind === 'cancel').length, 0)
  } finally { h.unmount() }
})
for (const name of ['useOrderDetail', 'useGuestOrderDetail']) test(`${name}: download cannot release stale fulfillment after leaving/clearing identity`, async () => {
  const h = harness(name); h.mount()
  let downloads = 0
  Object.assign(globalThis, { document: { createElement: () => ({ click() { downloads++ } }) } })
  try {
    void h.state.handleDownloadFulfillment('A'); await settle()
    if (name === 'useGuestOrderDetail') h.state.clearAuth()
    else h.unmount()
    h.calls.find((c: any) => c.kind === 'download').resolve({ data: 'synthetic-not-a-secret' }); await settle()
    assert.equal(downloads, 0)
  } finally { h.unmount() }
})
