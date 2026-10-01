import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync, existsSync } from 'node:fs'
import { fixture, settle, vue, pass, empty, dom } from './helpers/motionPolishRouteHarness.ts'

const source = (file: string) => readFileSync(new URL(`../src/${file}`, import.meta.url), 'utf8')
// Execute the exact production scroll callback with a real memory router. Only
// network/config guards and history transport are outside this isolated fixture.
function installScroll(h: any) {
  const file = source('router/index.ts')
  const callback = file.match(/    scrollBehavior\(to, _from, savedPosition\) \{[\s\S]*?\n    \},/)![0]
  const catalog = file.match(/const isCatalogCategoryTransition = ([\s\S]*?)\n\n/)![1]
  const helper = existsSync(new URL('../src/utils/authRouteScroll.ts', import.meta.url)) ? h.load('utils/authRouteScroll.ts') : {}
  h.router.options.scrollBehavior = new Function('router', 'deferAuthScroll', 'isCatalogCategoryTransition', `return ({${callback}}).scrollBehavior`)(h.router, helper.deferAuthScroll, new Function(`return (${catalog.replace(/: unknown/g, '')})`)())
  const calls: any[] = []
  const previous = dom.window.scrollTo
  dom.window.scrollTo = ((position: any) => { calls.push({ position, leaving: !!h.host.querySelector('.page-fade-leave-active'), entering: h.host.querySelector('.page-fade-enter-active')?.className }) }) as any
  return { calls, restore() { dom.window.scrollTo = previous } }
}
function transitionDriver() {
  const previousFrame = globalThis.requestAnimationFrame, previousStyle = dom.window.getComputedStyle
  let frames: FrameRequestCallback[] = [], reduced = false
  globalThis.requestAnimationFrame = (fn: FrameRequestCallback) => { frames.push(fn); return frames.length }
  dom.window.getComputedStyle = ((element: Element) => new Proxy(previousStyle.call(dom.window, element), { get(target, key) {
    if (/page-fade-(enter|leave)-active/.test(element.className)) {
      if (key === 'transitionDuration') return reduced ? '0s' : `${Number(source('App.vue').match(/transition:\s*opacity\s+(\d+)ms\s+ease/)![1]) / 1000}s`
      if (key === 'transitionDelay') return '0s'
      if (key === 'transitionProperty') return 'opacity'
    }
    return Reflect.get(target, key)
  } })) as any
  return {
    reduce() { reduced = true },
    async frame() { const batch = frames; frames = []; batch.forEach(fn => fn(0)); await settle() },
    async nextFrame() { await this.frame(); await this.frame() },
    async end(root: Element) { root.dispatchEvent(new dom.window.Event('transitionend')); await settle() },
    restore() { frames = []; globalThis.requestAnimationFrame = previousFrame; dom.window.getComputedStyle = previousStyle },
  }
}
const Input = { props: ['modelValue'], emits: ['update:modelValue'], setup: (p: any, { emit }: any) => () => vue.h('input', { value: p.modelValue, onInput: (e: any) => emit('update:modelValue', e.target.value) }) }
function authState(logo: string) {
  return { userAuthStore: vue.reactive({ loading: false }), brandSiteName: 'Fixture brand', brandLogo: logo, registrationEnabled: true,
    email: vue.ref('draft@example.invalid'), password: vue.ref(''), agreed: vue.ref(false), passwordStrength: vue.ref(''), step: vue.ref('password'),
    formValidation: { getError: () => '', hasError: () => false, touchField() {} },
  }
}
async function authFixture(theme: string, logo = '') {
  const prefix = theme === 'classic' ? '../..' : '../../..'
  const folder = theme === 'classic' ? 'views/auth' : 'templates/vault/auth'
  const register = authState(logo), login = authState(logo)
  const h = await fixture(theme, {
    [`${prefix}/composables/useRegister`]: { useRegister: () => register },
    [`${prefix}/composables/useLogin`]: { useLogin: () => login },
    [`${prefix}/components/captcha/ImageCaptcha.vue`]: { default: empty },
    [`${prefix}/components/captcha/TurnstileCaptcha.vue`]: { default: empty },
    [`${prefix}/components/auth/GoogleIdentityButton.vue`]: { default: empty },
    '@/components/ui/card': { Card: pass }, '@/components/ui/input': { Input },
    '@/components/ui/select': { Select: pass, SelectContent: pass, SelectItem: pass, SelectTrigger: pass, SelectValue: pass },
  }, load => [
    { path: '/', component: empty },
    { path: '/auth/login', name: 'user-login', component: load(`${folder}/Login.vue`).default },
    { path: '/auth/register', name: 'user-register', component: load(`${folder}/Register.vue`).default },
    { path: '/privacy', component: empty }, { path: '/terms', component: empty },
  ])
  return { ...h, register, login }
}

