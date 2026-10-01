import assert from 'node:assert/strict'
import fs from 'node:fs'
import test from 'node:test'

const read = (path: string) => fs.readFileSync(new URL(`../src/${path}`, import.meta.url), 'utf8')
const app = read('App.vue')
const productList = read('composables/useProductList.ts')
const navbar = read('components/Navbar.vue')
const mobileNav = read('components/MobileBottomNav.vue')

test('route swaps use upstream out-in opacity without query remounts or custom readiness', () => {
  assert.equal((app.match(/<RouterView v-slot="\{ Component \}">/g) || []).length, 2)
  assert.equal((app.match(/<Transition name="page-fade" mode="out-in"[^>]*>/g) || []).length, 2)
  assert.match(app, /transition: opacity 200ms ease;/)
  assert.doesNotMatch(app, /fullPath|:key=|playRouteEnter|routeAnimation|data-route-motion|transform/)
})

test('category and pagination clicks start requests without artificial waits', () => {
  assert.doesNotMatch(productList, /minimumLoadingMs|waitForMinimumLoading/)
  const categoryWatcher = productList.match(/watch\(selectedCategory,[\s\S]*?\n  \}\)/)?.[0] ?? ''
  assert.match(categoryWatcher, /void loadProducts\(\)/)
  const changePage = productList.match(/const changePage[\s\S]*?\n  \}/)?.[0] ?? ''
  assert.match(changePage, /debouncedLoadProducts\.cancel\(\)[\s\S]*void loadProducts\(\)/)
})

test('fixed mobile chrome avoids expensive backdrop blur and transition-all', () => {
  assert.doesNotMatch(navbar, /backdrop-blur|transition-all/)
  assert.doesNotMatch(mobileNav, /backdrop-blur|transition-all/)
})
