import { dom, vue, loader, deferred, settle } from './motion17DHarness.ts'
import { createRequire } from 'node:module'
const require = createRequire(import.meta.url)
Object.defineProperty(globalThis, 'history', { configurable: true, value: dom.window.history })
export { dom, vue, deferred, settle }
export const pass = { setup: (_: any, { slots }: any) => () => vue.h('div', slots.default?.()) }
export const empty = { setup: () => () => null }
export function motionSpy() {
  let reduced = false
  const listeners = new Set<() => void>()
  dom.window.matchMedia = (() => ({ get matches() { return reduced }, addEventListener: (_: string, fn: () => void) => listeners.add(fn), removeEventListener: (_: string, fn: () => void) => listeners.delete(fn) })) as any
  const events: any[] = []
  dom.window.HTMLElement.prototype.animate = function(frames: any, options: any) {
    const event = { target: this, frames, options, cancelled: false }; events.push(event)
    return { cancel() { event.cancelled = true } } as any
  }
  return { events, listeners, reduce(value: boolean) { reduced = value; listeners.forEach(fn => fn()) } }
}
export const product = (slug: string) => ({ slug, title: { 'en-US': 'Product ' + slug }, price_amount: '10', images: [], skus: [], fulfillment_type: 'manual', purchase_type: 'guest' })
export async function fixture(theme = 'classic', extra: Record<string, any> = {}, routes?: any[] | ((load: any) => any[])) {
  const spy = motionSpy()
  const { createRouter, createMemoryHistory } = require('vue-router')
  const calls: any[] = []
  const observers: any[] = []
  globalThis.IntersectionObserver = class {
    callback: any
    constructor(callback: any) { this.callback = callback }
    observe(el: any) { observers.push(el); this.callback([{ isIntersecting: false }]) }
    disconnect() {}
  } as any
  const mocks: Record<string, any> = {
    '../utils/image': { getImageUrl: (s: string) => s },
    './templates/registry': { getActiveTemplate: () => theme },
    './templates/vault/layout/VaultLayout.vue': { default: pass },
    './components/ErrorBoundary.vue': { default: pass },
    'vue-i18n': { useI18n: () => ({ t: (s: string) => s }) },
    '@unhead/vue': { useHead() {} },
    '../stores/app': { useAppStore: () => ({ locale: 'en-US', config: {} }) },
    '../stores/buyNow': { useBuyNowStore: () => ({ setItem() { throw Error('No writes') } }) },
    '../stores/userAuth': { useUserAuthStore: () => ({ isAuthenticated: false }) },
    '../stores/userProfile': { useUserProfileStore: () => ({ memberLevels: [] }) },
    '../utils/productDetailPrefetch': { takeProductDetailRequest: (slug: string) => { const d = deferred(); calls.push({ slug, ...d }); return d.promise } },
    './useProduct': { useLocalized: () => ({ getLocalizedText: (v: any) => typeof v === 'object' ? v?.['en-US'] || '' : v || '', siteCurrency: vue.ref('USD'), formatPrice: (v: any) => String(v) }), useProductLabels: () => new Proxy({}, { get: (_: any, key: string) => key.startsWith('has') ? () => false : () => '' }) },
    '../utils/richContent': { sanitizeRichHtml: (s: string) => s },
    '../../utils/richContent': { sanitizeRichHtml: (s: string) => s },
    '../components/product/ProductImageGallery.vue': { default: empty },
    '../components/EmptyState.vue': { default: { props: ['title'], setup: (p: any, { slots }: any) => () => vue.h('div', [p.title, slots.action?.()]) } },
    '@/components/ui/button': { Button: { setup: (_: any, { slots }: any) => () => vue.h('button', slots.default?.()) } },
    '@/components/ui/badge': { Badge: pass }, '@/components/ui/alert': { Alert: pass, AlertDescription: pass },
    ...extra,
  }
  for (const name of ['Navbar', 'Toast', 'ConfirmDialog', 'BackToTop', 'MobileBottomNav']) mocks[`./components/${name}.vue`] = { default: empty }
  const load = loader(mocks)
  const Detail = load(theme === 'classic' ? 'views/ProductDetail.vue' : 'templates/vault/ProductDetail.vue').default
  const router = createRouter({ history: createMemoryHistory(), routes: typeof routes === 'function' ? routes(load) : routes ?? [{ path: '/', component: empty }, { path: '/products', component: empty }, { path: '/products/:slug', component: Detail }, { path: '/other', component: { setup: () => () => vue.h('article', 'Other') } }] })
  await router.push('/'); await router.isReady()
  const host = document.createElement('div'); document.body.append(host)
  const app = vue.createApp(load('App.vue').default); app.use(router)
  app.mount(host); await settle()
  let disposed = false
  return { ...spy, calls, observers, host, app, router, load, async go(path: string) { await router.push(path); await settle(); await new Promise(resolve => setTimeout(resolve, 30)); await settle() }, unmount() { if (!disposed) { disposed = true; app.unmount(); host.remove() } } }
}
