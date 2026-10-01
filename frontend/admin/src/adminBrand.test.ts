import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { createApp, nextTick, type App } from 'vue'
import i18n from './i18n'
import Layout from './layouts/AdminLayout.vue'
import Login from './views/Login.vue'
import Settings from './views/admin/Settings.vue'
const api = vi.hoisted(() => ({ public: vi.fn(), get: vi.fn(), save: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: new Proxy({}, { get: (_, name: string) => name === 'getPublicConfig' ? api.public : name === 'updateSettings' ? api.save : api.get }) }))
vi.mock('@/stores/auth', () => ({ useAdminAuthStore: () => ({ hasPermission: () => true, loading: false }) }))
vi.mock('vue-router', () => ({ onBeforeRouteLeave: vi.fn(), useRoute: () => ({ path: '/', query: {} }), useRouter: () => ({ push: vi.fn() }), RouterLink: { template: '<a><slot /></a>' } }))
vi.mock('@/components/AdminRouteView.vue', () => ({ default: { template: '<div />' } }))
vi.mock('@/components/RichEditor.vue', () => ({ default: { template: '<div />' } }))
vi.mock('@/components/admin/MediaPicker.vue', () => ({ default: { template: '<div />' } }))
vi.mock('@/utils/notify', () => ({ notifyError: vi.fn(), notifySuccess: vi.fn() }))
const mounts: { app: App, el: HTMLElement }[] = []
const flush = async () => { await new Promise(r => setTimeout(r, 0)); await nextTick() }
function mount(component: any) { const el = document.createElement('div'); document.body.append(el); const app = createApp(component); app.use(i18n); const vm = app.mount(el); mounts.push({ app, el }); return { el, s: (vm as any).$.setupState, app } }
beforeEach(() => { localStorage.clear(); i18n.global.locale.value = 'zh-CN'; api.public.mockReset().mockResolvedValue({ data: { data: { brand: { site_name: '测试品牌' }, app_version: 'v1' } } }); api.get.mockReset().mockImplementation(async (payload: any) => ({ data: { data: payload?.key === 'site_config' ? { brand: { site_name: '测试品牌' } } : {} } })); api.save.mockReset().mockResolvedValue({ data: { data: { value: { brand: { site_name: '新品牌' } } } } }) })
afterEach(() => { mounts.splice(0).forEach(({ app, el }) => { app.unmount(); el.remove() }); document.getElementById('site-favicon')?.remove() })
const brandResponse = (site_name: string, site_icon: unknown) => ({ data: { data: { brand: { site_name, site_icon } } } })
function favicon() { const link = document.createElement('link'); link.id = 'site-favicon'; link.rel = 'icon'; document.head.append(link); return link }

