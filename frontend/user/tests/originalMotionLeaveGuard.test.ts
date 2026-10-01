import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fixture, settle, vue, dom, empty } from './helpers/motionPolishRouteHarness.ts'

const source = () => readFileSync(new URL('../src/App.vue', import.meta.url), 'utf8')
// Real Vue transition lifecycle; jsdom does not interpolate CSS or enforce inert.
function transitionDriver() {
  const previousFrame = globalThis.requestAnimationFrame
  const previousStyle = dom.window.getComputedStyle
  let frames: FrameRequestCallback[] = []
  globalThis.requestAnimationFrame = (fn: FrameRequestCallback) => { frames.push(fn); return frames.length }
  dom.window.getComputedStyle = ((element: Element) => new Proxy(previousStyle.call(dom.window, element), { get(target, key) {
    if (/page-fade-(enter|leave)-active/.test(element.className)) {
      if (key === 'transitionDuration') return `${Number(source().match(/transition:\s*opacity\s+(\d+)ms\s+ease/)![1]) / 1000}s`
      if (key === 'transitionDelay') return '0s'
      if (key === 'transitionProperty') return 'opacity'
    }
    return Reflect.get(target, key)
  } })) as any
  return {
    async frame() { for (let i = 0; i < 2; i++) { const batch = frames; frames = []; batch.forEach(fn => fn(0)); await settle() } },
    async end(element: Element) { element.dispatchEvent(new dom.window.Event('transitionend')); await settle() },
    restore() { frames = []; globalThis.requestAnimationFrame = previousFrame; dom.window.getComputedStyle = previousStyle },
  }
}

function assertActive(root: Element) {
  assert.equal(root.hasAttribute('inert'), false)
  assert.equal(root.hasAttribute('aria-hidden'), false)
  assert.equal((root as HTMLElement).style.pointerEvents, '')
}

async function personalFixture(theme: string) {
  const store = vue.reactive({ displayName: 'Fixture', profile: { email: 'fixture@example.invalid' }, memberLevels: [], currentLevel: null,
    personalCenterVisibility: { profile: true, security: true }, ensureProfileLoaded: async () => true, loadMemberLevels: async () => {},
  })
  const Panel = { setup: () => () => vue.h('input', { 'data-draft': '', value: 'draft' }) }
  const mocks: Record<string, any> = {
    'vue-i18n': { useI18n: () => ({ t: (s: string) => s, locale: vue.ref('en-US') }) },
    '../stores/userProfile': { useUserProfileStore: () => store }, '../../utils/image': { getImageUrl: (s: string) => s },
    '../components/shared/StatCard.vue': { default: empty },
  }
  for (const name of ['Profile', 'Security', 'Orders', 'Wallet', 'GiftCard', 'Affiliate', 'Api']) {
    for (const prefix of ['./personal/', '../../views/personal/']) mocks[`${prefix}${name}Panel.vue`] = { default: Panel }
  }
  let newClicks = 0
  const h = await fixture(theme, mocks, load => [
    { path: '/', component: empty },
    { path: '/other', component: { setup: () => () => vue.h('article', { 'data-other': '' }, [vue.h('button', { onClick: () => newClicks++ }, 'new action')]) } },
    { path: '/me/:section?', component: load(theme === 'classic' ? 'views/PersonalCenter.vue' : 'templates/vault/PersonalCenter.vue').default, props: (route: any) => ({ section: route.params.section || 'profile' }) },
  ])
  return { ...h, newClicks: () => newClicks }
}

for (const theme of ['classic', 'vault']) test(`${theme}: actual retired PersonalCenter button.click cannot redirect the committed route`, async () => {
  const driver = transitionDriver()
  const h = await personalFixture(theme)
  try {
    await h.go('/me/profile')
    const root = h.host.querySelector('.page-fade-enter-active') as HTMLElement
    await driver.frame(); await driver.end(root)
    const security = Array.from(root.querySelectorAll('button')).find(button => button.textContent?.includes('personalCenter.tabs.security'))!
    assert.ok(security, 'real PersonalCenter sidebar button, not a stub')
    await h.go('/other'); await driver.frame()
    assert.equal((root as any).__vueParentComponent.isUnmounted, true)
    assert.ok(root.isConnected && root.classList.contains('page-fade-leave-to'))
    security.click(); await settle(); await new Promise(resolve => setTimeout(resolve, 5)); await settle()
    assert.equal(h.router.currentRoute.value.fullPath, '/other', 'retired real switchSection must not run during exit fade')
    await driver.end(root)
    const next = h.host.querySelector('[data-other]') as HTMLElement
    assert.ok(next)
    next.querySelector('button')!.click()
    assert.equal(h.newClicks(), 1, 'entering page is immediately interactive')
    await driver.frame(); await driver.end(next)
    assert.equal(h.events.length, 0)
  } finally { h.unmount(); driver.restore() }
})

