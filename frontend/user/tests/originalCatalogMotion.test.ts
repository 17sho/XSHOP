import test from 'node:test'
import assert from 'node:assert/strict'
import { fixture, response, categoryRows, settle, clock } from './helpers/motionPolishCatalogHarness.ts'

const result = () => ({ data: { data: [1, 2, 3, 4].map(id => ({ ...response(id).data.data[0], category: { id: id < 3 ? 2 : 3, name: { en: id < 3 ? 'Two' : 'Three' } } })), pagination: { total_page: 3 } } })
const rows = (f: any) => Array.from(f.host.querySelectorAll('.catalog-feedback .theme-slide-up')) as HTMLElement[]

// Observe the real CSS restart boundary: every row is reset together before
// one layout flush, without replacing nodes or manufacturing WAAPI effects.
function observeBatches(f: any) {
  const content = f.host.querySelector('.catalog-content') as HTMLElement
  const batches: HTMLElement[][] = []
  Object.defineProperty(content, 'offsetWidth', { configurable: true, get() {
    const items = rows(f)
    assert.ok(items.length)
    assert.ok(items.every(row => row.style.animationName === 'none'), 'all row animations cancel before the single batch flush')
    batches.push(items)
    return 600
  } })
  return batches
}

for (const template of ['views/Products.vue', 'templates/vault/Products.vue']) {
  for (const layout of ['list', 'card']) test(`${template} ${layout}: only authoritative result replays rows once and retains keyed identity`, async () => {
    const f = await fixture(template, layout), batches = observeBatches(f)
    const originalError = console.error
    try {
      f.categories.resolve({ data: { data: categoryRows } }); f.requests[0].resolve(result()); await settle()
      const initial = rows(f)
      assert.equal(batches.length, 0, 'first entry is CSS only')
      f.state.selectCategory(2); await settle()
      assert.equal(f.requests.length, 2, 'explicit intent dispatch remains immediate')
      assert.deepEqual(rows(f), initial)
      assert.ok(initial.every(row => row.closest('[inert]')))
      assert.equal(batches.length, 0, 'intent does not replay')
      f.state.selectCategory(3); await settle()
      f.requests[2].resolve(result()); await settle()
      assert.equal(batches.length, 1, 'current accepted response performs one row batch restart')
      assert.deepEqual(rows(f), initial, 'same IDs preserve real row component DOM identity')
      assert.deepEqual(batches[0], initial)
      assert.ok(initial.every(row => row.style.animationName === '' && !row.closest('[inert]')))
      assert.equal(f.host.querySelector('.catalog-settled'), null, 'original stagger is never permanently disabled')
      assert.equal(f.animations.length, 0, 'no whole-container animation remains')
      f.requests[1].resolve(response(50)); await settle()
      assert.deepEqual(rows(f), initial); assert.equal(batches.length, 1, 'obsolete response cannot replay')
      f.state.categories.value = [...categoryRows]; await settle()
      assert.equal(batches.length, 1, 'metadata and render ticks cannot replay')
      console.error = () => {}
      const failed = f.state.loadProducts(); f.requests[3].reject(Error('synthetic failure')); await failed; await settle()
      assert.equal(batches.length, 1); assert.deepEqual(rows(f), initial)
      assert.ok(f.host.querySelector('[role="status"]')?.textContent?.includes('common.error'))
      ;(f.host.querySelector('[role="status"] button') as HTMLElement).click()
      f.requests[4].resolve(result()); await settle()
      assert.equal(batches.length, 2, 'retry success restarts once even for retained IDs')
      await settle(); assert.equal(batches.length, 2)
      assert.ok(initial.every(row => row.style.animationName === ''))
    } finally { console.error = originalError; f.cleanup() }
  })
}

for (const template of ['views/Products.vue', 'templates/vault/Products.vue']) {
  for (const layout of ['list', 'card']) {
    test(`${template} ${layout}: reduced motion never replays and rapid changes/unmount leave no queued work`, async () => {
      const f = await fixture(template, layout, { reduced: true }), batches = observeBatches(f)
      let cleaned = false
      try {
        f.categories.resolve({ data: { data: categoryRows } }); f.requests[0].resolve(result()); await settle()
        f.state.selectCategory(2); f.requests[1].resolve(result()); await settle()
        assert.equal(batches.length, 0, 'reduced preference skips even the CSS restart')
        assert.equal(f.state.catalogStale.value, false)
        f.reduce(false); await settle(); assert.equal(batches.length, 0, 'preference change is not a result')
        f.state.selectCategory(3); f.requests[2].resolve(result()); await settle()
        assert.equal(batches.length, 1)
        const retained = rows(f)
        f.state.selectCategory(2); f.requests[3].resolve(result()); await settle()
        assert.equal(batches.length, 2, 'rapid results cancel/reset CSS in one batch, without timers')
        assert.deepEqual(rows(f), retained)
        f.reduce(true)
        f.state.selectCategory(3); f.requests[4].resolve(result()); await settle()
        assert.equal(batches.length, 2)
        f.reduce(false)
        f.state.selectCategory(2)
        f.cleanup(); cleaned = true
        f.requests[5].resolve(result()); await settle()
        assert.equal(batches.length, 2, 'unmount fences pending replies and cannot leave a replay callback')
        assert.ok(retained.every(row => !row.isConnected && row.style.animationName === ''))
      } finally { if (!cleaned) f.cleanup() }
    })

    test(`${template} ${layout}: response superseded before render never replays stale rows`, async () => {
      const f = await fixture(template, layout), batches = observeBatches(f)
      try {
        f.categories.resolve({ data: { data: categoryRows } }); f.requests[0].resolve(result()); await settle()
        f.state.selectCategory(2); f.requests[1].resolve(result())
        // Product response commits, but a new intent arrives before Vue patches.
        await Promise.resolve()
        f.state.selectCategory(3)
        await settle()
        assert.equal(batches.length, 0, 'newer pending intent cancels not-yet-rendered replay')
        assert.ok(rows(f).every(row => row.closest('[inert]')))
        f.requests[2].resolve(result()); await settle()
        assert.equal(batches.length, 1)
      } finally { f.cleanup() }
    })
  }
}

