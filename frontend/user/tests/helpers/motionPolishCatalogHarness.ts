import { readFileSync } from 'node:fs'
import { loader, vue, settle, deferred } from './motion17DHarness.ts'
export { vue, settle }

export function clock() {
  const originalSet = window.setTimeout, originalClear = window.clearTimeout
  let now = 0, serial = 0
  const pending = new Map<number, { fn: () => void; at: number }>()
  window.setTimeout = ((fn: () => void, ms: number) => { pending.set(++serial, { fn, at: now + ms }); return serial }) as any
  window.clearTimeout = ((id: number) => pending.delete(id)) as any
  return { async tick(ms: number) { now += ms; for (const [id, timer] of pending) if (timer.at <= now) { pending.delete(id); timer.fn() }; await settle() }, restore() { window.setTimeout = originalSet; window.clearTimeout = originalClear } }
}

export const response = (id: number) => ({ data: { data: id ? [{ id, slug: `synthetic-${id}`, title: { en: `Product ${id}` }, price_amount: '1.00', stock_status: 'in_stock', category: { id: 2, name: { en: 'Two' } } }] : [], pagination: { total_page: 3 } } })
export const categoryRows = [{ id: 2, slug: 'two', name: { en: 'Two' } }, { id: 3, slug: 'three', name: { en: 'Three' } }]
export function styleSource(template: string) {
  return readFileSync(new URL('../../src/' + template, import.meta.url), 'utf8').split('<style scoped>')[1].split('</style>')[0]
}

// Actual views, cards and composable; only API/service boundaries are synthetic.
export async function fixture(template: string, layout: string, options: { reduced?: boolean; warm?: boolean; categoryRoute?: boolean } = {}) {
  window.sessionStorage.clear()
  let reduced = !!options.reduced
  const listeners = new Set<() => void>()
  window.matchMedia = (() => ({ get matches() { return reduced }, addEventListener(_type: string, fn: () => void) { listeners.add(fn) }, removeEventListener(_type: string, fn: () => void) { listeners.delete(fn) } })) as any
  const animations: any[] = []
  const originalAnimate = HTMLElement.prototype.animate
  HTMLElement.prototype.animate = function(frames: any, timing: any) {
    const animation = { target: this, frames, timing, cancelled: false, cancel() { this.cancelled = true }, onfinish: null }
    animations.push(animation)
    return animation as any
  }
  const requests: any[] = [], categories = deferred(), navigations: any[] = []
  const route = vue.reactive({ name: options.categoryRoute ? 'category-products' : 'products', path: '/', params: options.categoryRoute ? { slug: 'two' } : {} })
  const nil = { render: () => null }
  const localized = { useLocalized: () => ({ getLocalizedText: (v: any) => v?.en || '', siteCurrency: 'USD', formatPrice: (v: any) => String(v) }), useProductLabels: () => ({ isSoldOut: () => false, hasPromotionPrice: () => false, hasWholesalePrices: () => false, hasPromotionRules: () => false, getPromotionPriceAmount: () => 0, getPurchaseTypeLabel: () => 'Guest', getFulfillmentTypeLabel: () => 'Auto', getStockBadgeVariant: () => 'success', getStockStatusLabel: () => 'Available' }) }
  const mocks: any = {
    'vue-router': { useRoute: () => route, useRouter: () => ({ push: (to: any) => navigations.push(to), replace() {} }) },
    'vue-i18n': { useI18n: () => ({ t: (v: string) => v }) },
    '../api': { productAPI: { list: (params: any) => { const d = deferred(); requests.push({ ...d, params }); return d.promise } }, categoryAPI: { list: () => categories.promise }, postAPI: { list: async () => ({ data: { data: [] } }) } },
    '../composables/useAnnouncement': { useAnnouncement: () => ({ shouldShow: () => false }) },
    '../composables/useBannerCarousel': { useBannerCarousel: () => ({ loadBanners() {}, stopHeroAutoPlay() {} }) },
    '../utils/richContent': { sanitizeRichHtml: (v: string) => v },
    '../components/AnnouncementModal.vue': { default: nil },
    '../../components/ProductQuickBuy.vue': { default: { props: ['visible'], setup: (p: any) => () => p.visible ? vue.h('div', { 'data-quick-buy': '' }) : null } },
  }
  for (const prefix of ['../', '../../', '../../../']) {
    mocks[prefix + 'composables/useProduct'] = localized
    mocks[prefix + 'composables/usePageSeo'] = { usePageSeo() {} }
    mocks[prefix + 'stores/app'] = { useAppStore: () => ({ productCatalogLayout: layout, config: {} }) }
    mocks[prefix + 'utils/image'] = { getImageUrl: (v: string) => v, getFirstImageUrl: () => '' }
  }
  mocks['@/components/ui/badge'] = { Badge: { setup: (_p: any, { slots }: any) => () => vue.h('span', slots.default?.()) } }
  mocks['@/components/ui/card'] = { Card: { setup: (_p: any, { slots }: any) => () => vue.h('div', slots.default?.()) } }
  mocks['@/components/ui/button'] = { Button: { setup: (_p: any, { slots }: any) => () => vue.h('button', slots.default?.()) } }
  const load = loader(mocks)
  if (options.warm) load('utils/publicCatalogCache.ts').writePublicCatalogCache(window.sessionStorage, window.location.host, { products: response(99).data.data, categories: categoryRows, totalPages: 3 })
  const real = load('composables/useProductList.ts').useProductList
  let state: any
  for (const prefix of ['../', '../../']) mocks[prefix + 'composables/useProductList'] = { useProductList: (opts: any) => (state = real(opts)) }
  const host = document.createElement('div'); document.body.append(host)
  const app = vue.createApp(load(template).default)
  app.component('RouterLink', { props: ['to'], setup: (p: any, { slots }: any) => () => vue.h('a', { href: p.to }, slots.default?.()) })
  app.mount(host); await settle()
  return { host, state, route, requests, categories, navigations, animations,
    row: () => Array.from(host.querySelectorAll('h3, .compact-product-row, .vault-compact-row')).find(el => el.textContent?.includes('Product')) as HTMLElement,
    reduce(value: boolean) { reduced = value; listeners.forEach(fn => fn()) },
    cleanup() { app.unmount(); host.remove(); HTMLElement.prototype.animate = originalAnimate },
  }
}
