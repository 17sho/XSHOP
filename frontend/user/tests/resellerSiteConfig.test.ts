import test from 'node:test'
import assert from 'node:assert/strict'
import { loader, vue, settle } from './helpers/motion17DHarness.ts'
import {
  blankLocalizedText,
  hasLocalizedText,
  getLocalizedText,
  isResellerSiteSeoConfigured,
  normalizeFooterLinksForForm,
  canEditResellerSiteConfig,
} from '../src/utils/resellerSiteConfig.ts'

for (const home of [false, true, undefined]) test(`owner site editor preserves homepage=${home} on unrelated save and reload`, async () => {
  const custom = [{ name: { 'zh-CN': '', 'zh-TW': '', 'en-US': 'Help' }, url: 'https://example.invalid/help' }]
  const config = { id: 1, nav_config: { builtin: { notice: false }, homepage_notice_enabled: home, custom_items: custom } }
  const writes: any[] = []
  const wrap = { setup: (_: any, { slots }: any) => () => vue.h('div', slots.default?.()) }
  const nil = { render: () => null }
  const mocks: any = {
    'vue-i18n': { useI18n: () => ({ t: (s: string) => s }) },
    '../../stores/app': { useAppStore: () => ({ loadConfig: async () => {} }) },
    '../../utils/richContent': { sanitizeRichHtml: (s: string) => s },
    '../../api': { resellerAPI: {
      siteConfig: async () => ({ data: { data: { opened: true, can_edit: true, config } } }),
      updateSiteConfig: async (data: any) => { const copy = JSON.parse(JSON.stringify(data)); writes.push(copy); return { data: { data: copy } } },
    } },
    './ResellerImageField.vue': { default: nil }, './ResellerLocaleTabs.vue': { default: nil }, './ResellerRichText.vue': { default: nil },
  }
  for (const [module, names] of Object.entries({ alert: ['Alert', 'AlertDescription'], button: ['Button'], card: ['Card'], input: ['Input'], select: ['Select', 'SelectContent', 'SelectItem', 'SelectTrigger', 'SelectValue'], switch: ['Switch'], textarea: ['Textarea'] })) mocks[`@/components/ui/${module}`] = Object.fromEntries(names.map(name => [name, wrap]))
  const component = loader(mocks)('components/reseller/ResellerSiteConfigPanel.vue').default
  const host = document.createElement('div'); document.body.append(host)
  const app = vue.createApp(component); app.mount(host)
  try {
    await settle()
    assert.ok(host.querySelector('form'), 'editable loaded form')
    host.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })); await settle()
    assert.deepEqual(writes[0].nav_config, { ...config.nav_config, homepage_notice_enabled: home !== false })
    host.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })); await settle()
    assert.deepEqual(writes[1].nav_config, writes[0].nav_config, 'save-response hydration does not erase switch')
  } finally { app.unmount(); host.remove() }
})

test('blank localized text includes all supported storefront locales', () => {
  assert.deepEqual(blankLocalizedText(), { 'zh-CN': '', 'zh-TW': '', 'en-US': '' })
})

test('localized text fallback follows current locale then zh-CN then first non-empty', () => {
  const value = { 'zh-CN': '简体', 'zh-TW': '繁體', 'en-US': 'English' }
  assert.equal(getLocalizedText(value, 'en-US'), 'English')
  assert.equal(getLocalizedText({ 'zh-CN': '简体' }, 'zh-TW'), '简体')
  assert.equal(getLocalizedText({ 'en-US': 'English' }, 'zh-TW'), 'English')
})

test('footer links normalize missing localized names', () => {
  assert.deepEqual(
    normalizeFooterLinksForForm([{ name: { 'zh-CN': '客服' }, url: 'https://example.test' }]),
    [{ name: { 'zh-CN': '客服', 'zh-TW': '', 'en-US': '' }, url: 'https://example.test' }],
  )
})

test('site config editing requires opened and editable snapshot', () => {
  assert.equal(canEditResellerSiteConfig({ opened: true, can_edit: true }), true)
  assert.equal(canEditResellerSiteConfig({ opened: true, can_edit: false }), false)
  assert.equal(canEditResellerSiteConfig(null), false)
})

test('localized text helpers require at least one non-empty value', () => {
  assert.equal(hasLocalizedText({ 'zh-CN': '', 'zh-TW': ' ', 'en-US': '' }), false)
  assert.equal(hasLocalizedText({ 'zh-CN': '', 'zh-TW': '繁體', 'en-US': '' }), true)
})

test('site seo readiness ignores empty localized objects', () => {
  assert.equal(
    isResellerSiteSeoConfigured({
      title: { 'zh-CN': '', 'zh-TW': '', 'en-US': '' },
      keywords: { 'zh-CN': '', 'zh-TW': '', 'en-US': '' },
      description: { 'zh-CN': '', 'zh-TW': '', 'en-US': '' },
    }),
    false,
  )
  assert.equal(
    isResellerSiteSeoConfigured({
      title: { 'zh-CN': '品牌标题' },
      keywords: {},
      description: {},
    }),
    true,
  )
})
