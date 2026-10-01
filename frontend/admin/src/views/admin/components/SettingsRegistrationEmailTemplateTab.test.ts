import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h, nextTick, reactive, type App } from 'vue'
import { createI18n } from 'vue-i18n'
import { createMemoryHistory, createRouter, RouterView } from 'vue-router'
import RegistrationTab from './SettingsRegistrationEmailTemplateTab.vue'

const api = vi.hoisted(() => ({ get: vi.fn(), put: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { getRegistrationEmailTemplateSettings: api.get, updateRegistrationEmailTemplateSettings: api.put } }))
vi.mock('@/utils/notify', () => ({ notifyError: vi.fn(), notifySuccess: vi.fn() }))
const fixture = () => ({ templates: Object.fromEntries(['zh-CN', 'zh-TW', 'en-US'].map(lang => [lang, {
  subject: `Registration ${lang}`, body: '{{site_name}}: {{code}} / {{expire_minutes}}', custom_html: '', custom_html_enabled: false,
}])) })
const response = () => ({ data: { data: fixture() } })
const flush = async () => { await new Promise(resolve => setTimeout(resolve, 0)); await nextTick() }
let app: App | undefined
let container: HTMLDivElement
let exposed: InstanceType<typeof RegistrationTab>
const props = reactive({ currentLang: 'zh-CN' as 'zh-CN' | 'zh-TW' | 'en-US', expireMinutes: 17, brand: { site_name: '<Example>', site_url: 'https://example.test' } })
async function mount() {
  container = document.createElement('div')
  document.body.append(container)
  const page = defineComponent({ setup: () => () => h(RegistrationTab, { ...props, ref: (value: unknown) => { exposed = value as typeof exposed } }) })
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: page }, { path: '/other', component: { template: '<p>other</p>' } }] })
  await router.push('/')
  await router.isReady()
  app = createApp({ render: () => h(RouterView) })
  app.use(router).use(createI18n({ legacy: false, locale: 'en-US', messages: {}, missingWarn: false, fallbackWarn: false }))
  app.mount(container)
  return router
}
async function type(selector: string, value: string) {
  const input = container.querySelector<HTMLInputElement | HTMLTextAreaElement>(selector)!
  input.value = value
  input.dispatchEvent(new Event('input', { bubbles: true }))
  await nextTick()
  return input
}
beforeEach(() => {
  api.get.mockReset().mockResolvedValue(response())
  api.put.mockReset().mockImplementation(async payload => ({ data: { data: JSON.parse(JSON.stringify(payload)) } }))
  props.currentLang = 'zh-CN'
})
afterEach(() => { app?.unmount(); container?.remove(); vi.restoreAllMocks() })