for (const theme of ['classic', 'vault']) test(`${theme}: real PersonalCenter query and section navigation never acquire a disabled state`, async () => {
  const driver = transitionDriver()
  const h = await personalFixture(theme)
  try {
    await h.go('/me/profile')
    const root = h.host.querySelector('.page-fade-enter-active') as HTMLElement
    await driver.frame(); await driver.end(root)
    const input = root.querySelector('input')!
    input.value = 'retained draft'
    input.focus()
    const preexistingHidden = Array.from(root.querySelectorAll('[disabled],[inert],[aria-hidden="true"]'))
    await h.go('/me/profile?draft=keep')
    assert.equal(root.querySelector('input'), input)
    assert.equal(input.value, 'retained draft')
    assert.equal(document.activeElement, input)
    assertActive(root)
    const security = Array.from(root.querySelectorAll('button')).find(button => button.textContent?.includes('personalCenter.tabs.security'))!
    security.click(); await settle(); await new Promise(resolve => setTimeout(resolve, 5)); await settle()
    assert.equal(h.router.currentRoute.value.fullPath, '/me/security')
    assert.equal(h.host.contains(root), true)
    assertActive(root)
    assert.deepEqual(Array.from(root.querySelectorAll('[disabled],[inert],[aria-hidden="true"]')), preexistingHidden, 'no additional disabled state; decorative icons keep their original aria-hidden')
    assert.equal(h.host.querySelector('[class*="page-fade-"]'), null)
    assert.equal(h.events.length, 0)
  } finally { h.unmount(); driver.restore() }
})

for (const theme of ['classic', 'vault']) for (const boundary of ['leave-from', 'leave-to']) test(`${theme}: real PersonalCenter rapid A-B-A at ${boundary} reenters interactive without inherited guards`, async () => {
  const driver = transitionDriver()
  const h = await personalFixture(theme)
  try {
    await h.go('/me/profile')
    const old = h.host.querySelector('.page-fade-enter-active') as HTMLElement
    await driver.frame(); await driver.end(old)
    old.querySelector('input')!.focus()
    await h.go('/other')
    if (boundary === 'leave-to') await driver.frame()
    assert.equal(old.contains(document.activeElement), false)
    await h.go('/me/profile?returned=1')
    assert.ok(old.isConnected && old.hasAttribute('inert'))
    assert.equal(h.host.querySelector('[data-other]'), null)
    const oldSecurity = Array.from(old.querySelectorAll('button')).find(button => button.textContent?.includes('personalCenter.tabs.security'))!
    oldSecurity.click(); await settle()
    assert.equal(h.router.currentRoute.value.fullPath, '/me/profile?returned=1')
    if (boundary === 'leave-from') await driver.frame()
    await driver.end(old)
    const current = h.host.querySelector('.page-fade-enter-active') as HTMLElement
    assert.ok(current && current !== old)
    assert.equal(old.isConnected, false)
    assertActive(old); assertActive(current)
    const input = current.querySelector('input')!
    input.focus(); assert.equal(document.activeElement, input)
    const security = Array.from(current.querySelectorAll('button')).find(button => button.textContent?.includes('personalCenter.tabs.security'))!
    security.click(); await settle(); await new Promise(resolve => setTimeout(resolve, 5)); await settle()
    assert.equal(h.router.currentRoute.value.fullPath, '/me/security')
    assertActive(current)
    await driver.frame(); await driver.end(current)
    assert.equal(h.host.querySelectorAll('aside').length, 1)
    assert.equal(h.events.length, 0)
  } finally { h.unmount(); driver.restore() }
})

for (const theme of ['classic', 'vault']) test(`${theme}: missing transitionend still restores the guard at Vue's original bounded timeout`, async () => {
  const driver = transitionDriver()
  const h = await fixture(theme, {}, [{ path: '/', component: { render: () => vue.h('article', [vue.h('input')]) } }, { path: '/next', component: empty }])
  try {
    const root = h.host.querySelector('article') as HTMLElement
    await h.go('/next'); await driver.frame()
    assert.equal(root.hasAttribute('inert'), true)
    await new Promise(resolve => setTimeout(resolve, 240)); await settle()
    assert.equal(root.isConnected, false)
    assertActive(root)
  } finally { h.unmount(); driver.restore() }
})

