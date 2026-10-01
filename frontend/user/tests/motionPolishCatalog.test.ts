import test from 'node:test'
import assert from 'node:assert/strict'
import { fixture, response, categoryRows, settle, styleSource, clock } from './helpers/motionPolishCatalogHarness.ts'

for (const template of ['views/Products.vue', 'templates/vault/Products.vue']) {
  for (const layout of ['list', 'card']) test(`${template} ${layout}: only current success replaces retained pending nodes with original row motion`, async () => {
    const f = await fixture(template, layout)
    try {
      f.categories.resolve({ data: { data: categoryRows } }); f.requests[0].resolve(response(1)); await settle()
      const old = f.row()
      assert.ok(old, 'real product card is rendered')
      assert.equal(f.animations.length, 0, 'cold first row entrance is not doubled')
      assert.equal(f.host.querySelector('.catalog-settled'), null, 'initial row CSS remains enabled')
      f.state.selectCategory(2); await settle()
      assert.equal(f.requests.length, 2, 'category dispatch is immediate')
      assert.equal(f.row(), old, 'same populated node while pending')
      assert.ok(old.closest('[inert]'))
      assert.equal(f.animations.length, 0, 'intent alone does not animate')
      f.state.selectCategory(3); await settle()
      assert.equal(f.requests.length, 3, 'rapid category dispatch is immediate')
      f.requests[2].resolve(response(3)); await settle()
      assert.equal(f.animations.length, 0, 'no whole-catalog WAAPI replacement')
      assert.equal(f.host.querySelector('.catalog-settled'), null, 'original replacement rows retain their CSS reveal')
      assert.ok(f.row().closest('.theme-slide-up') || f.row().classList.contains('theme-slide-up'))
      assert.equal(f.row().closest('[inert]'), null)
      const current = f.row()
      f.requests[1].resolve(response(2)); await settle()
      assert.equal(f.row(), current)
      assert.equal(f.animations.length, 0, 'stale completion cannot animate')
      await settle(); assert.equal(f.animations.length, 0, 'render ticks do not animate')
    } finally { f.cleanup() }
  })

  for (const layout of ['list', 'card']) {
    test(`${template} ${layout}: query freshness is immediate, typing debounced, explicit page cancels timer`, async () => {
      const timer = clock(), f = await fixture(template, layout)
      try {
        f.categories.resolve({ data: { data: categoryRows } }); f.requests[0].resolve(response(1)); await settle()
        const old = f.row()
        const input = f.host.querySelector('input')
        // Classic has no rendered search input: exercise its exposed composable contract.
        if (input) { input.value = 'latest'; input.dispatchEvent(new Event('input', { bubbles: true })) }
        else f.state.searchQuery.value = 'latest'
        assert.equal(f.state.catalogStale.value, true, 'freshness invalidates before render/debounce')
        assert.equal(f.requests.length, 1)
        await settle(); assert.equal(f.row(), old); assert.ok(old.closest('[inert]'))
        assert.equal(f.animations.length, 0)
        await timer.tick(299); assert.equal(f.requests.length, 1)
        await timer.tick(1); assert.equal(f.requests.length, 2)
        f.state.searchQuery.value = 'newest'
        f.state.changePage(2)
        assert.equal(f.requests.length, 3)
        assert.equal(f.requests[2].params.search, 'newest'); assert.equal(f.requests[2].params.page, 2)
        await timer.tick(300); assert.equal(f.requests.length, 3, 'explicit page cancels queued typing')
        f.requests[1].resolve(response(2)); await settle(); assert.equal(f.row(), old); assert.equal(f.animations.length, 0)
        f.requests[2].resolve(response(3)); await settle(); assert.equal(f.animations.length, 0)
        f.state.pageSize.value = 6
        assert.equal(f.state.catalogStale.value, true, 'exposed query mutation cannot reuse remembered success')
        await settle(); assert.ok(f.row().closest('[inert]')); assert.equal(f.animations.length, 0)
      } finally { f.cleanup(); timer.restore() }
    })

    test(`${template} ${layout}: rapid accepted results retain latest identity across reduced preference changes`, async () => {
      const f = await fixture(template, layout)
      try {
        f.categories.resolve({ data: { data: categoryRows } }); f.requests[0].resolve(response(1)); await settle()
        f.state.selectCategory(2); f.requests[1].resolve(response(2)); await settle()
        f.state.selectCategory(3); f.requests[2].resolve(response(3)); await settle()
        assert.equal(f.animations.length, 0)
        assert.equal(f.state.products.value[0].id, 3)
        f.reduce(true)
        f.state.selectCategory(2); f.requests[3].resolve(response(4)); await settle()
        assert.equal(f.state.products.value[0].id, 4)
        assert.equal(f.animations.length, 0, 'no entrance under reduced motion')
        assert.equal(f.state.catalogStale.value, false, 'reduced motion does not delay current response')
        f.reduce(false); await settle(); assert.equal(f.animations.length, 0, 'preference toggle does not replay content')
        f.state.selectCategory(3); f.requests[4].resolve(response(5)); await settle()
        assert.equal(f.animations.length, 0)
        assert.equal(f.state.products.value[0].id, 5)
      } finally { f.cleanup() }
    })

    test(`${template} ${layout}: reduced from mount preserves authoritative replacement`, async () => {
      const f = await fixture(template, layout, { reduced: true })
      try {
        f.categories.resolve({ data: { data: categoryRows } }); f.requests[0].resolve(response(1)); await settle()
        f.state.selectCategory(2); f.state.selectCategory(3)
        f.requests[2].resolve(response(3)); f.requests[1].resolve(response(2)); await settle()
        assert.equal(f.animations.length, 0)
        assert.equal(f.state.products.value[0].id, 3)
        assert.equal(f.row().closest('[inert]'), null)
      } finally { f.cleanup() }
    })

    test(`${template} ${layout}: failure retains inert rows, retry/empty replacement and disposal preserve authority`, async () => {
      const f = await fixture(template, layout), originalError = console.error
      console.error = () => {}
      try {
        f.categories.resolve({ data: { data: categoryRows } }); f.requests[0].resolve(response(1)); await settle()
        const old = f.row()
        f.state.selectCategory(2); f.requests[1].reject(Error('synthetic read failure')); await settle()
        assert.equal(f.row(), old); assert.ok(old.closest('[inert]')); assert.equal(f.animations.length, 0)
        assert.ok(f.host.querySelector('[role="status"]')?.textContent?.includes('common.error'))
        ;(f.host.querySelector('[role="status"] button') as HTMLElement).click()
        assert.equal(f.requests.length, 3)
        f.requests[2].resolve(response(0)); await settle(); assert.equal(f.animations.length, 0)
        assert.equal(f.row(), undefined, 'empty state replaces old rows, not a duplicate list')
        f.state.selectCategory(3); f.requests[3].resolve(response(3)); await settle()
        assert.equal(f.animations.length, 0, 'empty to populated has no custom container motion')
        f.state.selectCategory(2); f.state.cleanup(); f.requests[4].resolve(response(4)); await settle()
        assert.equal(f.state.products.value[0].id, 3); assert.equal(f.animations.length, 0)
      } finally { console.error = originalError; f.cleanup() }
    })

    test(`${template} ${layout}: unresolved route rows stay inert and remain unauthorized on category metadata alone`, async () => {
      const f = await fixture(template, layout, { categoryRoute: true })
      try {
        f.requests[0].resolve(response(1)); await settle()
        const old = f.row(); assert.ok(old.closest('[inert]')); assert.equal(f.animations.length, 0)
        f.categories.resolve({ data: { data: categoryRows } }); await settle()
        assert.equal(f.row(), old); assert.ok(old.closest('[inert]')); assert.equal(f.animations.length, 0)
        assert.equal(f.requests[1].params.category_id, 2)
        f.requests[1].resolve(response(2)); await settle()
        assert.equal(f.animations.length, 0); assert.equal(f.row().closest('[inert]'), null)
      } finally { f.cleanup() }
    })

    for (const productsFirst of [false, true]) test(`${template} ${layout}: warm cache refresh authorizes only captured success (productsFirst=${productsFirst})`, async () => {
      const f = await fixture(template, layout, { warm: true })
      try {
        const old = f.row(); assert.ok(old); assert.ok(old.closest('[inert]'))
        assert.equal(f.animations.length, 0)
        if (productsFirst) f.requests[0].resolve(response(1))
        else f.categories.resolve({ data: { data: categoryRows } })
        await settle(); assert.equal(f.animations.length, 0)
        assert.equal(f.state.products.value[0].id, productsFirst ? 1 : 99, 'metadata alone cannot replace cached rows')
        if (productsFirst) f.categories.resolve({ data: { data: categoryRows } })
        else f.requests[0].resolve(response(1))
        await settle(); assert.equal(f.animations.length, 0)
        assert.equal(f.state.catalogStale.value, false)
      } finally { f.cleanup() }
    })
  }

  test(`${template}: pending feedback transitions modest opacity, with explicit reduced-motion opt-out`, () => {
    const css = styleSource(template)
    assert.match(css, /\.catalog-feedback\s*\{[^}]*transition:\s*opacity\s+(?:0?\.1[0-9]*s|1[0-9]{2}ms)/)
    assert.match(css, /\.catalog-stale\s*\{[^}]*opacity:\s*\.8[0-9]?\s*;/)
    assert.match(css, /@media\s*\(prefers-reduced-motion:\s*reduce\)\s*\{\s*\.catalog-feedback\s*\{\s*transition:\s*none/)
  })
}