describe('registration email editor', () => {
  it('roundtrips backend-valid full HTML documents and safe images without blocking other-language saves', async () => {
    const data = fixture()
    const html = '<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Registration</title></head><body style="color:#123456"><img src="https://example.test/logo.png" alt="Brand"><table><tr><td>{{code}} / {{expire_minutes}}</td></tr></table></body></html>'
    data.templates['en-US']!.custom_html = html
    data.templates['en-US']!.custom_html_enabled = true
    api.get.mockResolvedValueOnce({ data: { data } })
    await mount(); await flush()
    await type('#registration-email-subject', 'Updated Chinese subject')
    await exposed.save()
    expect(api.put).toHaveBeenCalledTimes(1)
    expect(api.put.mock.calls[0]![0].templates['en-US'].custom_html).toBe(html)
    expect(container.querySelector('[data-testid="registration-email-dirty"]')).toBeNull()
  })
  it('previews the complete localized default card and immutable plain/advanced security footers', async () => {
    const data = fixture()
    data.templates['en-US']!.body = 'Your verification code is: {{code}}\n\nUse this code to finish creating your account. This code expires in {{expire_minutes}} minutes.\n\nSite: {{site_name}}\nURL: {{site_url}}'
    api.get.mockResolvedValueOnce({ data: { data } })
    props.currentLang = 'en-US'
    await mount(); await flush()
    const frame = container.querySelector<HTMLIFrameElement>('iframe')
    expect(frame).not.toBeNull()
    expect(frame!.getAttribute('sandbox')).toBe('')
    const doc = new DOMParser().parseFromString(frame!.srcdoc, 'text/html')
    expect(doc.querySelector('h1')?.textContent).toBe('Email verification code')
    expect(doc.body.textContent).toContain('Use this code to finish creating your account.')
    expect(doc.body.textContent).not.toContain('Your verification code is:')
    expect(doc.body.textContent).toContain('This code expires in 17 minutes.')
    expect(doc.body.textContent).toContain('Shop with confidence')
    expect(doc.body.textContent).toContain('Do not share this code with anyone.')
    expect(doc.body.textContent).toContain('This is an automated email. We will never ask for your password, payment details, or verification code.')
    expect(container.textContent).toContain('Do not share this code with anyone.')
    await type('#registration-email-body', 'Edited body {{code}} / {{expire_minutes}}')
    expect(frame!.srcdoc).toContain('Edited body 000000 / 17')
    for (const [lang, warning] of [['zh-CN', '请勿向任何人透露验证码。'], ['zh-TW', '請勿向任何人透露驗證碼。'], ['en-US', 'Do not share this code with anyone.']] as const) {
      props.currentLang = lang; await nextTick()
      expect(container.textContent).toContain(warning)
      await type('#registration-email-html', '<html style="display:none"><head><title>Hidden head</title></head><body style="display:none"><table><tr><td>{{code}} {{expire_minutes}}')
      container.querySelector<HTMLInputElement>('#registration-email-html-enabled')!.click(); await nextTick()
      const preview = new DOMParser().parseFromString(frame!.srcdoc, 'text/html')
      expect(preview.body.textContent).toContain('000000 17')
      expect(preview.body.lastElementChild?.textContent).toContain(warning)
      expect(preview.body.lastElementChild?.tagName).toBe('P')
      expect(preview.body.getAttribute('style')).toBeNull()
      expect(preview.documentElement.getAttribute('style')).toBeNull()
      expect(preview.querySelector('title')?.textContent).not.toBe('Hidden head')
    }
  })
  it('uses the canonical save response for revert, while preserving later edits and failed saves', async () => {
    await mount(); await flush()
    await type('#registration-email-subject', '  canonical  ')
    let finish!: (value: unknown) => void
    api.put.mockReturnValueOnce(new Promise(resolve => { finish = resolve }))
    const saving = exposed.save()
    await exposed.save()
    expect(api.put).toHaveBeenCalledTimes(1)
    await type('#registration-email-subject', 'newer edit')
    const canonical = fixture()
    canonical.templates['zh-CN']!.subject = 'canonical'
    finish({ data: { data: canonical } }); await saving; await nextTick()
    expect(container.querySelector<HTMLInputElement>('#registration-email-subject')!.value).toBe('newer edit')
    container.querySelector<HTMLButtonElement>('[data-testid="registration-email-revert"]')!.click()
    await nextTick()
    expect(container.querySelector<HTMLInputElement>('#registration-email-subject')!.value).toBe('canonical')
    await type('#registration-email-subject', 'failed edit')
    api.put.mockRejectedValueOnce(new Error('offline'))
    await exposed.save(); await nextTick()
    expect(exposed.submitting).toBe(false)
    expect(container.querySelector('[data-testid="registration-email-dirty"]')).not.toBeNull()
    container.querySelector<HTMLButtonElement>('[data-testid="registration-email-revert"]')!.click()
    await nextTick()
    expect(container.querySelector<HTMLInputElement>('#registration-email-subject')!.value).toBe('canonical')
  })
  it('inserts tokens at subject/body/advanced cursor, shows synthetic preview and preserves languages', async () => {
    await mount(); await flush()
    for (const selector of ['#registration-email-subject', '#registration-email-body']) {
      const input = await type(selector, 'before AFTER')
      input.focus(); input.setSelectionRange(7, 12)
      container.querySelector<HTMLButtonElement>('[data-variable="code"]')!.click()
      await nextTick()
      expect(input.value).toBe('before {{code}}')
    }
    const advanced = container.querySelector<HTMLDetailsElement>('details')!
    expect(advanced.open).toBe(false)
    advanced.open = true
    const html = await type('#registration-email-html', '<p>{{site_name}} CODE {{expire_minutes}}</p>')
    html.focus(); html.setSelectionRange(html.value.indexOf('CODE'), html.value.indexOf('CODE') + 'CODE'.length)
    container.querySelector<HTMLButtonElement>('[data-variable="code"]')!.click()
    await nextTick()
    expect(html.value).toBe('<p>{{site_name}} {{code}} {{expire_minutes}}</p>')
    await type('#registration-email-html', '<p>{{site_name}} {{code}} {{expire_minutes}}</p>')
    container.querySelector<HTMLInputElement>('#registration-email-html-enabled')!.click()
    await nextTick()
    const frame = container.querySelector<HTMLIFrameElement>('iframe')!
    expect(frame.getAttribute('sandbox')).toBe('')
    expect(frame.srcdoc).toContain('&lt;Example&gt; 000000 17')
    expect(frame.srcdoc).toContain('Content-Security-Policy')
    expect(container.querySelector('img')).toBeNull()
    props.currentLang = 'en-US'; await nextTick()
    expect(container.querySelector<HTMLInputElement>('#registration-email-subject')!.value).toBe('Registration en-US')
    props.currentLang = 'zh-CN'; await nextTick()
    expect(container.querySelector<HTMLInputElement>('#registration-email-subject')!.value).toBe('before {{code}}')
  })
  it('protects unload and real SPA navigation; save snapshots retain edits made in flight and revert only current language', async () => {
    const router = await mount(); await flush()
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    await type('#registration-email-subject', 'sent snapshot')
    const before = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(before)
    expect(before.defaultPrevented).toBe(true)
    await router.push('/other')
    expect(router.currentRoute.value.path).toBe('/')
    expect(confirm).toHaveBeenCalled()
    let finish!: (value: unknown) => void
    api.put.mockReturnValueOnce(new Promise(resolve => { finish = resolve }))
    const saving = exposed.save()
    await type('#registration-email-subject', 'later edit')
    finish({ data: { data: api.put.mock.calls[0]![0] } }); await saving; await nextTick()
    expect(container.querySelector('[data-testid="registration-email-dirty"]')).not.toBeNull()
    props.currentLang = 'en-US'; await nextTick()
    await type('#registration-email-subject', 'English edit')
    props.currentLang = 'zh-CN'; await nextTick()
    container.querySelector<HTMLButtonElement>('[data-testid="registration-email-revert"]')!.click()
    await nextTick()
    expect(container.querySelector<HTMLInputElement>('#registration-email-subject')!.value).toBe('sent snapshot')
    props.currentLang = 'en-US'; await nextTick()
    expect(container.querySelector<HTMLInputElement>('#registration-email-subject')!.value).toBe('English edit')
    container.querySelector<HTMLButtonElement>('[data-testid="registration-email-revert"]')!.click()
    await nextTick()
    const clean = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(clean)
    expect(clean.defaultPrevented).toBe(false)
    await router.push('/other')
    expect(router.currentRoute.value.path).toBe('/other')
  })
  it('rejects invalid all-language fields and malformed loads without writes', async () => {
    api.get.mockResolvedValueOnce({ data: { data: { templates: {} } } })
    await mount(); await flush()
    expect(exposed.loadFailed).toBe(true)
    await exposed.save()
    expect(api.put).not.toHaveBeenCalled()
    container.querySelector<HTMLButtonElement>('[data-testid="registration-email-retry"]')!.click()
    await flush()
    await type('#registration-email-body', '{{code}} missing expiry')
    props.currentLang = 'en-US'; await nextTick()
    await exposed.save()
    expect(api.put).not.toHaveBeenCalled()
    expect(container.querySelector('[role="alert"]')!.textContent).toContain('zh-CN')
  })
  it('fails closed during loading and failure, then retry loads authoritative templates', async () => {
    let reject!: (error: Error) => void
    api.get.mockReturnValueOnce(new Promise((_, fail) => { reject = fail }))
    await mount()
    expect(exposed.loaded).toBe(false)
    await exposed.save()
    expect(api.put).not.toHaveBeenCalled()
    reject(new Error('offline'))
    await flush()
    expect(exposed.loadFailed).toBe(true)
    expect(container.querySelector('#registration-email-subject')).toBeNull()
    await exposed.save()
    expect(api.put).not.toHaveBeenCalled()
    container.querySelector<HTMLButtonElement>('[data-testid="registration-email-retry"]')!.click()
    await flush()
    expect(exposed.loaded).toBe(true)
    expect(exposed.loadFailed).toBe(false)
    expect(container.querySelector<HTMLInputElement>('#registration-email-subject')!.value).toBe('Registration zh-CN')
    await type('#registration-email-subject', 'Saved subject')
    await exposed.save()
    expect(api.put).toHaveBeenCalledWith(expect.objectContaining({ templates: expect.objectContaining({ 'zh-CN': expect.objectContaining({ subject: 'Saved subject' }) }) }))
  })
})