for (const theme of ['classic', 'vault']) test(`${theme}: cleanup does not overwrite a later DOM owner's attributes or styles`, async () => {
  const driver = transitionDriver()
  const h = await fixture(theme, {}, [{ path: '/', component: { render: () => vue.h('article', [vue.h('input')]) } }, { path: '/next', component: empty }])
  try {
    const root = h.host.querySelector('article') as HTMLElement
    await h.go('/next')
    root.setAttribute('inert', 'later-owner')
    root.setAttribute('aria-hidden', 'false')
    root.style.setProperty('pointer-events', 'auto')
    root.style.color = 'red'
    await driver.frame(); await driver.end(root)
    assert.equal(root.getAttribute('inert'), 'later-owner')
    assert.equal(root.getAttribute('aria-hidden'), 'false')
    assert.equal(root.style.pointerEvents, 'auto')
    assert.equal(root.style.getPropertyPriority('pointer-events'), '')
    assert.equal(root.style.color, 'red')
  } finally { h.unmount(); driver.restore() }
})

function transitionProps(vnode: any): any {
  if (!vnode || typeof vnode !== 'object') return undefined
  if (vnode.props?.mode === 'out-in' && vnode.props?.onBeforeLeave) return vnode.props
  return transitionProps(vnode.component?.subTree) || (Array.isArray(vnode.children) ? vnode.children.map(transitionProps).find(Boolean) : undefined)
}

for (const theme of ['classic', 'vault']) for (const boundary of ['before-enter', 'leave-cancelled', 'unmount']) test(`${theme}: ${boundary} releases only the retiring DOM ownership`, async () => {
  const driver = transitionDriver()
  let clicks = 0
  const Page = { render: () => vue.h('article', [vue.h('button', { onClick: () => clicks++ }, 'action')]) }
  const h = await fixture(theme, {}, [{ path: '/', component: Page }, { path: '/next', component: empty }])
  try {
    const root = h.host.querySelector('article') as HTMLElement
    const props = transitionProps((h.app as any)._instance.subTree)
    assert.ok(props, 'actual compiled App Transition bindings')
    root.setAttribute('inert', 'owned-elsewhere')
    root.setAttribute('aria-hidden', 'false')
    root.style.setProperty('pointer-events', 'none', 'important')
    await h.go('/next')
    assert.equal(root.getAttribute('aria-hidden'), 'true')
    root.querySelector('button')!.click(); assert.equal(clicks, 0)
    if (boundary === 'unmount') h.unmount()
    else {
      // Boundary contract probe: out-in normally removes rather than cancels.
      // Invoke the compiled hook to cover same-element reuse/cancellation too.
      const hook = boundary === 'before-enter' ? props.onBeforeEnter : props.onLeaveCancelled
      assert.equal(typeof hook, 'function', `${boundary} must release the guard`)
      hook(root)
      hook(root) // cleanup must be idempotent
    }
    assert.equal(root.getAttribute('inert'), 'owned-elsewhere')
    assert.equal(root.getAttribute('aria-hidden'), 'false')
    assert.equal(root.style.pointerEvents, 'none')
    assert.equal(root.style.getPropertyPriority('pointer-events'), 'important')
    root.querySelector('button')!.click(); assert.equal(clicks, 1, 'guard listener removed; original listener survives')
    if (boundary !== 'unmount') { await driver.frame(); await driver.end(root) }
  } finally { h.unmount(); driver.restore() }
})

