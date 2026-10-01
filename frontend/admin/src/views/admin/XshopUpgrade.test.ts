import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, nextTick, type App } from 'vue'
const mocks = vi.hoisted(() => ({ auth: { isSuper: true }, get: vi.fn(), post: vi.fn() }))
vi.mock('@/stores/auth', () => ({ useAdminAuthStore: () => mocks.auth }))
vi.mock('@/api/client', () => ({ api: { get: mocks.get, post: mocks.post } }))
import Panel from './XshopUpgrade.vue'
let app: App | undefined
let root: HTMLDivElement
const flush = async () => { await Promise.resolve(); await nextTick(); await Promise.resolve(); await nextTick() }
beforeEach(() => { vi.useFakeTimers(); mocks.auth.isSuper = true; mocks.get.mockReset(); mocks.post.mockReset(); mocks.get.mockResolvedValue({ data: { data: { state: 'available', version: 'xshop-preview-b1', sequence: 2, digest: 'a'.repeat(64) } } }); mocks.post.mockResolvedValue({ data: { data: { accepted: true } } }); root = document.createElement('div'); document.body.append(root) })
afterEach(() => { app?.unmount(); root.remove(); vi.restoreAllMocks(); vi.useRealTimers() })
async function mount() { app = createApp(Panel); app.mount(root); await flush(); root.querySelector<HTMLButtonElement>('[aria-label="版本与在线升级"]')?.click(); await flush() }
function installButton() { return [...root.querySelectorAll('button')].find(b => b.textContent?.includes('下载并应用'))! }
describe('XSHOP rendered upgrade actions', () => {
 it('shows Chinese target and submits only the verified digest after confirmation', async () => { await mount(); expect(root.textContent).toContain('XSHOP 在线升级'); expect(root.textContent).toContain('xshop-preview-b1'); vi.spyOn(window, 'confirm').mockReturnValue(true); installButton().click(); await flush(); expect(mocks.post).toHaveBeenCalledWith('/admin/xshop-upgrade/install', { digest: 'a'.repeat(64) }) })
 it('does not submit a cancelled install', async () => { await mount(); vi.spyOn(window, 'confirm').mockReturnValue(false); installButton().click(); await flush(); expect(mocks.post).not.toHaveBeenCalled() })
 it('rechecks permission after a rendered confirmation', async () => { await mount(); vi.spyOn(window, 'confirm').mockImplementation(() => { mocks.auth.isSuper = false; return true }); installButton().click(); await flush(); expect(mocks.post).not.toHaveBeenCalled() })
 it('hides controls and performs no helper calls for an ordinary administrator', async () => { mocks.auth.isSuper = false; await mount(); expect(root.querySelector('[role="dialog"]')).toBeNull(); expect(root.querySelectorAll('button')).toHaveLength(0); expect(mocks.get).not.toHaveBeenCalled() })
 it('clears the restart warning after a successful status refresh', async () => { mocks.get.mockRejectedValueOnce(new Error('synthetic restart')).mockRejectedValueOnce(new Error('synthetic restart')); await mount(); expect(root.textContent).toContain('读取升级状态失败'); const refresh = [...root.querySelectorAll('button')].find(b => b.textContent?.includes('刷新状态'))!; refresh.click(); await flush(); expect(root.textContent).not.toContain('读取升级状态失败'); expect(root.textContent).toContain('xshop-preview-b1') })
 it('cleans up status polling after leaving the page', async () => { await mount(); const calls = mocks.get.mock.calls.length; app?.unmount(); app = undefined; await vi.advanceTimersByTimeAsync(10000); expect(mocks.get.mock.calls.length).toBe(calls) })
})
