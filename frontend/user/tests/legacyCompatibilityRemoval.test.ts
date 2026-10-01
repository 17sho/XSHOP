import test from 'node:test'
import assert from 'node:assert/strict'
import { existsSync, readFileSync } from 'node:fs'

const root = new URL('../', import.meta.url)
const read = (path: string) => readFileSync(new URL(path, root), 'utf8')

const removedPages = [
  'src/views/Home.vue',
  'src/templates/vault/Home.vue',
  'src/components/CategorySidebar.vue',
  'src/components/ProductListItem.vue',
  'src/templates/vault/components/VaultProductListItem.vue',
  'src/composables/useProductListGroups.ts',
  'src/views/Cart.vue',
  'src/templates/vault/Cart.vue',
  'src/composables/useCart.ts',

]

test('legacy storefront pages and hidden panels are physically removed', () => {
  for (const path of removedPages) {
    assert.equal(existsSync(new URL(path, root)), false, path)
  }
})

test('router contains no legacy home or cart while advanced personal-center routes are restored', () => {
  const router = read('src/router/index.ts')
  assert.doesNotMatch(router, /homeViewLoader|templateView\('Home'|template_mode\s*===\s*['"]list['"]|path:\s*['"]\/cart['"]/)
  for (const path of ['/me/api', '/me/affiliate', '/me/reseller']) assert.match(router, new RegExp(`path:\\s*['"]${path.replaceAll('/', '\\/')}['"]`), path)
  assert.match(router, /path:\s*['"]\/reseller['"]/)
})

test('admin and public config no longer expose legacy card/list layout mode', () => {
  assert.doesNotMatch(read('../admin/src/views/admin/Settings.vue'), /template_mode|cardMode|listMode/)
  assert.doesNotMatch(read('../admin/src/i18n/index.ts'), /layoutTitle:|layoutSubtitle:|cardMode:|listMode:/)
  assert.doesNotMatch(read('../../internal/modules/settings/application/site_normalize.go'), /normalizeSiteTemplateMode/)
  assert.match(read('../../internal/modules/settings/application/core.go'), /normalizeSiteSetting\(value\)/)
})

test('backend cart runtime and schema are removed', () => {
  assert.equal(existsSync(new URL('../../internal/modules/cart', root)), false)
  assert.doesNotMatch(read('../../internal/app/httpserver/routes_storefront.go'), /carttransport|userCartHandler/)
  assert.doesNotMatch(read('../../internal/app/container/container.go'), /CartService|CartRepo/)
  assert.match(read('../../internal/bootstrap/database/migrations/migrations.go'), /DropTable\("cart_items"\)/)
})

test('buy-now remains while add-to-cart and standalone cart behavior are absent', () => {
  const quickBuy = read('src/components/ProductQuickBuy.vue')
  const detail = read('src/composables/useProductDetail.ts')
  const checkout = read('src/composables/useCheckout.ts')
  for (const source of [quickBuy, detail]) {
    assert.doesNotMatch(source, /useCartStore|addToCart|handleAddToCart|selectedCartQuantity/)
    assert.match(source, /buyNow|handleBuyNow/)
  }
  assert.doesNotMatch(checkout, /useCartStore|isBuyNowMode|mode=buynow|cartStore/)
  assert.match(checkout, /useBuyNowStore/)
  assert.doesNotMatch(read('src/components/checkout/CheckoutSteps.vue'), /['"]cart['"]|cart\.title/)
})

test('advanced personal-center clients and existing reseller console are active', () => {
  const apiIndex = read('src/api/index.ts')
  const affiliate = read('src/api/affiliate.ts')
  const resellerProducts = read('src/views/reseller/ResellerProducts.vue')
  assert.match(apiIndex, /apiCredentialAPI/)
  assert.match(affiliate, /dashboard|commissions|withdraws|applyWithdraw|open:/)
  assert.match(affiliate, /trackClick/)
  assert.match(resellerProducts, /ResellerProductSettingsPanel/)
  assert.match(read('../../internal/app/httpserver/routes_storefront.go'), /RegisterUserRoutes\(user, affiliateHandler\)/)
  assert.match(read('../../internal/app/httpserver/routes_storefront.go'), /RegisterUserRoutes\(user, userApiCredentialHandler\)/)
})
