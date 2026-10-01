<template>
  <div class="products-page min-h-screen bg-background pb-16 pt-20 text-foreground">
    <div class="catalog-layout container mx-auto px-4">
      <section v-if="showHeroSection" class="products-banner mt-6 md:mt-8">
        <div class="relative min-h-[220px] overflow-hidden rounded-2xl border bg-card shadow-sm md:min-h-[340px]" @touchstart="onBannerTouchStart" @touchend="onBannerTouchEnd">
          <Transition name="banner-fade" mode="out-in">
            <img v-if="!bannerLoading && heroImage" :key="heroImage" :src="heroImage" :alt="heroTitle" class="absolute inset-0 h-full w-full object-cover" />
          </Transition>
          <div class="absolute inset-0 bg-gradient-to-r from-black/75 via-black/45 to-black/10"></div>
          <div class="relative flex min-h-[220px] flex-col justify-end p-5 text-white md:min-h-[340px] md:p-9">
            <div v-if="bannerLoading" class="space-y-3">
              <div class="h-7 w-2/3 rounded theme-skeleton opacity-60"></div>
              <div class="h-4 w-1/2 rounded theme-skeleton opacity-50"></div>
            </div>
            <template v-else>
              <h1 class="max-w-3xl text-2xl font-bold md:text-4xl">{{ heroTitle }}</h1>
              <p class="mt-2 max-w-2xl text-sm text-white/85 md:text-base">{{ heroSubtitle }}</p>
              <button v-if="hasHeroLink" type="button" class="mt-4 w-fit rounded-lg bg-white px-4 py-2 text-sm font-semibold text-gray-950 transition hover:bg-white/90" @click="goToHeroLink">{{ heroPrimaryButtonText }}</button>
              <div v-if="bannerCount > 1" class="mt-5 flex gap-2">
                <button v-for="(_, index) in banners" :key="index" type="button" class="h-2 rounded-full transition-all" :class="index === currentBannerIndex ? 'w-7 bg-white' : 'w-2 bg-white/50'" @click="selectHeroBanner(index)"></button>
              </div>
            </template>
          </div>
        </div>
      </section>

      <section v-if="homepageAd" class="products-announcement mt-6 rounded-2xl border bg-card p-5 shadow-sm md:mt-8 md:p-6">
        <div class="mb-4 flex items-center gap-2 text-sm font-semibold text-foreground">
          <span class="grid h-8 w-8 place-items-center rounded-full bg-secondary text-primary"><Megaphone class="h-4 w-4" /></span>
          <span>{{ getLocalizedText(homepageAd.title) }}</span>
        </div>
        <div class="announcement-content text-sm leading-7 text-muted-foreground" v-html="homepageAdContent"></div>
      </section>

      <section v-if="homepageNoticeEnabled && latestNotices.length" :class="{ 'notices-cold': !noticesAboveCatalog }" class="products-notices mt-6 rounded-2xl border bg-card p-5 shadow-sm md:mt-8 md:p-6">
        <div class="mb-4 flex items-center justify-between gap-3">
          <div class="flex items-center gap-2 text-sm font-semibold text-foreground">
            <span class="grid h-8 w-8 place-items-center rounded-full bg-secondary text-primary"><Bell class="h-4 w-4" /></span>
            <span>{{ t('nav.notice') }}</span>
          </div>
          <RouterLink to="/notice" class="text-xs font-medium text-primary hover:underline">{{ t('common.viewDetails') }}</RouterLink>
        </div>
        <div class="grid gap-3 md:grid-cols-2">
          <button v-for="notice in latestNotices" :key="notice.id" type="button" class="rounded-xl border bg-background/60 p-4 text-left transition hover:border-primary/40 hover:bg-secondary/40" @click="router.push(`/notice/${notice.slug}`)">
            <div class="font-semibold text-foreground">{{ getLocalizedText(notice.title) }}</div>
            <div class="mt-1 line-clamp-2 text-sm text-muted-foreground">{{ getLocalizedText(notice.summary) }}</div>
          </button>
        </div>
      </section>

      <section class="category-card-grid mt-5 md:mt-6">
        <button type="button" class="category-pill" :class="{ 'category-pill-active': selectedCategory === null }" :aria-pressed="selectedCategory === null" @click="selectCategory(null)">
          <img :src="allProductsIcon" :alt="t('products.allCategories')" class="category-icon object-cover" />
          <span>{{ t('products.allCategories') }}</span>
          <span class="category-active-indicator" aria-hidden="true"></span>
        </button>
        <template v-for="group in categoryGroups" :key="group.id">
          <button type="button" class="category-pill" :class="{ 'category-pill-active': selectedCategory === group.id }" :aria-pressed="selectedCategory === group.id" @click="selectCategory(group.id)">
            <img v-if="group.icon" :src="getImageUrl(group.icon)" :alt="getLocalizedText(group.name)" class="category-icon object-cover" />
            <span v-else class="category-icon"><FolderOpen class="h-4 w-4" /></span>
            <span>{{ getLocalizedText(group.name) }}</span>
            <span class="category-active-indicator" aria-hidden="true"></span>
          </button>
          <button v-for="child in group.children" :key="child.id" type="button" class="category-pill" :class="{ 'category-pill-active': selectedCategory === child.id }" :aria-pressed="selectedCategory === child.id" @click="selectCategory(child.id)">
            <img v-if="child.icon" :src="getImageUrl(child.icon)" :alt="getLocalizedText(child.name)" class="category-icon object-cover" />
            <span v-else class="category-icon"><FolderOpen class="h-4 w-4" /></span>
            <span>{{ getLocalizedText(child.name) }}</span>
            <span class="category-active-indicator" aria-hidden="true"></span>
          </button>
        </template>
      </section>

      <div class="mb-5 mt-8 flex items-center gap-2 text-lg font-bold"><Package class="h-5 w-5 text-primary" /><span>{{ selectedCategoryTitle }}</span></div>
      <main class="category-results" :aria-busy="!loadError && (loading || catalogStale)">
        <div class="catalog-status" role="status">
          <template v-if="loadError"><span>{{ t('common.error') }}</span> <button type="button" class="underline" @click="loadProducts">{{ t('common.retry') }}</button></template>
          <span v-else-if="catalogStale && hasLoadedOnce">{{ t('common.loading') }}</span>
        </div>
        <div v-original-list-motion="{ revision: contentRevision, stale: catalogStale }" class="catalog-content">
        <div v-if="loading && !hasLoadedOnce" class="grid grid-cols-2 gap-3 md:grid-cols-3 md:gap-4 lg:grid-cols-4">
          <div v-for="i in 6" :key="i" class="overflow-hidden rounded-2xl border bg-card">
            <div class="h-36 theme-skeleton md:h-56"></div><div class="space-y-3 p-3 md:p-5"><div class="h-5 w-3/4 rounded theme-skeleton"></div><div class="h-3 w-full rounded theme-skeleton"></div></div>
          </div>
        </div>
        <div v-else-if="products.length" class="catalog-feedback" :inert="catalogStale || undefined" :aria-disabled="catalogStale" :class="{ 'catalog-stale': catalogStale }">
          <div v-if="productCatalogLayout === 'list'" class="grid gap-6">
            <section v-for="group in groupedListProducts" :key="group.key" class="grid gap-2.5">
              <div class="list-category-heading">
                <span class="list-category-accent"></span>
                <img v-if="group.icon" :src="group.icon" :alt="group.name" class="h-7 w-7 rounded-md object-cover" />
                <FolderOpen v-else class="h-6 w-6 text-primary" />
                <span class="min-w-0 truncate font-bold">{{ group.name }}</span>
                <span class="list-category-count">{{ group.products.length }}</span>
              </div>
              <ProductListCard v-for="(product, productIndex) in group.products" :key="product.id" :product="product" :index="productIndex" @open="goToProduct" @purchase="goToProduct" />
            </section>
          </div>
          <div v-else class="grid grid-cols-2 gap-3 md:grid-cols-3 md:gap-4 lg:grid-cols-4">
            <ProductCard v-for="(product, idx) in products" :key="product.id" :product="product" :index="idx" :max-tags="isMobileGrid ? 1 : 2" :animation-step="50" @click="goToProduct" />
          </div>
          <PaginationNav :current-page="currentPage" :total-pages="totalPages" :loading="loading" @change-page="changePage" />
        </div>
        <EmptyState v-else variant="soft" size="lg" icon="package" :title="t('products.empty')" />
        </div>
      </main>
    </div>
    <AnnouncementModal
      v-if="activeAnnouncement"
      :announcement="activeAnnouncement"
      :visible="announcementVisible"
      @update:visible="announcementVisible = $event"
    />

  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Bell, FolderOpen, Megaphone, Package } from 'lucide-vue-next'
