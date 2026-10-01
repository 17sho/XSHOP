import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync, existsSync, readdirSync } from 'node:fs'
import { fixture, settle, vue, dom, product, pass, empty } from './helpers/motionPolishRouteHarness.ts'
import { createRequire } from 'node:module'
const require = createRequire(import.meta.url)

const source = (file: string) => readFileSync(new URL(`../src/${file}`, import.meta.url), 'utf8')

const obsoleteMotionFiles = [
  'views/auth/Login.vue', 'views/auth/Register.vue',
  'templates/vault/auth/Login.vue', 'templates/vault/auth/Register.vue',
  'views/ProductDetail.vue', 'templates/vault/ProductDetail.vue',
  'views/PersonalCenter.vue', 'templates/vault/PersonalCenter.vue',
  'views/GuestOrders.vue', 'views/personal/OrdersPanel.vue',
  'views/reseller/ResellerConsoleLayout.vue', 'components/reseller/ResellerSiteConfigPanel.vue',
]

test('obsolete custom motion controller has no production consumers and is removed', () => {
  const root = new URL('../src/', import.meta.url)
  const consumers = readdirSync(root, { recursive: true }).filter((file): file is string => typeof file === 'string' && /\.(vue|ts)$/.test(file) && file !== 'utils/motionController.ts').filter(file => /motionController/.test(source(file)))
  assert.deepEqual(consumers, [], 'remove production consumers before deleting the helper')
  assert.equal(existsSync(new URL('utils/motionController.ts', root)), false, 'unreferenced invented animation controller must not remain')
})

test('owned views contain no invented motion owners, readiness markers or callback wiring', () => {
  for (const file of obsoleteMotionFiles) {
    assert.doesNotMatch(source(file), /v-motion-change|vMotionChange|motionController|data-route-motion|data-motion-opacity|playContentRouteEnter/, file)
  }
})

for (const theme of ['classic', 'vault']) test(`${theme}: actual Login step changes retain drafts without invented local WAAPI`, async () => {
  const driver = transitionDriver()
  const prefix = theme === 'classic' ? '../..' : '../../..'
  const state: any = {
    step: vue.ref('totp'), totpMode: vue.ref('code'), totpCode: vue.ref(''), recoveryCode: vue.ref(''), challengeRemainingSeconds: vue.ref(120),
    email: vue.ref('draft@example.invalid'), password: vue.ref(''), showPassword: vue.ref(false), rememberMe: vue.ref(false), error: vue.ref(''), info: vue.ref(''),
    userAuthStore: vue.reactive({ loading: false }), brandSiteName: 'Fixture', formValidation: { getError: () => '' },
  }
  const Input = { props: ['modelValue'], emits: ['update:modelValue'], setup: (p: any, { emit }: any) => () => vue.h('input', { value: p.modelValue, onInput: (e: any) => emit('update:modelValue', e.target.value) }) }
  const h = await fixture(theme, {
    [`${prefix}/composables/useLogin`]: { useLogin: () => state },
    [`${prefix}/components/FormField.vue`]: { default: { setup: (_: any, { slots }: any) => () => vue.h('label', slots.default?.({ id: 'fixture' })) } },
    [`${prefix}/components/captcha/ImageCaptcha.vue`]: { default: empty }, [`${prefix}/components/captcha/TurnstileCaptcha.vue`]: { default: empty }, [`${prefix}/components/auth/GoogleIdentityButton.vue`]: { default: empty },
    '@/components/ui/card': { Card: pass }, '@/components/ui/input': { Input },
  }, load => [{ path: '/:redirect?', component: load(theme === 'classic' ? 'views/auth/Login.vue' : 'templates/vault/auth/Login.vue').default }])
  try {
    const form = h.host.querySelector('form')!
    state.totpMode.value = 'recovery'; await settle()
    assert.equal(h.events.length, 0, 'upstream has no separate step owner')
    assert.equal(h.host.querySelector('form'), form)
    const input = form.querySelector('input')!
    input.value = 'synthetic draft'; input.dispatchEvent(new dom.window.Event('input', { bubbles: true })); await settle()
    await h.go('/?redirect=/products/A')
    assert.equal(h.host.querySelector('input'), input)
    assert.equal(state.recoveryCode.value, 'synthetic draft')
    assert.equal(h.host.querySelector('[class*="page-fade-"]'), null)
    assert.equal(h.events.length, 0)
  } finally { h.unmount(); driver.restore() }
})

