import test from 'node:test'
import assert from 'node:assert/strict'
import { loader, vue, settle } from './helpers/motion17DHarness.ts'

test('overlay opening and keyboard wrap skip controls inside responsive hidden ancestors', async () => {
  const { useOverlayMotionLifecycle } = loader()('composables/overlayMotionLifecycle.ts')
  const visible = vue.ref(false)
  const panel = vue.ref(null as HTMLElement | null)
  const host = document.createElement('div'); document.body.append(host)
  const trigger = document.querySelector('#trigger') as HTMLButtonElement
  trigger.focus()
  const app = vue.createApp({ setup() {
    useOverlayMotionLifecycle(() => visible.value, panel, () => { visible.value = false }, 50)
    return () => visible.value ? vue.h(vue.Teleport, { to: 'body' }, vue.h('div', { ref: panel, tabindex: -1 }, [
      vue.h('div', { style: { display: 'none' } }, [vue.h('button', { 'data-hidden-first': '' }, 'Desktop close')]),
      vue.h('button', { 'data-visible-first': '' }, 'Visible first'),
      vue.h('button', { 'data-visible-last': '' }, 'Visible last'),
      vue.h('div', { hidden: true }, [vue.h('button', { 'data-hidden-last': '' }, 'Hidden last')]),
    ])) : null
  } })
  app.mount(host)
  try {
    visible.value = true; await settle(); await settle()
    const first = document.querySelector('[data-visible-first]') as HTMLElement
    const last = document.querySelector('[data-visible-last]') as HTMLElement
    assert.equal(document.activeElement, first, 'initial focus must not target a desktop-only control')
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', shiftKey: true, bubbles: true, cancelable: true }))
    assert.equal(document.activeElement, last, 'reverse wrap must skip hidden trailing control')
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true }))
    assert.equal(document.activeElement, first, 'forward wrap must skip hidden leading control')
    assert.equal(document.body.style.overflow, 'hidden')
  } finally { app.unmount(); host.remove() }
  assert.equal(document.body.style.overflow, '')
  assert.equal(document.activeElement, trigger)
})
