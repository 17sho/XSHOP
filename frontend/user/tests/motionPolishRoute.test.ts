import test from 'node:test'
import assert from 'node:assert/strict'
import { fixture, product, settle } from './helpers/motionPolishRouteHarness.ts'

for (const theme of ['classic', 'vault']) test(`${theme}: actual App renders deferred detail and purchase observers without transformed ancestors`, async () => {
  const h = await fixture(theme)
  try {
    await h.go('/products/A')
    assert.equal(h.calls.length, 1)
    assert.equal(h.host.querySelector('h1'), null)
    h.calls[0].resolve(product('A')); await settle(); await settle()
    assert.match(h.host.textContent!, /Product A/)
    assert.ok(h.observers.length > 0, 'onLoaded still sees rendered purchase controls')
    const bar = h.host.querySelector('.fixed')
    assert.ok(bar, 'actual fixed purchase bar is present')
    for (let ancestor = bar.parentElement; ancestor; ancestor = ancestor.parentElement) {
      assert.ok(['', 'none'].includes(window.getComputedStyle(ancestor).transform))
    }
    assert.equal(h.events.length, 0, 'original opacity Transition never calls global WAAPI')
  } finally { h.unmount() }
})
