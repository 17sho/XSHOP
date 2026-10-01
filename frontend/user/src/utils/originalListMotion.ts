import type { ObjectDirective } from 'vue'

/** Replay the existing item CSS, without replacing keyed rows or their state. */
export const vOriginalListMotion: ObjectDirective<HTMLElement, { revision: number; stale: boolean }> = {
  updated(el, { value, oldValue }) {
    if (value.revision === oldValue?.revision || value.stale) return
    if (window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) return
    const rows = Array.from(el.querySelectorAll<HTMLElement>('.catalog-feedback .theme-slide-up'))
    if (!rows.length) return
    // Cancel as one batch, flush once, then let the original CSS own timing.
    for (const row of rows) row.style.animationName = 'none'
    void el.offsetWidth
    for (const row of rows) row.style.removeProperty('animation-name')
    // No frame/timer/listener survives this hook. CSS owns cancellation on
    // reduced-motion changes and DOM removal; the next result resets this batch.
  },
}
