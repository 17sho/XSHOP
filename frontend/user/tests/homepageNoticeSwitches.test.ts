import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { loader, vue, settle, deferred } from './helpers/motion17DHarness.ts'

const notice = (id: number) => ({ id, slug: `notice-${id}`, title: { 'en-US': `Notice ${id}` }, summary: {} })
const response = (id: number) => ({ data: { data: [notice(id)] } })
function fixture(theme: string, nav: boolean, home: boolean | undefined, warm = false, initialConfig: any = { nav_config: { builtin: { notice: nav }, homepage_notice_enabled: home } }) {
  window.sessionStorage.clear()
  const cache = loader()('utils/catalogNoticeCache.ts')
  if (warm) cache.writeCachedCatalogNotices(window.sessionStorage, window.location.host, [notice(99)])
  const store = vue.reactive({ locale: 'en-US', productCatalogLayout: 'list', config: initialConfig, loading: initialConfig === null })
  const requests: any[] = []
  const state: any = Object.fromEntries(Object.entries({ loading: false, loadError: false, hasLoadedOnce: true, catalogStale: false, contentRevision: 0, products: [{ id: 1, slug: 'product-fixture', title: {} }], selectedCategory: null, currentPage: 1, totalPages: 1, categoryGroups: [], categoryMap: new Map() }).map(([k, v]) => [k, vue.ref(v)]))
  Object.assign(state, { selectCategory() {}, changePage() {}, initialize() {}, cleanup() {} })
  const nil = { render: () => null }
  const mocks: any = {
    'vue-router': { useRouter: () => ({ push() {} }), useRoute: () => ({ name: 'products', path: '/' }) },
    'vue-i18n': { useI18n: () => ({ t: (key: string) => key, locale: vue.ref('en-US') }) },
    '../composables/useProductList': { useProductList: () => state },
    '../composables/usePageSeo': { usePageSeo() {} },
    '../composables/useProduct': { useLocalized: () => ({ getLocalizedText: (v: any) => v?.['en-US'] || '' }) },
    '../composables/useAnnouncement': { useAnnouncement: () => ({ shouldShow: () => false }) },
    '../composables/useBannerCarousel': { useBannerCarousel: () => ({ loadBanners() {}, stopHeroAutoPlay() {} }) },
    '../utils/richContent': { sanitizeRichHtml: (v: any) => v },
    '../utils/image': { getImageUrl: (v: any) => v },
    '../../../utils/image': { getImageUrl: (v: any) => v },
    '../stores/app': { useAppStore: () => store },
    '../../../stores/app': { useAppStore: () => store },
    '../../../stores/userAuth': { useUserAuthStore: () => ({ isAuthenticated: false }) },
    '../../../utils/theme': { useTheme: () => ({ theme: vue.ref('light'), toggleTheme() {} }) },
    '../api': { postAPI: { list: (params: any) => { const r = deferred(); requests.push({ ...r, params }); return r.promise } } },
    '../styles/vault.css': {},
  }
  for (const font of ['rubik', 'nunito-sans']) for (const weight of [400, 500, 600, 700, 800]) mocks[`@fontsource/${font}/latin-${weight}.css`] = {}
  for (const name of ['PaginationNav', 'EmptyState', 'AnnouncementModal', 'ProductCard']) mocks[`../components/${name}.vue`] = { default: nil }
  mocks['../components/ProductListCard.vue'] = { default: { render: () => vue.h('div', { 'data-catalog-product': '' }, 'catalog') } }
  const load = loader(mocks)
  const Products = load('views/Products.vue').default
  const Layout = theme === 'vault' ? load('templates/vault/layout/VaultLayout.vue').default : { setup: (_: any, { slots }: any) => () => slots.default() }
  let navState: any
  const host = document.createElement('div'); document.body.append(host)
  const app = vue.createApp({ setup() { navState = load('composables/useNavConfig.ts').useNavConfig(); return () => vue.h(Layout, null, { default: () => vue.h(Products) }) } })
  app.component('RouterLink', { props: ['to'], setup: (p: any, { slots }: any) => () => vue.h('a', { href: p.to }, slots.default?.()) })
  app.mount(host)
  let mounted = true
  return { host, store, requests, navState, cache: () => cache.readCachedCatalogNotices(window.sessionStorage, window.location.host), unmount() { if (mounted) app.unmount(); mounted = false; host.remove() } }
}

