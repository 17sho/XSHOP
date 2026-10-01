import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, nextTick, type App } from 'vue'
import { createI18n } from 'vue-i18n'
import ApiCredentials from './ApiCredentials.vue'
import translations from '@/i18n'

const api = vi.hoisted(() => ({ list: vi.fn(), update: vi.fn(), approve: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: {
  getApiCredentials: api.list,
  updateApiCredentialStatus: api.update,
  approveApiCredential: api.approve,
} }))
vi.mock('@/utils/confirm', () => ({ confirmAction: vi.fn(async () => true) }))
vi.mock('@/utils/notify', () => ({ notifyError: vi.fn(), notifySuccess: vi.fn() }))

let app: App | undefined
let container: HTMLDivElement
let record: { id: number; user_id: number; status: string; is_active: boolean; api_key: string; api_secret_tail: string }
const flush = async () => { await new Promise(resolve => setTimeout(resolve, 0)); await nextTick() }
const button = (label: string) => Array.from(container.querySelectorAll('button')).find(b => b.textContent?.trim() === `apiCredentials.actions.${label}`)
async function mount(locale = 'zh-CN', localized = false) {
  container = document.createElement('div')
  document.body.append(container)
  app = createApp(ApiCredentials)
  app.use(createI18n({ legacy: false, locale, messages: localized ? { [locale]: translations.global.getLocaleMessage(locale) } : {}, missingWarn: false, fallbackWarn: false }))
  app.mount(container)
  await flush()
}
beforeEach(() => {
  record = { id: 31, user_id: 7, status: 'approved', is_active: true, api_key: 'fixture-api-key', api_secret_tail: '1234' }
  api.list.mockReset().mockImplementation(async () => ({ data: { data: [{ ...record }] } }))
  api.update.mockReset().mockImplementation(async (_id: number, body: { is_active: boolean }) => {
    record = { ...record, is_active: body.is_active, status: body.is_active ? 'approved' : 'disabled' }
    return { data: { data: { ...record } } }
  })
  api.approve.mockReset()
})
afterEach(() => { app?.unmount(); container?.remove() })

describe('AUTH-03 admin credential restoration', () => {
  for (const [locale, label] of [['zh-CN', '已禁用'], ['zh-TW', '已停用'], ['en-US', 'Disabled']]) {
    it(`labels administrator-disabled credentials in ${locale}`, async () => {
      record.status = 'disabled'
      record.is_active = false
      await mount(locale, true)
      expect(container.textContent).toContain(label)
      expect(container.textContent).not.toContain('apiCredentials.status.disabled')
    })
  }

  it('retains enable after a disable/list refresh and restores through the status endpoint without approval or rotation', async () => {
    await mount()
    button('disable')!.click()
    await flush()
    expect(api.update).toHaveBeenNthCalledWith(1, 31, { is_active: false })
    expect(api.list).toHaveBeenCalledTimes(2)
    expect(container.textContent).toContain('disabled')
    expect(button('enable'), 'disabled credential must retain an admin restore action after refresh').toBeDefined()
    button('enable')!.click()
    await flush()
    expect(api.update).toHaveBeenNthCalledWith(2, 31, { is_active: true })
    expect(api.list).toHaveBeenCalledTimes(3)
    expect(api.approve).not.toHaveBeenCalled()
    expect(record).toMatchObject({ status: 'approved', is_active: true, api_key: 'fixture-api-key', api_secret_tail: '1234' })
    expect(container.textContent).toContain('fixture-api-key')
    expect(button('disable')).toBeDefined()
  })

  it('preserves enabling an owner-paused approved credential', async () => {
    record.is_active = false
    await mount()
    button('enable')!.click()
    await flush()
    expect(api.update).toHaveBeenCalledExactlyOnceWith(31, { is_active: true })
    expect(api.approve).not.toHaveBeenCalled()
  })

  for (const status of ['pending_review', 'rejected', 'unknown']) {
    it(`does not offer status toggle for ${status}`, async () => {
      record.status = status
      record.is_active = false
      await mount()
      expect(button('enable')).toBeUndefined()
      expect(button('disable')).toBeUndefined()
      expect(api.update).not.toHaveBeenCalled()
    })
  }
})
