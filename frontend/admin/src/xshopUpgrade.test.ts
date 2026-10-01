import { createApp, nextTick, reactive, type App } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import { useAdminAuthStore } from './stores/auth'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import Upgrade from './views/admin/XshopUpgrade.vue'
const mocks = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), auth: { isSuper: true, token: 'synthetic-A' } }))
vi.mock('@/api/client', () => ({ api: mocks }))
let pinia: ReturnType<typeof createPinia>
let app: App; let el: HTMLElement
const envelope = (data: object) => ({ data: { status_code: 0, data } })
async function flush() { await Promise.resolve(); await Promise.resolve(); await nextTick() }
async function mount() { el = document.createElement('div'); document.body.append(el); app = createApp(Upgrade); app.use(pinia); app.mount(el); await flush() }
function button(text: string) { if (text === '版本') return el.querySelector<HTMLButtonElement>('[aria-label="版本与在线升级"]')!; return [...el.querySelectorAll('button')].find(b => b.textContent?.includes(text))! }
beforeEach(() => { vi.useFakeTimers(); localStorage.clear(); pinia = createPinia(); setActivePinia(pinia); mocks.auth = useAdminAuthStore(); mocks.auth.token = 'synthetic-A'; mocks.auth.isSuper = true; mocks.get.mockResolvedValue(envelope({ state: 'available', version: 'v2', digest: 'digest' })); mocks.post.mockResolvedValue(envelope({})); vi.spyOn(window, 'confirm').mockReturnValue(true) })
afterEach(() => { app?.unmount(); el?.remove(); vi.restoreAllMocks(); vi.clearAllMocks(); vi.useRealTimers() })
it('renders an up-to-date phase as completed without suggesting pending confirmation', async () => {
 mocks.get.mockResolvedValue(envelope({ state: 'up_to_date', phase: 'up_to_date', current_version: 'xshop-preview-b3', version: 'xshop-preview-b3', sequence: 6, rollback_available: true, previous_version: 'xshop-preview-a3' })); await mount(); button('版本').click(); await flush();
 expect(el.textContent).toContain('状态：已是最新版本'); expect(el.textContent).toContain('阶段：检查完成，已是最新版本'); expect(el.textContent).not.toContain('等待状态确认'); expect(el.querySelector('[role="progressbar"]')).toBeNull(); expect(button('下载并应用').disabled).toBe(true); expect(button('回退程序').disabled).toBe(false); expect(mocks.post).not.toHaveBeenCalled()
})
it('uses a compact icon without version text in the header', async () => {
  await mount(); const trigger = el.querySelector<HTMLButtonElement>('[aria-label="版本与在线升级"]')!;
  expect(trigger.textContent?.trim()).toBe(''); expect(trigger.getAttribute('title')).toBe('版本与在线升级'); expect(trigger.querySelector('svg')).not.toBeNull()
})
it('keeps fast preparation feedback visible without delaying API completion or enabling duplicate submission', async () => {
 await mount(); button('版本').click(); await flush(); mocks.get.mockResolvedValue(envelope({ state: 'prepared', need_restart: true, phase: 'prepared' })); button('下载并应用').click(); await flush();
 expect(el.textContent).toContain('下载与验证已完成'); expect(button('立即重启').disabled).toBe(true); await vi.advanceTimersByTimeAsync(450); expect(el.textContent).not.toContain('下载与验证已完成'); expect(button('立即重启').disabled).toBe(false); expect(el.textContent).not.toContain('prepared')
})
it('reconciles a lost restart reply and temporary 502 only within a bounded explicit operation', async () => {
 mocks.get.mockResolvedValue(envelope({ state: 'prepared', version: 'v2', current_version: 'v1', need_restart: true, digest: 'digest' })); await mount(); button('版本').click(); await flush();
 mocks.post.mockRejectedValueOnce({ status: 502 }); mocks.get.mockRejectedValueOnce({ status: 503 }); button('立即重启').click(); await flush();
 expect(mocks.post).toHaveBeenCalledWith('/admin/xshop-upgrade/restart', undefined, expect.objectContaining({ expectedRestartUntil: expect.any(Number) })); expect(el.textContent).toContain('等待服务恢复'); expect(el.textContent).not.toContain('操作失败');
 mocks.get.mockResolvedValue(envelope({ state: 'installed', current_version: 'v2', need_restart: false })); await vi.advanceTimersByTimeAsync(2500); expect(el.textContent).toContain('升级完成'); expect(el.textContent).not.toContain('等待服务恢复')
})
it('expires restart uncertainty instead of inventing success', async () => {
 mocks.get.mockResolvedValue(envelope({ state: 'prepared', need_restart: true })); await mount(); button('版本').click(); await flush(); mocks.post.mockRejectedValue({ status: 502 }); mocks.get.mockRejectedValue({ status: 502 }); button('立即重启').click(); await flush(); await vi.advanceTimersByTimeAsync(122500); expect(el.textContent).toContain('确认超时'); expect(el.textContent).not.toContain('升级完成')
})
it('offers only explicitly available binary rollback and rechecks confirmation authority', async () => {
 mocks.get.mockResolvedValue(envelope({ state: 'installed', rollback_available: true, previous_version: 'v1', current_version: 'v2' })); await mount(); button('版本').click(); await flush(); expect(el.textContent).toContain('上一版本：v1');
 vi.mocked(window.confirm).mockReturnValue(false); button('回退程序').click(); await flush(); expect(mocks.post).not.toHaveBeenCalled();
 vi.mocked(window.confirm).mockReturnValue(true); mocks.get.mockResolvedValue(envelope({ state: 'rolled_back', current_version: 'v1', rollback_available: false })); button('回退程序').click(); await flush(); expect(window.confirm).toHaveBeenCalledWith('仅回退程序，不回滚数据库；会短暂重启'); expect(mocks.post).toHaveBeenCalledWith('/admin/xshop-upgrade/rollback', undefined, expect.any(Object)); expect(el.textContent).toContain('程序回退已确认')
})
it('disables unknown rollback capability', async () => { await mount(); button('版本').click(); await flush(); expect(button('回退程序').disabled).toBe(true) })
it('fences session ABA pending status and revokes unexpected auth failures', async () => {
 await mount(); button('版本').click(); await flush(); let resolve!: (v: any) => void; mocks.get.mockImplementationOnce(() => new Promise(r => { resolve = r })); button('刷新状态').click(); await flush(); const auth = reactive(mocks.auth); auth.isSuper = false; auth.isSuper = true; resolve(envelope({ state: 'available', version: 'stale' })); await flush(); expect(el.textContent).not.toContain('stale'); if (!el.querySelector('[role="dialog"]')) { button('版本').click(); await flush() }
 mocks.get.mockRejectedValueOnce({ status: 401 }); button('刷新状态').click(); await flush(); expect(el.querySelector('[aria-label="版本与在线升级"]')).toBeNull()
})
it('bounds the panel to client and visual viewport width and exposes pending step feedback', async () => {
 Object.defineProperty(document.documentElement, 'clientWidth', { configurable: true, value: 320 }); Object.defineProperty(window, 'visualViewport', { configurable: true, value: { width: 320, offsetLeft: 0, addEventListener: vi.fn(), removeEventListener: vi.fn() } }); mocks.get.mockResolvedValue(envelope({ state: 'installing', phase: 'verifying' })); await mount(); button('版本').click(); await flush(); const panel = el.querySelector<HTMLElement>('[role="dialog"]')!; expect(parseFloat(panel.style.maxWidth)).toBeLessThanOrEqual(296); expect(panel.querySelector('[role="progressbar"]')).not.toBeNull(); expect(el.textContent).toContain('签名与哈希验证中'); expect(panel.querySelector('[role="progressbar"]')?.getAttribute('aria-valuenow')).toBeNull()
})
it('revokes on restart authentication failure without reconciling it as success', async () => { mocks.get.mockResolvedValue(envelope({ state: 'prepared', need_restart: true })); await mount(); button('版本').click(); await flush(); const calls = mocks.get.mock.calls.length; mocks.post.mockRejectedValueOnce({ status: 401 }); button('立即重启').click(); await flush(); expect(el.querySelector('[aria-label="版本与在线升级"]')).toBeNull(); expect(mocks.get).toHaveBeenCalledTimes(calls) })
it('rejects retained leaving-panel actions after close', async () => { await mount(); button('版本').click(); await flush(); const install = button('下载并应用'); el.querySelector<HTMLButtonElement>('[aria-label="关闭升级面板"]')!.click(); await flush(); install.click(); await flush(); expect(mocks.post).not.toHaveBeenCalled() })
it('downloads and applies without restarting, then requires an explicit restart and refreshes the app version', async () => {
  await mount(); button('版本').click(); await flush(); expect(el.textContent).toContain('目标版本：v2')
  mocks.get.mockResolvedValue(envelope({ state: 'prepared', version: 'v2', digest: 'digest', need_restart: true, phase: 'prepared' }))
  button('下载并应用').click(); await flush(); expect(mocks.post).toHaveBeenCalledWith('/admin/xshop-upgrade/install', { digest: 'digest' }); expect(mocks.post).not.toHaveBeenCalledWith('/admin/xshop-upgrade/restart'); expect(el.textContent).toContain('需要重启')
  const listener = vi.fn(); window.addEventListener('appversionrefresh', listener)
  await vi.advanceTimersByTimeAsync(450); mocks.get.mockResolvedValue(envelope({ state: 'installed', current_version: 'v2', need_restart: false })); button('立即重启').click(); await flush(); expect(mocks.post).toHaveBeenCalledWith('/admin/xshop-upgrade/restart', undefined, expect.any(Object)); expect(listener).toHaveBeenCalledTimes(1); window.removeEventListener('appversionrefresh', listener)
})
it('closes on outside buttons and Escape, rechecks revoked permissions, and disposes polling', async () => {
  await mount(); button('版本').click(); await flush()
  document.body.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })); await flush(); await vi.advanceTimersByTimeAsync(250); expect(el.querySelector('[role="dialog"]')).toBeNull()
  button('版本').click(); await flush(); const outside = document.createElement('button'); document.body.append(outside); outside.click(); await flush(); await vi.advanceTimersByTimeAsync(250); expect(el.querySelector('[role="dialog"]')).toBeNull(); outside.remove()
  button('版本').click(); await flush(); vi.mocked(window.confirm).mockImplementation(() => { reactive(mocks.auth).isSuper = false; return true }); button('下载并应用').click(); await flush(); expect(mocks.post).not.toHaveBeenCalled(); await vi.advanceTimersByTimeAsync(250); expect(el.querySelector('[role="dialog"]')).toBeNull()
  app.unmount(); const calls = mocks.get.mock.calls.length; await vi.advanceTimersByTimeAsync(10000); expect(mocks.get).toHaveBeenCalledTimes(calls)
})
it('opens a version badge panel rather than an independent upgrade page', async () => { await mount(); expect(button('版本')).toBeTruthy(); expect(el.querySelector('[role="dialog"]')).toBeNull(); button('版本').click(); await flush(); expect(el.querySelector('[role="dialog"]')).toBeTruthy(); expect(el.textContent).toContain('当前版本') })

