import test, { afterEach } from 'node:test'
afterEach(async () => {
  // Explicit transition budgets survive unmount; drain them before global selectors.
  await new Promise(resolve => setTimeout(resolve, 320))
  assert.equal(document.querySelectorAll('[data-overlay-root]').length, 0)
})
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { loader, vue, settle } from './helpers/motion17DHarness.ts'
const mocks = {
  'vue-i18n': { useI18n: () => ({ t: (v: string) => v }) },
  '@/components/ui/button': { Button: 'button' },
  '@/components/ui/checkbox': { Checkbox: 'input' },
  '@/components/ui/badge': { Badge: 'span' },
  'vue-router': { useRouter: () => ({ push() {} }), useRoute: () => ({ fullPath: '/products' }) },
  '../stores/app': { useAppStore: () => ({ locale: 'en-US' }) },
  '../stores/buyNow': { useBuyNowStore: () => ({ setItem() { throw Error('No checkout in fixture') } }) },
  '../stores/userAuth': { useUserAuthStore: () => ({ isAuthenticated: false }) },
  '../stores/userProfile': { useUserProfileStore: () => ({ memberLevels: [] }) },
  '../utils/image': { getFirstImageUrl: () => '', getImageUrl: (v: string) => v },
  '../composables/useProduct': { useLocalized: () => ({ getLocalizedText: (v: any) => v?.['en-US'] || '', siteCurrency: vue.ref('USD'), formatPrice: () => '0' }), useProductLabels: () => new Proxy({}, { get: () => () => false }) },
  '../utils/richContent': { sanitizeRichHtml: (v: string) => v },
  '../composables/useAnnouncement': { useAnnouncement: () => ({ dismissToday() {} }) },
}
function overlayFixture(kind: string, sharedLoad?: any) {
  const visible = vue.ref(true)
  const confirm = { visible, options: vue.ref({ title: 'Fixture', message: 'Synthetic only' }), handleCancel: () => { visible.value = false }, handleConfirm: () => { visible.value = false } }
  const load = sharedLoad || loader({ ...mocks, '../composables/useConfirmDialog': { useConfirmDialog: () => load.confirm } })
  load.confirm = confirm
  const comp = load(`components/${kind}.vue`).default
  const host = document.createElement('div'); document.body.append(host)
  const app = vue.createApp({ setup: () => () => vue.h(comp, kind === 'ConfirmDialog' ? {} : { visible: visible.value, ...(kind === 'ProductQuickBuy' ? { product: { id: 1, slug: 'fixture', title: {} } } : { announcement: { version: 'fixture', type: 'info', title: { 'en-US': 'Fixture' }, content: {} } }), 'onUpdate:visible': (v: boolean) => { visible.value = v } }) })
  app.mount(host)
  return { visible, load, unmount() { app.unmount(); host.remove() } }
}

test('confirm painted backdrop, not only an unpainted wrapper, cancels', async () => {
  const f = overlayFixture('ConfirmDialog'); await settle()
  try {
    const painted = document.querySelector('[class*="bg-black/40"]') as HTMLElement
    assert.ok(painted)
    painted.click(); await settle()
    assert.equal(f.visible.value, false)
  } finally { f.unmount() }
})

test('close releases pointer/focus ownership immediately, fast reopen stays owned, unmount cleans listeners', async () => {
  const trigger = document.querySelector('#trigger') as HTMLElement
  trigger.focus()
  const f = overlayFixture('ConfirmDialog'); await settle()
  try {
    assert.equal(document.body.style.overflow, 'hidden')
    assert.equal(document.querySelector('#app')!.hasAttribute('inert'), true, 'background is not keyboard-interactive')
    assert.ok(document.querySelector('[role="alertdialog"]')?.contains(document.activeElement))
    const root = document.querySelector('[role="alertdialog"]')!.parentElement!
    f.visible.value = false; await vue.nextTick()
    assert.equal(root.hasAttribute('inert'), true, 'leaving pixels must not keep interaction ownership')
    assert.equal(root.style.pointerEvents, 'none')
    assert.equal(document.activeElement, trigger)
    assert.equal(document.body.style.overflow, '')
    f.visible.value = true; await settle()
    assert.equal(document.body.style.overflow, 'hidden')
    assert.equal(document.querySelector('[role="alertdialog"]')!.parentElement!.hasAttribute('inert'), false)
    const panel = document.querySelector('[role="alertdialog"]')!
    const buttons = panel.querySelectorAll('button')
    ;(buttons[buttons.length - 1] as HTMLElement).focus()
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true }))
    assert.equal(document.activeElement, buttons[0])
  } finally { f.unmount() }
  assert.equal(document.body.style.overflow, '')
  assert.equal(document.activeElement, trigger)
  document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
  assert.equal(f.visible.value, true, 'unmounted dialog no longer owns Escape')
})

