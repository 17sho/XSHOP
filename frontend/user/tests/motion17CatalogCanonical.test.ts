import test from 'node:test'
import assert from 'node:assert/strict'
import { loader, settle } from './helpers/motion17DHarness.ts'
import { fixture, button, rows, response, settleNavigation } from './helpers/motion17CatalogRouteHarness.ts'

test('classic home: query-only navigation does not reset a direct category filter', async () => {
  const f = await fixture({ home: true, warm: true })
  try {
    button(f, 'Three').click(); await settleNavigation()
    f.requests[1].resolve(response(3)); await settle()
    await f.router.push('/?view=card'); await settleNavigation()
    assert.equal(f.host.querySelector('.category-pill-active')?.textContent?.trim(), 'Three')
    assert.equal(f.requests.length, 2)
    assert.equal(f.host.querySelector('[data-row]')?.closest('[inert]'), null)
    f.requests[0].resolve(response(1)); f.categories.resolve({ data: { data: rows } }); await settleNavigation()
    assert.equal(f.requests.length, 2)
    assert.equal(f.host.querySelector('[data-row]')?.textContent, 'synthetic-3')
  } finally { f.cleanup() }
})

test('classic home: late categories cannot cache retained filtered rows as a pending All response', async () => {
  const f = await fixture({ home: true, warm: true })
  const readCache = () => loader()('utils/publicCatalogCache.ts').readPublicCatalogCache(window.sessionStorage, window.location.host)
  try {
    button(f, 'Three').click(); await settleNavigation()
    assert.equal(f.requests[1].params.category_id, 3)
    f.requests[1].resolve(response(3)); await settle()
    const retained = f.host.querySelector('[data-row="3"]')
    button(f, 'products.allCategories').click(); await settleNavigation()
    assert.equal(f.requests[2].params.category_id, undefined)
    assert.equal(f.host.querySelector('[data-row]'), retained)
    assert.ok(retained?.closest('[inert]'))
    f.categories.resolve({ data: { data: rows } }); await settleNavigation()
    assert.equal(readCache().products[0].id, 99, 'late categories must preserve the prior complete All snapshot')
    f.requests[0].resolve(response(1)); await settleNavigation()
    assert.equal(readCache().products[0].id, 99, 'obsolete initial All request has lost ownership too')
    f.requests[2].resolve({ data: { data: [{ id: 4, slug: 'latest-all' }], pagination: { total_page: 7 } } }); await settleNavigation()
    assert.deepEqual(readCache(), { products: [{ id: 4, slug: 'latest-all' }], categories: rows, totalPages: 7 })
    assert.equal(f.host.querySelector('[data-row]')?.closest('[inert]'), null)
    assert.equal(f.router.currentRoute.value.fullPath, '/', 'home filtering still does not navigate')
    assert.deepEqual(f.navigations, ['/'])
    assert.equal(f.requests.length, 3)
  } finally { f.cleanup() }
})

test('classic memory router: unresolved initial category cannot authorize unrelated unfiltered rows', async () => {
  const f = await fixture()
  try {
    f.requests[0].resolve(response(1)); await settle()
    const retained = f.host.querySelector('[data-row="1"]') as HTMLElement
    assert.ok(retained, 'keep returned rows for continuity')
    retained.click(); await settleNavigation()
    assert.equal(f.router.currentRoute.value.fullPath, '/categories/two', 'real parent guard blocks the synthetic emitter')
    assert.ok(retained.closest('[inert]'))
    f.categories.resolve({ data: { data: rows } }); await settleNavigation()
    assert.equal(f.requests.length, 2)
    assert.equal(f.requests[1].params.category_id, 2)
    assert.equal(f.host.querySelector('[data-row]'), retained)
    f.requests[1].resolve(response(2)); await settleNavigation()
    const fresh = f.host.querySelector('[data-row="2"]') as HTMLElement
    assert.equal(fresh.closest('[inert]'), null)
    fresh.click(); await settleNavigation()
    assert.equal(f.router.currentRoute.value.fullPath, '/products/synthetic-2')
  } finally { f.cleanup() }
})

