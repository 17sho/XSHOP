import test, { afterEach } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fixture, installCss, media, wait, settle, budgets } from './helpers/originalMotionCHarness.ts'
const source = (kind: string) => readFileSync(new URL(`../src/components/${kind}.vue`, import.meta.url), 'utf8')
const duration = (el: Element) => Math.max(...window.getComputedStyle(el).transitionDuration.split(',').map(v => parseFloat(v) * (v.trim().endsWith('ms') ? 1 : 1000)))
afterEach(async () => { await wait(350) })
for (const [kind, width, panelEnter, panelLeave, start, finish, curve] of [
  ['ProductQuickBuy', 390, 280, 200, 'translateY(100%)', 'translateY(100%)', 'cubic-bezier(.32,.72,0,1)'],
  ['ProductQuickBuy', 1024, 200, 150, 'scale(.96)', 'scale(.96)', 'ease-out'],
  ['AnnouncementModal', 390, 300, 200, 'translateY(12px) scale(.95)', 'translateY(8px) scale(.95)', 'cubic-bezier(.34,1.56,.64,1)'],
  ['ConfirmDialog', 390, 200, 150, 'translateY(8px) scale(.95)', 'translateY(8px) scale(.95)', 'ease-out'],
] as const) test(`${kind} ${width}: original pixels and curves, longest root budget, painted dismissal`, async () => {
  const m = media(width), css = installCss(kind, width), f = fixture(kind)
  try {
    const root = f.root, panel = root.querySelector('[data-overlay-panel]')!
    const [enter, leave] = budgets(kind, width)
    assert.deepEqual(f.transitionDuration(), { enter, leave })
    assert.equal(duration(root), enter)
    assert.equal(duration(panel), panelEnter)
    assert.equal(window.getComputedStyle(panel).transform, start)
    assert.ok(window.getComputedStyle(panel).transition.includes(curve))
    if (kind === 'ProductQuickBuy') {
      assert.ok(['', '1'].includes(window.getComputedStyle(root).opacity), 'solid panel never inherits backdrop fade')
      assert.equal(window.getComputedStyle(root).backgroundColor, 'rgba(0, 0, 0, 0)')
      assert.ok(root.classList.contains('bg-black/40'))
      if (width < 768) assert.ok(['', '1'].includes(window.getComputedStyle(panel).opacity))
    }
    assert.equal((source(kind).match(/<Transition\b/g) || []).length, 1)
    await wait(enter + 50)
    assert.ok(['', 'none'].includes(window.getComputedStyle(panel).transform), 'no permanent containing block')
    panel.dispatchEvent(new window.MouseEvent('click', { bubbles: true })); await settle()
    assert.equal(f.open.value, true)
    root.click(); await settle(); await wait(20)
    assert.equal(f.open.value, false)
    assert.equal(duration(root), leave)
    assert.equal(duration(panel), panelLeave)
    assert.equal(window.getComputedStyle(panel).transform, finish)
    assert.ok(root.hasAttribute('inert')); assert.equal(root.style.pointerEvents, 'none')
    assert.equal(document.body.style.overflow, '')
    await wait(leave + 30); assert.equal(root.isConnected, false)
    assert.deepEqual(f.actions, { purchase: 0, navigate: 0, confirm: 0, dismiss: 0, close: 1 })
  } finally { f.unmount(); css(); m.restore() }
})
