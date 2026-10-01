import test from 'node:test'
import assert from 'node:assert/strict'
import { harness, settle, response } from './helpers/motion17AHarness.ts'
for (const outcome of ['resolve', 'reject']) test(`clear guest credentials invalidates in-flight detail ${outcome}`, async () => {
  const h = harness('useGuestOrderDetail'); h.mount()
  try {
    h.state.clearAuth()
    assert.equal(h.state.loading.value, false, 'clear must exit pending spinner immediately')
    if (outcome === 'resolve') h.calls[0].resolve(response({ order_no: 'A', secret: 'synthetic' }))
    else h.calls[0].reject(new Error('old invalid auth'))
    await settle()
    assert.equal(h.state.order.value, null)
    assert.equal(h.state.authError.value, 'guestOrderDetail.authRequired')
    assert.equal(h.state.showAuthForm.value, true)
    h.state.auth.value = { email: 'new@example.invalid', order_password: 'synthetic-new' }
    void h.state.handleAuthSubmit(); await settle()
    h.calls[1].resolve(response({ order_no: 'A', fresh: true })); await settle()
    assert.equal(h.state.order.value.fresh, true)
  } finally { h.unmount() }
})
