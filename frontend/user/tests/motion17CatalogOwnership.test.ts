// Catalog-only reviewer counterprobes. Synthetic route fixture complements actual classic/router tests.
import test from 'node:test'
import assert from 'node:assert/strict'
import { loader, vue, settle, deferred, mountSetup } from './helpers/motion17DHarness.ts'

const rows = [{ id: 2, slug: 'two', parent_id: 0, name: { en: 'Two' } }, { id: 3, slug: 'three', parent_id: 2, name: { en: 'Three' } }]
const response = (id: number) => ({ data: { data: [{ id, slug: `synthetic-${id}` }], pagination: { total_page: 3 } } })
function clock() {
  const set = window.setTimeout, clear = window.clearTimeout
  let now = 0, serial = 0
  const pending = new Map<number, any>()
  window.setTimeout = ((fn: any, ms: number) => { pending.set(++serial, { fn, at: now + ms }); return serial }) as any
  window.clearTimeout = ((id: number) => pending.delete(id)) as any
  return { async tick(ms: number) { now += ms; for (const [id, t] of pending) if (t.at <= now) { pending.delete(id); t.fn() }; await settle() }, restore() { window.setTimeout = set; window.clearTimeout = clear } }
}
function catalog({ view = false, warm = false, category = false } = {}): any {
  window.sessionStorage.clear()
  const cache = loader()('utils/publicCatalogCache.ts')
  if (warm) cache.writePublicCatalogCache(window.sessionStorage, window.location.host, { products: [{ id: 99, slug: 'cached' }], categories: rows, totalPages: 3 })
  const requests: any[] = [], categories = deferred(), replacements: any[] = []
  const route = vue.reactive({ name: category ? 'category-products' : 'products', path: category ? '/category/two' : '/products', params: category ? { slug: 'two' } : {} })
  const localized = { useLocalized: () => ({ getLocalizedText: (x: any) => x?.en || '' }) }
  const card = { props: ['product'], emits: ['quickBuy'], setup(p: any, { emit }: any) { return () => vue.h('button', { 'data-row': p.product.id, onClick: () => emit('quickBuy', p.product) }, p.product.slug) } }
  const load = loader({
    'vue-router': { useRouter: () => ({ replace: (to: any) => { replacements.push(to); Object.assign(route, to) } }), useRoute: () => route },
    '../api': { productAPI: { list: (params: any) => { const d = deferred(); requests.push({ ...d, params }); return d.promise } }, categoryAPI: { list: () => categories.promise } },
    'vue-i18n': { useI18n: () => ({ t: (x: string) => x }) }, '@/components/ui/button': { Button: 'button' },
    '../../composables/useProduct': localized, '../../../composables/useProduct': localized,
    '../../composables/usePageSeo': { usePageSeo() {} }, '../../stores/app': { useAppStore: () => ({ productCatalogLayout: 'card', config: {} }) },
    '../../utils/image': { getImageUrl: (x: string) => x }, '../../../utils/image': { getImageUrl: (x: string) => x },
    './components/VaultProductCard.vue': { default: card }, './components/VaultProductListCard.vue': { default: card },
    '../../components/ProductQuickBuy.vue': { default: { props: ['visible'], setup: (p: any) => () => p.visible ? vue.h('div', { 'data-open-buy': '' }) : null } },
  })
  const readCache = () => cache.readPublicCatalogCache(window.sessionStorage, window.location.host)
  if (!view) {
    const f = mountSetup(() => load('composables/useProductList.ts').useProductList({ homeRouteName: 'products' }))
    return { ...f, requests, categories, route, replacements, readCache, cleanup() { f.state.cleanup(); f.unmount() } }
  }
  const host = document.createElement('div'); document.body.append(host)
  const app = vue.createApp(load('templates/vault/Products.vue').default)
  app.component('RouterLink', { template: '<a><slot /></a>' }); app.mount(host)
  return { host, requests, categories, route, replacements, readCache, cleanup() { app.unmount(); host.remove() } }
}
const type = (f: any, value: string) => { const input = f.host.querySelector('input'); input.value = value; input.dispatchEvent(new Event('input', { bubbles: true })) }
const all = (f: any) => Array.from(f.host.querySelectorAll('aside button')).find((b: any) => b.textContent.trim() === 'products.allCategories') as HTMLElement

