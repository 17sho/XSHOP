import test, { afterEach } from 'node:test'
import assert from 'node:assert/strict'
import { fixture, installCss, media, rootEnd, end, wait, vue, settle, kinds, budgets } from './helpers/motionPolishOverlayReversalHarness.ts'

// A failed assertion may unmount a still-leaving Teleport. Drain Vue's bounded
// timer before the next fixture, so it cannot accidentally select the old root.
afterEach(async () => { await wait(350) })

// The native counterexample ends ALL root properties BEFORE panel transform.
// JSDOM does not interpolate CSS: these events replay that ordering on real Vue.
for (const kind of kinds) for (const width of [390, 767, 768, 1024]) {
  test(`${kind} ${width}: interrupted enter ignores root end before panel end`, async () => {
    const m = media(width), css = installCss(kind, width), f = fixture(kind, false)
    try {
      f.open.value = true; await settle(); await wait(30)
      const root = document.querySelector('[data-overlay-root]') as HTMLElement
      const panel = root.querySelector('[data-overlay-panel]')!
      f.open.value = false; await settle(); await wait(20)
      const [enter, leave] = budgets(kind, width)
      assert.deepEqual(f.transitionDuration(), { enter, leave }, 'explicit lifetime matches this breakpoint')
      assert.ok(root.hasAttribute('inert'))
      assert.equal(root.style.pointerEvents, 'none')
      assert.equal(document.body.style.overflow, '')
      rootEnd(root, kind)
      await settle()
      assert.ok(root.isConnected, 'early root transitionend must NOT retire a still-moving panel')
      await wait(leave - 70)
      assert.ok(root.isConnected, 'root must remain until later panel completion')
      end(panel, 'transform')
      await wait(budgets(kind, width)[1] + 50)
      assert.equal(root.isConnected, false, 'single completion owner has a bounded fallback')
    } finally { f.unmount(); css(); m.restore() }
  })
}

// Each close re-samples the current preference; neither setup nor a cached
// computed without reactive media dependencies may freeze the first choice.
for (const kind of kinds) for (const width of [390, 1024]) {
  for (const initialReduced of [false, true]) test(`${kind} ${width}: current reduced preference on close (initial ${initialReduced})`, async () => {
    const m = media(width, initialReduced)
    let css = installCss(kind, width, initialReduced)
    const f = fixture(kind, false)
    try {
      f.open.value = true; await settle(); await wait(30)
      const root = document.querySelector('[data-overlay-root]')!
      // Switch while open, before this close interaction, including initial reduce.
      m.set(width, true); css(); css = installCss(kind, width, true)
      f.open.value = false; await settle(); await wait(30)
      assert.equal(f.transitionDuration(), 0, 'reduced interaction schedules no positive duration')
      assert.equal(root.isConnected, false, 'reduced close has zero duration, not a normal-motion timer')
      assert.equal(document.body.style.overflow, '')
      // Subsequent open+close must pick up normal motion again.
      m.set(width, false); css(); css = installCss(kind, width, false)
      f.open.value = true; await settle(); await wait(30)
      const reopened = document.querySelector('[data-overlay-root]')!
      f.open.value = false; await settle(); await wait(20)
      rootEnd(reopened, kind); await settle()
      assert.ok(reopened.isConnected, 'preference back to normal restores panel lifetime')
      await wait(budgets(kind, width)[1] + 50)
      assert.equal(reopened.isConnected, false)
    } finally { f.unmount(); css(); m.restore() }
  })
}

