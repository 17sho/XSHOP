import assert from 'node:assert/strict'
import fs from 'node:fs'
import test from 'node:test'

const products = fs.readFileSync(new URL('../src/views/Products.vue', import.meta.url), 'utf8')

test('mobile category cards wrap as compact reference-sized chips, without horizontal scrolling', () => {
  assert.match(products, /class="category-card-grid[^"\n]*"/)
  assert.match(products, /\.category-card-grid\s*\{[^}]*display:flex;[^}]*flex-wrap:wrap/s)
  assert.match(products, /\.category-pill\s*\{[^}]*flex-direction:row;[^}]*min-height:2\.75rem/s)
  assert.match(products, /\.category-pill\s*\{[^}]*padding:\.5rem \.75rem/s)
  assert.doesNotMatch(products, /category-card-grid[^"\n]*overflow-x-auto/)
  assert.doesNotMatch(products, /\.category-card-grid\s*\{[^}]*overflow-x:auto/s)
})

test('category labels and icons remain visible on narrow screens', () => {
  assert.match(products, /\.category-pill\s*\{[^}]*min-width:0/s)
  assert.match(products, /\.category-pill span:not\(\.category-active-indicator\):not\(\.category-icon\)\s*\{[^}]*overflow-wrap:anywhere/s)
  assert.match(products, /\.category-icon\s*\{[^}]*width:1\.625rem;[^}]*height:1\.625rem/s)
})
