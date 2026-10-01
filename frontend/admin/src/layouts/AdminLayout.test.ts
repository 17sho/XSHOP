import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, type App } from 'vue'
import { createMemoryHistory, createRouter, RouterView } from 'vue-router'
import i18n from '@/i18n'
import AdminLayout from './AdminLayout.vue'
import layoutSource from './AdminLayout.vue?raw'

vi.mock('@/stores/auth', () => ({
  useAdminAuthStore: () => ({ hasPermission: () => true, logout: vi.fn() }),
}))
vi.mock('@/api/admin', () => ({
  adminAPI: { getPublicConfig: vi.fn(async () => ({ data: { data: {} } })) },
}))

let app: App | undefined
let container: HTMLDivElement
let styles: HTMLStyleElement

beforeEach(() => {
  localStorage.clear()
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: 390 })
  vi.useFakeTimers()
  // jsdom does not load SFC CSS. Install the actual owned rules so Vue's
  // real Transition reads their duration; no component/transition stubs.
  styles = document.createElement('style')
  styles.textContent = layoutSource.match(/<style scoped>([\s\S]*?)<\/style>/)?.[1] ?? ''
  document.head.append(styles)
})
afterEach(() => {
  app?.unmount()
  container?.remove()
  styles.remove()
  vi.clearAllTimers()
  vi.useRealTimers()
})

async function mountLayout() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/', component: AdminLayout, children: [
      { path: ':pathMatch(.*)*', component: { render: () => h('input', { 'data-page-input': '' }) } },
    ] }],
  })
  await router.push('/products')
  await router.isReady()
  container = document.createElement('div')
  document.body.append(container)
  app = createApp({ render: () => h(RouterView) })
  app.use(router).use(i18n).mount(container)
  await nextTick()
  return router
}

function element(selector: string) {
  const node = container.querySelector<HTMLElement>(selector)
  expect(node, selector).not.toBeNull()
  return node!
}
async function click(selector: string) {
  element(selector).click()
  await nextTick()
}
async function frames() {
  await vi.advanceTimersByTimeAsync(40)
  await nextTick()
}
async function settle() {
  await vi.advanceTimersByTimeAsync(400)
  await nextTick()
}
const drawerSelector = '[data-admin-mobile-nav]'
const backdropSelector = '[data-admin-nav-backdrop]'
const triggerSelector = '[data-admin-nav-trigger]'

