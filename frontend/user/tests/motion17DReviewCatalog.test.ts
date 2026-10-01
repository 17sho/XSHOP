import test from 'node:test'
import assert from 'node:assert/strict'
import { loader, vue, settle, deferred, mountSetup } from './helpers/motion17DHarness.ts'

const response = (id: number) => ({ data: { data: [{ id, slug: `fixture-${id}` }], pagination: { total_page: 3 } } })
const categoryRows = [{ id: 2, slug: 'two', name: { en: 'Two' } }, { id: 3, slug: 'three', name: { en: 'Three' } }]
// Only the browser timer boundary is controlled; use the real debounce implementation.
function clock() {
  const originalSet = window.setTimeout, originalClear = window.clearTimeout
  let now = 0, id = 0
  const pending = new Map<number, { at: number; fn: () => void }>()
  window.setTimeout = ((fn: () => void, delay: number) => { pending.set(++id, { at: now + delay, fn }); return id }) as any
  window.clearTimeout = ((key: number) => { pending.delete(key) }) as any
  return {
    async tick(ms: number) { now += ms; for (const [key, timer] of pending) if (timer.at <= now) { pending.delete(key); timer.fn() }; await settle() },
    restore() { window.setTimeout = originalSet; window.clearTimeout = originalClear },
  }
}
const card = { props: ['product'], emits: ['quickBuy'], setup(p: any, { emit }: any) { return () => vue.h('button', { 'data-product': p.product.id, onClick: () => emit('quickBuy', p.product) }, p.product.slug) } }
function fixture({ view = false, warm = false, routeCategory = false, layout = 'card' } = {}): any {
  window.sessionStorage.clear()
  if (warm) loader()('utils/publicCatalogCache.ts').writePublicCatalogCache(window.sessionStorage, window.location.host, { products: [{ id: 99, slug: 'cached' }], categories: categoryRows, totalPages: 3 })
  const requests: any[] = [], categories = deferred(), replacements: any[] = []
  const route = vue.reactive({ name: routeCategory ? 'category-products' : 'products', path: '/products', params: routeCategory ? { slug: 'two' } : {} })
  const localized = { useLocalized: () => ({ getLocalizedText: (v: any) => v?.en || '' }) }
  const load = loader({
    'vue-router': { useRouter: () => ({ replace: (to: any) => { replacements.push(to); Object.assign(route, to) } }), useRoute: () => route },
    'vue-i18n': { useI18n: () => ({ t: (s: string) => s }) }, '@/components/ui/button': { Button: 'button' },
    '../api': { productAPI: { list: (params: any) => { const r = deferred(); requests.push({ ...r, params }); return r.promise } }, categoryAPI: { list: () => categories.promise } },
    '../../composables/useProduct': localized, '../../../composables/useProduct': localized,
    '../../composables/usePageSeo': { usePageSeo() {} },
    '../../stores/app': { useAppStore: () => ({ productCatalogLayout: layout, config: {} }) },
    '../../utils/image': { getImageUrl: (s: string) => s }, '../../../utils/image': { getImageUrl: (s: string) => s },
    './components/VaultProductCard.vue': { default: card }, './components/VaultProductListCard.vue': { default: card },
    '../../components/ProductQuickBuy.vue': { default: { props: ['visible'], setup: (p: any) => () => p.visible ? vue.h('div', { 'data-quick-buy': '' }, 'synthetic; no purchase') : null } },
  })
  if (!view) {
    const f = mountSetup(() => load('composables/useProductList.ts').useProductList({ homeRouteName: 'products' }))
    return { ...f, requests, categories, replacements, route, cleanup() { f.state.cleanup(); f.unmount() } }
  }
  const host = document.createElement('div'); document.body.append(host)
  const app = vue.createApp(load('templates/vault/Products.vue').default)
  app.component('RouterLink', { template: '<a><slot /></a>' }); app.mount(host)
  return { host, requests, categories, replacements, route, cleanup() { app.unmount(); host.remove() } }
}
function categoryButton(host: HTMLElement, label: string): HTMLButtonElement {
  const button = Array.from(host.querySelectorAll('aside button')).find(button => button.textContent?.trim() === label)
  assert.ok(button, `rendered category control ${label}`)
  return button as HTMLButtonElement
}
function type(host: HTMLElement, value: string) {
  const input = host.querySelector('input')!; input.value = value; input.dispatchEvent(new Event('input', { bubbles: true }))
}