const oldInstalled = { state: 'installed', current_version: 'v2', previous_version: 'v1', rollback_available: true }
it('F1 retains lost rollback uncertainty through unchanged installed polls and deadline; only readonly refresh continues', async () => {
 mocks.get.mockResolvedValue(envelope(oldInstalled)); await mount(); button('版本').click(); await flush(); mocks.post.mockRejectedValue({ status: 0 }); button('回退程序').click(); await flush();
 expect(el.textContent).toContain('等待服务恢复与运行核验'); expect(button('回退程序').disabled).toBe(true);
 await vi.advanceTimersByTimeAsync(125000); expect(el.textContent).toContain('确认超时'); expect(button('回退程序').disabled).toBe(true); expect(button('检查更新').disabled).toBe(true); button('回退程序').click(); expect(mocks.post).toHaveBeenCalledTimes(1);
 const n = mocks.get.mock.calls.length; button('刷新状态').click(); await flush(); expect(mocks.get).toHaveBeenCalledTimes(n + 1); expect(mocks.get.mock.lastCall?.[1]).toBeUndefined();
 mocks.get.mockResolvedValue(envelope({ state: 'rolled_back', current_version: 'v1', rollback_available: false })); button('刷新状态').click(); await flush(); expect(el.textContent).toContain('程序回退已确认'); expect(el.textContent).not.toContain('等待服务恢复与运行核验');
})
it.each([{state:'rolled_back',current_version:'wrong',rollback_available:false},{state:'rolled_back',current_version:'v1',rollback_available:true},{state:'installed',current_version:'v1',rollback_available:false}])('F1 rejects mismatched rollback terminal evidence %j', async terminal => {
 mocks.get.mockResolvedValue(envelope(oldInstalled)); await mount(); button('版本').click(); await flush(); mocks.post.mockRejectedValue({status:0}); mocks.get.mockResolvedValue(envelope(terminal)); button('回退程序').click(); await flush(); expect(el.textContent).toContain('等待服务恢复与运行核验'); expect(button('回退程序').disabled).toBe(true); expect(mocks.post).toHaveBeenCalledTimes(1);
})
it('F1 binds restart completion to prepared target, not unchanged old installed identity', async () => {
 mocks.get.mockResolvedValue(envelope({state:'prepared',version:'v3',digest:'new',current_version:'v2',need_restart:true})); await mount(); button('版本').click(); await flush(); mocks.post.mockRejectedValue({status:0}); mocks.get.mockResolvedValue(envelope(oldInstalled)); button('立即重启').click(); await flush(); expect(el.textContent).toContain('等待服务恢复与运行核验'); expect(button('回退程序').disabled).toBe(true);
 mocks.get.mockResolvedValue(envelope({state:'installed',current_version:'v3',need_restart:false})); button('刷新状态').click(); await flush(); expect(el.textContent).toContain('升级完成'); expect(el.textContent).not.toContain('等待服务恢复与运行核验');
})
it('F1 renders backend failure honestly but keeps unidentified failure write-fenced', async () => {
 mocks.get.mockResolvedValue(envelope(oldInstalled)); await mount(); button('版本').click(); await flush(); mocks.post.mockRejectedValue({status:0}); mocks.get.mockResolvedValue(envelope({state:'failed',message:'需要服务器管理员恢复'})); button('回退程序').click(); await flush(); expect(el.textContent).toContain('操作失败'); expect(el.textContent).toContain('需要服务器管理员恢复'); expect(button('回退程序').disabled).toBe(true); expect(button('检查更新').disabled).toBe(true);
})
it('F1 renders recovered restart failure without inventing requested-target success', async () => {
 mocks.get.mockResolvedValue(envelope({state:'prepared',version:'v3',digest:'new',need_restart:true})); await mount(); button('版本').click(); await flush(); mocks.post.mockRejectedValue({status:0}); mocks.get.mockResolvedValue(envelope({state:'failed',current_version:'v2',rollback_available:false,message:'安装失败，旧版本已恢复'})); button('立即重启').click(); await flush(); expect(el.textContent).toContain('操作失败'); expect(el.textContent).toContain('旧版本已恢复'); expect(el.textContent).not.toContain('升级完成'); expect(button('检查更新').disabled).toBe(true);
})
it('F1 actual Pinia token ABA and disposal reject rollback continuation without reads or events', async () => {
 mocks.get.mockResolvedValue(envelope(oldInstalled)); await mount(); button('版本').click(); await flush(); let done!: (x: any) => void; mocks.post.mockImplementationOnce(() => new Promise(r => done=r)); const event = vi.fn(); window.addEventListener('appversionrefresh',event); button('回退程序').click(); const n=mocks.get.mock.calls.length; mocks.auth.token='synthetic-B'; mocks.auth.token='synthetic-A'; done(envelope({})); await flush(); expect(mocks.get).toHaveBeenCalledTimes(n); expect(event).not.toHaveBeenCalled();
 await vi.advanceTimersByTimeAsync(250); button('版本').click(); await flush(); mocks.post.mockImplementationOnce(() => new Promise(r => done=r)); button('回退程序').click(); app.unmount(); const n2=mocks.get.mock.calls.length; done(envelope({})); await flush(); await vi.advanceTimersByTimeAsync(5000); expect(mocks.get).toHaveBeenCalledTimes(n2); expect(event).not.toHaveBeenCalled(); window.removeEventListener('appversionrefresh',event);
})

