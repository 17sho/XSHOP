import assert from 'node:assert/strict'
import fs from 'node:fs'
import test from 'node:test'

const app = fs.readFileSync(new URL('../src/App.vue', import.meta.url), 'utf8')
const router = fs.readFileSync(new URL('../src/router/index.ts', import.meta.url), 'utf8')

test('home and category URLs reuse the catalog component without a route key', () => {
  assert.match(router, /name: 'products',[\s\S]*?component: Products/)
  assert.match(router, /name: 'category-products',[\s\S]*?component: Products/)
  assert.match(router, /isCatalogCategoryTransition\(to\.name, _from\.name\)/)
  assert.doesNotMatch(app, /:key="route\.fullPath"|:key="routeRenderKey\(route\)"/)
})
