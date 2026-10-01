import test from 'node:test'
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import { loader, vue, settle, deferred } from './motion17DHarness.ts'
Object.defineProperty(globalThis, 'history', { configurable: true, value: window.history })
const require = createRequire(import.meta.url)
const { createRouter, createMemoryHistory } = require('vue-router')
export const rows = [{ id: 2, slug: 'two', name: { en: 'Two' } }, { id: 3, slug: 'three', name: { en: 'Three' } }]
export const response = (id: number) => ({ data: { data: [{ id, slug: `synthetic-${id}` }], pagination: { total_page: 1 } } })
export async function fixture({ home = false, warm = false } = {}) {
  window.sessionStorage.clear()
  if (warm) loader()('utils/publicCatalogCache.ts').writePublicCatalogCache(window.sessionStorage, window.location.host, { products: [{ id: 99, slug: 'cached' }], categories: rows, totalPages: 1 })
  const requests: any[] = [], categories = deferred(), navigations: any[] = []
  const nil = { render: () => null }
  const card = { props: ['product'], emits: ['click'], setup(p: any, { emit }: any) { return () => vue.h('button', { 'data-row': p.product.id, onClick: () => emit('click', p.product.slug) }, p.product.slug) } }
  const load = loader({
    '../api': { productAPI: { list: (params: any) => { const d = deferred(); requests.push({ ...d, params }); return d.promise } }, categoryAPI: { list: () => categories.promise }, postAPI: { list: async () => ({ data: { data: [] } }) } },
    'vue-i18n': { useI18n: () => ({ t: (x: string) => x }) },
    '../composables/useProduct': { useLocalized: () => ({ getLocalizedText: (x: any) => x?.en || '' }) },
    '../composables/usePageSeo': { usePageSeo() {} }, '../composables/useAnnouncement': { useAnnouncement: () => ({ shouldShow: () => false }) },
    '../composables/useBannerCarousel': { useBannerCarousel: () => ({ loadBanners() {}, stopHeroAutoPlay() {} }) },
    '../stores/app': { useAppStore: () => ({ productCatalogLayout: 'card', config: {} }) },
    '../utils/image': { getImageUrl: (s: string) => s }, '../utils/richContent': { sanitizeRichHtml: (s: string) => s },
    '../components/ProductCard.vue': { default: card }, '../components/ProductListCard.vue': { default: nil }, '../components/PaginationNav.vue': { default: nil },
    '../components/EmptyState.vue': { default: nil }, '../components/AnnouncementModal.vue': { default: nil },
  })
  const View = load('views/Products.vue').default
  const router = createRouter({ history: createMemoryHistory(), routes: [
    { path: '/', name: 'products', component: View }, { path: '/categories/:slug', name: 'category-products', component: View },
    { path: '/products/:slug', name: 'product-detail', component: nil },
  ] })
  router.afterEach((to: any) => navigations.push(to.fullPath))
  await router.push(home ? '/' : '/categories/two'); await router.isReady()
  const host = document.createElement('div'); document.body.append(host)
  const app = vue.createApp({ render: () => vue.h(require('vue-router').RouterView) }); app.use(router); app.mount(host); await settle()
  return { host, router, navigations, requests, categories, cleanup() { app.unmount(); host.remove() } }
}
export const button = (f: any, title: string) => Array.from(f.host.querySelectorAll('.category-pill')).find((el: any) => el.textContent.trim() === title) as HTMLElement


// Vue nextTick alone does not settle memory-router navigation.
export const settleNavigation = async () => { await new Promise(resolve => setTimeout(resolve, 0)); await settle() }