it('disposed canonical save cannot restore its favicon over a replacement page', async () => {
 const link = favicon(); mount(Layout); await flush(); const old = mount(Settings); await flush(); old.s.form.brand.site_icon = '/old.svg'
 let resolve!: (v: any) => void; api.save.mockImplementation(async (payload: any) => payload.key === 'site_config' ? await new Promise(r => { resolve = r }) : ({ data: { data: {} } }))
 const pending = old.s.saveSettings(); await flush(); old.app.unmount()
 api.public.mockResolvedValue(brandResponse('NEW PAGE', '/new.svg')); mount(Login); await flush()
 resolve({ data: { data: { value: { brand: { site_name: 'OLD SAVE', site_icon: '/old.svg' } } } } }); await pending; await flush()
 expect(document.title).toBe('NEW PAGE 后台管理'); expect(link.getAttribute('href')).toBe('/new.svg')
})
it('disposed late fallback cannot restore its favicon over a replacement page', async () => {
 const link = favicon(); mount(Layout); await flush(); const old = mount(Settings); await flush(); old.s.form.brand.site_icon = '/old.svg'
 api.save.mockResolvedValue({ data: { data: {} } }); let resolve!: (v: any) => void
 api.public.mockReturnValueOnce(new Promise(r => { resolve = r })); const pending = old.s.saveSettings(); await flush(); old.app.unmount()
 api.public.mockResolvedValue(brandResponse('NEW PAGE', '/new.svg')); mount(Login); await flush()
 resolve(brandResponse('OLD SAVE', '/old.svg')); await pending; await flush()
 expect(document.title).toBe('NEW PAGE 后台管理'); expect(link.getAttribute('href')).toBe('/new.svg')
})
it('late fallback cannot replace a newer owner save favicon', async () => {
 const link = favicon(); mount(Layout); await flush(); const old = mount(Settings); await flush(); old.s.form.brand.site_icon = '/old.svg'
 api.save.mockResolvedValue({ data: { data: {} } }); let resolve!: (v: any) => void
 api.public.mockReturnValueOnce(new Promise(r => { resolve = r })); const pending = old.s.saveSettings(); await flush()
 const fresh = mount(Settings); await flush(); fresh.s.form.brand.site_icon = '/new-save.svg'
 api.save.mockResolvedValue({ data: { data: { value: { brand: { site_name: 'NEW SAVE' } } } } }); await fresh.s.saveSettings(); await flush()
 resolve(brandResponse('OLD SAVE', '/old.svg')); await pending; await flush()
 expect(document.title).toBe('NEW SAVE 后台管理'); expect(link.getAttribute('href')).toBe('/new-save.svg')
})
it('current save applies the validated canonical favicon without replacing drafts', async () => {
 const link = favicon(); mount(Layout); await flush(); const settings = mount(Settings); await flush(); settings.s.form.brand.site_icon = '/draft.svg'
 api.save.mockResolvedValue({ data: { data: { value: { brand: { site_name: 'CANONICAL', site_icon: '/canonical.svg' } } } } })
 const draft = JSON.stringify(settings.s.form); await settings.s.saveSettings(); await flush()
 expect(link.getAttribute('href')).toBe('/canonical.svg'); expect(JSON.stringify(settings.s.form)).toBe(draft)
})
it('pending save applies submitted icon rather than a later mutable draft', async () => {
 const link = favicon(); mount(Layout); await flush(); const settings = mount(Settings); await flush(); settings.s.form.brand.site_icon = '/submitted.svg'
 let resolve!: (v: any) => void; api.save.mockImplementation(async (payload: any) => payload.key === 'site_config' ? await new Promise(r => { resolve = r }) : ({ data: { data: {} } }))
 const pending = settings.s.saveSettings(); await flush(); settings.s.form.brand.site_icon = '/pending-draft.svg'
 resolve({ data: { data: { value: { brand: { site_name: 'SAVED', site_icon: { invalid: true } } } } } }); await pending; await flush()
 expect(link.getAttribute('href')).toBe('/submitted.svg'); expect(settings.s.form.brand.site_icon).toBe('/pending-draft.svg')
})
it('renders actual configured branding in desktop/mobile titles, subtitles, collapsed initials, footers and document title', async () => {
 const { el, s } = mount(Layout); await flush(); s.sidebarCollapsed = false; await flush()
 for (const selector of ['[data-admin-desktop-nav]', '[data-admin-mobile-nav]']) { const text = el.querySelector(selector)!.textContent!; expect(text).toContain('测试品牌 后台管理'); expect(text).toContain('测试品牌 控制台'); expect(text).toMatch(/©.*测试品牌/); expect(text).not.toContain('Dujiao-Next') }
 expect(document.title).toBe('测试品牌 后台管理'); expect(api.public).toHaveBeenCalledTimes(1)
 s.sidebarCollapsed = true; await flush(); expect(el.querySelector('[data-admin-desktop-nav]')!.textContent).toContain('测试')
})
it('login renders dynamic safe text and footer; locale changes title', async () => {
 const raw = '😀雪<img src=x onerror=alert(1)>' + '长'.repeat(100)
 api.public.mockResolvedValue({ data: { data: { brand: { site_name: raw } } } })
 const { el } = mount(Login); await flush()
 expect(el.textContent).toContain(raw + ' 后台管理'); expect(el.textContent).toMatch(/©.*😀雪/)
 expect(el.querySelector('img')).toBeNull(); expect(el.querySelector('a')?.href).toBe('https://github.com/dujiao-next')
 expect(api.public).toHaveBeenCalledTimes(1)
 i18n.global.locale.value = 'en-US'; await flush(); expect(document.title).toBe(raw + ' Admin')
})
it('successful basic save updates shared shell from canonical response without wiping drafts', async () => {
 const shell = mount(Layout); await flush(); shell.s.sidebarCollapsed = false
 const settings = mount(Settings); await flush(); settings.s.form.brand.site_name = 'draft'
 await settings.s.saveSettings(); await flush()
 expect(shell.el.textContent).toContain('新品牌 后台管理'); expect(document.title).toBe('新品牌 后台管理')
 expect(settings.s.form.brand.site_name).toBe('draft')
})
it('empty, malformed and failed loads have localized fallback', async () => {
 for (const value of ['', '   ', 12, {}]) {
 api.public.mockResolvedValue({ data: { data: { brand: { site_name: value } } } }); mount(Login); await flush(); expect(document.title).toBe('站点 后台管理')
 }
 api.public.mockRejectedValue(new Error('offline')); mount(Login); await flush(); expect(document.title).toBe('站点 后台管理')
 i18n.global.locale.value = 'en-US'; await flush(); expect(document.title).toBe('Site Admin')
})
it('disposed login late public load cannot overwrite newer layout branding/title', async () => {
 let resolve!: (v: any) => void
 api.public.mockReturnValueOnce(new Promise(r => { resolve = r })); const old = mount(Login); old.app.unmount()
 mount(Layout); await flush(); resolve({ data: { data: { brand: { site_name: 'OLD' } } } }); await flush()
 expect(document.title).toBe('测试品牌 后台管理')
})
it('fresh public read after save preserves drafts and fences a late pre-save shell load', async () => {
 let oldResolve!: (v: any) => void
 api.public.mockReturnValueOnce(new Promise(r => { oldResolve = r }))
 const shell = mount(Layout); const settings = mount(Settings); await flush()
 api.save.mockResolvedValue({ data: { data: {} } })
 api.public.mockResolvedValue({ data: { data: { brand: { site_name: 'canonical public' } } } })
 settings.s.form.brand.site_name = 'unsaved'; await settings.s.saveSettings(); await flush()
 expect(document.title).toBe('canonical public 后台管理'); expect(settings.s.form.brand.site_name).toBe('unsaved')
 oldResolve({ data: { data: { brand: { site_name: 'STALE' } } } }); await flush()
 expect(document.title).toBe('canonical public 后台管理'); expect(shell.el.textContent).not.toContain('STALE')
})
it('disposed Settings refresh cannot replace newer page name and title', async () => {
 mount(Layout); await flush(); const settings = mount(Settings); await flush()
 api.save.mockResolvedValue({ data: { data: {} } }); let resolve!: (v: any) => void
 api.public.mockReturnValueOnce(new Promise(r => { resolve = r }))
 const save = settings.s.saveSettings(); await flush(); settings.app.unmount()
 api.public.mockResolvedValue({ data: { data: { brand: { site_name: 'NEW PAGE' } } } }); mount(Login); await flush()
 resolve({ data: { data: { brand: { site_name: 'OLD SAVE' } } } }); await save; await flush()
 expect(document.title).toBe('NEW PAGE 后台管理')
})
it('unicode collapsed initials are intact and long mobile text is constrained', async () => {
 api.public.mockResolvedValue({ data: { data: { brand: { site_name: '😀雪' + '长'.repeat(150) } } } })
 const { el, s } = mount(Layout); await flush(); s.sidebarCollapsed = true; await flush()
 expect(el.querySelector('[data-admin-desktop-nav] [title]')?.textContent).toBe('😀雪')
 expect(el.querySelector('[data-admin-mobile-nav] div.text-xl')?.className).toContain('[overflow-wrap:anywhere]')
 i18n.global.locale.value = 'zh-TW'; await flush(); expect(document.title).toContain('後台管理')
})