// jsdom does not interpolate CSS. Drive real Vue transition classes/events,
// deriving its timeout from the actual App declaration, not an invented clock.
function transitionDriver() {
  const previousFrame = globalThis.requestAnimationFrame
  const previousStyle = dom.window.getComputedStyle
  let frames: FrameRequestCallback[] = []
  let reduced = false
  globalThis.requestAnimationFrame = (callback: FrameRequestCallback) => { frames.push(callback); return frames.length }
  dom.window.getComputedStyle = ((element: Element) => {
    const style = previousStyle.call(dom.window, element)
    const fade = /page-fade-(enter|leave)-active/.test(element.className)
    const declaration = source('App.vue').match(/transition:\s*opacity\s+(\d+)ms\s+ease/)
    return new Proxy(style, { get(target, key) {
      if (fade && declaration) {
        if (key === 'transitionDuration') return reduced ? '0s' : `${Number(declaration[1]) / 1000}s`
        if (key === 'transitionDelay') return '0s'
        if (key === 'transitionProperty') return 'opacity'
      }
      return Reflect.get(target, key)
    } })
  }) as any
  return {
    reduce(value: boolean) { reduced = value },
    async frame() { const batch = frames; frames = []; batch.forEach(fn => fn(0)); await settle() },
    async nextFrame() { await this.frame(); await this.frame() },
    async end(element: Element) { element.dispatchEvent(new dom.window.Event('transitionend')); await settle() },
    restore() { frames = []; globalThis.requestAnimationFrame = previousFrame; dom.window.getComputedStyle = previousStyle },
  }
}

for (const theme of ['classic', 'vault']) test(`${theme}: actual PersonalCenter uses upstream direct panels without remounting its shell`, async () => {
  const driver = transitionDriver()
  let loads = 0, mounts = 0
  const store = vue.reactive({ displayName: 'Synthetic fixture', profile: { email: 'fixture@example.invalid' }, memberLevels: [], currentLevel: null,
    personalCenterVisibility: { overview: true, profile: true, security: true, wallet: true, orders: false },
    ensureProfileLoaded: async () => { loads++; return true }, loadMemberLevels: async () => {},
  })
  const Panel = { setup() { mounts++; const draft = vue.ref('draft'); return () => vue.h('div', [vue.h('input', { value: draft.value, onInput: (e: any) => { draft.value = e.target.value } }), vue.h('div', { style: 'position:fixed', 'data-fixed-dialog': '' }, 'Dialog')]) } }
  const mocks: Record<string, any> = {
    'vue-i18n': { useI18n: () => ({ t: (s: string) => s, locale: vue.ref('en-US') }) },
    '../stores/userProfile': { useUserProfileStore: () => store }, '../../utils/image': { getImageUrl: (s: string) => s }, '../components/shared/StatCard.vue': { default: pass },
  }
  for (const name of ['Profile', 'Security', 'Orders', 'Wallet', 'GiftCard', 'Affiliate', 'Api']) {
    mocks[`./personal/${name}Panel.vue`] = { default: Panel }
    mocks[`../../views/personal/${name}Panel.vue`] = { default: Panel }
  }
  const file = theme === 'classic' ? 'views/PersonalCenter.vue' : 'templates/vault/PersonalCenter.vue'
  const h = await fixture(theme, mocks, load => [{ path: '/', component: empty }, { path: '/me/:section?', component: load(file).default, props: (route: any) => ({ section: route.params.section || 'profile' }) }])
  try {
    await h.go('/me/profile'); await settle()
    const entering = h.host.querySelector('.page-fade-enter-active')!
    await driver.nextFrame(); await driver.end(entering)
    const header = h.host.querySelector('header'), aside = h.host.querySelector('aside')
    const input = h.host.querySelector('input')!
    assert.ok(aside); assert.ok(h.host.querySelector('[data-fixed-dialog]'))
    input.value = 'retained draft'; input.dispatchEvent(new dom.window.Event('input', { bubbles: true })); await settle()
    await h.go('/me/profile?tab=preserved')
    assert.equal(h.host.querySelector('input'), input); assert.equal(input.value, 'retained draft')
    assert.equal(mounts, 1); assert.equal(loads, 1)
    await h.go('/me/security')
    assert.equal(h.host.querySelector('header'), header); assert.equal(h.host.querySelector('aside'), aside)
    assert.equal(loads, 1)
    assert.equal(h.host.querySelector('[class*="page-fade-"]'), null, 'same outer component means no repeated route fade')
    assert.equal(h.events.length, 0)
    // Frozen d2e4461 uses direct conditional panels, not a nested Transition.
    assert.doesNotMatch(source(file), /<Transition|<transition/)
  } finally { h.unmount(); driver.restore() }
})

