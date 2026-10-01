import test, { afterEach } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import postcss from 'postcss'
import { loader, vue, settle } from './helpers/motion17DHarness.ts'

const read = (kind: string) => readFileSync(new URL(`../src/components/${kind}.vue`, import.meta.url), 'utf8')
const wait = (ms: number) => new Promise(resolve => setTimeout(resolve, ms))
afterEach(async () => {
  await wait(320)
  assert.equal(document.querySelectorAll('[data-overlay-root]').length, 0)
})

// JSDOM doesn't expand transition shorthand or evaluate media queries. Flatten only
// the selected real SFC rules and expand their shorthand, never invent timing values.
// This exercises real Vue Transition roots/timers, not browser rendering/interpolation.
function installCss(kind: string, width = 390, reduced = false) {
  const previousMatchMedia = window.matchMedia
  window.matchMedia = ((query: string) => ({ matches: query.includes('prefers-reduced-motion') ? reduced : query.includes('min-width') && width >= 768, media: query, addEventListener() {}, removeEventListener() {} })) as any
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
  return () => { style.remove(); window.matchMedia = previousMatchMedia }
}
function fixture(kind: string, initiallyOpen = true) {
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
  return { open, actions, root: document.querySelector('[data-overlay-root]') as HTMLElement, unmount() { app.unmount(); host.remove() } }
}
const duration = (element: Element) => Math.max(...window.getComputedStyle(element).transitionDuration.split(',').map(v => parseFloat(v) * (v.trim().endsWith('ms') ? 1 : 1000)))

for (const [width, enter, leave] of [[390, 300, 200], [1024, 300, 200]]) test(`QuickBuy ${width}px: independent backdrop fade, solid sheet or centered settle, root leave timing`, async () => {
  const removeCss = installCss('ProductQuickBuy', width)
  const f = fixture('ProductQuickBuy')
  try {
    const root = f.root, panel = root.querySelector('[data-overlay-panel]') as HTMLElement
    assert.ok(root.classList.contains('quick-buy-enter-from'))
    assert.ok(['', '1'].includes(window.getComputedStyle(root).opacity), 'root opacity must not fade its sheet')
    assert.match(window.getComputedStyle(root).transitionProperty, /background-color/)
    assert.equal(window.getComputedStyle(root).backgroundColor, 'rgba(0, 0, 0, 0)')
    assert.equal(duration(root), enter)
    assert.equal(duration(panel), width < 768 ? 280 : 200)
    if (width < 768) {
      assert.ok(panel.classList.contains('bg-card'), 'mobile panel background must be solid')
      assert.ok(['', '1'].includes(window.getComputedStyle(panel).opacity))
      assert.equal(window.getComputedStyle(panel).transform, 'translateY(100%)')
      assert.match(window.getComputedStyle(panel).transition, /cubic-bezier\(\.32,\.72,0,1\)/)
    } else {
      assert.equal(window.getComputedStyle(panel).transform, 'scale(.96)')
      assert.equal(window.getComputedStyle(panel).opacity, '0')
    }
    await wait(enter + 60)
    assert.equal(window.getComputedStyle(panel).transform, '')
    panel.click(); await settle()
    assert.equal(f.open.value, true, 'panel surface is not backdrop')
    assert.ok(root.classList.contains('bg-black/40'), 'actual painted backdrop is the clickable root')
    root.click(); await vue.nextTick(); await wait(25)
    assert.equal(f.open.value, false)
    assert.equal(duration(root), leave)
    assert.equal(duration(panel), width < 768 ? 200 : 150)
    assert.ok(root.hasAttribute('inert'))
    assert.equal(root.style.pointerEvents, 'none')
    panel.dispatchEvent(new Event('transitionend', { bubbles: true }))
    await wait(leave - 70)
    assert.ok(root.isConnected, 'leave persists until the longest child finishes')
    await wait(85)
    assert.equal(root.isConnected, false)
    assert.deepEqual(f.actions, { purchase: 0, navigate: 0, confirm: 0, dismiss: 0, close: 1 })
    assert.equal((read('ProductQuickBuy').match(/<Transition\b/g) || []).length, 1)
  } finally { f.unmount(); removeCss() }
})