for (const theme of ['classic', 'vault']) test(`${theme}: real router back honors browser saved position at the incoming hidden boundary`, async () => {
  const driver = transitionDriver(), h = await authFixture(theme), scroll = installScroll(h)
  try {
    await h.go('/auth/login'); await driver.nextFrame(); await driver.end(h.host.querySelector('.page-fade-enter-active')!)
    await h.go('/auth/register'); await driver.nextFrame(); await driver.end(h.host.querySelector('.page-fade-leave-active')!)
    await driver.nextFrame(); await driver.end(h.host.querySelector('.page-fade-enter-active')!)
    scroll.calls.length = 0
    dom.window.history.replaceState({ scroll: { left: 7, top: 137 } }, '')
    h.router.back(); await new Promise(resolve => setTimeout(resolve, 10)); await settle()
    assert.equal(h.router.currentRoute.value.path, '/auth/login')
    assert.equal(scroll.calls.length, 0)
    await driver.nextFrame(); await driver.end(h.host.querySelector('.page-fade-leave-active')!)
    assert.equal(scroll.calls.length, 1)
    assert.equal(scroll.calls[0].position.top, 137); assert.equal(scroll.calls[0].position.left, 7)
    assert.match(scroll.calls[0].entering, /page-fade-enter-from/)
    await driver.nextFrame(); await driver.end(h.host.querySelector('.page-fade-enter-active')!)
  } finally { dom.window.history.replaceState(null, ''); h.unmount(); scroll.restore(); driver.restore() }
})

for (const arrival of ['before-hooks', 'after-hooks']) test(`saved scroll request arriving ${arrival} is applied without a second transition clock`, async () => {
  const driver = transitionDriver(), h = await authFixture('classic'), scroll = installScroll(h)
  try {
    await h.go('/auth/register'); await driver.nextFrame(); await driver.end(h.host.querySelector('.page-fade-enter-active')!)
    const callback = h.router.options.scrollBehavior!, from = h.router.currentRoute.value
    h.router.options.scrollBehavior = () => false
    let result: any, completed = false
    const remove = h.router.afterEach((to: any, previous: any, failure: any) => {
      if (!failure && arrival === 'before-hooks') result = Promise.resolve(callback(to, previous, { top: 83, left: 2 })).then(value => { completed = true; return value })
    })
    await h.go('/auth/login')
    assert.equal(completed, false)
    await driver.nextFrame(); await driver.end(h.host.querySelector('.page-fade-leave-active')!)
    if (arrival === 'after-hooks') result = callback(h.router.currentRoute.value, from, { top: 83, left: 2 })
    assert.deepEqual(await result, { top: 83, left: 2 })
    assert.ok(h.host.querySelector('.page-fade-enter-from'))
    await driver.nextFrame(); await driver.end(h.host.querySelector('.page-fade-enter-active')!)
    remove()
  } finally { h.unmount(); scroll.restore(); driver.restore() }
})

for (const mode of ['abort', 'error', 'superseded']) test(`real router ${mode} does not transfer pending auth scroll authority`, async () => {
  const driver = transitionDriver(), h = await authFixture('classic'), scroll = installScroll(h)
  try {
    await h.go('/auth/register'); await driver.nextFrame(); await driver.end(h.host.querySelector('.page-fade-enter-active')!)
    await h.go('/auth/login'); scroll.calls.length = 0
    let release!: () => void
    const error = Error('synthetic guard rejection')
    const removeError = h.router.onError(() => {})
    const removeGuard = h.router.beforeEach((to: any) => {
      if (to.name !== 'user-register') return
      if (mode === 'abort') return false
      if (mode === 'error') throw error
      return new Promise<void>(resolve => { release = resolve })
    })
    const attempt = h.router.push('/auth/register')
    if (mode === 'error') await assert.rejects(attempt, /synthetic guard rejection/)
    else if (mode === 'abort') assert.ok(await attempt)
    else {
      await settle(); await h.go('/auth/login?latest=1'); release(); assert.ok(await attempt)
    }
    assert.equal(h.router.currentRoute.value.name, 'user-login')
    assert.equal(scroll.calls.length, 0)
    await driver.nextFrame(); await driver.end(h.host.querySelector('.page-fade-leave-active')!)
    assert.equal(scroll.calls.length, 1, 'the last successful committed route owns the scroll')
    await driver.nextFrame(); await driver.end(h.host.querySelector('.page-fade-enter-active')!)
    removeGuard(); removeError()
  } finally { h.unmount(); scroll.restore(); driver.restore() }
})