test('actual reseller nested RouterView retains shell and query draft with no invented child transition', async () => {
  const driver = transitionDriver()
  let loads = 0, mounts = 0
  const Panel = { setup() { mounts++; const draft = vue.ref('draft'); return () => vue.h('section', { 'data-panel': '' }, [vue.h('input', { value: draft.value, onInput: (e: any) => { draft.value = e.target.value } })]) } }
  const Other = { setup: () => () => vue.h('section', { 'data-other-panel': '' }, 'Other module') }
  const file = 'views/reseller/ResellerConsoleLayout.vue'
  const h = await fixture('classic', {
    '../../components/reseller-console/ResellerConsoleTopbar.vue': { default: { setup: () => () => vue.h('header', 'Console') } },
    '../../components/reseller-console/ResellerPageState.vue': { default: empty },
    '../../composables/reseller/useResellerProfile': { useResellerProfile: () => ({ loading: vue.ref(false), state: vue.ref({ status: 'active' }), load: async () => { loads++ } }) },
    '../../utils/resellerConsole': { canRenderResellerConsoleModule: () => true }, '@/components/ui/card': { Card: pass },
  }, load => [{ path: '/', component: empty }, { path: '/reseller', component: load(file).default, meta: { resellerConsole: true }, children: [{ path: '', component: Panel }, { path: 'site', component: Panel }, { path: 'orders', component: Other }, { path: ':rest(.*)*', component: empty }] }])
  try {
    await h.go('/reseller'); await settle()
    const entering = h.host.querySelector('.page-fade-enter-active')!
    await driver.nextFrame(); await driver.end(entering)
    const header = h.host.querySelector('header'), aside = h.host.querySelector('aside'), input = h.host.querySelector('input')!
    assert.ok(header); assert.ok(aside); assert.ok(input)
    input.value = 'console draft'; input.dispatchEvent(new dom.window.Event('input', { bubbles: true })); await settle()
    for (const path of ['/reseller?draft=keep', '/reseller/site']) {
      await h.go(path)
      assert.equal(h.host.querySelector('input'), input); assert.equal(input.value, 'console draft')
      assert.equal(h.host.querySelector('header'), header); assert.equal(h.host.querySelector('aside'), aside)
      assert.equal(h.host.querySelector('[class*="page-fade-"]'), null)
    }
    assert.equal(mounts, 1); assert.equal(loads, 1)
    await h.go('/reseller/orders')
    assert.ok(h.host.querySelector('[data-other-panel]'))
    assert.equal(h.host.querySelector('header'), header); assert.equal(h.host.querySelector('aside'), aside)
    assert.equal(h.host.querySelector('[class*="page-fade-"]'), null)
    assert.equal(loads, 1); assert.equal(h.events.length, 0)
    // Frozen d2e4461 uses the same direct, unkeyed child RouterView.
    assert.match(source(file), /<RouterView v-else-if="canRenderCurrentModule" \/>/)
  } finally { h.unmount(); driver.restore() }
})