for (const outcome of ['unknown', 'failure']) test(`classic category resolution characterization: ${outcome} keeps URL and does not invent fallback requests`, async () => {
  const f = await fixture(), originalError = console.error
  console.error = () => {}
  try {
    f.requests[0].resolve(response(1)); await settle()
    if (outcome === 'unknown') f.categories.resolve({ data: { data: [rows[1]] } })
    else f.categories.reject(Error('synthetic category outage'))
    await settleNavigation()
    assert.equal(f.router.currentRoute.value.fullPath, '/categories/two')
    assert.equal(f.host.querySelector('.category-pill-active')?.textContent?.trim(), 'products.allCategories')
    assert.equal(f.requests.length, 1)
    assert.equal(f.host.querySelector('[data-row]')?.textContent, 'synthetic-1')
  } finally { f.cleanup(); console.error = originalError }
})

for (const productsFirst of [false, true]) test(`classic memory router: same-value All reconciles home without a duplicate fetch (productsFirst=${productsFirst})`, async () => {
  const f = await fixture()
  try {
    if (productsFirst) { f.requests[0].resolve(response(1)); await settle() }
    button(f, 'products.allCategories').click(); await settleNavigation()
    assert.equal(f.router.currentRoute.value.fullPath, '/')
    assert.equal(f.host.querySelector('.category-pill-active')?.textContent?.trim(), 'products.allCategories')
    f.requests[0].resolve(response(1)); f.categories.resolve({ data: { data: rows } }); await settleNavigation()
    assert.equal(f.requests.length, 1, 'reuse the same unfiltered query, pending or already returned')
    assert.equal(f.host.querySelector('[data-row]')?.textContent, 'synthetic-1')
    assert.equal(f.host.querySelector('[data-row]')?.closest('[inert]'), null)
    assert.equal(f.router.currentRoute.value.fullPath, '/')
  } finally { f.cleanup() }
})

test('classic memory router: later navigation fences both pending initial and pending clicked responses', async () => {
  const f = await fixture()
  try {
    f.categories.resolve({ data: { data: rows } }); await settle()
    button(f, 'Three').click(); await settleNavigation()
    assert.equal(f.router.currentRoute.value.fullPath, '/categories/three')
    await f.router.push('/categories/two'); await settleNavigation()
    assert.equal(f.requests.length, 3)
    assert.equal(f.requests[2].params.category_id, 2)
    f.requests[1].resolve(response(3)); f.requests[0].resolve(response(1)); await settleNavigation()
    assert.equal(f.host.querySelector('.category-pill-active')?.textContent?.trim(), 'Two')
    assert.equal(f.host.querySelector('[data-row]'), null, 'neither obsolete response replaces the latest pending query')
    f.requests[2].resolve(response(2)); await settleNavigation()
    assert.equal(f.host.querySelector('[data-row]')?.textContent, 'synthetic-2')
    assert.equal(f.host.querySelector('[data-row]')?.closest('[inert]'), null)
    assert.equal(f.router.currentRoute.value.fullPath, '/categories/two')
  } finally { f.cleanup() }
})

test('classic memory router: later navigation beats an earlier category click during initialization', async () => {
  const f = await fixture()
  try {
    f.categories.resolve({ data: { data: rows } }); await settle()
    button(f, 'Three').click(); await settleNavigation()
    assert.equal(f.router.currentRoute.value.fullPath, '/categories/three')
    assert.equal(f.requests[1].params.category_id, 3)
    f.requests[1].resolve(response(3)); await settle()
    const retained = f.host.querySelector('[data-row="3"]') as HTMLElement
    assert.equal(retained.closest('[inert]'), null)
    await f.router.push('/categories/two')
    // Parent guard must reject an old card even before the inert render patch.
    retained.click(); await settleNavigation()
    assert.equal(f.router.currentRoute.value.fullPath, '/categories/two')
    assert.equal(f.host.querySelector('.category-pill-active')?.textContent?.trim(), 'Two')
    assert.equal(f.host.querySelector('[data-row]'), retained)
    assert.ok(retained.closest('[inert]'))
    assert.equal(f.requests.length, 3)
    assert.equal(f.requests[2].params.category_id, 2)
    f.requests[0].resolve(response(1)); await settleNavigation()
    assert.equal(f.host.querySelector('[data-row]'), retained)
    f.requests[2].resolve(response(2)); await settleNavigation()
    assert.equal(f.host.querySelector('[data-row]')?.textContent, 'synthetic-2')
    assert.equal(f.host.querySelector('[data-row]')?.closest('[inert]'), null)
    assert.equal(f.requests.length, 3)
  } finally { f.cleanup() }
})
