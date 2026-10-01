import { afterEach, describe, expect, it } from 'vitest'
import { createApp, defineComponent, h, nextTick, onUnmounted, ref, type App } from 'vue'
import { createMemoryHistory, createRouter, RouterView, useRoute } from 'vue-router'
import AdminRouteView from './AdminRouteView.vue'
import outletSource from './AdminRouteView.vue?raw'
import layoutSource from '../layouts/AdminLayout.vue?raw'

let app: App | undefined
let container: HTMLDivElement
afterEach(() => { app?.unmount(); container?.remove() })

async function mountRoutes() {
  let mounts = 0
  let unmounts = 0
  const Page = defineComponent({
    setup() {
      mounts++
      onUnmounted(() => { unmounts++ })
      const draft = ref('')
      const route = useRoute()
      return () => h('section', { 'data-page': route.path }, [
        h('input', {
          value: draft.value,
          onInput: (event: Event) => { draft.value = (event.target as HTMLInputElement).value },
        }),
        h('output', route.fullPath),
      ])
    },
  })
  const Shell = defineComponent({
    setup: () => () => h('div', [h('header', 'Header'), h('aside', 'Sidebar'), h('main', [h(AdminRouteView)])]),
  })
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/admin', component: Shell, children: [
      { path: 'products/:id?', component: Page },
      { path: 'orders', component: Page },
      { path: 'settings', component: Page },
    ] }],
  })
  await router.push('/admin/products')
  await router.isReady()
  container = document.createElement('div')
  document.body.append(container)
  app = createApp({ render: () => h(RouterView) })
  app.use(router)
  app.mount(container)
  await nextTick()
  return { router, counts: () => ({ mounts, unmounts }) }
}

describe('admin content route entrance', () => {
  it('places the motion outlet only inside main, leaving the header and sidebars outside it', () => {
    expect(layoutSource).toMatch(/import AdminRouteView from ['"]@\/components\/AdminRouteView.vue['"]/)
    expect(layoutSource).toMatch(/<main\b[^>]*>\s*<AdminRouteView\s*\/>\s*<\/main>/)
    expect(layoutSource.match(/<AdminRouteView\b/g)).toHaveLength(1)
  })

  it('replaces only the content wrapper on a path change, including reused page components', async () => {
    const { router, counts } = await mountRoutes()
    const header = container.querySelector('header')
    const sidebar = container.querySelector('aside')
    const wrapper = container.querySelector('main > div')!
    await router.push('/admin/products/2')
    await nextTick()
    expect(container.querySelector('main > div')).not.toBe(wrapper)
    expect(wrapper.isConnected).toBe(false)
    expect(container.querySelector('[data-admin-route-view]')).toBe(container.querySelector('main > div'))
    expect(container.querySelector('header')).toBe(header)
    expect(container.querySelector('aside')).toBe(sidebar)
    expect(container.querySelectorAll('[data-page]')).toHaveLength(1)
    expect(container.querySelector('[data-page]')?.getAttribute('data-page')).toBe('/admin/products/2')
    expect(counts()).toEqual({ mounts: 2, unmounts: 1 })
  })

  it('preserves the wrapper, component, focused input and draft for query/hash-only navigation', async () => {
    const { router, counts } = await mountRoutes()
    const wrapper = container.querySelector('[data-admin-route-view]')!
    const input = container.querySelector('input')!
    input.value = 'unsaved draft'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    input.focus()
    await nextTick()
    const classes = wrapper.className
    for (const target of ['/admin/products?page=2', '/admin/products?page=2#draft', '/admin/products#top']) {
      await router.push(target)
      await nextTick()
      expect(container.querySelector('[data-admin-route-view]')).toBe(wrapper)
      expect(wrapper.className).toBe(classes)
      expect(container.querySelector('input')).toBe(input)
      expect(input.value).toBe('unsaved draft')
      expect(document.activeElement).toBe(input)
      expect(container.querySelector('output')?.textContent).toBe(target)
      expect(counts()).toEqual({ mounts: 1, unmounts: 0 })
    }
  })

  it('commits rapid navigation without waiting for animation events or retaining an outgoing page', async () => {
    const { router } = await mountRoutes()
    const initial = container.querySelector('[data-admin-route-view]')!
    for (const path of ['/admin/orders', '/admin/settings', '/admin/products']) {
      const previous = container.querySelector('[data-admin-route-view]')!
      await router.push(path)
      await nextTick()
      expect(router.currentRoute.value.path).toBe(path)
      expect(container.querySelectorAll('[data-admin-route-view]')).toHaveLength(1)
      expect(container.querySelectorAll('[data-page]')).toHaveLength(1)
      expect(container.querySelector('output')?.textContent).toBe(path)
      expect(previous.isConnected).toBe(false)
    }
    await Promise.all([router.push('/admin/orders'), router.push('/admin/settings')])
    await nextTick()
    expect(container.querySelector('output')?.textContent).toBe('/admin/settings')
    expect(container.querySelectorAll('[data-page]')).toHaveLength(1)
    expect(initial.isConnected).toBe(false)
  })

  it('disables the entrance entirely for reduced motion', () => {
    expect(outletSource).toMatch(/@media\s*\(prefers-reduced-motion:\s*reduce\)\s*\{\s*\.admin-route-content\s*\{\s*animation:\s*none;\s*\}\s*\}/)
  })

  it('uses only a short opacity/upward entrance without persistent transform or an exit lifecycle', () => {
    const styles = (outletSource.match(/<style scoped>([\s\S]*?)<\/style>/)?.[1] ?? '').replace(/\/\*[\s\S]*?\*\//g, '')
    expect(styles).toMatch(/animation:\s*admin-route-enter 160ms ease-out;/)
    expect(styles).toMatch(/from\s*\{\s*opacity:\s*0;\s*transform:\s*translateY\(6px\);\s*\}/)
    expect(styles).toMatch(/to\s*\{\s*opacity:\s*1;\s*transform:\s*none;\s*\}/)
    const baseRule = styles.match(/\.admin-route-content\s*\{([^}]*)\}/)?.[1] ?? ''
    expect(baseRule).not.toMatch(/transform|will-change|opacity|forwards|both/)
    expect(styles).not.toMatch(/will-change|animation-fill-mode|transition:/)
    expect(outletSource).not.toMatch(/<Transition|<KeepAlive|setTimeout|requestAnimationFrame|out-in/)
  })
})
