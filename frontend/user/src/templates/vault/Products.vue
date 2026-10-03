<template>
  <div class="mx-auto w-full max-w-[1180px] px-6 pb-6">
    <nav class="flex items-center gap-1.5 py-5 pb-1 text-[13.5px] font-semibold text-muted-foreground">
      <RouterLink to="/" class="flex-none hover:text-primary">{{ t('nav.home') }}</RouterLink>
      <ChevronRight class="h-4 w-4 flex-none" />
      <span class="min-w-0 truncate text-foreground">{{ pageTitle }}</span>
    </nav>
    <h1 class="mb-3.5 break-words text-3xl font-extrabold">{{ pageTitle }}</h1>

    <div class="grid min-w-0 grid-cols-1 items-start gap-7 py-1.5 pb-9 lg:grid-cols-[248px_minmax(0,1fr)]">
      <!-- 筛选侧栏：桌面竖排 + 移动端横向 chips -->
      <div class="grid min-w-0 gap-3 lg:sticky lg:top-[88px] lg:gap-5">
        <!-- 搜索框 -->
        <div class="relative">
          <Search class="pointer-events-none absolute left-3.5 top-1/2 h-[18px] w-[18px] -translate-y-1/2 text-muted-foreground" />
          <input
            v-model="searchQuery"
            class="h-11 w-full rounded-xl border border-border bg-card pl-10 pr-10 text-[15px] text-foreground transition-colors placeholder:text-muted-foreground hover:border-hairline-strong"
            :placeholder="t('products.searchBoxPlaceholder')"
            :aria-label="t('products.searchLabel')"
          />
          <button v-if="searchQuery" type="button" class="absolute right-2.5 top-1/2 grid h-6 w-6 -translate-y-1/2 place-items-center rounded-full text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground" :aria-label="t('blog.searchClear')" @click="clearSearch"><X class="h-3.5 w-3.5" /></button>
        </div>

        <!-- 分类：使用 VaultCategorySidebar，移动端自动变横向 chips -->
        <VaultCategorySidebar
          :category-groups="categoryGroups"
          :selected-category="selectedCategory"
          :expanded-parent-ids="expandedParentIds"
          @select="selectCategory"
          @toggle="toggleParentCategory"
        />
      </div>

      <!-- 商品区 -->
      <section class="min-w-0" :aria-busy="!loadError && (loading || catalogStale)">
        <div class="catalog-status" role="status">
          <span v-if="!loadError && (loading || catalogStale)" class="sr-only">{{ t('common.loading') }}</span>
          <template v-if="loadError"><span>{{ t('common.error') }}</span> <button type="button" class="underline" @click="loadProducts">{{ t('common.retry') }}</button></template>
        </div>
        <div v-if="!loadError && (loading || catalogStale)" data-category-loading-region class="flex min-h-[320px] items-center justify-center" role="status" :aria-label="t('common.loading')">
          <Loader2 data-category-refresh-spinner class="h-10 w-10 animate-spin motion-reduce:animate-none text-primary" aria-hidden="true" />
        </div>
        <div v-show="!(loading || catalogStale) || !!loadError" v-original-list-motion="{ revision: contentRevision, stale: catalogStale }" class="catalog-content">
        <div v-if="loading && !hasLoadedOnce" class="grid gap-4 grid-cols-[repeat(auto-fill,minmax(228px,1fr))]">
          <div v-for="i in 9" :key="i" class="h-[280px] rounded-lg border bg-card"></div>
        </div>

        <div v-else-if="products.length" class="catalog-feedback" :inert="catalogStale || undefined" :aria-disabled="catalogStale" :class="{ 'catalog-stale': catalogStale }">
          <div v-if="productCatalogLayout === 'list'" class="grid min-w-0 grid-cols-1 gap-6">
            <section v-for="group in groupedListProducts" :key="group.key" class="grid min-w-0 grid-cols-1 gap-2.5">
              <div class="list-category-heading flex min-w-0 items-center gap-2 px-0.5">
                <span class="h-7 w-1 flex-none rounded-full bg-primary"></span>
                <img v-if="group.icon" :src="group.icon" :alt="group.name" class="h-7 w-7 rounded-md object-cover" />
                <PackageOpen v-else class="h-6 w-6 text-primary" />
                <span class="min-w-0 truncate font-extrabold">{{ group.name }}</span>
                <span class="list-category-count">{{ group.products.length }}</span>
              </div>
              <VaultProductListCard v-for="(product, productIndex) in group.products" :key="product.id" :product="product" :index="productIndex" @quick-buy="openQuickBuy" />
            </section>
          </div>
          <div v-else class="grid gap-4 grid-cols-[repeat(auto-fill,minmax(228px,1fr))]">
            <VaultProductCard
              v-for="(product, idx) in products"
              :key="product.id"
              :product="product"
              :index="idx"
              @quick-buy="openQuickBuy"
            />
          </div>

          <nav v-if="totalPages > 1" class="mt-[30px] flex justify-center gap-2">
            <button class="grid h-[42px] min-w-[42px] place-items-center rounded-full border bg-card px-3 font-bold text-muted-foreground hover:border-primary hover:text-primary disabled:cursor-not-allowed disabled:opacity-40" :disabled="currentPage <= 1" @click="changePage(currentPage - 1)"><ChevronLeft class="h-[17px] w-[17px]" /></button>
            <button
              v-for="p in pageWindow"
              :key="p"
              class="grid h-[42px] min-w-[42px] place-items-center rounded-full border px-3 text-sm font-bold"
              :class="p === currentPage ? 'border-primary bg-primary text-white' : 'border-border bg-card text-muted-foreground hover:border-primary hover:text-primary'"
              @click="changePage(p)"
            >{{ p }}</button>
            <button class="grid h-[42px] min-w-[42px] place-items-center rounded-full border bg-card px-3 font-bold text-muted-foreground hover:border-primary hover:text-primary disabled:cursor-not-allowed disabled:opacity-40" :disabled="currentPage >= totalPages" @click="changePage(currentPage + 1)"><ChevronRight class="h-[17px] w-[17px]" /></button>
          </nav>
        </div>

        <div v-else class="flex flex-col items-center gap-3 rounded-lg border border-dashed py-14 text-center text-muted-foreground">
          <component :is="searchQuery || selectedCategory ? SearchX : PackageOpen" class="h-12 w-12 opacity-60" />
          <p>{{ (searchQuery || selectedCategory) ? t('products.emptyFiltered') : t('products.empty') }}</p>
          <Button v-if="searchQuery || selectedCategory" variant="outline" size="sm" class="mt-2 rounded-full" @click="resetFilters">{{ t('products.clearFilters') }}</Button>
        </div>
        </div>
      </section>
    </div>

    <ProductQuickBuy
      v-if="quickBuyProduct"
      :product="quickBuyProduct"
      :visible="quickBuyVisible"
      @update:visible="quickBuyVisible = $event"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Loader2, ChevronLeft, ChevronRight, PackageOpen, Search, SearchX, X } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { useProductList } from '../../composables/useProductList'