for (const template of ['views/Products.vue', 'templates/vault/Products.vue']) {
  for (const layout of ['list', 'card']) {
    test(`${template} ${layout}: typing debounce and immediate page request preserve the displayed batch`, async () => {
      const timer = clock(), f = await fixture(template, layout), batches = observeBatches(f)
      try {
        f.categories.resolve({ data: { data: categoryRows } }); f.requests[0].resolve(result()); await settle()
        const retained = rows(f)
        f.state.searchQuery.value = 'one'
        assert.equal(f.state.catalogStale.value, true)
        await settle(); assert.deepEqual(rows(f), retained)
        assert.ok(retained.every(row => row.closest('[inert]')))
        await timer.tick(299); assert.equal(f.requests.length, 1)
        await timer.tick(1); assert.equal(f.requests.length, 2)
        f.state.searchQuery.value = 'two'
        f.state.changePage(2)
        assert.equal(f.requests.length, 3)
        assert.equal(f.requests[2].params.search, 'two'); assert.equal(f.requests[2].params.page, 2)
        await timer.tick(300); assert.equal(f.requests.length, 3)
        f.requests[1].resolve(response(100)); await settle()
        assert.deepEqual(rows(f), retained); assert.equal(batches.length, 0)
        f.requests[2].resolve(result()); await settle()
        assert.equal(batches.length, 1); assert.deepEqual(rows(f), retained)
        f.state.pageSize.value = 6; await settle()
        assert.ok(retained.every(row => row.closest('[inert]'))); assert.equal(batches.length, 1)
      } finally { f.cleanup(); timer.restore() }
    })

    test(`${template} ${layout}: unresolved category metadata and empty result never manufacture row replays`, async () => {
      const f = await fixture(template, layout, { categoryRoute: true }), batches = observeBatches(f)
      try {
        f.requests[0].resolve(result()); await settle()
        const retained = rows(f)
        assert.ok(retained.every(row => row.closest('[inert]')))
        f.categories.resolve({ data: { data: categoryRows } }); await settle()
        assert.equal(batches.length, 0); assert.deepEqual(rows(f), retained)
        assert.equal(f.requests[1].params.category_id, 2)
        f.requests[1].resolve(result()); await settle(); assert.equal(batches.length, 1)
        f.state.selectCategory(3); f.requests[2].resolve(response(0)); await settle()
        assert.equal(rows(f).length, 0); assert.equal(batches.length, 1, 'no whole-container empty entrance')
        f.state.selectCategory(2); f.requests[3].resolve(result()); await settle()
        assert.equal(batches.length, 2); assert.equal(rows(f).length, 4)
      } finally { f.cleanup() }
    })

    for (const productsFirst of [false, true]) test(`${template} ${layout}: cached first entry and refresh replay once (productsFirst=${productsFirst})`, async () => {
      const f = await fixture(template, layout, { warm: true }), batches = observeBatches(f)
      try {
        assert.equal(rows(f).length, 1)
        assert.ok(rows(f)[0].closest('[inert]')); assert.equal(batches.length, 0)
        if (productsFirst) f.requests[0].resolve(result())
        else f.categories.resolve({ data: { data: categoryRows } })
        await settle(); assert.equal(batches.length, productsFirst ? 1 : 0)
        if (productsFirst) f.categories.resolve({ data: { data: categoryRows } })
        else f.requests[0].resolve(result())
        await settle(); assert.equal(batches.length, 1)
        assert.equal(f.state.catalogStale.value, false)
      } finally { f.cleanup() }
    })
  }
}

for (const layout of ['list', 'card']) test(`vault ${layout}: row replay cannot remount the open business dialog`, async () => {
  const f = await fixture('templates/vault/Products.vue', layout), batches = observeBatches(f)
  try {
    f.categories.resolve({ data: { data: categoryRows } }); f.requests[0].resolve(result()); await settle()
    ;(f.host.querySelector('[aria-label="products.quickBuyAria"]') as HTMLElement).click(); await settle()
    const dialog = f.host.querySelector('[data-quick-buy]')
    assert.ok(dialog)
    f.state.selectCategory(2); f.requests[1].resolve(result()); await settle()
    assert.equal(batches.length, 1)
    assert.equal(f.host.querySelector('[data-quick-buy]'), dialog)
  } finally { f.cleanup() }
})

for (const template of ['views/Products.vue', 'templates/vault/Products.vue']) {
  for (const layout of ['list', 'card']) test(`${template} ${layout}: original per-item first-entry stagger with per-group indexes`, async () => {
    const f = await fixture(template, layout)
    try {
      f.categories.resolve({ data: { data: categoryRows } }); f.requests[0].resolve(result()); await settle()
      assert.equal(rows(f).length, 4, 'every actual item owns the original CSS entrance')
      assert.deepEqual(rows(f).map(row => row.style.animationDelay), layout === 'list' ? (template === 'views/Products.vue' ? ['0ms', '30ms', '0ms', '30ms'] : ['0ms', '20ms', '0ms', '20ms']) : ['0ms', '50ms', '100ms', '150ms'])
      assert.equal(f.animations.length, 0, 'no parallel whole-container animation')
    } finally { f.cleanup() }
  })
}