for (const theme of ['classic', 'vault']) {
  test(`${theme}: initial null config with warm cache waits for authoritative false without blocking products`, async () => {
    const f = fixture(theme, true, false, true, null)
    try {
      const catalog = f.host.querySelector('[data-catalog-product]')
      assert.ok(catalog, 'ready products render before config resolves')
      assert.equal(f.host.querySelector('.products-notices'), null, 'unknown config must not flash cached notices even on first render')
      assert.equal(f.requests.length, 0, 'unknown config must not fetch notices')
      assert.deepEqual(f.cache(), [notice(99)], 'pending config retains validated cache metadata')
      await settle()
      assert.equal(f.host.querySelector('.products-notices'), null)
      assert.equal(f.requests.length, 0)
      f.store.config = { nav_config: { homepage_notice_enabled: false } }
      assert.deepEqual(f.cache(), [], 'resolved false must purge even though unknown was already hidden')
      await settle()
      assert.equal(f.host.querySelector('.products-notices'), null)
      assert.equal(f.requests.length, 0, 'resolved false must never dispatch the notice request')
      assert.equal(f.host.querySelector('[data-catalog-product]'), catalog)
    } finally { f.unmount() }
  })
  for (const warm of [false, true]) for (const legacy of [false, true]) {
    test(`${theme}: delayed config null to ${legacy ? 'legacy {}' : 'true'} restores warm=${warm} placement after products are ready`, async () => {
      const f = fixture(theme, false, true, warm, null)
      const config = deferred()
      const loaded = config.promise.then(value => { f.store.config = value; f.store.loading = false })
      try {
        const catalog = f.host.querySelector('[data-catalog-product]')
        assert.ok(catalog)
        assert.equal(f.requests.length, 0, 'pending config must not request notices')
        assert.equal(f.host.querySelector('.products-notices'), null)
        await settle()
        assert.equal(f.requests.length, 0, 'page readiness must not stand in for config readiness')
        assert.deepEqual(f.cache(), warm ? [notice(99)] : [])
        config.resolve(legacy ? {} : { nav_config: { homepage_notice_enabled: true } })
        await loaded; await settle()
        assert.equal(f.requests.length, 1, 'authoritative enabled config starts exactly one refresh')
        const cachedCard = f.host.querySelector('.products-notices')
        assert.equal(!!cachedCard, warm)
        if (warm) {
          assert.ok(cachedCard?.textContent?.includes('Notice 99'))
          assert.equal(cachedCard?.classList.contains('notices-cold'), false, 'hidden warm metadata retains original upper slot')
        }
        f.requests[0].resolve(response(5)); await settle()
        const card = f.host.querySelector('.products-notices')!
        assert.ok(card.textContent?.includes('Notice 5'))
        assert.equal(card.classList.contains('notices-cold'), !warm, 'cold arrivals remain below the catalog')
        assert.equal(f.host.querySelector('[data-catalog-product]'), catalog, 'config/notice completion must not remount ready products')
        f.store.config = legacy ? {} : { nav_config: { homepage_notice_enabled: true } }
        await settle(); assert.equal(f.requests.length, 1, 'equivalent enabled config does not replay refresh')
      } finally { f.unmount() }
    })
  }
  for (const warm of [false, true]) {
    test(`${theme}: failed config remaining null stays closed with warm=${warm} and products available`, async () => {
      const f = fixture(theme, true, true, warm, null)
      const config = deferred()
      // Match the app store failure boundary: loading settles, config remains null.
      const loaded = config.promise.catch(() => {}).finally(() => { f.store.loading = false })
      try {
        const catalog = f.host.querySelector('[data-catalog-product]')
        assert.ok(catalog)
        assert.equal(f.requests.length, 0)
        assert.equal(f.host.querySelector('.products-notices'), null)
        config.reject(new Error('config offline')); await loaded; await settle()
        assert.equal(f.store.config, null)
        assert.equal(f.store.loading, false)
        assert.equal(f.requests.length, 0, 'settled loading without config does not authorize notice fetch')
        assert.equal(f.host.querySelector('.products-notices'), null)
        assert.deepEqual(f.cache(), warm ? [notice(99)] : [], 'failure is not an explicit opt-out')
        assert.equal(f.host.querySelector('[data-catalog-product]'), catalog)
      } finally { f.unmount() }
    })
  }
  test(`${theme}: resolved legacy empty config retains default-on behavior`, async () => {
    const f = fixture(theme, false, undefined, true, {})
    try {
      assert.equal(f.requests.length, 1)
      assert.ok(f.host.querySelector('.products-notices')?.textContent?.includes('Notice 99'))
      f.requests[0].resolve(response(4)); await settle()
      assert.ok(f.host.querySelector('.products-notices')?.textContent?.includes('Notice 4'))
      assert.equal(f.host.querySelector('.products-notices')?.classList.contains('notices-cold'), false)
    } finally { f.unmount() }
  })
  test(`${theme}: null config invalidates an enabled in-flight request without discarding warm metadata`, async () => {
    const f = fixture(theme, false, true, true)
    try {
      f.store.config = null; await settle()
      assert.equal(f.host.querySelector('.products-notices'), null)
      assert.deepEqual(f.cache(), [notice(99)])
      f.store.config = {}; await settle()
      assert.equal(f.requests.length, 2)
      f.requests[0].resolve(response(1)); await settle()
      assert.deepEqual(f.cache(), [notice(99)], 'an old generation cannot commit after config returns')
      f.requests[1].resolve(response(2)); await settle()
      assert.deepEqual(f.cache(), [notice(2)])
      assert.equal(f.host.querySelector('.products-notices')?.classList.contains('notices-cold'), false)
    } finally { f.unmount() }
  })
  test(`${theme}: unmount while config is unknown prevents delayed enable fetching`, async () => {
    const f = fixture(theme, false, true, true, null)
    try {
      assert.equal(f.requests.length, 0)
      f.unmount(); f.store.config = {}; await settle()
      assert.equal(f.requests.length, 0)
      assert.deepEqual(f.cache(), [notice(99)])
    } finally { f.unmount() }
  })
  for (const nav of [false, true]) for (const home of [false, true]) for (const warm of [false, true]) {
    test(`${theme} homepage nav=${nav} home=${home} warm=${warm}: independent cards, cache and network`, async () => {
      const f = fixture(theme, nav, home, warm)
      try {
        await settle()
        assert.equal(f.navState.noticeEnabled.value, nav)
        if (theme === 'vault') assert.equal(!!f.host.querySelector('header a[href="/notice"]'), nav)
        assert.equal(!!f.host.querySelector('.products-notices'), home && warm)
        assert.equal(f.requests.length, home ? 1 : 0, 'disabled homepage must not fetch notice list')
        const catalog = f.host.querySelector('[data-catalog-product]')
        assert.ok(catalog)
        if (home) {
          assert.deepEqual(f.requests[0].params, { type: 'notice', page: 1, page_size: 2 })
          f.requests[0].resolve(response(2)); await settle()
          const card = f.host.querySelector('.products-notices')!
          assert.ok(card.textContent?.includes('Notice 2'))
          assert.equal(card.classList.contains('notices-cold'), !warm)
        } else assert.deepEqual(f.cache(), [], 'disabled homepage purges cached summaries')
        assert.equal(f.host.querySelector('[data-catalog-product]'), catalog, 'notice policy must not remount catalog')
      } finally { f.unmount() }
    })
  }
  test(`${theme}: disable clears warm cache synchronously and fences in-flight responses across re-enable`, async () => {
    const f = fixture(theme, true, true, true)
    try {
      const catalog = f.host.querySelector('[data-catalog-product]')
      f.store.config.nav_config.homepage_notice_enabled = false
      assert.deepEqual(f.cache(), [], 'clear cache at the policy change, before next tick')
      await settle(); assert.equal(f.host.querySelector('.products-notices'), null)
      f.requests[0].resolve(response(1)); await settle()
      assert.deepEqual(f.cache(), [], 'disabled request cannot repopulate cache')
      f.store.config.nav_config.homepage_notice_enabled = true; await settle()
      assert.equal(f.requests.length, 2)
      assert.equal(f.host.querySelector('.products-notices'), null, 're-enable refreshes, never restores hidden cache')
      f.store.config.nav_config.homepage_notice_enabled = false
      f.store.config.nav_config.homepage_notice_enabled = true
      await settle(); assert.equal(f.requests.length, 3, 'same-tick transitions still own separate generations')
      f.requests[2].resolve(response(3)); await settle()
      f.requests[1].resolve(response(2)); await settle()
      assert.ok(f.host.querySelector('.products-notices')?.textContent?.includes('Notice 3'))
      assert.equal(f.cache()[0].id, 3, 'old enabled generation cannot replace fresh cache')
      f.store.config.nav_config.builtin.notice = false; await settle()
      assert.equal(f.requests.length, 3, 'navigation changes do not reload homepage')
      assert.ok(f.host.querySelector('.products-notices'))
      assert.equal(f.host.querySelector('[data-catalog-product]'), catalog)
    } finally { f.unmount() }
  })
  test(`${theme}: initially disabled re-enable fetches, unmount prevents writes`, async () => {
    const f = fixture(theme, false, false, true)
    try {
      f.store.config.nav_config.homepage_notice_enabled = true; await settle()
      assert.equal(f.requests.length, 1)
      f.unmount(); f.requests[0].resolve(response(7)); await settle()
      assert.deepEqual(f.cache(), [], 'unmounted response cannot write')
    } finally { f.unmount() }
  })
  test(`${theme}: refresh failure keeps enabled warm footprint and disabled stays empty`, async () => {
    const f = fixture(theme, false, true, true)
    try {
      f.requests[0].reject(new Error('offline')); await settle()
      assert.ok(f.host.querySelector('.products-notices')?.textContent?.includes('Notice 99'))
      f.store.config.nav_config.homepage_notice_enabled = false; await settle()
      assert.equal(f.host.querySelector('.products-notices'), null); assert.deepEqual(f.cache(), [])
    } finally { f.unmount() }
  })
  test(`${theme}: missing homepage switch retains default-on behavior`, async () => {
    const f = fixture(theme, false, undefined)
    try { assert.equal(f.requests.length, 1); f.requests[0].resolve(response(3)); await settle(); assert.ok(f.host.querySelector('.products-notices')) }
    finally { f.unmount() }
  })
}

test('actual homepage routing shares Products in the Vault wrapper; its separate catalog has no notice fetch', () => {
  const router = readFileSync(new URL('../src/router/index.ts', import.meta.url), 'utf8')
  assert.match(router, /path: '\/',\s*name: 'products',\s*component: Products/)
  const vaultCatalog = readFileSync(new URL('../src/templates/vault/Products.vue', import.meta.url), 'utf8')
  assert.doesNotMatch(vaultCatalog, /postAPI|latestNotices|catalogNoticeCache/)
})
