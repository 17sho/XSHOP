import assert from 'node:assert/strict'
import fs from 'node:fs'
import test from 'node:test'

const read = (path: string) => fs.readFileSync(new URL(`../${path}`, import.meta.url), 'utf8')

const classicProducts = read('src/views/Products.vue')
const vaultProducts = read('src/templates/vault/Products.vue')
const appStore = read('src/stores/app.ts')
const classicList = read('src/components/ProductListCard.vue')
const vaultList = read('src/templates/vault/components/VaultProductListCard.vue')
const globalStyle = read('src/style.css')

test('storefront exposes a new product_catalog_layout config independent from removed template_mode', () => {
  assert.match(appStore, /product_catalog_layout/)
  assert.doesNotMatch(appStore, /template_mode/)
})

test('classic catalog switches between native card and list renderers', () => {
  assert.match(classicProducts, /productCatalogLayout/)
  assert.match(classicProducts, /ProductCard/)
  assert.match(classicProducts, /ProductListCard/)
  assert.match(classicProducts, /productCatalogLayout === 'list'/)
  assert.match(classicProducts, /groupedListProducts/)
  assert.match(classicProducts, /list-category-heading/)
  assert.match(classicList, /common\.viewDetails/)
  assert.doesNotMatch(classicList, /products\.quickBuy/)
  assert.match(classicList, /getFulfillmentTypeLabel/)
})

test('vault catalog switches between native card and list renderers', () => {
  assert.match(vaultProducts, /productCatalogLayout/)
  assert.match(vaultProducts, /VaultProductCard/)
  assert.match(vaultProducts, /VaultProductListCard/)
  assert.match(vaultProducts, /productCatalogLayout === 'list'/)
  assert.match(vaultProducts, /groupedListProducts/)
  assert.match(vaultProducts, /list-category-heading/)
  assert.match(vaultList, /products\.quickBuy/)
  assert.match(vaultList, /getFulfillmentTypeLabel/)
})

test('list renderers preserve pricing signals without nested interactive controls', () => {
  for (const source of [classicList, vaultList]) {
    assert.match(source, /originalPrice/)
    assert.match(source, /priceSignal/)
    assert.match(source, /wholesaleTag/)
  }
  const nestedInteractiveLink = vaultList.match(/<RouterLink\b[\s\S]*?<\/RouterLink>/)?.[0] ?? ''
  assert.doesNotMatch(nestedInteractiveLink, /<button\b/)
})

test('list rows and category selection have visible motion feedback', () => {
  assert.match(classicProducts, /v-for="\(product, productIndex\) in group\.products"/)
  assert.match(classicProducts, /:index="productIndex"/)
  assert.match(vaultProducts, /v-for="\(product, productIndex\) in group\.products"/)
  assert.match(vaultProducts, /:index="productIndex"/)
  for (const source of [classicList, vaultList]) {
    assert.match(source, /theme-slide-up/)
    assert.match(source, /animationDelay/)
  }
  assert.match(classicProducts, /category-active-indicator[^}]*transition:/s)
  assert.match(classicProducts, /\.category-pill:active[^}]*transform:/s)
  assert.match(globalStyle, /\.theme-slide-up\s*\{[^}]*animation:[^;]*backwards/s)
  assert.match(classicProducts, /prefers-reduced-motion:\s*reduce[\s\S]*\.category-pill/s)
  assert.match(classicList, /prefers-reduced-motion:\s*reduce[\s\S]*\.compact-product-row/s)
  assert.match(vaultList, /prefers-reduced-motion:\s*reduce[\s\S]*\.vault-compact-row/s)
})

test('list rows follow the upstream compact single-row hierarchy', () => {
  assert.match(classicList, /compact-product-row[^\n]*flex-row/)
  assert.match(classicList, /h-11 w-11[^\n]*sm:h-16 sm:w-16/)
  assert.match(classicList, /truncate text-xs[^\n]*sm:text-sm/)
  assert.match(classicList, /h-7 w-7[^\n]*sm:h-8 sm:w-8/)
  assert.match(classicList, /ArrowRight/)
  assert.doesNotMatch(classicList, /products\.quickBuyAria/)
  assert.match(classicList, /class="hidden flex-none sm:block"[^\n]*ChevronRight/)
  assert.doesNotMatch(classicList, /compact-product-footer|@media \(max-width:767px\)/)

  assert.match(vaultList, /vault-compact-row[^\n]*flex items-center gap-3/)
  assert.match(vaultList, /h-14 w-14[^\n]*sm:h-16 sm:w-16/)
  assert.match(vaultList, /truncate text-sm font-bold/)
  assert.match(vaultList, /h-9 w-9[^\n]*rounded-full/)
  assert.match(vaultList, /ShoppingCart/)
  assert.doesNotMatch(vaultList, /vault-product-actions[^\n]*grid-column|@media \(max-width:767px\)/)
})

test('category headings use a compact count badge in both themes', () => {
  for (const source of [classicProducts, vaultProducts]) {
    assert.match(source, /list-category-count/)
    assert.doesNotMatch(source, />\(\{\{ group\.products\.length \}\}\)</)
  }
})