describe('AdminLayout sidebar motion (real mounted layout)', () => {
  it('disables desktop, drawer and backdrop transitions under the reduced-motion CSS rules', async () => {
    // jsdom does not evaluate media queries. Activate the exact production
    // media rule, then exercise real Vue transitions with those declarations.
    const reduced = Array.from(styles.sheet!.cssRules).find((rule) =>
      rule instanceof CSSMediaRule && rule.conditionText === '(prefers-reduced-motion: reduce)',
    ) as CSSMediaRule | undefined
    expect(reduced).toBeDefined()
    styles.textContent += '\n' + Array.from(reduced!.cssRules).map((rule) => rule.cssText).join('\n')
    await mountLayout()
    const desktop = element('[data-admin-desktop-nav]')
    expect(getComputedStyle(desktop).transitionDuration).toBe('0s')
    await click(triggerSelector)
    const drawer = element(drawerSelector)
    const backdrop = element(backdropSelector)
    for (const node of [drawer, backdrop]) {
      expect(getComputedStyle(node).transitionProperty).toBe('none')
      expect(getComputedStyle(node).transitionDuration).toBe('0s')
    }
    await frames()
    expect(drawer.className).not.toMatch(/admin-drawer-enter-/)
    await click(backdropSelector)
    expect(drawer.hasAttribute('inert')).toBe(true)
    for (const node of [drawer, backdrop]) {
      expect(getComputedStyle(node).transitionProperty).toBe('none')
      expect(getComputedStyle(node).transitionDuration).toBe('0s')
    }
    await frames()
    expect(drawer.style.display).toBe('none')
    expect(backdrop.style.display).toBe('none')
  })

  it('animates only desktop width while preserving collapse persistence and auto sizing', async () => {
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1440 })
    await mountLayout()
    const desktop = element('aside')
    expect(getComputedStyle(desktop).transitionProperty).toBe('width')
    expect(getComputedStyle(desktop).transitionDuration).toBe('0.3s')
    expect(desktop.classList.contains('transition-all')).toBe(false)
    expect(desktop.matches('[data-admin-desktop-nav]')).toBe(true)
    expect(desktop.classList.contains('w-64')).toBe(true)
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1000 })
    window.dispatchEvent(new Event('resize'))
    await nextTick()
    expect(desktop.classList.contains('w-16')).toBe(true)
    await click('header button:nth-child(2)')
    expect(desktop.classList.contains('w-64')).toBe(true)
    expect(localStorage.getItem('admin_sidebar_collapsed')).toBe('false')
    window.dispatchEvent(new Event('resize'))
    await nextTick()
    expect(desktop.classList.contains('w-64')).toBe(true)
    await click('header button:nth-child(2)')
    expect(desktop.classList.contains('w-16')).toBe(true)
    expect(localStorage.getItem('admin_sidebar_collapsed')).toBe('true')
  })

  it('closes at the desktop breakpoint even with a saved desktop collapse preference', async () => {
    localStorage.setItem('admin_sidebar_collapsed', 'false')
    await mountLayout()
    await click(triggerSelector)
    await frames()
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 768 })
    window.dispatchEvent(new Event('resize'))
    await nextTick()
    expect(element(drawerSelector).hasAttribute('inert')).toBe(true)
    await settle()
    expect(element(drawerSelector).style.display).toBe('none')
    expect(element(backdropSelector).style.display).toBe('none')
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 390 })
    window.dispatchEvent(new Event('resize'))
    await nextTick()
    expect(element(drawerSelector).style.display).toBe('none')
  })

  it.each([0, 40, 140])('settles open-close-open interruptions after %i ms without queued overlays', async (delay) => {
    await mountLayout()
    const drawer = element(drawerSelector)
    const backdrop = element(backdropSelector)
    for (let repeat = 0; repeat < 3; repeat++) {
      await click(triggerSelector)
      await vi.advanceTimersByTimeAsync(delay)
      await click(backdropSelector)
      await vi.advanceTimersByTimeAsync(delay)
      await click(triggerSelector)
      await settle()
      expect(element(drawerSelector)).toBe(drawer)
      expect(element(backdropSelector)).toBe(backdrop)
      expect(container.querySelectorAll(drawerSelector)).toHaveLength(1)
      expect(container.querySelectorAll(backdropSelector)).toHaveLength(1)
      expect(drawer.style.display).not.toBe('none')
      expect(backdrop.style.display).not.toBe('none')
      expect(drawer.hasAttribute('inert')).toBe(false)
      expect(getComputedStyle(drawer).pointerEvents).not.toBe('none')
      expect(drawer.className).not.toMatch(/admin-drawer-(enter|leave)-/)
      expect(backdrop.className).not.toMatch(/admin-backdrop-(enter|leave)-/)
      await click(backdropSelector)
      await settle()
      expect(drawer.style.display).toBe('none')
      expect(backdrop.style.display).toBe('none')
    }
  })

  it('closes for real link and programmatic path navigation without delaying the routed page', async () => {
    const router = await mountLayout()
    for (const path of ['/orders', '/settings']) {
      await click(triggerSelector)
      await frames()
      if (path === '/orders') {
        await click(`${drawerSelector} a[href="/orders"]`)
        await vi.advanceTimersByTimeAsync(0)
      } else {
        await router.push(path)
      }
      await nextTick()
      expect(router.currentRoute.value.path).toBe(path)
      expect(element(drawerSelector).hasAttribute('inert')).toBe(true)
      expect(container.querySelectorAll('[data-page-input]')).toHaveLength(1)
      await settle()
      expect(element(drawerSelector).style.display).toBe('none')
      expect(element(backdropSelector).style.display).toBe('none')
    }
    // Clicking the active link still closes without relying on a route change.
    await click(triggerSelector)
    await click(`${drawerSelector} a[href="/settings"]`)
    await settle()
    expect(element(drawerSelector).style.display).toBe('none')
  })

  it('makes a closing or hidden drawer inert immediately and releases its focused control', async () => {
    await mountLayout()
    const drawer = element(drawerSelector)
    const backdrop = element(backdropSelector)
    expect(drawer.hasAttribute('inert')).toBe(true)
    expect(drawer.getAttribute('aria-hidden')).toBe('true')
    expect(getComputedStyle(drawer).pointerEvents).toBe('none')
    expect(getComputedStyle(backdrop).pointerEvents).toBe('none')
    await click(triggerSelector)
    await settle()
    expect(drawer.hasAttribute('inert')).toBe(false)
    expect(drawer.getAttribute('aria-hidden')).toBe('false')
    expect(getComputedStyle(drawer).pointerEvents).not.toBe('none')
    const input = drawer.querySelector('input')!
    input.focus()
    expect(document.activeElement).toBe(input)
    await click(backdropSelector)
    expect(drawer.style.display).not.toBe('none') // still visually leaving
    expect(drawer.hasAttribute('inert')).toBe(true)
    expect(drawer.getAttribute('aria-hidden')).toBe('true')
    expect(getComputedStyle(drawer).pointerEvents).toBe('none')
    expect(getComputedStyle(backdrop).pointerEvents).toBe('none')
    expect(drawer.contains(document.activeElement)).toBe(false)
    await settle()
    expect(drawer.style.display).toBe('none')
    expect(backdrop.style.display).toBe('none')
  })

  it('slides the same mobile drawer in and out while fading the backdrop in sync', async () => {
    await mountLayout()
    await click('header button')
    const drawer = element('aside[aria-label="Mobile navigation"]')
    const backdrop = element('.fixed.inset-0')
    expect(drawer.classList.contains('admin-drawer-enter-from')).toBe(true)
    expect(backdrop.classList.contains('admin-backdrop-enter-from')).toBe(true)
    expect(getComputedStyle(drawer).transform).toBe('translateX(-100%)')
    expect(getComputedStyle(backdrop).opacity).toBe('0')
    expect(getComputedStyle(drawer).transitionProperty).toBe('transform')
    expect(getComputedStyle(backdrop).transitionProperty).toBe('opacity')
    expect(getComputedStyle(drawer).transitionDuration).toBe('0.24s')
    expect(getComputedStyle(backdrop).transitionDuration).toBe('0.24s')
    await frames()
    expect(drawer.classList.contains('admin-drawer-enter-to')).toBe(true)
    await settle()
    expect(drawer.className).not.toMatch(/admin-drawer-enter-/)
    expect(drawer.style.display).not.toBe('none')
    await click(`${drawerSelector} [aria-label="Close navigation"]`)
    expect(drawer.classList.contains('admin-drawer-leave-active')).toBe(true)
    expect(backdrop.classList.contains('admin-backdrop-leave-active')).toBe(true)
    await frames()
    expect(getComputedStyle(drawer).transform).toBe('translateX(-100%)')
    expect(getComputedStyle(backdrop).opacity).toBe('0')
    await settle()
    expect(element(drawerSelector)).toBe(drawer)
    expect(element(backdropSelector)).toBe(backdrop)
    expect(drawer.style.display).toBe('none')
    expect(backdrop.style.display).toBe('none')
  })
})