// Dispatch bypasses jsdom's missing native inert implementation, as button.click
// can bypass browser hit testing. The capture fence must stop real handlers too.
for (const theme of ['classic', 'vault']) test(`${theme}: retiring capture fence cancels click submit and every keyboard phase only for its lifetime`, async () => {
  const driver = transitionDriver()
  const calls: string[] = []
  const Page = { setup: () => () => vue.h('article', [vue.h('form', {
    onClickCapture: () => calls.push('click'), onSubmitCapture: () => calls.push('submit'),
    onKeydownCapture: () => calls.push('keydown'), onKeyupCapture: () => calls.push('keyup'), onKeypressCapture: () => calls.push('keypress'),
  }, [vue.h('input'), vue.h('button', { type: 'button' }, 'action')])]) }
  const h = await fixture(theme, {}, [{ path: '/', component: Page }, { path: '/next', component: empty }])
  try {
    const root = h.host.querySelector('article')!
    const form = root.querySelector('form')!
    const input = root.querySelector('input')!
    root.querySelector('button')!.click(); assert.deepEqual(calls, ['click']); calls.length = 0
    await h.go('/next')
    const cancelled: Record<string, boolean> = {}
    for (const type of ['click', 'submit', 'keydown', 'keyup', 'keypress']) {
      const event = new dom.window.Event(type, { bubbles: true, cancelable: true })
      ;(type === 'submit' ? form : input).dispatchEvent(event)
      cancelled[type] = event.defaultPrevented
    }
    // A non-cancelable synthetic event must still be stopped.
    input.dispatchEvent(new dom.window.Event('keydown', { bubbles: true }))
    assert.deepEqual(cancelled, { click: true, submit: true, keydown: true, keyup: true, keypress: true })
    assert.deepEqual(calls, [], 'no descendant capture handler receives a retired event')
    await driver.frame(); await driver.end(root)
    for (const type of ['click', 'submit', 'keydown', 'keyup', 'keypress']) {
      const event = new dom.window.Event(type, { bubbles: true, cancelable: true })
      input.dispatchEvent(event)
      assert.equal(event.defaultPrevented, false, `${type} guard listener was removed`)
    }
    assert.deepEqual(calls, ['click', 'submit', 'keydown', 'keyup', 'keypress'], 'preexisting listeners are not removed')
  } finally { h.unmount(); driver.restore() }
})

for (const theme of ['classic', 'vault']) for (const restricted of [false, true]) test(`${theme}: leave isolates focus and native interaction then restores ${restricted ? 'existing root restrictions' : 'absent attributes'}`, async () => {
  const driver = transitionDriver()
  const Page = { setup: () => () => vue.h('article', { 'data-old': '' }, [
    vue.h('input'), vue.h('div', { inert: '', 'aria-hidden': 'true' }, [vue.h('button', 'already disabled subtree')]),
  ]) }
  const h = await fixture(theme, {}, [{ path: '/', component: Page }, { path: '/next', component: { render: () => vue.h('section', 'new page') } }])
  try {
    const root = h.host.querySelector('article') as HTMLElement
    const input = root.querySelector('input')!
    input.focus(); assert.equal(document.activeElement, input)
    if (restricted) {
      root.setAttribute('inert', 'preexisting')
      root.setAttribute('aria-hidden', 'false')
      root.style.setProperty('pointer-events', 'auto', 'important')
      root.style.color = 'red'
    }
    const original = { inert: root.getAttribute('inert'), aria: root.getAttribute('aria-hidden'), style: root.style.cssText, hasStyle: root.hasAttribute('style') }
    const nested = root.querySelector('div')!
    const nestedBefore = nested.outerHTML
    await h.go('/next')
    assert.ok(root.classList.contains('page-fade-leave-from'))
    assert.equal(root.hasAttribute('inert'), true, 'before-leave removes all native old tab targets')
    assert.equal(root.getAttribute('aria-hidden'), 'true')
    assert.equal(root.style.pointerEvents, 'none')
    assert.equal(root.style.getPropertyPriority('pointer-events'), 'important')
    assert.equal(root.contains(document.activeElement), false, 'focus must leave the retiring subtree')
    assert.equal(h.host.querySelector('section'), null, 'original out-in wait is retained')
    await driver.frame()
    assert.ok(root.classList.contains('page-fade-leave-to'))
    assert.equal(dom.window.getComputedStyle(root).pointerEvents, 'none')
    assert.equal(root.querySelectorAll('button,input,a[href],[tabindex]').length, 2, 'do not rewrite descendant tabindex or disabled state')
    assert.equal(nested.outerHTML, nestedBefore)
    await driver.end(root)
    assert.equal(root.isConnected, false)
    assert.deepEqual({ inert: root.getAttribute('inert'), aria: root.getAttribute('aria-hidden'), style: root.style.cssText, hasStyle: root.hasAttribute('style') }, original, 'restore only the owned root mutations')
    const next = h.host.querySelector('section')!
    assertActive(next)
    await driver.frame(); await driver.end(next)
  } finally { h.unmount(); driver.restore() }
})
