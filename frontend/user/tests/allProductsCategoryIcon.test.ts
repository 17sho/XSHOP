import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('../src/views/Products.vue', import.meta.url), 'utf8')

test('All Products category uses a full-size image icon consistent with other category tiles', () => {
  const allButton = source.match(/<button type="button" class="category-pill"[^>]*selectedCategory === null[^>]*>[\s\S]*?<\/button>/)?.[0]
  assert.ok(allButton, 'All Products category button exists')
  assert.match(allButton, /<img[^>]+:src="allProductsIcon"[^>]+class="category-icon object-cover"/)
  assert.match(source, /const allProductsIcon = '\/uploads\/category\/2026\/09\/all-products-category-v3\.svg'/)
  assert.doesNotMatch(allButton, /<FolderOpen/)
})
