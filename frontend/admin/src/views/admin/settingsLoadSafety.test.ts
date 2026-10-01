import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, nextTick, type App } from 'vue'
import { createI18n } from 'vue-i18n'
import Settings from './Settings.vue'
import Navigation from './components/SettingsNavigationTab.vue'
import ResellerConfigs from './ResellerSiteConfigs.vue'
import Announcement from './components/SettingsHomeAnnouncementTab.vue'
import Ad from './components/SettingsHomepageAdTab.vue'
import Upstream from './components/SettingsUpstreamSyncTab.vue'

const api = vi.hoisted(() => ({ get: vi.fn(), put: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: new Proxy({}, { get: (_target, name: string) => name.startsWith('get') ? (...args: any[]) => api.get(name, ...args) : (...args: any[]) => api.put(name, ...args) }) }))
vi.mock('vue-router', async (importOriginal) => ({ ...await importOriginal<typeof import('vue-router')>(), useRoute: () => ({ query: {} }) }))
vi.mock('@/utils/notify', () => ({ notifyError: vi.fn(), notifySuccess: vi.fn() }))
vi.mock('@/components/RichEditor.vue', () => ({ default: { template: '<div />' } }))
vi.mock('@/components/admin/MediaPicker.vue', () => ({ default: { template: '<div />' } }))
const flush = async () => { await new Promise(resolve => setTimeout(resolve, 0)); await nextTick() }
let app: App | undefined
let container: HTMLDivElement
function mount(component: any = Settings, props: any = {}) {
  container = document.createElement('div'); document.body.append(container)
  app = createApp(component, props)
  app.use(createI18n({ legacy: false, locale: 'en-US', messages: {}, missingWarn: false, fallbackWarn: false }))
  const vm = app.mount(container)
  return (vm as any).$.setupState
}
beforeEach(() => {
  api.get.mockReset().mockImplementation(async (_name, payload) => ({ data: { data: payload?.key === 'site_config' ? { brand: { site_name: 'PERSISTED' }, storefront_template: 'vault', product_catalog_layout: 'list' } : {} } }))
  api.put.mockReset().mockResolvedValue({ data: { data: {} } })
})
afterEach(() => { app?.unmount(); container?.remove() })

describe('reseller navigation preservation', () => {
  for (const home of [false, true, undefined]) it(`admin reseller edit preserves homepage=${home} and custom links`, async () => {
    const nav = { builtin: { notice: false }, homepage_notice_enabled: home, custom_items: [{ name: { 'zh-CN': '', 'zh-TW': '', 'en-US': 'Help' }, url: 'https://example.invalid/help' }] }
    api.get.mockImplementation(async (method) => ({ data: { data: method === 'getResellerSiteConfigs' ? [] : { reseller_id: 7, nav_config: nav } } }))
    api.put.mockImplementation(async (_method, _id, value) => ({ data: { data: { reseller_id: 7, ...value } } }))
    const s = mount(ResellerConfigs); await flush()
    await s.openEditor({ reseller_id: 7 }); await flush()
    await s.saveConfig()
    expect(api.put).toHaveBeenCalledWith('updateResellerSiteConfig', 7, expect.objectContaining({ nav_config: { ...nav, homepage_notice_enabled: home !== false } }))
    expect(s.form.nav_config.homepage_notice_enabled).toBe(home !== false)
  })
})

