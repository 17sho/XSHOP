import assert from 'node:assert/strict'
import fs from 'node:fs'
import test from 'node:test'

const read = (path: string) => fs.readFileSync(new URL(`../${path}`, import.meta.url), 'utf8')
const navbar = read('src/components/Navbar.vue')
const router = read('src/router/index.ts')
const productList = read('src/composables/useProductList.ts')
const productDetail = read('src/composables/useProductDetail.ts')
const banner = read('src/composables/useBannerCarousel.ts')
const appStore = read('src/stores/app.ts')

test('cold shell renders the final local brand mark inline before config resolves', () => {
  assert.doesNotMatch(navbar, /Dujiao-Next/)
  assert.match(navbar, /XSHOP/)
  assert.match(navbar, /<svg[^>]+aria-label="XSHOP"/)
  assert.doesNotMatch(navbar, /<img[\s\S]*?:src="brandLogo"/)
  assert.doesNotMatch(navbar, /const brandLogo/)
})

test('root product route is eagerly available without config-selected loader', () => {
  assert.match(router, /import Products from '\.\.\/views\/Products\.vue'/)
  assert.match(router, /const productsViewLoader[^\n]*Promise\.resolve\(\{ default: Products \}\)/)
  assert.doesNotMatch(router, /import\('\.\.\/views\/Products\.vue'\)/)
  const root = router.match(/\{\s*path: '\/',[\s\S]*?\n\s*\},/)
  assert.ok(root)
  assert.match(root[0], /component: Products/)
  assert.doesNotMatch(root[0], /templateView|productsViewLoader/)
})

test('home categories and products start concurrently', () => {
  const initialize = productList.match(/const initialize = async \(\) => \{[\s\S]*?\n  \}/)
  assert.ok(initialize)
  assert.match(initialize[0], /const categoriesRequest = loadCategories\(\)/)
  assert.match(initialize[0], /const productsRequest = loadProducts\(\)/)
  assert.match(initialize[0], /Promise\.all\(\[categoriesRequest, productsRequest\]\)/)
  assert.doesNotMatch(initialize[0], /await loadCategories\(\)[\s\S]*await loadProducts\(\)/)
})

test('product detail request starts in the route guard before the lazy view resolves', () => {
  assert.match(router, /void prefetchProductDetail\(String\(to\.params\.slug \|\| ''\)\)/)
  assert.match(productDetail, /takeProductDetailRequest\(slug\)/)
  assert.match(productDetail, /const initialProductRequest = loadProduct\(\)/)
  assert.doesNotMatch(productDetail, /onMounted\(\(\) => \{\s*loadProduct\(\)/)
  assert.match(productDetail, /void initialProductRequest/)
})

test('an empty banner request does not flash a large hero frame', () => {
  assert.match(banner, /const showHeroSection = computed\(\(\) => bannerCount\.value > 0\)/)
  assert.doesNotMatch(banner, /bannerLoading\.value \|\| bannerCount\.value > 0/)
})

test('concurrent config consumers share one public config request', () => {
  assert.match(appStore, /let configRequest: Promise<void> \| null = null/)
  assert.match(appStore, /if \(configRequest && !force\) return configRequest/)
})

test('homepage restores cached catalog before refreshing it in background', () => {
  assert.match(productList, /readPublicCatalogCache/)
  assert.match(productList, /products\.value = cached\.products/)
  assert.match(productList, /categories\.value = cached\.categories/)
  assert.match(productList, /hasLoadedOnce\.value = true/)
  assert.match(productList, /writePublicCatalogCache/)
  assert.match(productList, /getBrowserStorage\('sessionStorage'\)/)
  assert.doesNotMatch(productList, /window\.sessionStorage/)
  // Cache hydration is not network completion. Persist only a current,
  // captured product response plus freshly loaded categories.
  const cacheWriter = productList.match(/const writeCatalogCache = \(\) => \{[\s\S]*?\n  \}/)
  assert.ok(cacheWriter)
  assert.match(cacheWriter[0], /!snapshot \|\| !refreshCategoriesLoaded \|\| !canUseCatalogCache\(\) \|\| !ownsSnapshot\(snapshot\)/)
  assert.match(cacheWriter[0], /snapshot\.query\.page !== 1 \|\| snapshot\.query\.categoryId !== null \|\| snapshot\.query\.search/)
  assert.match(cacheWriter[0], /products: snapshot\.products/)
  assert.match(cacheWriter[0], /totalPages: snapshot\.totalPages/)
  assert.match(productList, /refreshCategoriesLoaded = true/)
  assert.match(productList, /productSnapshot\.value = undefined\s+refreshCategoriesLoaded = false/)
  assert.doesNotMatch(productList, /productsLoaded = true|categoriesLoaded = true/)
})