for (const theme of ['classic', 'vault']) for (const boundary of ['leave-from', 'leave-to', 'enter-from', 'enter-to']) test(`${theme}: latest navigation wins at ${boundary}`, async () => {
  const driver = transitionDriver()
  const mounted: string[] = []
  const page = (name: string) => ({ setup() { mounted.push(name); return () => vue.h('article', { 'data-page': name }, name) } })
  const h = await fixture(theme, {}, [{ path: '/', component: page('A') }, { path: '/B', component: page('B') }, { path: '/C', component: page('C') }])
  try {
    const outgoing = h.host.querySelector('[data-page="A"]')!
    await h.go('/B')
    if (boundary !== 'leave-from') await driver.nextFrame()
    if (boundary.startsWith('enter')) {
      await driver.end(outgoing)
      if (boundary === 'enter-to') await driver.nextFrame()
    }
    const leaving = h.host.querySelector('article')!
    await h.go('/C')
    assert.equal(h.host.querySelector('[data-page="C"]'), null, 'out-in retains only the leaving page')
    await driver.nextFrame(); await driver.end(leaving)
    const latest = h.host.querySelector('[data-page="C"]')!
    assert.ok(latest, 'latest route replaces the outgoing page even during interrupted enter')
    await driver.nextFrame(); await driver.end(latest)
    assert.equal(h.host.querySelectorAll('article').length, 1)
    assert.equal(latest.className, '')
    assert.deepEqual(mounted, boundary.startsWith('leave') ? ['A', 'C'] : ['A', 'B', 'C'])
    assert.equal(h.events.length, 0)
  } finally { h.unmount(); driver.restore() }
})

for (const theme of ['classic', 'vault']) test(`${theme}: unkeyed same component retains form across parameter and query routes`, async () => {
  const driver = transitionDriver()
  let mounts = 0
  const Page = { setup() {
    mounts++
    const route = require('vue-router').useRoute()
    const draft = vue.ref('')
    return () => vue.h('article', [vue.h('span', String(route.params.id)), vue.h('input', { value: draft.value, onInput: (e: any) => { draft.value = e.target.value } }), vue.h('div', { style: 'position:fixed', 'data-chrome': '' })])
  } }
  const h = await fixture(theme, {}, [{ path: '/:id?', component: Page }])
  try {
    const root = h.host.querySelector('article'), input = h.host.querySelector('input')!, chrome = h.host.querySelector('[data-chrome]')
    input.value = 'retained draft'; input.dispatchEvent(new dom.window.Event('input', { bubbles: true })); await settle()
    for (const path of ['/?query=one', '/second', '/second?query=two']) {
      await h.go(path)
      assert.equal(h.host.querySelector('article'), root)
      assert.equal(h.host.querySelector('input'), input)
      assert.equal(input.value, 'retained draft')
      assert.equal(h.host.querySelector('[data-chrome]'), chrome)
      assert.equal(h.host.querySelector('[class*="page-fade-"]'), null)
    }
    assert.equal(mounts, 1); assert.equal(h.events.length, 0)
  } finally { h.unmount(); driver.restore() }
})

for (const theme of ['classic', 'vault']) test(`${theme}: actual detail data remains latest through reuse and leave-boundary reversal`, async () => {
  const driver = transitionDriver()
  const h = await fixture(theme)
  try {
    await h.go('/products/A')
    const initial = h.host.querySelector('.page-fade-enter-active')!
    await driver.nextFrame(); if (initial) await driver.end(initial)
    const root = theme === 'classic' ? h.host.querySelector('.product-detail-page') : h.host.querySelector('[class*="max-w-[1180px]"]')
    assert.ok(root)
    await h.go('/products/A?draft=keep')
    assert.equal(h.calls.length, 1, 'query navigation does not reload entity data')
    await h.go('/products/B')
    assert.deepEqual(h.calls.map((call: any) => call.slug), ['A', 'B'])
    assert.equal(h.host.contains(root), true)
    assert.equal(h.host.querySelector('[class*="page-fade-"]'), null, 'same detail component is not keyed by entity or fullPath')
    h.calls[1].resolve(product('B')); await settle(); await settle()
    h.calls[0].resolve(product('A')); await settle(); await settle()
    assert.match(root.textContent!, /Product B/)
    assert.doesNotMatch(root.textContent!, /Product A/)
    assert.ok(h.observers.length > 0, 'real onLoaded still attaches after rendered controls')
    assert.ok(root.querySelector('.fixed'), 'fixed purchase bar remains present without a transformed ancestor')
    await h.go('/other')
    assert.ok(root.classList.contains('page-fade-leave-active'))
    await h.go('/products/C')
    await driver.nextFrame(); await driver.end(root)
    assert.equal(h.calls.at(-1).slug, 'C')
    h.calls.at(-1).resolve(product('C')); await settle(); await settle()
    const latest = h.host.querySelector('.page-fade-enter-active')!
    await driver.nextFrame(); await driver.end(latest)
    assert.match(h.host.textContent!, /Product C/)
    assert.doesNotMatch(h.host.textContent!, /Product A|Product B/)
    assert.equal(h.events.length, 0, 'async data never adds a second route animation')
  } finally { h.unmount(); driver.restore() }
})