describe('homepage notice switches', () => {
  for (const nav of [false, true]) for (const home of [false, true]) {
    it(`loads/saves independent nav=${nav} homepage=${home} and preserves custom links`, async () => {
      const link = { id: 42, title: { 'zh-CN': '帮助', 'zh-TW': '幫助', 'en-US': 'Help' }, url: 'https://example.invalid/help', link_type: 'external', target: '_blank', sort_order: 3, enabled: true, icon: 'book' }
      api.get.mockResolvedValue({ data: { data: { builtin: { notice: nav }, homepage_notice_enabled: home, custom_items: [link] } } })
      const s = mount(Navigation, { currentLang: 'en-US' }); await flush()
      expect(container.querySelector('#nav-notice')?.getAttribute('aria-checked')).toBe(String(nav))
      expect(container.querySelector('#homepage-notice')?.getAttribute('aria-checked')).toBe(String(home))
      await s.save()
      expect(api.put).toHaveBeenCalledWith('updateSettings', { key: 'nav_config', value: { builtin: { notice: nav }, homepage_notice_enabled: home, custom_items: [link] } })
      ;(container.querySelector('#homepage-notice') as HTMLElement).click(); await flush()
      await s.save()
      expect(api.put).toHaveBeenLastCalledWith('updateSettings', expect.objectContaining({ value: expect.objectContaining({ builtin: { notice: nav }, homepage_notice_enabled: !home }) }))
      const saved = api.put.mock.calls[api.put.mock.calls.length - 1]![1].value
      api.get.mockResolvedValue({ data: { data: saved } })
      await s.fetchNavConfig(); await flush()
      expect(container.querySelector('#homepage-notice')?.getAttribute('aria-checked')).toBe(String(!home))
      expect(container.querySelector('#nav-notice')?.getAttribute('aria-checked')).toBe(String(nav))
    })
  }
  it('global Save remains disabled while navigation is pending or failed and retry preserves explicit false', async () => {
    let reject!: (error: Error) => void
    api.get.mockImplementation(async (method, payload) => payload?.key === 'nav_config'
      ? new Promise((_, fail) => { reject = fail }) : { data: { data: method === 'getOrderEmailTemplateSettings' ? {
        templates: Object.fromEntries(['default', 'paid', 'delivered', 'delivered_with_content', 'refunded', 'partially_refunded'].map(scene => [scene, Object.fromEntries(['zh-CN', 'zh-TW', 'en-US'].map(lang => [lang, { subject: 'Order', body: '{{order_no}}', custom_html: '', custom_html_enabled: false }]))])),
        guest_tip: { 'zh-CN': '', 'zh-TW': '', 'en-US': '' },
      } : {} } })
    const s = mount(); s.currentTab = 'navigation'; await flush()
    const saveButton = () => Array.from(container.querySelectorAll('button')).find(b => b.textContent?.trim() === 'admin.settings.actions.save')!
    expect(saveButton().disabled).toBe(true)
    await s.saveSettings(); expect(api.put).not.toHaveBeenCalled()
    reject(new Error('offline')); await flush()
    expect(saveButton().disabled).toBe(true)
    await s.saveSettings(); expect(api.put).not.toHaveBeenCalled()
    api.get.mockResolvedValue({ data: { data: { builtin: { notice: true }, homepage_notice_enabled: false, custom_items: [] } } })
    const retry = Array.from(container.querySelectorAll('button')).find(b => b.textContent?.trim() === 'admin.common.retry')!
    retry.click(); await flush()
    expect(saveButton().disabled).toBe(false)
    saveButton().click(); await flush()
    expect(api.put).toHaveBeenCalledWith('updateSettings', expect.objectContaining({ key: 'nav_config', value: expect.objectContaining({ homepage_notice_enabled: false, builtin: { notice: true } }) }))
  })
  it('missing homepage field defaults true and can save explicit false', async () => {
    const s = mount(Navigation, { currentLang: 'en-US' }); await flush()
    expect(container.querySelector('#homepage-notice')?.getAttribute('aria-checked')).toBe('true')
    ;(container.querySelector('#homepage-notice') as HTMLElement).click(); await flush(); await s.save()
    expect(api.put).toHaveBeenLastCalledWith('updateSettings', expect.objectContaining({ value: expect.objectContaining({ homepage_notice_enabled: false }) }))
  })
})

describe('FE01 authoritative settings ownership', () => {
  it('hydrates and saves basic settings while an unrelated SMTP request is still pending', async () => {
    api.get.mockImplementation(async (name, payload) => name === 'getSMTPSettings' ? new Promise(() => {}) : ({ data: { data: payload?.key === 'site_config' ? { brand: { site_name: 'PERSISTED' }, storefront_template: 'vault', product_catalog_layout: 'list' } : {} } }))
    const s = mount(); await flush()
    expect(s.form.brand.site_name).toBe('PERSISTED')
    await s.saveSettings()
    expect(api.put).toHaveBeenCalledWith('updateSettings', expect.objectContaining({ key: 'site_config', value: expect.objectContaining({ storefront_template: 'vault', product_catalog_layout: 'list' }) }))
  })
  for (const tab of ['basic', 'template', 'legal', 'telegram', 'google', 'github', 'dashboard', 'smtp', 'captcha', 'email_templates']) {
    it(`${tab} cannot save after a failed load`, async () => {
      api.get.mockRejectedValue(new Error('offline'))
      const s = mount(); s.currentTab = tab; await flush()
      await s.saveSettings(); expect(api.put).not.toHaveBeenCalled()
    })
  }
  it('basic save is atomic with respect to load authority and retry retains independent edits', async () => {
    api.get.mockImplementation(async (_name, payload) => {
      if (payload?.key === 'registration_config') throw new Error('offline')
      return { data: { data: payload?.key === 'site_config' ? { brand: { site_name: 'PERSISTED' } } : {} } }
    })
    const s = mount(); await flush(); s.form.brand.site_name = 'UNSAVED EDIT'
    await s.saveSettings(); expect(api.put).not.toHaveBeenCalled()
    api.get.mockResolvedValue({ data: { data: {} } })
    await s.fetchSettings(); await flush()
    expect(s.form.brand.site_name).toBe('UNSAVED EDIT')
    await s.saveSettings(); expect(api.put).toHaveBeenCalledTimes(3)
  })
  for (const [name, component, load] of [['navigation', Navigation, 'fetchNavConfig'], ['announcement', Announcement, 'fetchAnnouncement'], ['ad', Ad, 'fetchAd'], ['upstream', Upstream, 'loadConfig']] as const) {
    it(`${name} save is blocked pending/failure; retry enables authoritative save`, async () => {
      let fail!: (error: Error) => void
      api.get.mockReturnValueOnce(new Promise((_, reject) => { fail = reject }))
      const s = mount(component, { currentLang: 'en-US' })
      await s.save(); expect(api.put).not.toHaveBeenCalled()
      fail(new Error('offline')); await flush()
      await s.save(); expect(api.put).not.toHaveBeenCalled()
      await s[load](); await s.save(); expect(api.put).toHaveBeenCalledTimes(1)
    })
  }
})