for (const theme of ['classic', 'vault']) for (const boundary of ['leave-from', 'leave-to', 'enter-from', 'enter-to']) test(`${theme}: auth A-B-A at ${boundary} applies only the latest remaining scroll`, async () => {
  const driver = transitionDriver(), h = await authFixture(theme), scroll = installScroll(h)
  try {
    await h.go('/auth/register'); await driver.nextFrame(); await driver.end(h.host.querySelector('.page-fade-enter-active')!)
    scroll.calls.length = 0
    await h.go('/auth/login')
    if (boundary !== 'leave-from') await driver.nextFrame()
    if (boundary.startsWith('enter')) {
      await driver.end(h.host.querySelector('.page-fade-leave-active')!)
      assert.equal(scroll.calls.length, 1)
      scroll.calls.length = 0
      if (boundary === 'enter-to') await driver.nextFrame()
    }
    await h.go('/auth/register')
    assert.equal(scroll.calls.length, 0)
    const old = h.host.querySelector('.page-fade-leave-active')!
    assert.ok(old.hasAttribute('inert'))
    await driver.nextFrame(); await driver.end(old)
    assert.equal(scroll.calls.length, 1)
    assert.equal(scroll.calls[0].leaving, false)
    assert.match(scroll.calls[0].entering, /page-fade-enter-from/)
    assert.match(h.host.querySelector('h1')!.textContent!, /auth.register.title/)
    await driver.nextFrame(); await driver.end(h.host.querySelector('.page-fade-enter-active')!)
    assert.equal(h.host.querySelectorAll('form').length, 1)
  } finally { h.unmount(); scroll.restore(); driver.restore() }
})

for (const reduced of [false, true]) test(`${reduced ? 'reduced motion' : 'missing transitionend'} completes the scroll through Vue's own completion`, async () => {
  const driver = transitionDriver(), h = await authFixture('classic'), scroll = installScroll(h)
  try {
    await h.go('/auth/register'); await driver.nextFrame(); await driver.end(h.host.querySelector('.page-fade-enter-active')!)
    if (reduced) driver.reduce()
    scroll.calls.length = 0
    await h.go('/auth/login')
    assert.equal(scroll.calls.length, 0)
    await driver.nextFrame()
    if (!reduced) { await new Promise(resolve => setTimeout(resolve, 240)); await settle() }
    assert.equal(scroll.calls.length, 1)
    assert.match(scroll.calls[0].entering, /page-fade-enter-from/)
    await driver.nextFrame()
    if (!reduced) await driver.end(h.host.querySelector('.page-fade-enter-active')!)
    assert.equal(h.host.querySelector('[class*="page-fade-"]'), null)
  } finally { h.unmount(); scroll.restore(); driver.restore() }
})

test('non-auth, catalog and hash scroll policies keep their original precedence', async () => {
  const h = await authFixture('classic'), scroll = installScroll(h)
  try {
    const callback = h.router.options.scrollBehavior!
    const route = (name: string, hash = '') => ({ name, hash }) as any
    const saved = { left: 19, top: 44 }
    assert.equal(callback(route('category-products'), route('products'), saved), false)
    assert.equal(callback(route('other', '#target'), route('user-register'), saved), saved)
    assert.deepEqual(callback(route('user-login', '#target'), route('user-register'), null), { el: '#target', top: 80 })
    assert.deepEqual(callback(route('other'), route('user-register'), null), { top: 0 })
    assert.deepEqual(callback(route('user-login'), route('user-login'), null), { top: 0 })
  } finally { h.unmount(); scroll.restore() }
})

