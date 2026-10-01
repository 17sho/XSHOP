import assert from 'node:assert/strict'
import test from 'node:test'
import { dom, vue, loadSource, settle } from './helpers/motion17CHarness.ts'

for (const path of ['views/GuestOrders.vue', 'templates/vault/GuestOrders.vue']) {
test(`${path}: tabs preserve inputs without custom local animation across unrelated updates`, async () => {
  dom.window.matchMedia = (() => ({ matches: false, addEventListener() {}, removeEventListener() {} })) as any
  const events: any[] = []
  dom.window.HTMLElement.prototype.animate = function() { const event = { target: this, cancelled: false }; events.push(event); return { cancel: () => { event.cancelled = true } } as any }
  const state = {
    activeTab: vue.ref('browser'), email: vue.ref('draft@example.invalid'), orderPassword: vue.ref(''), loading: vue.ref(false), error: vue.ref(''), orders: vue.ref([]), pagination: vue.reactive({ page: 1, total_page: 1 }), emptyMessage: vue.ref('No orders'),
    setActiveTab: (tab: string) => { state.activeTab.value = tab }, searchByCredentials() {}, changePage() {}, statusLabel: () => '', statusVariant: () => '', formatMoney: () => '', formatDate: () => '',
  }
  const wrapper = { setup: (_: any, { slots }: any) => () => vue.h('div', slots.default?.()) }
  const Button = { setup: (_: any, { slots }: any) => () => vue.h('button', slots.default?.()) }
  const Input = { props: ['modelValue'], setup: (props: any) => () => vue.h('input', { value: props.modelValue }) }
  const Guest = loadSource(path, {
    'vue-i18n': { useI18n: () => ({ t: (key: string) => key }) },
    '../composables/useGuestOrders': { useGuestOrders: () => state },
    '@/components/ui/alert': { Alert: wrapper, AlertDescription: wrapper }, '@/components/ui/badge': { Badge: wrapper }, '@/components/ui/button': { Button, buttonVariants: () => '' }, '@/components/ui/input': { Input },
    '../components/PaginationNav.vue': { default: { setup: () => () => null } },
  }).default
  const host = document.createElement('div'); document.body.append(host)
  const app = vue.createApp(Guest); app.component('RouterLink', wrapper); app.mount(host)
  try {
    await settle(); assert.equal(events.length, 0)
    const buttons = host.querySelectorAll('button')
    ;(buttons[1] as HTMLElement).click(); await settle()
    assert.equal(events.length, 0)
    const input = host.querySelector('input')!
    state.loading.value = true; await settle()
    assert.equal(host.querySelector('input'), input)
    assert.equal(input.value, 'draft@example.invalid')
    assert.equal(events.length, 0)
    ;(buttons[0] as HTMLElement).click(); await settle()
    assert.equal(events.length, 0)
  } finally { app.unmount(); host.remove() }
})
}
