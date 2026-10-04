import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { vue, dom, loadSource, settle } from './helpers/motion17CHarness.ts'
const { createRouter, createMemoryHistory } = await import('vue-router')

const locales = {
  'zh-CN': ['浏览器订单', '邮箱查询'],
  'zh-TW': ['瀏覽器訂單', '信箱查詢'],
  'en-US': ['This browser', 'Email lookup'],
}
const read = (path: string) => readFileSync(new URL('../' + path, import.meta.url), 'utf8')
const reply = (orders: any[] = [], page = 1, totalPages = 1) => ({ data: {
  data: orders, pagination: { page, page_size: 20, total: orders.length, total_page: totalPages },
} })

async function mountLookup(path = 'views/GuestOrders.vue', locale = 'en-US', orders: any[] = [], captchaEnabled = false) {
  dom.window.sessionStorage.clear(); dom.window.localStorage.clear()
  const messages = JSON.parse(read(`src/i18n/locales/${locale}.json`))
  const t = (key: string) => key.split('.').reduce((value, part) => value?.[part], messages) ?? key
  const calls: any[] = []
  const api = loadSource('api/order.ts', { './client': { userApi: {
    get: async (url: string, options: any) => { calls.push({ url, options }); return reply(orders) },
  } } }).guestOrderAPI
  const wrapper = { setup: (_: any, { slots }: any) => () => vue.h('div', slots.default?.()) }
  const Button = { setup: (_: any, { slots }: any) => () => vue.h('button', slots.default?.()) }
  const Input = { props: ['modelValue'], emits: ['update:modelValue'], setup: (props: any, { emit }: any) => () => vue.h('input', {
    value: props.modelValue, onInput: (event: any) => emit('update:modelValue', event.target.value),
  }) }
  const Guest = loadSource(path, {
    '../components/captcha/ImageCaptcha.vue': { default: { setup: () => () => vue.h('div', { 'data-lookup-captcha': 'image' }, 'captcha fixture') } },
    '../components/captcha/TurnstileCaptcha.vue': { default: wrapper },
    '../stores/app': { useAppStore: () => ({ config: { captcha: { provider: captchaEnabled ? 'image' : 'none', scenes: { guest_create_order: captchaEnabled } } }, loadConfig: async () => {} }) },
    'vue-i18n': { useI18n: () => ({ t }) }, '../api': { guestOrderAPI: api },
    '@/components/ui/alert': { Alert: wrapper, AlertDescription: wrapper }, '@/components/ui/badge': { Badge: wrapper },
    '@/components/ui/button': { Button, buttonVariants: () => '' }, '@/components/ui/input': { Input },
  }).default
  const host = document.createElement('div'); document.body.append(host)
  const router = createRouter({ history: createMemoryHistory(), routes: [
    { path: '/guest/orders', component: wrapper },
    { path: '/guest/orders/:order_no', name: 'guest-order-detail', component: wrapper },
  ] })
  await router.push('/guest/orders')
  const app = vue.createApp(Guest); app.use(router); app.mount(host); await settle()
  return { host, calls, messages, router, close: () => { app.unmount(); host.remove() } }
}

