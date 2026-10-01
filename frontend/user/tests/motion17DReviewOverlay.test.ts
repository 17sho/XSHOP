import test, { afterEach } from 'node:test'
import assert from 'node:assert/strict'
import { loader, vue, settle } from './helpers/motion17DHarness.ts'

afterEach(async () => { await new Promise(resolve => setTimeout(resolve, 30)) })
// Real shared composable + Vue Teleport/Transition. Only the dialog content is synthetic.
function overlay(load: any, priority: number, initiallyOpen = true) {
  const open = vue.ref(initiallyOpen), panel = vue.ref(null)
  let closes = 0
  const host = document.createElement('div'); document.body.append(host)
  const app = vue.createApp({ setup() {
    const lifecycle = load('composables/overlayMotionLifecycle.ts').useOverlayMotionLifecycle(() => open.value, panel, () => { closes++; open.value = false }, priority)
    return () => [
      vue.h('button', { 'data-open': '', onClick: () => { open.value = true } }, 'Open'),
      vue.h(vue.Teleport, { to: 'body' }, vue.h(vue.Transition, { onBeforeEnter: lifecycle.beforeEnter, onBeforeLeave: lifecycle.beforeLeave }, () => open.value ? vue.h('div', { 'data-overlay-root': '' }, [vue.h('div', { ref: panel, tabindex: -1 }, [vue.h('button', 'First'), vue.h('button', 'Last')])]) : null)),
    ]
  } })
  app.mount(host)
  return { open, panel, host, get closes() { return closes }, unmount() { app.unmount(); host.remove() } }
}
const escape = (cancelable = true) => new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable })

for (const cancelable of [true, false]) test(`one Escape closes only synchronous top registered first (cancelable=${cancelable})`, async () => {
  const trigger = document.querySelector('#trigger') as HTMLElement; trigger.focus()
  const load = loader(), top = overlay(load, 110); await settle()
  const lower = overlay(load, 50); await settle()
  try {
    const first = escape(cancelable); document.dispatchEvent(first)
    assert.equal(top.open.value, false)
    assert.equal(lower.open.value, true, 'synchronous release must not transfer this Escape event to another owner')
    assert.equal(top.closes, 1); assert.equal(lower.closes, 0)
    assert.equal(first.defaultPrevented, cancelable)
    assert.equal(document.body.style.overflow, 'hidden')
    assert.ok(lower.panel.value.contains(document.activeElement))
    await vue.nextTick()
    document.dispatchEvent(escape(cancelable)); await settle()
    assert.equal(lower.open.value, false); assert.equal(lower.closes, 1)
    assert.equal(document.body.style.overflow, '')
    assert.equal(document.activeElement, trigger, 'return-focus rebasing skips the dismissed upper panel')
  } finally { lower.unmount(); top.unmount() }
})

test('a previously consumed cancelable Escape is not claimed by the overlay owner', async () => {
  const f = overlay(loader(), 50); await settle()
  try {
    const event = escape(); event.preventDefault(); document.dispatchEvent(event)
    assert.equal(f.open.value, true)
    assert.equal(f.closes, 0)
    document.dispatchEvent(escape()); assert.equal(f.open.value, false)
  } finally { f.unmount() }
})

for (const priority of [50, 110]) test(`normal lower-first stack consumes one key and same-priority latest owner wins (upper=${priority})`, async () => {
  const load = loader(), lower = overlay(load, 50); await settle()
  const top = overlay(load, priority); await settle()
  try {
    assert.ok(top.panel.value.contains(document.activeElement))
    document.dispatchEvent(escape(false))
    assert.equal(top.open.value, false); assert.equal(lower.open.value, true)
    document.dispatchEvent(escape(false))
    assert.equal(lower.open.value, false)
  } finally { top.unmount(); lower.unmount() }
})

test('button false-to-true open waits for rendered panel; closing inertness is next-patch and fast reopen retains ownership', async () => {
  const f = overlay(loader(), 50, false)
  const trigger = f.host.querySelector('[data-open]') as HTMLElement; trigger.focus()
  try {
    trigger.click(); await settle()
    const panel = f.panel.value, root = panel.parentElement, buttons = panel.querySelectorAll('button')
    assert.equal(document.activeElement, buttons[0], 'sync visibility watch must await the scheduled Vue render')
    assert.equal(document.querySelector('#app')!.hasAttribute('inert'), true)
    buttons[1].focus()
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true }))
    assert.equal(document.activeElement, buttons[0])
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', shiftKey: true, bubbles: true, cancelable: true }))
    assert.equal(document.activeElement, buttons[1])
    f.open.value = false
    assert.equal(document.body.style.overflow, '', 'lock release remains synchronous')
    assert.equal(root.hasAttribute('inert'), false, 'Vue has not patched leave yet')
    await vue.nextTick()
    assert.equal(root.hasAttribute('inert'), true); assert.equal(root.getAttribute('aria-hidden'), 'true'); assert.equal(root.style.pointerEvents, 'none')
    assert.equal(document.activeElement, trigger)
    trigger.click(); await settle()
    assert.equal(document.body.style.overflow, 'hidden')
    assert.equal(f.panel.value.parentElement.hasAttribute('inert'), false)
    assert.equal(f.panel.value.parentElement.hasAttribute('aria-hidden'), false)
    assert.ok(f.panel.value.contains(document.activeElement))
  } finally { f.unmount() }
  assert.equal(document.body.style.overflow, '')
})

test('out-of-order lower dismissal preserves upper ownership and rebases eventual return focus', async () => {
  const trigger = document.querySelector('#trigger') as HTMLElement; trigger.focus()
  const load = loader(), lower = overlay(load, 50); await settle()
  const root = lower.panel.value.parentElement
  const top = overlay(load, 110); await settle()
  try {
    lower.open.value = false; await vue.nextTick()
    assert.ok(top.panel.value.contains(document.activeElement))
    assert.equal(document.body.style.overflow, 'hidden')
    document.dispatchEvent(escape()); await vue.nextTick()
    assert.equal(root.hasAttribute('inert'), true)
    assert.equal(root.style.pointerEvents, 'none')
    assert.equal(document.activeElement, trigger)
    assert.equal(document.body.style.overflow, '')
  } finally { top.unmount(); lower.unmount() }
})
