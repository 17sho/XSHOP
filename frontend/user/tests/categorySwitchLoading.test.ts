import assert from 'node:assert/strict'
import fs from 'node:fs'
import test from 'node:test'

const composable = fs.readFileSync(new URL('../src/composables/useProductList.ts', import.meta.url), 'utf8')
const products = fs.readFileSync(new URL('../src/views/Products.vue', import.meta.url), 'utf8')

test('category changes fetch immediately instead of waiting for search debounce', () => {
  const watcher = composable.match(/watch\(selectedCategory,[\s\S]*?\n  \}\)/)?.[0] ?? ''
  assert.match(watcher, /void loadProducts\(\)/)
  assert.doesNotMatch(watcher, /debouncedLoadProducts\(\)/)
})

test('stale category responses cannot replace the latest selection', () => {
  assert.match(composable, /let productRequestId = 0/)
  assert.match(composable, /const requestId = \+\+productRequestId/)
  assert.match(composable, /if \(requestId !== productRequestId\) return/)
})

test('category refresh preserves populated results with a centered loading region', () => {
  assert.match(products, /v-if="loading && !hasLoadedOnce"/)
  assert.match(products, /class="catalog-status" role="status"/)
  assert.match(products, /data-category-loading-region/)
  assert.match(products, /v-else-if="products\.length" class="catalog-feedback" :inert="catalogStale \|\| undefined"/)
  assert.doesNotMatch(products, /v-else-if="loading"/)
})