for (const kind of kinds) for (const width of [390, 1024]) {
  for (const mode of ['fast-close-no-events', 'fully-open-root-first', 'rapid-reopen'] as const) {
    test(`${kind} ${width}: ${mode} keeps bounded lifetime and focus ownership`, async () => {
      const m = media(width), css = installCss(kind, width), f = fixture(kind, false)
      const trigger = document.querySelector('#trigger') as HTMLElement
      trigger.focus()
      const [enter, leave] = budgets(kind, width)
      try {
        f.open.value = true; await settle(); await wait(mode === 'fully-open-root-first' ? enter + 40 : 25)
        const root = document.querySelector('[data-overlay-root]') as HTMLElement
        assert.ok(root.contains(document.activeElement), 'opening focus reaches current rendered refs')
        f.open.value = false; await settle(); await wait(15)
        assert.equal(document.activeElement, trigger)
        assert.equal(document.body.style.overflow, '')
        assert.ok(root.hasAttribute('inert'))
        assert.equal(root.getAttribute('aria-hidden'), 'true')
        assert.equal(root.style.pointerEvents, 'none')
        if (mode !== 'fast-close-no-events') rootEnd(root, kind)
        assert.ok(root.isConnected, 'early root end cannot own completion')
        if (mode === 'rapid-reopen') {
          f.open.value = true; await settle(); await wait(enter + leave + 50)
          const roots = document.querySelectorAll('[data-overlay-root]')
          assert.equal(roots.length, 1, 'obsolete leave timer cannot remove reopened generation')
          const current = roots[0] as HTMLElement
          assert.ok(current.contains(document.activeElement))
          assert.equal(current.hasAttribute('inert'), false)
          assert.equal(current.hasAttribute('aria-hidden'), false)
          assert.equal(current.style.pointerEvents, '')
          assert.equal(document.body.style.overflow, 'hidden')
          assert.equal(document.documentElement.style.overflow, 'hidden')
          assert.equal(document.querySelector('#app')!.hasAttribute('inert'), true)
          f.open.value = false; await settle(); await wait(15)
          rootEnd(current, kind)
          assert.ok(current.isConnected)
          await wait(leave + 50)
          assert.equal(current.isConnected, false)
        } else {
          await wait(leave + 50)
          assert.equal(root.isConnected, false, 'no events must still retire the root within its budget')
        }
        assert.equal(document.body.style.overflow, '')
        assert.equal(document.activeElement, trigger)
        assert.deepEqual(f.actions, { purchase: 0, navigate: 0, confirm: 0, dismiss: 0, close: 0 })
      } finally { f.unmount(); css(); m.restore() }
    })
  }

  test(`${kind} ${width}: reduce during leave releases interaction and cannot strand an invisible root`, async () => {
    const m = media(width), f = fixture(kind, false)
    let css = installCss(kind, width)
    try {
      f.open.value = true; await settle(); await wait(25)
      const root = document.querySelector('[data-overlay-root]') as HTMLElement
      f.open.value = false; await settle(); await wait(15)
      rootEnd(root, kind)
      assert.ok(root.isConnected)
      m.set(width, true); css(); css = installCss(kind, width, true)
      assert.ok(root.hasAttribute('inert'))
      assert.equal(root.style.pointerEvents, 'none')
      assert.equal(document.body.style.overflow, '')
      // CSS cancels the effect; no transitionend will be delivered. The captured
      // leave timer may retain noninteractive pixels, but only for <=200ms.
      await wait(budgets(kind, width)[1] + 50)
      assert.equal(root.isConnected, false)
      f.open.value = true; await settle(); await wait(15)
      const reducedRoot = document.querySelector('[data-overlay-root]')!
      f.open.value = false; await settle(); await wait(25)
      assert.equal(reducedRoot.isConnected, false, 'next interaction has no reduced-motion delay')
    } finally { f.unmount(); css(); m.restore() }
  })
}

for (const kind of kinds) for (const [from, to] of [[767, 768], [768, 767]]) {
  test(`${kind}: breakpoint ${from} to ${to} sampled on close, not setup`, async () => {
    const m = media(from), f = fixture(kind, false)
    let css = installCss(kind, from)
    try {
      f.open.value = true; await settle(); await wait(30)
      const root = document.querySelector('[data-overlay-root]')!
      m.set(to, false); css(); css = installCss(kind, to)
      f.open.value = false; await settle(); await wait(15)
      rootEnd(root, kind)
      const [enter, leave] = budgets(kind, to)
      assert.deepEqual(f.transitionDuration(), { enter, leave }, 'responsive duration is sampled now')
      await wait(leave - 65)
      assert.ok(root.isConnected, 'root survives the current panel leave, including md-to-mobile')
      await wait(100)
      assert.equal(root.isConnected, false, 'sampled budget is bounded after responsive change')
    } finally { f.unmount(); css(); m.restore() }
  })
}