for (const theme of ['classic', 'vault']) test(`${theme}: reduced motion completes actual Vue route lifecycle without a duration or WAAPI`, async () => {
  const driver = transitionDriver()
  driver.reduce(true)
  const page = (name: string) => ({ setup: () => () => vue.h('article', { 'data-page': name }, name) })
  const h = await fixture(theme, {}, [{ path: '/', component: page('A') }, { path: '/next', component: page('B') }])
  try {
    h.reduce(true)
    await h.go('/next')
    await driver.nextFrame(); await driver.nextFrame()
    const latest = h.host.querySelector('[data-page="B"]')!
    assert.ok(latest)
    assert.equal(latest.className, '', 'zero-duration transition has no lingering class/event dependency')
    assert.equal(h.host.querySelector('[data-page="A"]'), null)
    assert.equal(h.events.length, 0)
    assert.match(source('style.css'), /@media\s*\(prefers-reduced-motion:\s*reduce\)\s*\{[\s\S]*?transition-duration:\s*0s\s*!important/)
  } finally { h.unmount(); driver.restore() }
})

for (const theme of ['classic', 'vault']) test(`${theme}: actual App restores original unkeyed 200ms opacity out-in lifecycle`, async () => {
  const driver = transitionDriver()
  const mounts: string[] = []
  const page = (name: string) => ({ setup() { mounts.push(name); return () => vue.h('article', { 'data-page': name }, name) } })
  const h = await fixture(theme, {}, [{ path: '/', component: page('A') }, { path: '/next', component: page('B') }])
  try {
    const initial = h.host.querySelector('[data-page="A"]')!
    assert.ok(initial)
    assert.equal(initial.className, '', 'upstream does not animate first mount')
    await h.go('/next')
    assert.ok(initial.classList.contains('page-fade-leave-active'), 'outgoing actual route must leave before mounting its replacement')
    assert.ok(initial.classList.contains('page-fade-leave-from'))
    assert.equal(h.host.querySelector('[data-page="B"]'), null)
    assert.deepEqual(mounts, ['A'])
    await driver.nextFrame()
    assert.ok(initial.classList.contains('page-fade-leave-to'))
    assert.equal(h.host.querySelector('[data-page="B"]'), null)
    await driver.end(initial)
    const incoming = h.host.querySelector('[data-page="B"]')!
    assert.ok(incoming.classList.contains('page-fade-enter-active'))
    assert.ok(incoming.classList.contains('page-fade-enter-from'))
    assert.equal(h.host.contains(initial), false)
    await driver.nextFrame()
    assert.ok(incoming.classList.contains('page-fade-enter-to'))
    await driver.end(incoming)
    assert.equal(incoming.className, '')
    assert.equal(h.events.length, 0, 'no custom WAAPI route effect may stack on the original fade')
    const app = source('App.vue')
    assert.match(app, /transition:\s*opacity 200ms ease;/)
    assert.match(app, /\.page-fade-enter-from,\s*\.page-fade-leave-to\s*\{\s*opacity: 0;/)
    assert.doesNotMatch(app, /transform|:key=|playContentRouteEnter|routePage|motionController|<Loading|<Footer/)
  } finally { h.unmount(); driver.restore() }
})
