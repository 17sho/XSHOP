import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { loader, vue, settle, deferred } from './helpers/motion17DHarness.ts'

for (const template of ['views/Products.vue', 'templates/vault/Products.vue']) {
  for (const layout of ['list', 'card']) test(`${template} ${layout}: refresh retains the populated DOM, marks busy, blocks stale actions without replay`, async () => {
    const state = Object.fromEntries(Object.entries({ loading: false, loadError: false, hasLoadedOnce: true, catalogStale: false, products: [{ id: 1, slug: 'fixture', title: {}, category: { id: 1 } }], selectedCategory: null, currentPage: 1, totalPages: 2, categoryGroups: [], categoryMap: new Map(), searchQuery: '', expandedParentIds: [] }).map(([k, v]) => [k, vue.ref(v)])) as any
    Object.assign(state, { selectCategory() {}, changePage() {}, initialize() {}, cleanup() {}, clearSearch() {}, toggleParentCategory() {} })
    const card = { props: ['product'], emits: ['open', 'purchase', 'quickBuy', 'click'], setup(p: any, { emit }: any) { return () => vue.h('button', { class: 'fixture-product theme-slide-up', onClick: () => { emit('open', p.product.slug); emit('purchase', p.product.slug); emit('click', p.product.slug); emit('quickBuy', p.product) } }, p.product.slug) } }
    const nil = { render: () => null }
    const noticeRequest = deferred()
    window.sessionStorage.clear()
    const warmNotices = layout === 'list'
    if (warmNotices) loader()('utils/catalogNoticeCache.ts').writeCachedCatalogNotices(window.sessionStorage, window.location.host, [{ id: 1, slug: 'fixture-old', title: { 'en-US': 'Cached fixture' }, summary: {} }])
    let navigations = 0
    const mocks: any = {
      'vue-router': { useRouter: () => ({ push() { navigations++ } }), useRoute: () => ({ path: '/products' }) },
      'vue-i18n': { useI18n: () => ({ t: (v: string) => v }) },
      '@/components/ui/button': { Button: 'button' },
      '../utils/richContent': { sanitizeRichHtml: (v: any) => v },
    }
    for (const prefix of ['../', '../../']) {
      mocks[prefix+'composables/useProductList'] = { useProductList: () => state }
      mocks[prefix+'composables/usePageSeo'] = { usePageSeo() {} }
      mocks[prefix+'composables/useProduct'] = { useLocalized: () => ({ getLocalizedText: (v: any) => v?.['en-US'] || 'fixture' }) }
      mocks[prefix+'utils/image'] = { getImageUrl: (v: any) => v }
      mocks[prefix+'stores/app'] = { useAppStore: () => ({ productCatalogLayout: layout, config: {} }) }
    }
    mocks['../composables/useAnnouncement'] = { useAnnouncement: () => ({ shouldShow: () => false }) }
    mocks['../composables/useBannerCarousel'] = { useBannerCarousel: () => ({ loadBanners() {}, stopHeroAutoPlay() {} }) }
    mocks['../api'] = { postAPI: { list: () => noticeRequest.promise } }
    for (const name of ['ProductCard','ProductListCard']) mocks[`../components/${name}.vue`] = { default: card }
    for (const name of ['PaginationNav','EmptyState','AnnouncementModal']) mocks[`../components/${name}.vue`] = { default: nil }
    for (const name of ['VaultProductCard','VaultProductListCard']) mocks[`./components/${name}.vue`] = { default: card }
    mocks['./components/VaultCategorySidebar.vue'] = { default: nil }
    mocks['../../components/ProductQuickBuy.vue'] = { default: { props: ['visible'], setup: (p: any) => () => p.visible ? vue.h('div', { 'data-fixture-quick-buy': '' }) : null } }
    const comp = loader(mocks)(template).default
    const host = document.createElement('div'); document.body.append(host)
    const app = vue.createApp(comp); app.component('RouterLink', { template: '<a><slot /></a>' }); app.mount(host)
    await settle()
    try {
      const before = host.querySelector('.fixture-product')
      assert.ok(before)
      if (template.startsWith('views')) {
        assert.equal(!!host.querySelector('.products-notices'), warmNotices, 'only known notices reserve an upper footprint')
        noticeRequest.resolve({ data: { data: [{ id: 2, slug: 'fixture-new', title: { 'en-US': 'Latest fixture' }, summary: {} }] } })
        await settle()
        const notice = host.querySelector('.products-notices')!
        assert.ok(notice.textContent?.includes('Latest fixture'), 'latest notice is not dropped')
        assert.equal(notice.classList.contains('notices-cold'), !warmNotices, 'cold insertion remains below catalog; warm insertion keeps known slot')
        assert.equal(host.querySelector('.fixture-product'), before, 'asynchronous notice cannot replace populated catalog')
      }
      state.loading.value = true; state.catalogStale.value = true
      await settle()
      assert.equal(host.querySelector('.fixture-product'), before, 'populated card identity survives pending refresh')
      assert.ok(host.querySelector('[aria-busy="true"]'))
      assert.ok(before.closest('[inert]'), 'stale interactive catalog is inert')
      assert.ok(host.querySelector('[role="status"]')?.textContent?.includes('common.loading'))
      ;(before as HTMLElement).click(); await settle()
      assert.equal(navigations, 0)
      assert.equal(host.querySelector('[data-fixture-quick-buy]'), null)
      const source = readFileSync(new URL('../src/'+template, import.meta.url), 'utf8')
      assert.doesNotMatch(source, /catalog-settled|v-motion-change/)
      state.loading.value = false; state.catalogStale.value = false; await settle()
      assert.equal(host.querySelector('.fixture-product'), before)
      assert.equal(before.closest('[inert]'), null)
      ;(before as HTMLElement).click(); await settle()
      if (template.startsWith('views')) assert.ok(navigations > 0, 'classic retains detail-navigation contract')
      else assert.ok(host.querySelector('[data-fixture-quick-buy]'), 'Vault retains quick-buy contract')
      state.catalogStale.value = true; state.loadError.value = true; await settle()
      assert.ok(host.querySelector('[aria-busy="false"]'))
      assert.ok(host.querySelector('[role="status"]')?.textContent?.includes('common.error'))
      assert.ok(host.querySelector('[role="status"] button')?.textContent?.includes('common.retry'))
    } finally { app.unmount(); host.remove() }
  })
}