import { vOriginalListMotion } from '../../utils/originalListMotion'
import { usePageSeo } from '../../composables/usePageSeo'
import { useLocalized } from '../../composables/useProduct'
import { getImageUrl } from '../../utils/image'
import type { PublicCategory } from '../../utils/category'
import VaultProductCard from './components/VaultProductCard.vue'
import VaultProductListCard from './components/VaultProductListCard.vue'
import VaultCategorySidebar from './components/VaultCategorySidebar.vue'
import ProductQuickBuy from '../../components/ProductQuickBuy.vue'
import { useAppStore } from '../../stores/app'

const { t } = useI18n()
const route = useRoute()
const appStore = useAppStore()
const productCatalogLayout = computed(() => appStore.productCatalogLayout)
const { getLocalizedText } = useLocalized()

const quickBuyProduct = ref<any>(null)
const quickBuyVisible = ref(false)
const openQuickBuy = (product: any) => {
  if (catalogStale.value) return
  quickBuyProduct.value = product
  quickBuyVisible.value = true
}

const {
  loading,
  hasLoadedOnce,
  catalogStale,
  loadError,
  loadProducts,
  contentRevision,
  products,
  selectedCategory,
  searchQuery,
  currentPage,
  totalPages,
  expandedParentIds,
  categoryGroups,
  categoryMap,
  selectCategory,
  toggleParentCategory,
  changePage,
  clearSearch,
  initialize,
  cleanup,
} = useProductList({ pageSize: 12, homeRouteName: 'products' })

const catName = (cat: PublicCategory) => getLocalizedText(cat.name) || cat.slug || ''

const selectedCategoryName = computed(() => {
  if (!selectedCategory.value) return ''
  const cat = categoryMap.value.get(selectedCategory.value)
  return cat ? catName(cat) : ''
})

const pageTitle = computed(() => {
  if (route.name === 'category-products') return selectedCategoryName.value || t('nav.products')
  return t('nav.products')
})

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

// 分页窗口：当前页前后各 2 页
const pageWindow = computed(() => {
  const total = totalPages.value
  const cur = currentPage.value
  const start = Math.max(1, cur - 2)
  const end = Math.min(total, start + 4)
  const realStart = Math.max(1, end - 4)
  const pages: number[] = []
  for (let p = realStart; p <= end; p++) pages.push(p)
  return pages
})

const resetFilters = () => {
  clearSearch()
  selectCategory(null)
}

usePageSeo({
  canonicalPath: () => route.path,
  title: () => pageTitle.value,
})

onMounted(() => { void initialize() })
onUnmounted(() => cleanup())
</script>

<style scoped>
.catalog-status { min-height:1.5rem; color:hsl(var(--muted-foreground)); font-size:.875rem; }
.catalog-feedback { transition:opacity 140ms ease; }
.catalog-stale { opacity:.82; pointer-events:none; }
@media (prefers-reduced-motion: reduce) { .catalog-feedback { transition:none; } }
.list-category-count { display:inline-grid; min-width:1.55rem; height:1.35rem; place-items:center; border-radius:999px; background:hsl(var(--secondary)); padding:0 .42rem; color:hsl(var(--muted-foreground)); font-size:.7rem; font-weight:700; }
</style>