for (const kind of ['AnnouncementModal', 'ConfirmDialog', 'ProductQuickBuy']) {
  for (const width of [390, 1024]) test(`${kind} ${width}px: reduced motion has no root/child timer or transform and no extra action`, async () => {
    const removeCss = installCss(kind, width, true)
    const f = fixture(kind)
    try {
      const root = f.root, panel = root.querySelector('[data-overlay-panel]')!
      assert.equal(duration(root), 0)
      assert.equal(duration(panel), 0)
      assert.equal(window.getComputedStyle(panel).transform, 'none')
      await wait(35)
      f.open.value = false; await vue.nextTick(); await wait(35)
      assert.equal(root.isConnected, false)
      assert.equal(document.body.style.overflow, '')
      assert.deepEqual(f.actions, { purchase: 0, navigate: 0, confirm: 0, dismiss: 0, close: 0 })
    } finally { f.unmount(); removeCss() }
  })

  test(`${kind}: warm open, rapid close/reopen and ref generation retain one focus/scroll owner`, async () => {
    const removeCss = installCss(kind)
    const trigger = document.querySelector('#trigger') as HTMLElement; trigger.focus()
    const f = fixture(kind, false)
    try {
      f.open.value = true; await settle(); await settle()
      const oldRoot = document.querySelector('[data-overlay-root]') as HTMLElement
      assert.ok(oldRoot.querySelector('[data-overlay-panel]')!.contains(document.activeElement))
      f.open.value = false; await vue.nextTick()
      assert.equal(document.activeElement, trigger)
      assert.equal(document.body.style.overflow, '')
      assert.ok(oldRoot.hasAttribute('inert'))
      assert.equal(oldRoot.getAttribute('aria-hidden'), 'true')
      f.open.value = true; await settle(); await settle(); await wait(350)
      const roots = document.querySelectorAll('[data-overlay-root]')
      assert.equal(roots.length, 1)
      assert.ok(!roots[0].hasAttribute('inert'))
      assert.ok(!roots[0].hasAttribute('aria-hidden'))
      assert.equal((roots[0] as HTMLElement).style.pointerEvents, '')
      assert.ok(roots[0].contains(document.activeElement))
      assert.equal(document.body.style.overflow, 'hidden')
      assert.equal(document.documentElement.style.overflow, 'hidden')
      assert.ok(document.querySelector('#app')!.hasAttribute('inert'))
      assert.deepEqual(f.actions, { purchase: 0, navigate: 0, confirm: 0, dismiss: 0, close: 0 })
    } finally { f.unmount(); removeCss() }
    assert.equal(document.body.style.overflow, '')
    assert.equal(document.activeElement, trigger)
  })
}

for (const [kind, name, enter, leave, transform, curve] of [
  ['AnnouncementModal', 'announcement-motion', 300, 200, 'translateY(12px) scale(.95)', 'cubic-bezier(.34,1.56,.64,1)'],
  ['ConfirmDialog', 'confirm-motion', 200, 150, 'translateY(8px) scale(.95)', 'ease-out'],
] as const) test(`${kind}: mounted appear uses restrained settling and root survives longest child leave`, async () => {
  const removeCss = installCss(kind)
  const f = fixture(kind)
  try {
    const root = f.root, panel = root.querySelector('[data-overlay-panel]')!
    assert.ok(root.classList.contains(`${name}-enter-active`))
    assert.ok(root.classList.contains(`${name}-enter-from`))
    assert.equal(duration(root), enter)
    assert.equal(duration(panel), enter)
    assert.equal(window.getComputedStyle(panel).transform, transform)
    assert.ok(window.getComputedStyle(panel).transition.includes(curve))
    await wait(enter + 60)
    assert.equal(window.getComputedStyle(panel).transform, '', 'no permanent settling transform')
    f.open.value = false; await vue.nextTick(); await wait(25)
    assert.equal(duration(root), leave)
    assert.equal(duration(panel), leave)
    assert.ok(root.hasAttribute('inert'))
    assert.equal(root.style.pointerEvents, 'none')
    panel.dispatchEvent(new Event('transitionend', { bubbles: true }))
    await wait(leave - 70)
    assert.ok(root.isConnected, 'child event must not truncate root-owned leave')
    await wait(85)
    assert.equal(root.isConnected, false)
    assert.deepEqual(f.actions, { purchase: 0, navigate: 0, confirm: 0, dismiss: 0, close: 0 })
    assert.equal((read(kind).match(/<Transition\b/g) || []).length, 1)
  } finally { f.unmount(); removeCss() }
})