test('ownership: exposed page identity binds freshness as well as a late cache commit', async () => {
  // Direct-composable contract: the live views use changePage, covered separately below.
  const f = catalog({ warm: true })
  try {
    const init = f.state.initialize()
    f.requests[0].resolve(response(1)); await settle()
    assert.equal(f.state.catalogStale.value, false)
    f.state.currentPage.value = 2
    assert.equal(f.state.catalogStale.value, true, 'freshness is derived from the displayed query, not a remembered success flag')
    f.categories.resolve({ data: { data: rows } }); await init
    assert.equal(f.readCache().products[0].id, 99)
    assert.equal(f.state.catalogStale.value, true)
  } finally { f.cleanup() }
})

for (const productsFirst of [false, true]) test(`ownership: warm refresh commits a complete captured tuple only after both responses (productsFirst=${productsFirst})`, async () => {
  const f = catalog({ warm: true })
  try {
    const init = f.state.initialize(), before = f.readCache()
    assert.equal(f.state.loading.value, false, 'warm refresh is silent')
    const products = { data: { data: [{ id: 8, slug: 'fresh-all' }], pagination: { total_page: 7 } } }
    const categories = { data: { data: [rows[1]] } }
    if (productsFirst) f.requests[0].resolve(products)
    else f.categories.resolve(categories)
    await settle()
    assert.deepEqual(f.readCache(), before)
    if (productsFirst) f.categories.resolve(categories)
    else f.requests[0].resolve(products)
    await init
    assert.deepEqual(f.readCache(), { products: products.data.data, categories: categories.data.data, totalPages: 7 })
    assert.equal(f.state.catalogStale.value, false)
    assert.equal(f.requests.length, 1)
  } finally { f.cleanup() }
})

test('ownership: page two rows cannot become a page one cache tuple when categories arrive during the return request', async () => {
  const f = catalog({ warm: true })
  try {
    const init = f.state.initialize()
    f.state.changePage(2); assert.equal(f.requests[1].params.page, 2)
    f.requests[1].resolve(response(2)); await settle()
    f.state.changePage(1); assert.equal(f.requests[2].params.page, 1)
    assert.equal(f.state.catalogStale.value, true)
    f.categories.resolve({ data: { data: rows } }); await settle()
    assert.equal(f.readCache().products[0].id, 99)
    f.requests[0].resolve(response(1)); await init
    assert.equal(f.state.products.value[0].id, 2)
    assert.equal(f.state.loading.value, true)
    f.requests[2].resolve({ data: { data: [{ id: 7 }], pagination: { total_page: 5 } } }); await settle()
    assert.deepEqual(f.readCache(), { products: [{ id: 7 }], categories: rows, totalPages: 5 })
    assert.equal(f.state.catalogStale.value, false)
  } finally { f.cleanup() }
})

test('ownership: category, trimmed search and page jointly bind request display and exclude filtered cache writes', async () => {
  const t = clock(), f = catalog({ warm: true })
  try {
    const init = f.state.initialize()
    f.state.searchQuery.value = '  latest  '
    f.state.selectCategory(3)
    f.state.changePage(2)
    assert.equal(f.requests.length, 3, 'each explicit action dispatches synchronously with the current query')
    assert.deepEqual(f.requests[2].params, { page: 2, page_size: 20, category_id: 3, search: 'latest' })
    await t.tick(300); assert.equal(f.requests.length, 3, 'explicit page cancels the search timer')
    f.requests[1].resolve(response(3)); f.requests[0].resolve(response(1))
    f.categories.resolve({ data: { data: rows } }); await init
    assert.equal(f.state.products.value[0].id, 99)
    assert.equal(f.state.loading.value, true)
    f.requests[2].resolve(response(8)); await settle()
    assert.equal(f.state.products.value[0].id, 8)
    assert.equal(f.state.catalogStale.value, false)
    assert.equal(f.readCache().products[0].id, 99)
  } finally { f.cleanup(); t.restore() }
})

test('independent: search before initialize retains intent; immediate explicit pagination cancels queued search', async () => {
  const t = clock(), f = catalog({ warm: true })
  try {
    f.state.searchQuery.value = 'prior'; const init = f.state.initialize()
    assert.equal(f.requests[0].params.search, 'prior')
    f.state.changePage(2); assert.equal(f.requests[1].params.page, 2)
    await t.tick(300); assert.equal(f.requests.length, 2)
    f.requests[0].resolve(response(1)); f.categories.resolve({ data: { data: rows } }); await init
    assert.equal(f.state.catalogStale.value, true)
    f.requests[1].resolve(response(2)); await settle()
    assert.equal(f.state.products.value[0].id, 2); assert.equal(f.state.catalogStale.value, false)
    assert.equal(f.readCache().products[0].id, 99, 'filtered/page-2 response cannot replace home snapshot')
  } finally { f.cleanup(); t.restore() }
})