test('query replacement during auth leave retains the barrier without replaying an intermediate form', async () => {
  const driver = transitionDriver(), h = await authFixture('classic'), scroll = installScroll(h)
  try {
    await h.go('/auth/register'); await driver.nextFrame(); await driver.end(h.host.querySelector('.page-fade-enter-active')!)
    scroll.calls.length = 0
    await h.go('/auth/login')
    await h.go('/auth/login?redirect=/products')
    assert.equal(scroll.calls.length, 0, 'query replacement cannot move a still-visible outgoing auth form')
    await driver.nextFrame(); await driver.end(h.host.querySelector('.page-fade-leave-active')!)
    assert.equal(scroll.calls.length, 1)
    assert.match(scroll.calls[0].entering, /page-fade-enter-from/)
    const input = h.host.querySelector('input')!
    input.value = 'query@example.invalid'; input.dispatchEvent(new dom.window.Event('input', { bubbles: true })); await settle()
    await driver.nextFrame(); await driver.end(h.host.querySelector('.page-fade-enter-active')!)
    await h.go('/auth/login?redirect=/other')
    assert.equal(h.host.querySelector('input'), input)
    assert.equal(h.login.email.value, 'query@example.invalid')
    assert.equal(h.host.querySelector('[class*="page-fade-"]'), null)
    assert.equal(scroll.calls.length, 2, 'settled query scroll policy is unchanged')
  } finally { h.unmount(); scroll.restore(); driver.restore() }
})

test('App disposal settles pending auth scroll and refuses a late router callback', async () => {
  const driver = transitionDriver(), h = await authFixture('classic'), scroll = installScroll(h)
  try {
    await h.go('/auth/register'); await driver.nextFrame(); await driver.end(h.host.querySelector('.page-fade-enter-active')!)
    const from = h.router.currentRoute.value
    await h.go('/auth/login')
    const to = h.router.currentRoute.value
    const pending = h.router.options.scrollBehavior!(to, from, { left: 4, top: 101 })
    scroll.calls.length = 0
    h.unmount()
    assert.equal(await pending, false)
    assert.equal(await h.router.options.scrollBehavior!(to, from, { left: 8, top: 199 }), false, 'disposed App cannot regain scroll ownership')
    await driver.nextFrame(); await settle()
    assert.equal(scroll.calls.length, 0)
  } finally { h.unmount(); scroll.restore(); driver.restore() }
})

test('late stale scroll request cannot evict the latest A-B-A owner', async () => {
  const driver = transitionDriver(), h = await authFixture('classic'), scroll = installScroll(h)
  try {
    await h.go('/auth/register'); await driver.nextFrame(); await driver.end(h.host.querySelector('.page-fade-enter-active')!)
    const firstA = h.router.currentRoute.value
    scroll.calls.length = 0
    await h.go('/auth/login')
    const staleB = h.router.currentRoute.value
    await h.go('/auth/register')
    const late = h.router.options.scrollBehavior!(staleB, firstA, { left: 3, top: 91 })
    await driver.nextFrame(); await driver.end(h.host.querySelector('.page-fade-leave-active')!)
    assert.equal(await late, false, 'stale route identity must not regain scroll authority')
    assert.equal(scroll.calls.length, 1, 'latest scroll must survive stale callback arrival')
    assert.equal(scroll.calls[0].position.top, 0)
    await driver.nextFrame(); await driver.end(h.host.querySelector('.page-fade-enter-active')!)
  } finally { h.unmount(); scroll.restore(); driver.restore() }
})