import { useAppStore } from '../stores/app'
import { useProductList } from '../composables/useProductList'
import { vOriginalListMotion } from '../utils/originalListMotion'
import { usePageSeo } from '../composables/usePageSeo'
import { useLocalized } from '../composables/useProduct'
import { getImageUrl } from '../utils/image'
import { sanitizeRichHtml } from '../utils/richContent'
import { readCachedHomepageAd, writeCachedHomepageAd } from '../utils/homepageAdCache'
import { readCachedCatalogNotices, writeCachedCatalogNotices, clearCachedCatalogNotices, validateCatalogNotices } from '../utils/catalogNoticeCache'
import { getBrowserStorage } from '../utils/browserStorage'
import ProductCard from '../components/ProductCard.vue'
import ProductListCard from '../components/ProductListCard.vue'

import PaginationNav from '../components/PaginationNav.vue'
import EmptyState from '../components/EmptyState.vue'
import AnnouncementModal from '../components/AnnouncementModal.vue'
import { useAnnouncement, type HomeAnnouncement } from '../composables/useAnnouncement'
import { useBannerCarousel } from '../composables/useBannerCarousel'
import { postAPI } from '../api'

const allProductsIcon = '/uploads/category/2026/09/all-products-category-v3.svg'
const router = useRouter()
const route = useRoute()
const { t } = useI18n()
const appStore = useAppStore()
const productCatalogLayout = computed(() => appStore.productCatalogLayout)
const { getLocalizedText } = useLocalized()
const { banners, bannerLoading, currentBannerIndex, bannerCount, showHeroSection, heroImage, heroTitle, heroSubtitle, hasHeroLink, heroPrimaryButtonText, loadBanners, selectHeroBanner, goToHeroLink, onBannerTouchStart, onBannerTouchEnd, stopHeroAutoPlay } = useBannerCarousel()
const noticeStorage = getBrowserStorage('sessionStorage')
const noticeHost = typeof window === 'undefined' ? '' : window.location.host
// Unknown config gates presentation/fetch, but is not an authoritative opt-out.
const homepageNoticeEnabled = computed(() => appStore.config == null ? null : appStore.config.nav_config?.homepage_notice_enabled !== false)
if (homepageNoticeEnabled.value === false) clearCachedCatalogNotices(noticeStorage, noticeHost)
const latestNotices = ref(homepageNoticeEnabled.value === false ? [] : readCachedCatalogNotices(noticeStorage, noticeHost))
// Unknown cold content arrives below products; only known public summaries sit above.
const noticesAboveCatalog = latestNotices.value.length > 0
let noticesDisposed = false
let noticesMounted = false
let noticeGeneration = 0

