import { nextTick, onBeforeUnmount, watch, type Ref } from 'vue'

// Only interaction ownership is shared. Each overlay's single Vue Transition owns its pixels.
type Owner = { panel: Ref<HTMLElement | null>; previous: HTMLElement | null; priority: number }
const owners: Owner[] = []
const consumedEscapes = new WeakSet<KeyboardEvent>()
let bodyOverflow = ''
let htmlOverflow = ''
const topOwner = () => owners.reduce<Owner | undefined>((top, owner) => !top || owner.priority >= top.priority ? owner : top, undefined)
const inerted = new Map<HTMLElement, boolean>()
const syncInert = () => {
  for (const [element, wasInert] of inerted) {
    if (!wasInert && !element.hasAttribute('data-overlay-closing')) element.removeAttribute('inert')
  }
  inerted.clear()
  const panel = topOwner()?.panel.value
  if (!panel) return
  for (const child of Array.from(document.body.children)) {
    if (!(child instanceof HTMLElement) || child.contains(panel) || child.matches('script,style,link')) continue
    inerted.set(child, child.hasAttribute('inert'))
    child.setAttribute('inert', '')
  }
}
const focusableSelector = 'a[href],button:not([disabled]),input:not([disabled]),select:not([disabled]),textarea:not([disabled]),[tabindex]:not([tabindex="-1"])'
const isVisibleFocusTarget = (el: HTMLElement) => {
  if (el.closest('[inert], [hidden]')) return false
  const style = window.getComputedStyle(el)
  if (style.display === 'none' || style.visibility === 'hidden') return false
  // A responsive wrapper can be display:none while its button still computes
  // to inline-flex. Such controls cannot receive native browser focus.
  for (let ancestor = el.parentElement; ancestor; ancestor = ancestor.parentElement) {
    if (window.getComputedStyle(ancestor).display === 'none') return false
  }
  return true
}
const focusables = (panel: HTMLElement | null) => Array.from(panel?.querySelectorAll<HTMLElement>(focusableSelector) || []).filter(isVisibleFocusTarget)
const focusPanel = (owner: Owner) => (focusables(owner.panel.value)[0] || owner.panel.value)?.focus()

export function useOverlayMotionLifecycle(visible: () => boolean, panel: Ref<HTMLElement | null>, close: () => void, priority = 0) {
  const owner: Owner = { panel, previous: null, priority }
  let generation = 0
  let disposed = false
  const beforeLeave = (element: Element) => {
    element.setAttribute('data-overlay-closing', '')
    element.setAttribute('inert', '')
    element.setAttribute('aria-hidden', 'true')
    ;(element as HTMLElement).style.pointerEvents = 'none'
  }
  const beforeEnter = (element: Element) => {
    element.removeAttribute('data-overlay-closing')
    element.removeAttribute('inert')
    element.removeAttribute('aria-hidden')
    ;(element as HTMLElement).style.pointerEvents = ''
  }
  const onKeydown = (event: KeyboardEvent) => {
    if (topOwner() !== owner || !visible()) return
    if (event.key === 'Escape') {
      if (event.defaultPrevented || consumedEscapes.has(event)) return
      // Consume before close can synchronously release ownership. Synthetic
      // non-cancelable events cannot use defaultPrevented as their marker.
      consumedEscapes.add(event)
      event.preventDefault()
      close()
      return
    }
    if (event.key !== 'Tab') return
    const nodes = focusables(panel.value)
    const first = nodes[0]; const last = nodes[nodes.length - 1]
    if (!first || !last) { event.preventDefault(); panel.value?.focus(); return }
    if (!panel.value?.contains(document.activeElement) || (event.shiftKey && document.activeElement === first)) {
      event.preventDefault(); (event.shiftKey ? last : first).focus()
    } else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus() }
  }
  const release = () => {
    const index = owners.indexOf(owner)
    if (index < 0) return
    const wasTop = topOwner() === owner
    owners.splice(index, 1)
    // A later overlay may have captured focus inside this one. Skip it when it retires.
    for (const remaining of owners) {
      if (remaining.previous && panel.value?.contains(remaining.previous)) remaining.previous = owner.previous
    }
    syncInert()
    document.removeEventListener('keydown', onKeydown)
    if (!owners.length) {
      if (document.body.style.overflow === 'hidden') document.body.style.overflow = bodyOverflow
      if (document.documentElement.style.overflow === 'hidden') document.documentElement.style.overflow = htmlOverflow
    }
    if (wasTop) {
      const top = topOwner()
      if (top) focusPanel(top)
      else if (owner.previous?.isConnected && !owner.previous.closest('[inert]')) owner.previous.focus()
    }
    owner.previous = null
  }
  watch(visible, async open => {
    const current = ++generation
    if (!open) { release(); return }
    owner.previous = document.activeElement instanceof HTMLElement ? document.activeElement : null
    if (!owners.length) {
      bodyOverflow = document.body.style.overflow
      htmlOverflow = document.documentElement.style.overflow
      document.body.style.overflow = 'hidden'
      document.documentElement.style.overflow = 'hidden'
    }
    if (!owners.includes(owner)) owners.push(owner)
    document.addEventListener('keydown', onKeydown)
    // A synchronous watcher can run before Vue queues the owner's render.
    // Yield first, then await that render so the Teleport panel ref is available.
    await Promise.resolve()
    await nextTick()
    if (!disposed && current === generation && visible()) {
      syncInert()
      if (topOwner() === owner) focusPanel(owner)
    }
  }, { immediate: true, flush: 'sync' })
  onBeforeUnmount(() => { disposed = true; generation++; release() })
  return { beforeEnter, beforeLeave }
}