test('unmounting a lower overlay cannot unlock or steal focus from a live upper overlay', async () => {
  const a = overlayFixture('ConfirmDialog'); await settle()
  const b = overlayFixture('AnnouncementModal', a.load); await settle()
  try {
    const topFocus = document.activeElement
    a.unmount(); await settle()
    assert.equal(document.body.style.overflow, 'hidden')
    assert.equal(document.documentElement.style.overflow, 'hidden')
    assert.equal(document.activeElement, topFocus)
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' })); await settle()
    assert.equal(b.visible.value, false)
    assert.equal(document.body.style.overflow, '')
  } finally { b.unmount() }
})

test('one Escape only closes the painted top overlay even when a lower layer mounted later', async () => {
  const a = overlayFixture('AnnouncementModal'); await settle()
  const b = overlayFixture('ConfirmDialog', a.load); await settle()
  try {
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', cancelable: true })); await settle()
    assert.equal(a.visible.value, false)
    assert.equal(b.visible.value, true)
  } finally { b.unmount(); a.unmount() }
})

test('closing stacked overlays never reactivates the lower leaving pixels and returns original focus', async () => {
  const trigger = document.querySelector('#trigger') as HTMLElement; trigger.focus()
  const a = overlayFixture('ConfirmDialog'); await settle()
  const lower = document.querySelector('[role="alertdialog"]')!.parentElement!
  const b = overlayFixture('AnnouncementModal', a.load); await settle()
  try {
    a.visible.value = false; await vue.nextTick()
    b.visible.value = false; await vue.nextTick()
    assert.equal(lower.hasAttribute('inert'), true, 'releasing the upper mask must not undo the lower leave guard')
    assert.equal(document.activeElement, trigger, 'detached/closing intermediate focus targets must be skipped')
  } finally { b.unmount(); a.unmount() }
})

test('close/unmount before the pending focus tick cannot reclaim focus or release a pre-existing lock', async () => {
  const trigger = document.querySelector('#trigger') as HTMLElement
  document.body.style.overflow = 'hidden'; document.documentElement.style.overflow = 'clip'
  document.querySelector('#app')!.setAttribute('inert', '')
  trigger.focus()
  const a = overlayFixture('ConfirmDialog')
  a.visible.value = false
  await settle()
  assert.equal(document.activeElement, trigger)
  a.unmount()
  const b = overlayFixture('AnnouncementModal')
  b.unmount(); await settle()
  assert.equal(document.activeElement, trigger)
  assert.equal(document.body.style.overflow, 'hidden')
  assert.equal(document.documentElement.style.overflow, 'clip')
  assert.equal(document.querySelector('#app')!.hasAttribute('inert'), true)
  document.body.style.overflow = ''; document.documentElement.style.overflow = ''; document.querySelector('#app')!.removeAttribute('inert')
})

test('conditionally mounted quick-buy owns scroll, Escape, focus and leave inertness', async () => {
  const f = overlayFixture('ProductQuickBuy'); await settle()
  try {
    assert.equal(document.body.style.overflow, 'hidden')
    const panel = document.querySelector('[role="dialog"]')!
    assert.ok(panel.contains(document.activeElement))
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', cancelable: true })); await vue.nextTick()
    assert.equal(f.visible.value, false)
    assert.ok(panel.closest('[inert]'))
    assert.equal(document.body.style.overflow, '')
  } finally { f.unmount() }
})

for (const kind of ['AnnouncementModal', 'ConfirmDialog', 'ProductQuickBuy']) {
  test(`${kind} has one transition owner with enter and leave reduced-motion coverage`, () => {
    const src = readFileSync(new URL(`../src/components/${kind}.vue`, import.meta.url), 'utf8')
    assert.equal((src.match(/<Transition\b/g) || []).length, 1)
    assert.match(src, /prefers-reduced-motion:[\s\S]*reduce[\s\S]*leave-active/)
  })
}