const loadLatestNotices = async () => {
  if (noticesDisposed || !homepageNoticeEnabled.value) return
  const generation = ++noticeGeneration
  try {
    const response = await postAPI.list({ type: 'notice', page: 1, page_size: 2 })
    if (noticesDisposed || !homepageNoticeEnabled.value || generation !== noticeGeneration) return
    latestNotices.value = validateCatalogNotices(response.data.data)
    writeCachedCatalogNotices(noticeStorage, noticeHost, latestNotices.value)
  } catch {
    // A failed refresh does not collapse a known public summary footprint.
  }
}
// Synchronous invalidation also fences false → true changes within one Vue tick.
watch(homepageNoticeEnabled, (enabled) => {
  ++noticeGeneration
  if (enabled === false) {
    latestNotices.value = []
    clearCachedCatalogNotices(noticeStorage, noticeHost)
  } else if (enabled && noticesMounted) {
    void loadLatestNotices()
  }
}, { flush: 'sync' })
const { loading, hasLoadedOnce, catalogStale, loadError, loadProducts, contentRevision, products, selectedCategory, currentPage, totalPages, categoryGroups, categoryMap, selectCategory, changePage, initialize, cleanup } = useProductList({ pageSize: 12, homeRouteName: 'products' })
type HomepageAd = { title: Record<string, string>; content: Record<string, string> }
const homepageAdStorage = typeof window === 'undefined' ? null : window.sessionStorage
const homepageAdHost = typeof window === 'undefined' ? '' : window.location.host
const cachedHomepageAd = ref<HomepageAd | null>(
  readCachedHomepageAd(homepageAdStorage, homepageAdHost) as HomepageAd | null,
)
const homepageAd = computed(() => (
  appStore.config?.homepage_ad ?? cachedHomepageAd.value
) as HomepageAd | undefined)
watch(() => appStore.config?.homepage_ad, (ad) => {
  cachedHomepageAd.value = ad as HomepageAd | null
  writeCachedHomepageAd(homepageAdStorage, homepageAdHost, cachedHomepageAd.value)
})
const homepageAdContent = computed(() => sanitizeRichHtml(getLocalizedText(homepageAd.value?.content)))
const { shouldShow } = useAnnouncement()
const activeAnnouncement = ref<HomeAnnouncement | null>(null)
const announcementVisible = ref(false)
watch(() => appStore.config?.announcement, (announcement) => {
  if (announcement && shouldShow(announcement)) {
    activeAnnouncement.value = announcement
    announcementVisible.value = true
  } else {
    activeAnnouncement.value = null
    announcementVisible.value = false
  }
}, { immediate: true })
const selectedCategoryTitle = computed(() => selectedCategory.value ? getLocalizedText(categoryMap.value.get(selectedCategory.value)?.name) : t('products.allCategories'))
const groupedListProducts = computed(() => {
  const groups = new Map<string, { key: string; name: string; icon: string; products: any[] }>()
  for (const product of products.value) {
    const category = product.category
    const key = String(category?.id || 'uncategorized')
    if (!groups.has(key)) groups.set(key, { key, name: getLocalizedText(category?.name) || t('products.allCategories'), icon: category?.icon ? getImageUrl(category.icon) : '', products: [] })
    groups.get(key)!.products.push(product)
  }
  return [...groups.values()]
})
usePageSeo({ canonicalPath: () => route.path, title: () => selectedCategoryTitle.value })
const goToProduct = (slug: string) => { if (!catalogStale.value) router.push(`/products/${slug}`) }
const isMobileGrid = ref(window.innerWidth < 768)
const handleResize = () => { isMobileGrid.value = window.innerWidth < 768 }
onMounted(async () => {
  noticesMounted = true
  window.addEventListener('resize', handleResize, { passive: true })
  await Promise.all([initialize(), loadBanners(), loadLatestNotices()])
})
onUnmounted(() => { noticesDisposed = true; window.removeEventListener('resize', handleResize); stopHeroAutoPlay(); cleanup() })
</script>

