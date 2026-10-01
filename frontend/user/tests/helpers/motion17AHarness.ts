import { reactive, ref, effectScope, nextTick } from 'vue'
import { evaluate, deferred } from './sourceRuntime.ts'
import * as purchase from '../../src/utils/productPurchase.ts'
import * as sku from '../../src/utils/sku.ts'
import * as stock from '../../src/utils/publicStock.ts'
import * as policy from '../../src/utils/paymentResumePolicy.ts'
import * as money from '../../src/utils/money.ts'
import { resolveGuestOrderDetailViewState } from '../../src/utils/guestOrderDetailState.ts'
export { deferred }
export const response = (data: any) => ({ data: { data } })
export const settle = async () => { for (let i = 0; i < 12; i++) { await Promise.resolve(); await nextTick() } }
export function harness(name: string, extra: Record<string, any> = {}, query: any = {}) {
  const route = reactive({ params: { slug: 'A', order_no: 'A', recharge_no: 'A' }, query, path: '/pay', fullPath: '/pay', name: 'blog-detail' })
  const mounts: Function[] = [], unmounts: Function[] = [], calls: any[] = [], redirects: any[] = []
  const timers = new Map<number, Function>(); let timerID = 0
  const schedule = (fn: Function, delay: number) => { Object.assign(fn, { delay }); timers.set(++timerID, fn); return timerID }
  Object.assign(globalThis, { window: { setTimeout: schedule, clearTimeout: (id: number) => timers.delete(id), setInterval: schedule, clearInterval: (id: number) => timers.delete(id), scrollTo() {}, location: { origin: 'https://synthetic.invalid', assign: (url: any) => redirects.push(url) } }, clearInterval: (id: number) => timers.delete(id) })
  const request = (kind: string) => (...args: any[]) => { const d = deferred(); calls.push({ kind, args, ...d }); return d.promise }
  const api = { detail: request('detail'), latestPayment: request('latest'), latest: request('latest'), account: request('wallet'), rechargeDetail: request('recharge'), captureRechargePayment: request('capture'), getPaymentChannels: request('channels'), capture: request('capture'), capturePayment: request('capture'), create: request('create'), createPayment: request('create'), cancel: request('cancel'), downloadFulfillment: request('download') }
  const router = { push: async (target: any) => { redirects.push(target) }, replace: async (target: any) => { redirects.push(target); if (target.query) route.query = target.query }, resolve: () => ({ matched: [{}] }) }
  const bindings = {
    ...purchase, ...sku, ...stock, ...policy, ...money,
    useRoute: () => route, useRouter: () => router,
    useI18n: () => ({ t: (s: string) => s }),
    useAppStore: () => ({ locale: 'en-US', config: { payment_channels: [] }, getServerTime: () => Date.now() }),
    useBuyNowStore: () => ({ setItem() {} }), useUserAuthStore: () => ({}), useUserProfileStore: () => ({ memberLevels: [] }),
    useLocalized: () => ({ siteCurrency: ref('CNY'), formatPrice: String, getLocalizedText: (v: any) => v?.['en-US'] || '' }), useProductLabels: () => ({}),
    useHead() {}, usePageSeo() {}, getImageUrl: (s: string) => s,
    useTelegramMiniAppStore: () => ({}),
    useOrderDisplayHelpers: () => ({}), useConfirmDialog: () => ({ confirm: async () => true }), toast: { error() {} },
    debounceAsync: (fn: Function) => Object.assign(fn, { cancel() {} }),
    loadGuestOrderAuth: () => ({ email: 'synthetic@example.invalid', order_password: 'synthetic' }), saveGuestOrderAuth() {}, clearGuestOrderAuth() {}, resolveGuestOrderDetailViewState,
    takeProductDetailRequest: request('product'), userOrderAPI: api, guestOrderAPI: api, postAPI: api, walletAPI: api, paymentAPI: api,
    QRCode: { toDataURL: async () => 'data:image/synthetic' }, consumeCheckoutRedirectIntent: () => false, getBrowserStorage: () => null,
    hasActualCustomerSurcharge: () => false, scrollPageToTop() {},
    onMounted: (fn: Function) => mounts.push(fn), onUnmounted: (fn: Function) => unmounts.push(fn),
    ...extra,
  }
  const scope = effectScope()
  const exposed = evaluate(`src/composables/${name}.ts`, [name], bindings)
  const state = scope.run(() => exposed[name](extra.composableOptions))
  return { state, route, calls, redirects, timers, router, bindings, mount: () => mounts.forEach(fn => void fn()), unmount: () => { unmounts.forEach(fn => fn()); scope.stop() } }
}
