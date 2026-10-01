import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const router = readFileSync(new URL('../src/router/index.ts', import.meta.url), 'utf8')
const products = readFileSync(new URL('../src/views/Products.vue', import.meta.url), 'utf8')
const list = readFileSync(new URL('../src/composables/useProductList.ts', import.meta.url), 'utf8')

test('homepage filter changes do not navigate, as on the reference storefront', () => {
  assert.match(products, /useProductList\(\{ pageSize: 12, homeRouteName: 'products' \}\)/)
  assert.doesNotMatch(list, /syncCategoryRoute\?: boolean/)
  assert.match(list, /if \(route\.name !== homeRouteName\) \{[\s\S]*?router\.replace/)
})

test('home and category URLs reuse the same catalog component and retain scroll', () => {
  assert.match(router, /if \(isCatalogCategoryTransition\(to\.name, _from\.name\)\) \{\s*return false/)
  assert.match(router, /path: '\/categories\/:slug',[\s\S]*?component: Products,/)
})

test('category switches retain results and prevent stale-result actions', () => {
  assert.doesNotMatch(products, /v-else-if="loading"/)
  assert.match(products, /v-else-if="products\.length"/)
  assert.match(products, /<main class="category-results"[^>]*:aria-busy=/)
  assert.match(products, /:inert="catalogStale \|\| undefined"/)
  assert.match(products, /\.category-results \{ min-height:100dvh; \}/)
})

test('category cards retain original per-row entry without whole-catalog motion', () => {
  assert.doesNotMatch(products, /\.category-results\s+:deep\(\.theme-slide-up\)\s*\{\s*animation:\s*none/)
  assert.match(products, /<ProductCard\s+v-for=/)
  assert.match(products, /:animation-step="50"/)
  assert.doesNotMatch(products, /catalog-settled|v-motion-change|suppressEntrance/)
})

test('category results appear as soon as the API responds', () => {
  assert.match(list, /void loadProducts\(\)/)
  assert.doesNotMatch(list, /minimumLoadingMs|waitForMinimumLoading/)
})