for (const theme of ['classic', 'vault']) for (const start of ['register', 'login']) test(`${theme}: real ${start} bottom link defers router scroll until fade finishes and before enter paint`, async () => {
  const driver = transitionDriver(), h = await authFixture(theme, '/fixture-logo.svg'), scroll = installScroll(h)
  try {
    await h.go(`/auth/${start}`)
    await driver.nextFrame(); await driver.end(h.host.querySelector('.page-fade-enter-active')!)
    scroll.calls.length = 0
    const target = start === 'register' ? 'login' : 'register'
    const links = h.host.querySelectorAll<HTMLAnchorElement>(`a[href="/auth/${target}"]`)
    assert.ok(links.length)
    links[links.length - 1]!.click(); await settle(); await new Promise(resolve => setTimeout(resolve, 10)); await settle()
    assert.equal(h.router.currentRoute.value.path, `/auth/${target}`)
    const leaving = h.host.querySelector('.page-fade-leave-active')!
    assert.ok(leaving, 'actual Vue out-in holds the outgoing auth DOM')
    assert.ok(leaving.hasAttribute('inert'), 'security guard stays active')
    assert.equal(scroll.calls.length, 0, 'do not reset scroll while outgoing form is visible')
    await driver.nextFrame()
    assert.equal(scroll.calls.length, 0, 'leave-to is not leave completion')
    await driver.end(leaving)
    assert.equal(scroll.calls.length, 1)
    assert.equal(scroll.calls[0].position.top, 0)
    assert.equal(scroll.calls[0].leaving, false)
    assert.match(scroll.calls[0].entering, /page-fade-enter-from/, 'router applies before incoming visible frame')
    await driver.nextFrame(); await driver.end(h.host.querySelector('.page-fade-enter-active')!)
    assert.equal(h.events.length, 0)
  } finally { h.unmount(); scroll.restore(); driver.restore() }
})

for (const theme of ['classic', 'vault']) for (const logo of ['', '/fixture-logo.svg']) test(`${theme}: auth headers have parity with ${logo ? 'configured' : 'absent'} logo while forms and branding survive`, async () => {
  const h = await authFixture(theme, logo)
  try {
    await h.go('/auth/register')
    const title = h.host.querySelector('h1')!, header = title.parentElement!
    assert.ok(h.host.querySelector('form input'), 'registration form remains rendered')
    assert.equal(header.querySelector('p')!.textContent, 'Fixture brand')
    assert.equal(header.querySelector('img'), null, 'registration-only logo must not displace title')
    assert.equal(header.querySelector('p')!.classList.contains('mt-3'), false, 'no registration-only brand spacing even without a logo')
    const brandClass = header.querySelector('p')!.className
    const titleClass = title.className
    const input = h.host.querySelector('input')!
    input.value = 'retained@example.invalid'; input.dispatchEvent(new dom.window.Event('input', { bubbles: true })); await settle()
    await h.go('/auth/register?draft=keep')
    assert.equal(h.host.querySelector('input'), input); assert.equal(h.register.email.value, 'retained@example.invalid')
    await h.go('/auth/login'); await new Promise(resolve => setTimeout(resolve, 40)); await settle()
    assert.equal(h.host.querySelector('h1')!.className, titleClass)
    assert.equal(h.host.querySelector('h1')!.parentElement!.querySelector('p')!.className, brandClass)
    assert.match(h.host.textContent!, /Fixture brand/)
    assert.ok(h.host.querySelector('form input'), 'login form remains rendered')
    assert.equal(h.events.length, 0)
  } finally { h.unmount() }
})

test('query committed during auth leave cannot scroll after successful afterEach disposes App', async () => {
  const driver = transitionDriver(), h = await authFixture('classic'), scroll = installScroll(h)
  let remove = () => {}
  try {
    await h.go('/auth/register'); await driver.nextFrame(); await driver.end(h.host.querySelector('.page-fade-enter-active')!)
    await h.go('/auth/login'); scroll.calls.length = 0
    remove = h.router.afterEach((to: any, _from: any, failure: any) => {
      if (!failure && to.query.dispose) h.unmount()
    })
    await h.go('/auth/login?dispose=1')
    assert.equal(scroll.calls.length, 0, 'the real queued query callback must not outlive its App owner')
  } finally { remove(); h.unmount(); scroll.restore(); driver.restore() }
})

test('retained auth query callback stays stale after the latest before-enter', async () => {
  const driver = transitionDriver(), h = await authFixture('classic'), scroll = installScroll(h)
  try {
    await h.go('/auth/register'); await driver.nextFrame(); await driver.end(h.host.querySelector('.page-fade-enter-active')!)
    await h.go('/auth/login'); const login = h.router.currentRoute.value
    await h.go('/auth/login?old=1'); const oldQuery = h.router.currentRoute.value
    await h.go('/auth/register')
    await driver.nextFrame(); await driver.end(h.host.querySelector('.page-fade-leave-active')!)
    // Adversarial retained callback, not a claimed natural router scheduling delay.
    assert.equal(await h.router.options.scrollBehavior!(oldQuery, login, { top: 411 }), false)
    assert.equal(await h.router.options.scrollBehavior!(oldQuery, login, null), false)
    await driver.nextFrame(); await driver.end(h.host.querySelector('.page-fade-enter-active')!)
  } finally { h.unmount(); scroll.restore(); driver.restore() }
})

