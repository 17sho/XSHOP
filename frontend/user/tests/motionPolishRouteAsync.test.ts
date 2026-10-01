import test from 'node:test'
import assert from 'node:assert/strict'
import { fixture, product, settle, dom } from './helpers/motionPolishRouteHarness.ts'

for (const theme of ['classic', 'vault']) {
  test(`${theme}: already-fulfilled detail read renders, query preserves quantity and nodes`, async () => {
    let requests = 0
    const h = await fixture(theme, { '../utils/productDetailPrefetch': { takeProductDetailRequest: (slug: string) => { requests++; return Promise.resolve({ ...product(slug), skus: [{ id: 1, is_active: true, price_amount: '10', manual_stock_total: -1 }] }) } } })
    try {
      await h.go('/products/A'); await settle()
      assert.equal(h.events.length, 0)
      assert.match(h.host.textContent!, /Product A/)
      const content = h.host.querySelector('h1')!
      const input = h.host.querySelector('input[inputmode="numeric"]') as HTMLInputElement
      assert.ok(input)
      input.value = '3'; input.dispatchEvent(new dom.window.Event('change', { bubbles: true })); await settle()
      await h.go('/products/A?campaign=same')
      assert.equal(h.host.querySelector('h1'), content)
      assert.equal(h.host.querySelector('input[inputmode="numeric"]'), input)
      assert.equal(input.value, '3'); assert.equal(requests, 1); assert.equal(h.events.length, 0)
    } finally { h.unmount() }
  })
  test(`${theme}: deferred A-B-A only latest intended result renders; unrelated route cannot be revived`, async () => {
    const h = await fixture(theme)
    try {
      await h.go('/products/A'); await h.go('/products/B'); await h.go('/products/A')
      assert.deepEqual(h.calls.map((c: any) => c.slug), ['A', 'B', 'A'])
      assert.equal(h.events.length, 0)
      h.calls[0].resolve(product('A')); h.calls[1].resolve(product('B')); await settle()
      assert.equal(h.events.length, 0); assert.equal(h.host.querySelector('h1'), null)
      h.calls[2].resolve(product('A')); await settle(); await settle()
      assert.equal(h.events.length, 0); assert.match(h.host.textContent!, /Product A/)
      await h.go('/products/B')
      await h.go('/other'); const total = h.events.length
      h.calls[3].resolve(product('B')); await settle(); await settle()
      assert.equal(h.events.length, total); assert.equal(h.host.querySelector('article')?.textContent, 'Other')
    } finally { h.unmount() }
  })
  test(`${theme}: controlled failed result retries within the same route identity`, async () => {
    const h = await fixture(theme)
    const errors: any[] = []; const original = console.error; console.error = (...args: any[]) => errors.push(args)
    try {
      await h.go('/products/missing'); assert.equal(h.events.length, 0)
      h.calls[0].reject(new Error('synthetic not-found')); await settle(); await settle()
      assert.equal(errors.length, 1)
      assert.equal(h.events.length, 0); assert.match(h.host.textContent!, /productDetail.notFound/)
      const retry = [...h.host.querySelectorAll('button')].find((b: any) => b.textContent.includes('errorBoundary.retry')) as HTMLButtonElement
      assert.ok(retry); retry.click(); await settle()
      assert.equal(h.calls.length, 2); assert.equal(h.events.length, 0)
      h.calls[1].resolve(product('missing')); await settle(); await settle()
      assert.equal(h.events.length, 0); assert.match(h.host.textContent!, /Product missing/)
    } finally { console.error = original; h.unmount() }
  })
  test(`${theme}: unmount while detail request is pending never plays or attaches late observers`, async () => {
    const h = await fixture(theme)
    await h.go('/products/A'); h.unmount()
    h.calls[0].resolve(product('A')); await settle(); await settle()
    assert.equal(h.events.length, 0); assert.equal(h.observers.length, 0); assert.equal(h.listeners.size, 0)
  })
  test(`${theme}: reduced preference retains visible current data without custom animation`, async () => {
    const h = await fixture(theme)
    try {
      await h.go('/products/A'); h.calls[0].resolve(product('A')); await settle(); await settle()
      assert.equal(h.events.length, 0); h.reduce(true)
      await h.go('/products/B'); h.calls[1].resolve(product('B')); await settle(); await settle()
      assert.equal(h.events.length, 0); assert.match(h.host.textContent!, /Product B/)
      h.reduce(false); await h.go('/products/B?still=same'); assert.equal(h.events.length, 0)
      await h.go('/products/C'); h.calls[2].resolve(product('C')); await settle(); await settle()
      assert.equal(h.events.length, 0)
    } finally { h.unmount() }
  })
}