test('independent: stale rejection/finally cannot release latest pending initialization search', async () => {
  const t = clock(), f = catalog({ warm: true })
  try {
    const init = f.state.initialize(); f.state.searchQuery.value = 'new'; await t.tick(300)
    f.requests[0].reject(Error('stale synthetic failure')); f.categories.resolve({ data: { data: rows } }); await init
    assert.equal(f.state.loading.value, true); assert.equal(f.state.loadError.value, false); assert.equal(f.state.catalogStale.value, true)
    f.requests[1].resolve(response(2)); await settle()
    assert.equal(f.state.loading.value, false); assert.equal(f.state.products.value[0].id, 2)
  } finally { f.cleanup(); t.restore() }
})

test('independent: late product errors and category success after disposal leave presentation/cache untouched', async () => {
  const f = catalog({ warm: true })
  try {
    const init = f.state.initialize(); const before = JSON.stringify(f.readCache()); f.state.cleanup()
    const state = [f.state.loading.value, f.state.hasLoadedOnce.value, f.state.loadError.value]
    f.requests[0].reject(Error('disposed synthetic failure')); f.categories.resolve({ data: { data: [] } }); await init
    assert.deepEqual([f.state.loading.value, f.state.hasLoadedOnce.value, f.state.loadError.value], state)
    assert.equal(JSON.stringify(f.readCache()), before)
  } finally { f.cleanup() }
})

test('independent: category endpoint failure does not discard initialization search', async () => {
  const t = clock(), f = catalog(), old = console.error; console.error = () => {}
  try {
    const init = f.state.initialize(); f.state.searchQuery.value = 'latest'; await t.tick(300)
    f.requests[1].resolve(response(2)); f.requests[0].resolve(response(1)); f.categories.reject(Error('synthetic category outage')); await init; await settle()
    assert.equal(f.state.products.value[0].id, 2); assert.equal(f.state.catalogStale.value, false); assert.equal(f.readCache(), null)
  } finally { f.cleanup(); t.restore(); console.error = old }
})

test('independent: cached child choice expands its latest parent after categories refresh', async () => {
  const f = catalog({ warm: true })
  try {
    const init = f.state.initialize(); f.state.selectCategory(3); await settle()
    f.requests[1].resolve(response(3)); f.requests[0].resolve(response(1)); f.categories.resolve({ data: { data: rows } }); await init; await settle()
    assert.equal(f.state.selectedCategory.value, 3); assert.ok(f.state.expandedParentIds.value.includes(2)); assert.equal(f.requests.length, 2)
  } finally { f.cleanup() }
})

test('boundary invariant: explicit initial All also reconciles the category URL', async () => {
  const f = catalog({ view: true, category: true })
  try {
    all(f).click(); await settle(); f.requests[0].resolve(response(1)); f.categories.resolve({ data: { data: rows } }); await settle()
    assert.equal(f.route.name, 'products', 'explicit All must not leave category/two URL showing unfiltered products')
  } finally { f.cleanup() }
})

test('boundary invariant: later route intent beats an earlier category override during initialize', async () => {
  const f = catalog({ category: true })
  try {
    const init = f.state.initialize(); f.categories.resolve({ data: { data: rows } }); await settle()
    f.state.selectCategory(3); await settle(); assert.equal(f.route.params.slug, 'three')
    f.route.params.slug = 'two'; await settle()
    f.requests[1].resolve(response(3)); f.requests[0].resolve(response(1)); await init; await settle()
    assert.equal(f.state.selectedCategory.value, 2, 'newer navigation cannot be swallowed by historical initial override')
  } finally { f.cleanup() }
})

test('boundary invariant: category route must not expose unfiltered rows as actionable while categories pending', async () => {
  const f = catalog({ view: true, category: true })
  try {
    f.requests[0].resolve(response(1)); await settle()
    const row = f.host.querySelector('[data-row]'); row?.click(); await settle()
    assert.equal(f.host.querySelector('[data-open-buy]'), null, 'route filter unresolved: unfiltered first response must not become actionable')
  } finally { f.cleanup() }
})

test('boundary invariant: filtered rows cannot be persisted as All snapshot when search cleared before categories arrive', async () => {
  const t = clock(), f = catalog({ view: true })
  try {
    f.requests[0].resolve(response(1)); await settle(); type(f, 'filtered'); await t.tick(300)
    f.requests[1].resolve(response(2)); await settle(); type(f, '')
    f.categories.resolve({ data: { data: rows } }); await settle()
    assert.notEqual(f.readCache()?.products[0].id, 2, 'pending unfiltered intent must not relabel filtered rows as home snapshot')
  } finally { f.cleanup(); t.restore() }
})