for (const saved of [null, { left: 9, top: 117 }]) test(`current auth query keeps ${saved ? 'saved-position' : 'top'} scroll at and after before-enter`, async () => {
  const driver = transitionDriver(), h = await authFixture('classic'), scroll = installScroll(h)
  try {
    await h.go('/auth/register'); await driver.nextFrame(); await driver.end(h.host.querySelector('.page-fade-enter-active')!)
    await h.go('/auth/login'); const login = h.router.currentRoute.value
    await h.go('/auth/login?current=1'); const query = h.router.currentRoute.value
    const callback = h.router.options.scrollBehavior!
    let completed = false
    const pending = Promise.resolve(callback(query, login, saved)).then(value => { completed = true; return value })
    await settle(); assert.equal(completed, false)
    await driver.nextFrame(); await driver.end(h.host.querySelector('.page-fade-leave-active')!)
    assert.deepEqual(await pending, saved ?? { top: 0 })
    assert.deepEqual(await callback(query, login, saved), saved ?? { top: 0 })
    await driver.nextFrame(); await driver.end(h.host.querySelector('.page-fade-enter-active')!)
  } finally { h.unmount(); scroll.restore(); driver.restore() }
})

for (const order of ['after-disposal', 'before-disposal']) test(`new App owner mounted ${order} retains auth scroll authority on the same router`, async () => {
  const driver = transitionDriver(), h = await authFixture('classic'), scroll = installScroll(h)
  const host = document.createElement('div'); document.body.append(host)
  let app: any
  try {
    await h.go('/auth/register'); await driver.nextFrame(); await driver.end(h.host.querySelector('.page-fade-enter-active')!)
    await h.go('/auth/login'); const login = h.router.currentRoute.value
    await h.go('/auth/login?old=1'); const oldQuery = h.router.currentRoute.value
    const pending = h.router.options.scrollBehavior!(oldQuery, login, { top: 199 })
    if (order === 'after-disposal') h.unmount()
    app = vue.createApp(h.load('App.vue').default); app.use(h.router); app.mount(host)
    if (order === 'before-disposal') h.unmount()
    assert.equal(await pending, false)
    // Removing the router's last app resets its current route; a new app starts
    // a fresh initial navigation before it can own a new auth transition.
    await h.router.isReady(); await settle()
    if (order === 'after-disposal') await h.go('/auth/login?current=1')
    await driver.nextFrame()
    const initialEnter = host.querySelector('.page-fade-enter-active')
    if (initialEnter) await driver.end(initialEnter)
    const currentQuery = h.router.currentRoute.value
    assert.equal(currentQuery.path, '/auth/login')
    const saved = { left: 6, top: 125 }
    assert.deepEqual(await h.router.options.scrollBehavior!(currentQuery, login, saved), saved, 'new owner may adopt the current route')
    scroll.calls.length = 0
    await h.go('/auth/register')
    assert.equal(await h.router.options.scrollBehavior!(oldQuery, login, { top: 411 }), false)
    assert.equal(scroll.calls.length, 0, 'new owner must still defer its auth switch')
    await driver.nextFrame(); await driver.end(host.querySelector('.page-fade-leave-active')!)
    assert.equal(scroll.calls.length, 1)
    assert.equal(scroll.calls[0].position.top, 0)
    await driver.nextFrame(); await driver.end(host.querySelector('.page-fade-enter-active')!)
  } finally { app?.unmount(); host.remove(); h.unmount(); scroll.restore(); driver.restore() }
})

test('ordinary non-auth callbacks keep top and saved policy before and after App disposal', async () => {
  const h = await authFixture('classic'), scroll = installScroll(h)
  try {
    const from = h.router.currentRoute.value
    await h.go('/privacy'); const privacy = h.router.currentRoute.value
    await h.go('/terms')
    const callback = h.router.options.scrollBehavior!, saved = { left: 5, top: 53 }
    for (const disposed of [false, true]) {
      if (disposed) h.unmount()
      // Deliberately stale ordinary routes never inherited the auth barrier.
      assert.equal(callback(privacy, from, saved), saved)
      assert.deepEqual(callback(privacy, from, null), { top: 0 })
    }
  } finally { h.unmount(); scroll.restore() }
})