it('F1 shows authoritative restored old process after restart while keeping unresolved target fenced', async () => {
 mocks.get.mockResolvedValue(envelope({state:'prepared',version:'v3',current_version:'v2',digest:'new',need_restart:true})); await mount(); button('版本').click(); await flush(); mocks.post.mockRejectedValue({status:0}); mocks.get.mockResolvedValue(envelope({state:'rolled_back',current_version:'v2',rollback_available:false,message:'旧程序已恢复；失败序号已禁用'})); button('立即重启').click(); await flush(); expect(el.textContent).toContain('旧程序已恢复；目标版本未确认'); expect(el.textContent).not.toContain('升级完成'); expect(button('检查更新').disabled).toBe(true);
})
it('F1 same starting/target version cannot prove a new restart from a terminal label alone', async () => {
 mocks.get.mockResolvedValue(envelope({state:'prepared',version:'v2',current_version:'v2',digest:'new',need_restart:true})); await mount(); button('版本').click(); await flush(); mocks.post.mockRejectedValue({status:0}); mocks.get.mockResolvedValue(envelope({state:'installed',current_version:'v2',need_restart:false})); button('立即重启').click(); await flush(); expect(el.textContent).toContain('等待服务恢复与运行核验'); expect(button('检查更新').disabled).toBe(true);
})
