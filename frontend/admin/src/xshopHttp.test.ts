import { afterEach, expect, it, vi } from 'vitest'
const m = vi.hoisted(() => ({ notify: vi.fn() }))
vi.mock('@/utils/notify', () => ({ notifyError: m.notify }))
vi.mock('@/i18n', () => ({ default: { global: { t: (k: string) => k, locale: { value: 'zh-CN' } } } }))
import { api } from './api/client'
afterEach(() => { vi.unstubAllGlobals(); vi.clearAllMocks() })
it('quietly rejects expected bounded restart gateways while preserving status and normal notifications', async () => {
 vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('gateway', { status: 502 })))
 await expect(api.get('/admin/xshop-upgrade/status', { expectedRestartUntil: Date.now() + 1000 })).rejects.toMatchObject({ status: 502 }); expect(m.notify).not.toHaveBeenCalled()
 await expect(api.get('/admin/xshop-upgrade/status')).rejects.toMatchObject({ status: 502 }); expect(m.notify).toHaveBeenCalledTimes(1)
 await expect(api.get('/admin/settings', { expectedRestartUntil: Date.now() + 1000 })).rejects.toMatchObject({ status: 502 }); expect(m.notify).toHaveBeenCalledTimes(2)
 await expect(api.get('/admin/xshop-upgrade/status', { expectedRestartUntil: Date.now() - 1 })).rejects.toMatchObject({ status: 502 }); expect(m.notify).toHaveBeenCalledTimes(3)
})
it('never silences forbidden responses during a restart window', async () => { vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('{}', { status: 403 }))); await expect(api.get('/admin/xshop-upgrade/status', { expectedRestartUntil: Date.now() + 1000 })).rejects.toMatchObject({ status: 403 }); expect(m.notify).toHaveBeenCalledTimes(1) })
