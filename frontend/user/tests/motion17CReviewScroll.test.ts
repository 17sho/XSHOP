import test from 'node:test'
import assert from 'node:assert/strict'
import { dom, vue, loadSource, settle } from './helpers/motion17CHarness.ts'

for (const initialReduced of [true, false]) {
  test(`reseller editor reads reduced-motion at each deferred scroll (initial=${initialReduced})`, async () => {
    let reduce = initialReduced
    dom.window.matchMedia = (() => ({ get matches() { return reduce }, addEventListener() {}, removeEventListener() {} })) as any
    const calls: any[] = []
    dom.window.HTMLElement.prototype.scrollIntoView = function(options: any) { calls.push({ id: this.id, options }) }
    const detail = { product: { id: 901, slug: 'synthetic-only', title: { 'en-US': 'Synthetic item' }, price_amount: '1.00' }, product_setting: null, skus: [] }
    let resolveDetail!: (value: any) => void
    const pendingDetail = new Promise(resolve => { resolveDetail = resolve })
    const forbidden = () => { throw new Error('Business write forbidden') }
    const wrapper = { setup: (_: any, { slots }: any) => () => vue.h('div', slots.default?.()) }
    const Empty = { setup: () => () => null }
    const component = loadSource('views/personal/ResellerProductSettingsPanel.vue', {
      'vue-i18n': { useI18n: () => ({ t: (key: string) => key, locale: vue.ref('en-US') }) },
      '@/components/ui/button': { Button: 'button' }, '@/components/ui/badge': { Badge: 'span' },
      '@/components/ui/card': { Card: wrapper }, '@/components/ui/alert': { Alert: wrapper, AlertDescription: wrapper },
      '@/components/ui/input': { Input: 'input' }, './ResellerProductRuleEditor.vue': { default: Empty },
      '../../api/reseller': { resellerAPI: {
        productSettings: async () => ({ data: { data: [detail], pagination: { page: 1, page_size: 20, total: 1, total_page: 1 } } }),
        productSettingDetail: () => pendingDetail,
        previewProductSettings: forbidden, updateProductSettings: forbidden,
      } },
    }).default
    const host = document.createElement('div'); document.body.append(host)
    const app = vue.createApp(component); app.mount(host)
    try {
      await settle(); await settle()
      const edit = [...host.querySelectorAll('button')].find(button => button.textContent?.trim() === 'personalCenter.reseller.productSettings.edit')
      assert.ok(edit)
      edit.click(); await settle(); await settle()
      assert.equal(calls.length, 1, 'cached row is scrolled before detail returns')
      assert.deepEqual(calls[0], { id: 'reseller-product-row-901', options: { behavior: initialReduced ? 'auto' : 'smooth', block: 'nearest' } })
      reduce = !initialReduced
      resolveDetail({ data: { data: detail } }); await settle(); await settle()
      assert.equal(calls.length, 2, 'loaded editor may change row height and retains its second positioning pass')
      assert.deepEqual(calls[1], { id: 'reseller-product-row-901', options: { behavior: reduce ? 'auto' : 'smooth', block: 'nearest' } })
    } finally {
      const cancel = [...host.querySelectorAll('button')].find(button => button.textContent?.trim() === 'common.cancel')
      cancel?.click(); await settle(); app.unmount(); host.remove()
    }
  })
}
