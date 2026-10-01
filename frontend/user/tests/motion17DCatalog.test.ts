import test from 'node:test'
import assert from 'node:assert/strict'
import { loader, deferred, mountSetup, vue, settle } from './helpers/motion17DHarness.ts'

function catalog() {
  const requests: any[] = []
  const load = loader({ 'vue-router': { useRouter: () => ({ replace() {} }), useRoute: () => vue.reactive({ name: 'products', params: {} }) }, '../api': { productAPI: { list: (params: any) => { const d = deferred(); requests.push({ ...d, params }); return d.promise } }, categoryAPI: { list: async () => ({ data: { data: [] } }) } } })
  const fixture = mountSetup(() => load('composables/useProductList.ts').useProductList({ homeRouteName: 'products' }))
  return { ...fixture, requests }
}
test('latest category wins immediately and cleanup invalidates in-flight writes', async () => {
  window.sessionStorage.clear()
  const f = catalog()
  const init = f.state.initialize(); f.requests[0].resolve(response(1)); await init
  f.state.selectCategory(2); await settle()
  f.state.selectCategory(3); await settle()
  assert.equal(f.requests.length, 3)
  f.requests[2].resolve(response(3)); await settle()
  f.requests[1].resolve(response(2)); await settle()
  assert.equal(f.state.products.value[0].id, 3)
  assert.equal(f.state.catalogStale.value, false)
  const pending = f.state.loadProducts()
  f.state.cleanup(); f.unmount()
  f.requests[3].resolve(response(4)); await pending
  assert.equal(f.state.products.value[0].id, 3)
})

test('failed refresh retains rows without re-enabling stale purchase or pretending it is still loading', async () => {
  window.sessionStorage.clear()
  const f = catalog()
  const init = f.state.initialize(); f.requests[0].resolve(response(1)); await init
  const pending = f.state.loadProducts()
  const original = console.error; console.error = () => {}
  try { f.requests[1].reject(Error('synthetic')); await pending } finally { console.error = original }
  assert.equal(f.state.products.value[0].id, 1)
  assert.equal(f.state.catalogStale.value, true)
  assert.equal(f.state.loading.value, false)
  assert.equal(f.state.loadError?.value, true)
  f.state.cleanup(); f.unmount()
})

test('catalog pagination uses central reduced-motion scroll without delaying the request', async () => {
  window.sessionStorage.clear()
  const f = catalog()
  const init = f.state.initialize(); f.requests[0].resolve(response(1)); await init
  const original = window.scrollTo
  const media = window.matchMedia
  let options: any
  window.matchMedia = (() => ({ matches: true })) as any
  window.scrollTo = (value: any) => { options = value }
  try {
    f.state.changePage(2)
    assert.equal(f.requests.length, 2)
    assert.deepEqual(options, { top: 0, behavior: 'auto' })
  } finally { window.scrollTo = original; window.matchMedia = media; f.state.cleanup(); f.unmount() }
})

const response = (id: number) => ({ data: { data: [{ id, slug: `fixture-${id}` }], pagination: { total_page: 3 } } })

test('catalog keeps rows but invalidates purchase freshness immediately during search debounce', async () => {
  window.sessionStorage.clear()
  const f = catalog()
  const init = f.state.initialize(); f.requests[0].resolve(response(1)); await init
  assert.equal(f.state.catalogStale?.value, false)
  const acceptedRevision = f.state.contentRevision.value
  f.state.searchQuery.value = 'new'
  await settle()
  assert.equal(f.requests.length, 1, 'typing remains debounced')
  assert.equal(f.state.catalogStale.value, true)
  assert.equal(f.state.products.value[0].id, 1)
  assert.equal(f.state.contentRevision.value, acceptedRevision, 'pending search must not replay authoritative row motion')
  f.state.cleanup(); f.unmount()
})