for (const layout of ['card', 'list']) test(`real Vault ${layout} search during categories wait disables retained row actions and reconciles latest query`, async () => {
  const time = clock(), f = fixture({ view: true, layout })
  try {
    f.requests[0].resolve(response(1)); await settle()
    const row = f.host!.querySelector('[data-product]') as HTMLElement
    assert.ok(row); assert.equal(row.closest('[inert]'), null)
    type(f.host!, 'first'); row.click(); await settle()
    assert.equal(f.host!.querySelector('[data-quick-buy]'), null, 'synchronous search invalidation blocks actions before Vue patches inert')
    assert.equal(f.host!.querySelector('[data-product]'), row, 'keep row DOM during refresh')
    assert.ok(row.closest('[inert]'), 'typing before initialization finishes must invalidate row actionability')
    row.click(); await settle()
    assert.equal(f.host!.querySelector('[data-quick-buy]'), null, 'real parent guard rejects synthetic quick-buy emit even without native inert')
    await time.tick(200); type(f.host!, 'latest')
    f.categories.resolve({ data: { data: categoryRows } }); await settle()
    await time.tick(299); assert.equal(f.requests.length, 1)
    await time.tick(1); assert.equal(f.requests.length, 2)
    assert.deepEqual(f.requests[1].params, { page: 1, page_size: 12, search: 'latest' })
    f.requests[1].resolve(response(2)); await settle()
    const fresh = f.host!.querySelector('[data-product="2"]') as HTMLElement
    assert.equal(fresh.closest('[inert]'), null)
    fresh.click(); await settle(); assert.ok(f.host!.querySelector('[data-quick-buy]'), 'fresh rows remain actionable')
  } finally { f.cleanup(); time.restore() }
})

test('initial search invalidates an in-flight unfiltered generation synchronously and survives late categories', async () => {
  const time = clock(), f = fixture()
  try {
    const init = f.state.initialize()
    f.state.searchQuery.value = 'new'
    assert.equal(f.state.catalogStale.value, true)
    await time.tick(299); assert.equal(f.requests.length, 1)
    await time.tick(1); assert.equal(f.requests.length, 2, 'initialization must not discard debounced search')
    f.requests[1].resolve(response(2)); await settle()
    assert.equal(f.state.catalogStale.value, false)
    f.requests[0].resolve(response(1)); f.categories.resolve({ data: { data: categoryRows } }); await init; await settle()
    assert.equal(f.state.products.value[0].id, 2)
    assert.equal(f.state.catalogStale.value, false)
    assert.equal(f.requests.length, 2, 'initialization does not replay an already authoritative search')
  } finally { f.cleanup(); time.restore() }
})

test('real warm Vault sidebar selection survives pending initial refresh and fences unfiltered rows', async () => {
  const f = fixture({ view: true, warm: true })
  try {
    await settle()
    const cachedRow = f.host.querySelector('[data-product="99"]')
    assert.ok(cachedRow.closest('[inert]'))
    categoryButton(f.host, 'Two').click(); await settle()
    f.requests[0].resolve(response(1)); f.categories.resolve({ data: { data: categoryRows } }); await settle()
    assert.ok(categoryButton(f.host, 'Two').classList.contains('bg-primary'), 'initialization must preserve the clicked category')
    assert.equal(f.requests.length, 2)
    assert.equal(f.requests[1].params.category_id, 2)
    assert.equal(f.host.querySelector('[data-product]'), cachedRow, 'old unfiltered response cannot replace retained cache')
    cachedRow.click(); await settle(); assert.equal(f.host.querySelector('[data-quick-buy]'), null)
    f.requests[1].resolve(response(2)); await settle()
    const fresh = f.host.querySelector('[data-product="2"]') as HTMLElement
    assert.equal(fresh.closest('[inert]'), null)
    fresh.click(); await settle(); assert.ok(f.host.querySelector('[data-quick-buy]'))
  } finally { f.cleanup() }
})

test('warm initial category changes stay immediate, cancel typing and latest intent wins out of order', async () => {
  const time = clock(), f = fixture({ warm: true })
  try {
    const init = f.state.initialize()
    f.state.searchQuery.value = 'query'
    f.state.selectCategory(2); await settle()
    assert.equal(f.requests.length, 2, 'category selection remains immediate, including during initialization')
    assert.deepEqual(f.requests[1].params, { page: 1, page_size: 20, category_id: 2, search: 'query' })
    f.state.selectCategory(3); await settle()
    assert.equal(f.requests.length, 3)
    await time.tick(300); assert.equal(f.requests.length, 3, 'explicit category cancels queued search')
    f.requests[2].resolve(response(3)); await settle()
    f.requests[1].resolve(response(2)); f.requests[0].resolve(response(1))
    f.categories.resolve({ data: { data: categoryRows } }); await init; await settle()
    assert.equal(f.state.selectedCategory.value, 3)
    assert.equal(f.state.products.value[0].id, 3)
    assert.equal(f.state.catalogStale.value, false)
    assert.equal(f.requests.length, 3, 'initialization must not issue an unfiltered replacement')
  } finally { f.cleanup(); time.restore() }
})

for (const view of [false, true]) test(`route initial category without override stays correct (${view ? 'real Vault' : 'composable'})`, async () => {
  const f = fixture({ view, routeCategory: true })
  try {
    const init = view ? null : f.state.initialize()
    f.requests[0].resolve(response(1)); f.categories.resolve({ data: { data: categoryRows } }); await init; await settle()
    assert.equal(f.requests.length, 2)
    assert.equal(f.requests[1].params.category_id, 2)
    if (view) assert.ok(categoryButton(f.host, 'Two').classList.contains('bg-primary'))
    else assert.equal(f.state.selectedCategory.value, 2)
    f.requests[1].resolve(response(2)); await settle()
    if (view) assert.equal(f.host.querySelector('[data-product="2"]').closest('[inert]'), null)
    else { assert.equal(f.state.products.value[0].id, 2); assert.equal(f.state.catalogStale.value, false) }
  } finally { f.cleanup() }
})

