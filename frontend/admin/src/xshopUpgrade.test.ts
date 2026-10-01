import { createApp, nextTick, reactive, type App } from 'vue'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import Upgrade from './views/admin/XshopUpgrade.vue'
const mocks = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), auth: { isSuper: true } }))
vi.mock('@/api/client', () => ({ api: mocks }))
vi.mock('@/stores/auth', () => ({ useAdminAuthStore: () => reactive(mocks.auth) }))
let app: App; let el: HTMLElement
const envelope = (data: object) => ({ data: { status_code: 0, data } })
async function flush() { await Promise.resolve(); await Promise.resolve(); await nextTick() }
async function mount() { el = document.createElement('div'); document.body.append(el); app = createApp(Upgrade); app.mount(el); await flush() }
function button(text: string) { return [...el.querySelectorAll('button')].find(b => b.textContent?.includes(text))! }
beforeEach(() => { vi.useFakeTimers(); mocks.auth.isSuper = true; mocks.get.mockResolvedValue(envelope({ state: 'available', version: 'v2', digest: 'digest' })); mocks.post.mockResolvedValue(envelope({})); vi.spyOn(window, 'confirm').mockReturnValue(true) })
afterEach(() => { app?.unmount(); el?.remove(); vi.restoreAllMocks(); vi.clearAllMocks(); vi.useRealTimers() })
it('downloads and applies without restarting, then requires an explicit restart and refreshes the app version', async () => {
  await mount(); button('版本').click(); await flush(); expect(el.textContent).toContain('目标版本：v2')
  mocks.get.mockResolvedValue(envelope({ state: 'prepared', version: 'v2', digest: 'digest', need_restart: true, phase: 'prepared' }))
  button('下载并应用').click(); await flush(); expect(mocks.post).toHaveBeenCalledWith('/admin/xshop-upgrade/install', { digest: 'digest' }); expect(mocks.post).not.toHaveBeenCalledWith('/admin/xshop-upgrade/restart'); expect(el.textContent).toContain('需要重启')
  const listener = vi.fn(); window.addEventListener('appversionrefresh', listener)
  mocks.get.mockResolvedValue(envelope({ state: 'installed', need_restart: false })); button('立即重启').click(); await flush(); expect(mocks.post).toHaveBeenCalledWith('/admin/xshop-upgrade/restart'); expect(listener).toHaveBeenCalledTimes(1); window.removeEventListener('appversionrefresh', listener)
})
it('closes on outside buttons and Escape, rechecks revoked permissions, and disposes polling', async () => {
  await mount(); button('版本').click(); await flush()
  document.body.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })); await flush(); expect(el.querySelector('[role="dialog"]')).toBeNull()
  button('版本').click(); await flush(); const outside = document.createElement('button'); document.body.append(outside); outside.click(); await flush(); expect(el.querySelector('[role="dialog"]')).toBeNull(); outside.remove()
  button('版本').click(); await flush(); vi.mocked(window.confirm).mockImplementation(() => { reactive(mocks.auth).isSuper = false; return true }); button('下载并应用').click(); await flush(); expect(mocks.post).not.toHaveBeenCalled(); expect(el.querySelector('[role="dialog"]')).toBeNull()
  app.unmount(); const calls = mocks.get.mock.calls.length; await vi.advanceTimersByTimeAsync(10000); expect(mocks.get).toHaveBeenCalledTimes(calls)
})
it('opens a version badge panel rather than an independent upgrade page', async () => { await mount(); expect(button('版本')).toBeTruthy(); expect(el.querySelector('[role="dialog"]')).toBeNull(); button('版本').click(); await flush(); expect(el.querySelector('[role="dialog"]')).toBeTruthy(); expect(el.textContent).toContain('当前版本') })