<style scoped>
.catalog-layout { display:flex; flex-direction:column; }
.notices-cold { order:1; }
.banner-fade-enter-active,.banner-fade-leave-active { transition:opacity .2s ease; }
.banner-fade-enter-from,.banner-fade-leave-to { opacity:0; }
@media (prefers-reduced-motion: reduce) { .banner-fade-enter-active,.banner-fade-leave-active { transition:none; } }
.category-results { min-height:100dvh; }
.catalog-status { min-height:1.5rem; color:var(--ui-text-muted); font-size:.875rem; }
.catalog-feedback { transition:opacity 140ms ease; }
.catalog-stale { opacity:.82; pointer-events:none; }
@media (prefers-reduced-motion: reduce) { .catalog-feedback { transition:none; } }

.category-card-grid { display:flex; flex-wrap:wrap; gap:.5rem; }
.category-pill { position:relative; display:flex; flex-direction:row; min-width:0; align-items:center; justify-content:center; gap:.5rem; min-height:2.75rem; padding:.5rem .75rem; border:1px solid var(--ui-border); border-radius:.625rem; background:var(--ui-bg-elevated); color:var(--ui-text-primary); box-shadow:var(--ui-shadow-1); font-size:.875rem; font-weight:500; line-height:1.3; text-align:center; transition:transform .18s ease,border-color .18s ease,box-shadow .18s ease,color .18s ease; }
.category-pill span:not(.category-active-indicator):not(.category-icon) { max-width:100%; overflow-wrap:anywhere; }
.category-pill:hover { border-color:hsl(var(--primary)/.45); color:hsl(var(--foreground)); transform:translateY(-1px); }
.category-pill:active { transform:scale(.96); }
.category-pill-active { border-color:var(--ui-accent); background:var(--ui-bg-elevated); color:var(--ui-text-primary); box-shadow:0 0 0 1px color-mix(in srgb, var(--ui-accent) 30%, transparent); font-weight:600; }
.category-icon { display:grid; width:1.625rem; height:1.625rem; flex:none; place-items:center; border-radius:.5rem; background:var(--ui-bg-soft); color:var(--ui-accent); }
.category-pill-active .category-icon { background:var(--ui-bg-soft); color:var(--ui-accent); }
.category-active-indicator { position:absolute; right:.75rem; bottom:-1px; left:.75rem; height:3px; border-radius:999px 999px 0 0; background:hsl(var(--primary)); opacity:0; transform:scaleX(.45); transform-origin:center; transition:opacity .18s ease,transform .18s ease; }
.category-pill-active .category-active-indicator { background:var(--ui-accent); opacity:1; transform:scaleX(1); }
.announcement-content :deep(a) { color:hsl(var(--primary)); text-decoration:underline; text-underline-offset:3px; }
.list-category-heading { display:flex; min-width:0; align-items:center; gap:.55rem; padding:0 .15rem; color:var(--ui-text-primary); }
.list-category-accent { width:.25rem; height:1.7rem; flex:none; border-radius:999px; background:hsl(var(--primary)); }
.list-category-count { display:inline-grid; min-width:1.55rem; height:1.35rem; place-items:center; border-radius:999px; background:var(--ui-bg-soft); padding:0 .42rem; color:var(--ui-text-muted); font-size:.7rem; font-weight:700; }
@media (prefers-reduced-motion: reduce) { .category-pill,.category-active-indicator { transition:none; } .category-pill:active { transform:none; } }
</style>
