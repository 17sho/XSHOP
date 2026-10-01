import { readFileSync } from 'node:fs'
import postcss from 'postcss'
import { loader, vue, settle } from './motion17DHarness.ts'

const read = (kind: string) => readFileSync(new URL(`../../src/components/${kind}.vue`, import.meta.url), 'utf8')
export const wait = (ms: number) => new Promise(resolve => setTimeout(resolve, ms))


// JSDOM doesn't expand transition shorthand or evaluate media queries. Flatten only
// the selected real SFC rules and expand their shorthand, never invent timing values.
// This exercises real Vue Transition roots/timers, not browser rendering/interpolation.
export function installCss(kind: string, width = 390, reduced = false) {
  const parsed = postcss.parse(read(kind).match(/<style scoped>([\s\S]*?)<\/style>/)![1])
  let css = ''
  parsed.walkRules(rule => {
    for (let p: postcss.Container | postcss.Document | undefined = rule.parent; p; p = p.parent) {
      if (p.type !== 'atrule') continue
      const media = p as postcss.AtRule
      if (media.name !== 'media') continue
      if (media.params.includes('min-width:768px') && width < 768) return
      if (media.params.includes('prefers-reduced-motion') && !reduced) return
    }
    let declarations = ''
    rule.walkDecls(decl => {
      declarations += `${decl.prop}:${decl.value};`
      if (decl.prop === 'transition') {
        const parts = decl.value.split(/,(?![^()]*\))/)
        const durations = parts.map(part => part.match(/(?:^|\s)([.\d]+m?s)(?=\s|$)/)?.[1] || '0s')
        declarations += `transition-duration:${durations.join(', ')};transition-delay:0s;transition-property:${parts.map(p => p.trim().split(/\s/)[0]).join(', ')};`
      }
    })
    css += `${rule.selector}{${declarations}}\n`
  })
  const style = document.createElement('style'); style.textContent = css; document.head.append(style)
  return () => style.remove()
}
export function fixture(kind: string, initiallyOpen = true) {
  const open = vue.ref(initiallyOpen)
  const actions = { purchase: 0, navigate: 0, confirm: 0, dismiss: 0, close: 0 }
  const close = () => { actions.close++; open.value = false }
  const load = loader({
    'vue-i18n': { useI18n: () => ({ t: (v: string) => v }) },
    '@/components/ui/button': { Button: 'button' }, '@/components/ui/checkbox': { Checkbox: 'input' }, '@/components/ui/badge': { Badge: 'span' },
    'vue-router': { useRouter: () => ({ push() { actions.navigate++ } }), useRoute: () => ({ fullPath: '/products' }) },
    '../stores/app': { useAppStore: () => ({ locale: 'en-US' }) },
    '../stores/buyNow': { useBuyNowStore: () => ({ setItem() { actions.purchase++ } }) },
    '../stores/userAuth': { useUserAuthStore: () => ({ isAuthenticated: false }) },
    '../stores/userProfile': { useUserProfileStore: () => ({ memberLevels: [] }) },
    '../utils/image': { getFirstImageUrl: () => '', getImageUrl: (v: string) => v },
    '../composables/useProduct': { useLocalized: () => ({ getLocalizedText: (v: any) => v?.['en-US'] || '', siteCurrency: vue.ref('USD'), formatPrice: () => '0' }), useProductLabels: () => new Proxy({}, { get: () => () => false }) },
    '../utils/richContent': { sanitizeRichHtml: (v: string) => v },
    '../composables/useAnnouncement': { useAnnouncement: () => ({ dismissToday() { actions.dismiss++ } }) },
    '../composables/useConfirmDialog': { useConfirmDialog: () => ({ visible: open, options: vue.ref({ title: 'Fixture', message: 'Synthetic only' }), handleCancel: close, handleConfirm() { actions.confirm++; close() } }) },
  })
  const component = load(`components/${kind}.vue`).default
  const host = document.createElement('div'); document.body.append(host)
  const app = vue.createApp({ setup: () => () => vue.h(component, kind === 'ConfirmDialog' ? {} : {
    visible: open.value,
    ...(kind === 'ProductQuickBuy' ? { product: { id: 1, slug: 'fixture', title: {} } } : { announcement: { version: 'fixture', type: 'info', title: {}, content: {} } }),
    'onUpdate:visible': close,
  }) })
  app.mount(host)
  return { open, actions,
    // Read the actual Transition props, not the source function: this catches
    // slot-only ref changes that fail to re-render the duration binding.
    transitionDuration() {
      const componentVNode = app._instance!.subTree
      const teleport = componentVNode.component!.subTree
      return teleport.children[0].props.duration
    },
    root: document.querySelector('[data-overlay-root]') as HTMLElement, unmount() { app.unmount(); host.remove() } }
}

export { vue, settle }
export const kinds = ['AnnouncementModal', 'ConfirmDialog', 'ProductQuickBuy'] as const
export function media(width = 390, reduced = false) {
  const old = window.matchMedia
  window.matchMedia = ((query: string) => ({
    matches: query.includes('prefers-reduced-motion') ? reduced : query.includes('min-width') ? width >= 768 : false,
    media: query, onchange: null, addListener() {}, removeListener() {},
    addEventListener() {}, removeEventListener() {}, dispatchEvent() { return true },
  })) as typeof window.matchMedia
  return { set(w: number, r: boolean) { width = w; reduced = r }, restore() { window.matchMedia = old } }
}
export function end(element: Element, propertyName: string) {
  const event = new Event('transitionend', { bubbles: true })
  Object.defineProperty(event, 'propertyName', { value: propertyName })
  element.dispatchEvent(event)
}
export function rootEnd(root: Element, kind: string) {
  for (const p of kind === 'ProductQuickBuy' ? ['background-color', 'backdrop-filter'] : ['opacity']) end(root, p)
}
// Original longest-layer budgets, not a new responsive motion design.
export const budgets = (kind: string, _width: number) => kind === 'ConfirmDialog' ? [200, 150] : [300, 200]