for (const path of ['views/GuestOrders.vue', 'templates/vault/GuestOrders.vue']) {
  test(`${path}: captcha is exclusive to email tab with Chinese email validation before any lookup`, async () => {
    const h = await mountLookup(path, 'zh-CN', [], true)
    try {
      assert.equal(h.host.querySelector('[data-lookup-captcha]'), null)
      ;(h.host.querySelectorAll('button')[1] as HTMLElement).click(); await settle()
      const captcha = h.host.querySelector('[data-lookup-captcha]')!
      assert.ok(captcha.parentElement!.classList.contains('sm:col-span-3'))
      const inputs = h.host.querySelectorAll('input')
      inputs[0]!.value = '123'; inputs[0]!.dispatchEvent(new dom.window.Event('input'))
      inputs[1]!.value = 'fixture'; inputs[1]!.dispatchEvent(new dom.window.Event('input')); await settle()
      const search = [...h.host.querySelectorAll('button')].find(button => button.textContent?.includes(h.messages.guestOrders.search))!
      search.click(); await settle()
      assert.ok(h.host.textContent?.includes('邮箱格式不正确'))
      assert.equal(h.calls.length, 1)
    } finally { h.close() }
  })
  test(`${path}: both lookup paths retain displayed order identity and the exact detail route`, async () => {
    const orderNo = 'ORDER/KEEP-123'
    const h = await mountLookup(path, 'en-US', [{ order_no: orderNo, total_amount: '10.00', currency: 'USD', status: 'pending_payment' }])
    try {
      const checkOrder = () => {
        assert.ok(h.host.textContent?.includes(orderNo))
        assert.equal(h.host.querySelector('a')?.getAttribute('href'), '/guest/orders/' + encodeURIComponent(orderNo))
      }
      checkOrder()
      ;(h.host.querySelectorAll('button')[1] as HTMLElement).click(); await settle()
      const inputs = h.host.querySelectorAll('input')
      inputs[0]!.value = 'fixture@example.invalid'; inputs[0]!.dispatchEvent(new dom.window.Event('input'))
      inputs[1]!.value = 'fixture-password'; inputs[1]!.dispatchEvent(new dom.window.Event('input'))
      inputs[2]!.value = '  ' + orderNo + '  '; inputs[2]!.dispatchEvent(new dom.window.Event('input'))
      await settle()
      const search = [...h.host.querySelectorAll('button')].find(button => button.textContent?.includes(h.messages.guestOrders.search))!
      search.click(); await new Promise(resolve => setTimeout(resolve, 350)); await settle()
      assert.equal(h.calls.at(-1).url, '/guest/orders')
      assert.deepEqual(h.calls.at(-1).options.params, { order_no: orderNo, page: 1, page_size: 20 })
      checkOrder()
      h.host.querySelector('a')!.click(); await new Promise(resolve => setTimeout(resolve, 0)); await settle()
      assert.equal(h.router.currentRoute.value.name, 'guest-order-detail')
      assert.equal(h.router.currentRoute.value.params.order_no, orderNo)
    } finally { h.close() }
  })
  for (const [locale, labels] of Object.entries(locales)) {
    test(`${path} ${locale}: renders only browser and email lookup, preserving same-browser explanation`, async () => {
      const h = await mountLookup(path, locale)
      try {
        const tabs = [...h.host.querySelectorAll('button')]
        assert.deepEqual(tabs.map(button => button.textContent?.trim()), labels)
        assert.ok(tabs[0]!.parentElement!.classList.contains('grid-cols-2'))
        assert.ok(h.host.textContent?.includes(h.messages.guestOrders.browserEmptyHint))
        assert.match(h.messages.guestOrders.browserEmptyHint, /当前浏览器|目前瀏覽器|this browser/)
        assert.equal(h.host.querySelectorAll('input').length, 0)
        tabs[1]!.click(); await settle()
        assert.deepEqual([...h.host.querySelectorAll('input')].map(input => input.type), ['email', 'password', 'text'])
        assert.deepEqual([...h.host.querySelectorAll('input')].map(input => input.placeholder), [
          h.messages.guestOrders.emailPlaceholder, h.messages.guestOrders.passwordPlaceholder, h.messages.guestOrders.orderNoPlaceholder,
        ])
        tabs[0]!.click(); await settle()
        assert.equal(h.host.querySelectorAll('input').length, 0)
        assert.deepEqual(Object.keys(h.messages.guestOrders.tabs), ['browser', 'credentials'])
        assert.match(h.messages.guestOrders.orderNoPlaceholder, /可选|可選|optional/)
        for (const key of ['orderNoPlaceholderRequired', 'emptyOrderNo']) {
          assert.equal(Object.hasOwn(h.messages.guestOrders, key), false)
        }
        for (const key of ['orderNoMissing', 'notFound']) assert.equal(Object.hasOwn(h.messages.guestOrders.errors, key), false)
      } finally { h.close() }
    })
  }
}
