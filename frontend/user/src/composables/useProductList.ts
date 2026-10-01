import { computed, ref, shallowRef, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { productAPI, categoryAPI } from '../api'
import { buildCategoryGroups, createCategoryMap, normalizeCategoryParentId, type PublicCategory } from '../utils/category'
import { debounceAsync } from '../utils/debounce'
import { readPublicCatalogCache, writePublicCatalogCache } from '../utils/publicCatalogCache'
import { getBrowserStorage } from '../utils/browserStorage'
import { scrollPageToTop } from '../utils/motionPolicy'

export interface UseProductListOptions {
  pageSize?: number
  homeRouteName?: string
  categoryRouteName?: string
}

export function useProductList(options: UseProductListOptions = {}) {
  const {
    pageSize: defaultPageSize = 20,
    homeRouteName = 'home',
    categoryRouteName = 'category-products',
  } = options

  const router = useRouter()
  const route = useRoute()

  const loading = ref(true)
  const hasLoadedOnce = ref(false)
  const loadError = ref(false)
  // Presentation only: never advance for pending intent or an obsolete response.
  const contentRevision = ref(0)
  const products = ref<any[]>([])
  const categories = ref<PublicCategory[]>([])
  const selectedCategory = ref<number | null>(null)
  const searchQuery = ref('')
  const currentPage = ref(1)
  const pageSize = ref(defaultPageSize)
  const totalPages = ref(0)
  const showFilterDrawer = ref(false)
  const expandedParentIds = ref<number[]>([])

  const categoryGroups = computed(() => buildCategoryGroups(categories.value))
  const categoryMap = computed(() => createCategoryMap(categories.value))

  type CategoryIntent = { kind: 'route'; slug: string | undefined } | { kind: 'selection'; id: number | null }
  const routeCategoryIntent = (): CategoryIntent => route.name === categoryRouteName
    ? { kind: 'route', slug: route.params.slug as string | undefined }
    : { kind: 'selection', id: null }
  // Each route or explicit selection replaces the previous intent, even during initialization.
  const categoryIntent = shallowRef(routeCategoryIntent())
  const intendedCategory = () => {
    const intent = categoryIntent.value
    return intent.kind === 'selection' ? intent.id : categories.value.find(category => category.slug === intent.slug)?.id
  }
  let disposed = false
  let productRequestId = 0
  let refreshCatalogSilently = false
  let refreshCategoriesLoaded = false
  const captureQuery = () => ({
    categoryId: selectedCategory.value,
    search: searchQuery.value.trim(),
    page: currentPage.value,
    pageSize: pageSize.value,
  })
  type Query = ReturnType<typeof captureQuery>
  type ProductSnapshot = { requestId: number; query: Query; products: any[]; totalPages: number }
  const productSnapshot = shallowRef<ProductSnapshot>()
  const sameQuery = (a: Query, b: Query) => a.categoryId === b.categoryId && a.search === b.search
    && a.page === b.page && a.pageSize === b.pageSize
  const ownsSnapshot = (snapshot: ProductSnapshot) => !disposed && snapshot.requestId === productRequestId
    && sameQuery(snapshot.query, captureQuery()) && snapshot.query.categoryId === intendedCategory()
  // Retained rows and cached rows are not authority. Only the captured response
  // matching the actual query and resolved latest intent can enable actions.
  const catalogStale = computed(() => !productSnapshot.value || !ownsSnapshot(productSnapshot.value))
  const catalogCacheStorage = getBrowserStorage('sessionStorage')
  const catalogCacheHost = typeof window === 'undefined' ? '' : window.location.host
  const canUseCatalogCache = () => route.name === homeRouteName

  const writeCatalogCache = () => {
    const snapshot = productSnapshot.value
    if (!snapshot || !refreshCategoriesLoaded || !canUseCatalogCache() || !ownsSnapshot(snapshot)) return
    if (snapshot.query.page !== 1 || snapshot.query.categoryId !== null || snapshot.query.search) return
    writePublicCatalogCache(catalogCacheStorage, catalogCacheHost, {
      products: snapshot.products,
      categories: categories.value,
      totalPages: snapshot.totalPages,
    })
  }

  const isParentExpanded = (categoryId: number) => {
    return expandedParentIds.value.includes(categoryId)
  }

  const expandParentCategory = (categoryId: number) => {
    if (!categoryId || isParentExpanded(categoryId)) return
    expandedParentIds.value = [...expandedParentIds.value, categoryId]
  }

  const toggleParentCategory = (categoryId: number) => {
    if (isParentExpanded(categoryId)) {
      expandedParentIds.value = expandedParentIds.value.filter((id) => id !== categoryId)
      return
    }
    expandParentCategory(categoryId)
  }

  const getParentToggleButtonClass = (categoryId: number) => {
    return isParentExpanded(categoryId)
      ? 'bg-primary text-primary-foreground border-transparent'
      : 'bg-card/70 text-muted-foreground hover:text-foreground'
  }

  const syncExpandedCategoryState = () => {
    if (!selectedCategory.value) return

    const matched = categoryMap.value.get(selectedCategory.value)
    if (!matched) return

    const parentId = normalizeCategoryParentId(matched.parent_id)
    if (parentId > 0) {
      expandParentCategory(parentId)
      return
    }

    const selectedGroup = categoryGroups.value.find((group) => group.id === matched.id)
    if (selectedGroup?.children.length) {
      expandParentCategory(selectedGroup.id)
    }
  }

  const selectCategory = (categoryId: number | null, closeDrawer = false) => {
    // Even selecting the already-active All option is explicit user intent.
    categoryIntent.value = { kind: 'selection', id: categoryId }
    if (selectedCategory.value === categoryId) syncCategoryRoute()
    selectedCategory.value = categoryId
    if (closeDrawer) {
      showFilterDrawer.value = false
    }
  }

  const loadProducts = async () => {
    if (disposed) return
    const requestId = ++productRequestId
    const query = captureQuery()
    loadError.value = false
    productSnapshot.value = undefined
    const showLoading = !refreshCatalogSilently
    refreshCatalogSilently = false
    if (showLoading) loading.value = true
    try {
      const params: any = {
        page: query.page,
        page_size: query.pageSize,
      }
      if (query.categoryId) {
        params.category_id = query.categoryId
      }
      if (query.search) {
        params.search = query.search
      }
      const response = await productAPI.list(params)
      if (requestId !== productRequestId) return
      if (!sameQuery(query, captureQuery())) return
      const snapshot = { requestId, query, products: response.data.data || [], totalPages: response.data.pagination?.total_page || 0 }
      products.value = snapshot.products
      totalPages.value = snapshot.totalPages
      productSnapshot.value = snapshot
      if (hasLoadedOnce.value && ownsSnapshot(snapshot)) contentRevision.value++
      writeCatalogCache()
    } catch (error) {
      if (requestId !== productRequestId) return
      loadError.value = true
      console.error('Failed to load products:', error)
    } finally {
      if (requestId === productRequestId) {
        loading.value = false
        hasLoadedOnce.value = true
      }
    }
  }

  const loadCategories = async () => {
    try {
      const response = await categoryAPI.list()
      if (disposed) return
      categories.value = response.data.data || []
      refreshCategoriesLoaded = true
      writeCatalogCache()
    } catch (error) {
      console.error('Failed to load categories:', error)
    }
  }

  const debouncedLoadProducts = debounceAsync(loadProducts, 300)

  const changePage = (page: number) => {
    if (page < 1 || page > totalPages.value) return
    currentPage.value = page
    debouncedLoadProducts.cancel()
    void loadProducts()
    scrollPageToTop()
  }

  const clearSearch = () => {
    if (!searchQuery.value) return
    searchQuery.value = ''
    currentPage.value = 1
    debouncedLoadProducts()
  }

  const onSearch = () => {
    currentPage.value = 1
    debouncedLoadProducts()
  }

  const syncSelectedCategoryFromRoute = () => {
    const categoryId = intendedCategory()
    if (categoryId === undefined) return false
    selectedCategory.value = categoryId
    return categoryIntent.value.kind === 'route'
  }

  const syncCategoryRoute = () => {
    if (categoryIntent.value.kind !== 'selection') return
    if (route.name !== homeRouteName) {
      if (selectedCategory.value) {
        const matched = categories.value.find((category) => category.id === selectedCategory.value)
        if (matched?.slug && route.params.slug !== matched.slug) {
          router.replace({ name: categoryRouteName, params: { slug: matched.slug } })
        }
      } else if (route.name === categoryRouteName) {
        router.replace({ name: homeRouteName })
      }
    }
  }

  watch(selectedCategory, () => {
    // Direct ref consumers must not turn route reconciliation into a local click.
    if (selectedCategory.value !== intendedCategory()) {
      categoryIntent.value = { kind: 'selection', id: selectedCategory.value }
    }
    currentPage.value = 1
    syncExpandedCategoryState()
    debouncedLoadProducts.cancel()
    void loadProducts()
    syncCategoryRoute()
  }, {
    flush: 'sync',
  })

  watch(searchQuery, () => {
    // Search is interactive even while the initial categories are pending.
    // Invalidate the old request before the debounce fires, not after it.
    productRequestId++
    productSnapshot.value = undefined
    loadError.value = false
    loading.value = true
    currentPage.value = 1
    debouncedLoadProducts()
  }, { flush: 'sync' })

  watch(
    [() => route.name, () => route.params.slug],
    () => {
      categoryIntent.value = routeCategoryIntent()
      syncSelectedCategoryFromRoute()
    },
    { flush: 'sync' },
  )

  const initialize = async () => {
    if (canUseCatalogCache()) {
      const cached = readPublicCatalogCache(catalogCacheStorage, catalogCacheHost)
      if (cached) {
        products.value = cached.products
        categories.value = cached.categories
        totalPages.value = cached.totalPages
        hasLoadedOnce.value = true
        loading.value = false
        refreshCatalogSilently = true
      }
    }
    productSnapshot.value = undefined
    refreshCategoriesLoaded = false
    const categoriesRequest = loadCategories()
    const productsRequest = loadProducts()
    await Promise.all([categoriesRequest, productsRequest])
    if (disposed) return
    syncSelectedCategoryFromRoute()
    syncExpandedCategoryState()
  }

  const cleanup = () => {
    disposed = true
    productRequestId++
    productSnapshot.value = undefined
    debouncedLoadProducts.cancel()
  }

  return {
    loading,
    hasLoadedOnce,
    catalogStale,
    loadError,
    contentRevision,
    products,
    categories,
    selectedCategory,
    searchQuery,
    currentPage,
    pageSize,
    totalPages,
    showFilterDrawer,
    expandedParentIds,
    categoryGroups,
    categoryMap,
    isParentExpanded,
    toggleParentCategory,
    getParentToggleButtonClass,
    selectCategory,
    loadProducts,
    loadCategories,
    changePage,
    clearSearch,
    onSearch,
    syncSelectedCategoryFromRoute,
    syncExpandedCategoryState,
    initialize,
    cleanup,
  }
}