test('real route category does not overwrite an explicit All click while categories are pending', async () => {
  const f = fixture({ view: true, routeCategory: true })
  try {
    categoryButton(f.host, 'products.allCategories').click(); await settle()
    f.requests[0].resolve(response(1)); f.categories.resolve({ data: { data: categoryRows } }); await settle()
    assert.ok(categoryButton(f.host, 'products.allCategories').classList.contains('bg-primary'))
    assert.equal(f.requests.length, 1, 'same-value explicit All choice needs no redundant request')
  } finally { f.cleanup() }
})

test('real route category honors a newer category click while the initial products are pending', async () => {
  const f = fixture({ view: true, routeCategory: true })
  try {
    f.categories.resolve({ data: { data: categoryRows } }); await settle()
    categoryButton(f.host, 'Three').click(); await settle()
    assert.equal(f.requests[1].params.category_id, 3)
    assert.equal(f.route.params.slug, 'three')
    f.requests[1].resolve(response(3)); f.requests[0].resolve(response(1)); await settle()
    assert.ok(categoryButton(f.host, 'Three').classList.contains('bg-primary'))
    assert.equal(f.requests.length, 2)
    assert.ok(f.host.querySelector('[data-product="3"]'))
  } finally { f.cleanup() }
})

test('initial search failure retains stale rows and explicit retry restores actions', async () => {
  const time = clock(), f = fixture(), originalError = console.error
  console.error = () => {}
  try {
    const init = f.state.initialize(); f.requests[0].resolve(response(1)); await settle()
    f.state.searchQuery.value = 'retry'; await time.tick(300)
    f.requests[1].reject(Error('synthetic read outage')); await settle()
    f.categories.resolve({ data: { data: categoryRows } }); await init
    assert.equal(f.state.products.value[0].id, 1)
    assert.equal(f.state.catalogStale.value, true)
    assert.equal(f.state.loadError.value, true)
    assert.equal(f.state.loading.value, false)
    const retry = f.state.loadProducts()
    assert.equal(f.requests[2].params.search, 'retry')
    f.requests[2].resolve(response(2)); await retry
    assert.equal(f.state.catalogStale.value, false); assert.equal(f.state.loadError.value, false)
  } finally { f.cleanup(); time.restore(); console.error = originalError }
})

test('disposal during initialization cancels queued search and fences late initial responses', async () => {
  const time = clock(), f = fixture({ warm: true })
  try {
    const init = f.state.initialize(); f.state.searchQuery.value = 'disposed'
    f.state.cleanup()
    await time.tick(300); assert.equal(f.requests.length, 1)
    f.requests[0].resolve(response(1)); f.categories.resolve({ data: { data: [] } }); await init
    assert.equal(f.state.products.value[0].id, 99)
    assert.equal(f.state.categories.value.length, categoryRows.length)
    assert.equal(f.state.catalogStale.value, true)
  } finally { f.cleanup(); time.restore() }
})

test('normal warm initialization remains silent, with one authoritative refresh and no redundant request', async () => {
  const f = fixture({ warm: true })
  try {
    const init = f.state.initialize()
    assert.equal(f.state.loading.value, false)
    assert.equal(f.state.products.value[0].id, 99)
    assert.equal(f.state.catalogStale.value, true)
    f.requests[0].resolve(response(1)); f.categories.resolve({ data: { data: categoryRows } }); await init; await settle()
    assert.equal(f.requests.length, 1)
    assert.equal(f.state.selectedCategory.value, null)
    assert.equal(f.state.catalogStale.value, false)
    assert.equal(f.state.products.value[0].id, 1)
  } finally { f.cleanup() }
})

test('normal initialization keeps immediate requests, cache timing and post-init debounce', async () => {
  const time = clock(), f = fixture()
  try {
    const init = f.state.initialize()
    assert.equal(f.requests.length, 1)
    f.requests[0].resolve(response(1)); await settle()
    assert.equal(f.state.catalogStale.value, false, 'category wait alone must not disable authoritative rows')
    const readCache = () => loader()('utils/publicCatalogCache.ts').readPublicCatalogCache(window.sessionStorage, window.location.host)
    assert.equal(readCache(), null)
    f.categories.resolve({ data: { data: categoryRows } }); await init
    assert.equal(readCache().products[0].id, 1)
    assert.equal(f.requests.length, 1)
    f.state.searchQuery.value = 'post-init'
    assert.equal(f.state.catalogStale.value, true)
    await time.tick(299); assert.equal(f.requests.length, 1)
    await time.tick(1); assert.equal(f.requests[1].params.search, 'post-init')
  } finally { f.cleanup(); time.restore() }
})
