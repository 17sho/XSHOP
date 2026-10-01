import test from 'node:test'
import assert from 'node:assert/strict'
import { harness, settle, response } from './helpers/motion17AHarness.ts'
for (const exit of ['replace', 'unmount', 'retry']) test(`product ${exit} fences stale completion and onLoaded`, async () => {
  let loaded = 0
  const h = harness('useProductDetail', { composableOptions: { onLoaded: () => { loaded++ } } })
  try {
    if (exit === 'replace') h.route.params.slug = 'B'
    if (exit === 'retry') void h.state.loadProduct()
    if (exit === 'unmount') h.unmount()
    h.calls[0].resolve({ slug: 'stale', images: ['stale'] }); await settle()
    assert.equal(h.state.product.value, null)
    assert.equal(loaded, 0)
    if (exit !== 'unmount') {
      assert.equal(h.state.loading.value, true)
      h.calls[1].resolve({ slug: 'current' }); await settle()
      assert.equal(h.state.product.value.slug, 'current')
    }
  } finally { h.unmount() }
})
for (const name of ['useOrderDetail', 'useGuestOrderDetail', 'useBlogDetail']) {
  for (const finish of ['resolve', 'reject', 'unmount']) test(`${name}: identity reload isolates ${finish}; query-only retains entity`, async () => {
    const h = harness(name); h.mount()
    const key = name === 'useBlogDetail' ? 'slug' : 'order_no'
    const field = name === 'useBlogDetail' ? 'post' : 'order'
    try {
      const first = h.calls[0]
      h.route.params[key] = 'B'; await settle()
      assert.equal(h.calls.length, 2, 'new entity must reload')
      assert.equal(h.state[field].value, null)
      if (finish === 'reject') first.reject(new Error('stale'))
      else first.resolve(response({ order_no: 'A', slug: 'A' }))
      await settle()
      assert.equal(h.state[field].value, null); assert.equal(h.state.loading.value, true)
      if (finish === 'unmount') h.unmount()
      h.calls[1].resolve(response({ order_no: 'B', slug: 'B' })); await settle()
      if (finish === 'unmount') assert.equal(h.state[field].value, null)
      else {
        assert.equal(h.state[field].value[key], 'B')
        h.route.query = { tab: 'same' }; await settle()
        assert.equal(h.calls.length, 2)
      }
    } finally { h.unmount() }
  })
}
test('product onLoaded runs only after loading branch renders the current entity', async () => {
  const observed: boolean[] = []
  const h = harness('useProductDetail', { composableOptions: { onLoaded: () => observed.push(h.state.loading.value) } })
  try {
    h.calls[0].resolve({ slug: 'A' }); await settle()
    assert.deepEqual(observed, [false], 'actual classic/vault observer callbacks need the loaded DOM, not loading branch')
    h.route.params.slug = 'B'; await settle(); h.unmount()
    h.calls[1].resolve({ slug: 'B' }); await settle(); assert.deepEqual(observed, [false])
  } finally { h.unmount() }
})
const product = (slug: string) => ({ slug, title: { 'en-US': `Product ${slug}` }, images: [slug + '.png'], skus: [{ id: slug === 'A' ? 1 : 2, is_active: true, manual_stock_total: -1 }], fulfillment_type: 'manual' })
test('product A -> B reuses composable but reloads title/image/SKU/draft; query-only preserves draft', async () => {
  const h = harness('useProductDetail'); h.mount()
  try {
    h.calls[0].resolve(product('A')); await settle()
    h.state.quantity.value = 7; h.state.purchaseWarning.value = 'A warning'
    h.route.params.slug = 'B'; await settle()
    assert.equal(h.calls.length, 2, 'B must fetch instead of showing Product A under B URL')
    assert.equal(h.state.product.value, null); assert.equal(h.state.currentImage.value, ''); assert.equal(h.state.quantity.value, 1)
    h.calls[1].resolve(product('B')); await settle()
    assert.equal(h.state.product.value.title['en-US'], 'Product B'); assert.equal(h.state.selectedSkuId.value, 2)
    assert.equal(h.state.currentImage.value, 'B.png'); assert.equal(h.state.purchaseWarning.value, '')
    h.state.quantity.value = 5; h.route.query = { tracking: 'safe' }; await settle()
    assert.equal(h.calls.length, 2); assert.equal(h.state.quantity.value, 5)
  } finally { h.unmount() }
})
