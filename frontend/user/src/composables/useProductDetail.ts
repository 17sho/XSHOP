import { ref, onMounted, onUnmounted, computed, watch, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useHead } from '@unhead/vue'
import { useAppStore } from '../stores/app'
import { getImageUrl } from '../utils/image'
import { useBuyNowStore } from '../stores/buyNow'
import { useUserAuthStore } from '../stores/userAuth'
import { useUserProfileStore } from '../stores/userProfile'
import { debounceAsync } from '../utils/debounce'
import { buildSkuDisplayText, normalizeSkuId } from '../utils/sku'
import { resolveSkuStockDisplay, type PublicStockDisplay } from '../utils/publicStock'
import { useLocalized, useProductLabels } from './useProduct'

import { takeProductDetailRequest } from '../utils/productDetailPrefetch'
import {
  buildProductCheckoutItemSnapshot,
  findSelectedProductSku,
  isProductSkuPurchasable,
  resolveActiveProductSkus,
  resolveProductPurchaseState,
  resolveProductPurchaseViolation,
  resolveProductQuantityBounds,
  resolveProductSkuAvailableStock,

  resolveSelectedProductSkuId,
  resolveSelectedSkuPriceState,
} from '../utils/productPurchase'

/**
 * 商品详情页的全部业务逻辑（数据加载、SKU/数量、促销/会员/批发定价、库存约束、
 * 加购 / 立即购买、SEO/JSON-LD）。classic 与 vault 模板共用此 composable，
 * 仅各自负责 markup，保证功能严格一致。
 *
 * 视图专属的移动端购买条（IntersectionObserver + DOM ref）仍留在各视图中，
 * 通过 options.onLoaded 在商品加载完成后回调以初始化。
 */
export function useProductDetail(options: { onLoaded?: () => void } = {}) {
  const route = useRoute()
  const router = useRouter()
  const { t } = useI18n()
  const appStore = useAppStore()
  const buyNowStore = useBuyNowStore()
  const userAuthStore = useUserAuthStore()
  const userProfileStore = useUserProfileStore()

  const { getLocalizedText, siteCurrency, formatPrice } = useLocalized()
  const {
    getPurchaseTypeLabel, getFulfillmentTypeLabel, getStockBadgeVariant, getStockStatusLabel,
    hasPromotionPrice, getPromotionPriceAmount, getPromotionSaveAmount,
    hasSkuPromotionPrice, getSkuPromotionPriceAmount, getSkuPromotionSaveAmount,
    hasPromotionRules, getPromotionRules, hasWholesalePrices, getWholesalePrices,
    resolveWholesalePriceAmount, resolveMemberPriceAmount,
  } = useProductLabels()

  const formatPromotionRule = (rule: any) => {
    const amount = formatPrice(rule.min_amount, siteCurrency.value)
    const value = rule.type === 'percent' ? String(rule.value) : formatPrice(rule.value, siteCurrency.value)
    const hasMin = Number(rule.min_amount) > 0
    switch (rule.type) {
      case 'percent':
        return hasMin ? t('products.promotionHintPercent', { amount, value }) : t('products.promotionHintPercentNoMin', { value })
      case 'fixed':
        return hasMin ? t('products.promotionHintFixed', { amount, value }) : t('products.promotionHintFixedNoMin', { value })
      case 'special_price':
        return hasMin ? t('products.promotionHintSpecial', { amount, value }) : t('products.promotionHintSpecialNoMin', { value })
      default:
        return rule.name || ''
    }
  }

  const loading = ref(true)
  const product = ref<any>(null)
  const relatedPosts = computed<any[]>(() => product.value?.related_posts || [])
  const formatRelatedPostDate = (dateString: string) => {
    if (!dateString) return ''
    const date = new Date(dateString)
    return date.toLocaleDateString(appStore.locale, { year: 'numeric', month: 'long', day: 'numeric' })
  }
  const currentImage = ref<string>('')
  const selectedSkuId = ref(0)
  const quantity = ref(1)
  const purchaseWarning = ref('')

  const activeSkus = computed(() => {
    return resolveActiveProductSkus(product.value)
  })

  const selectedSku = computed(() => {
    return findSelectedProductSku(activeSkus.value, selectedSkuId.value)
  })

  // 会员价相关
  const userMemberLevelId = computed(() => Number(userAuthStore.user?.member_level_id || 0))

  const currentMemberDiscountRate = computed(() => {
    if (!userMemberLevelId.value) return 0
    const level = userProfileStore.memberLevels.find((item: any) => Number(item?.id || 0) === userMemberLevelId.value)
    return Number(level?.discount_rate || 0)
  })

  const ensureMemberLevels = () => {
    if (userMemberLevelId.value > 0 && userProfileStore.memberLevels.length === 0) {
      void userProfileStore.loadMemberLevels()
    }
  }

  const getMemberPriceForSku = (skuId: number, basePrice: any): number | null => {
    const price = resolveMemberPriceAmount(product.value, skuId, basePrice, userMemberLevelId.value, currentMemberDiscountRate.value)
    return price === null ? null : Number(price)
  }

  const selectedSkuMemberPrice = computed(() => {
    if (!selectedSku.value) return null
    const skuId = normalizeSkuId(selectedSku.value.id)
    return getMemberPriceForSku(skuId, selectedSku.value.price_amount)
  })

  const hasMemberPrice = computed(() => {
    if (!selectedSkuMemberPrice.value) return false
    const basePrice = Number(selectedSku.value?.price_amount || 0)
    return selectedSkuMemberPrice.value < basePrice
  })

  const selectedSkuWholesaleRules = computed(() => {
    if (!product.value || !selectedSku.value) return []
    return getWholesalePrices(
      product.value,
      normalizeSkuId(selectedSku.value.id),
      selectedSku.value.sku_code,
    )
  })

  const selectedSkuWholesalePrice = computed(() => {
    if (!product.value || !selectedSku.value) return null
    return resolveWholesalePriceAmount(
      product.value,
      selectedSku.value.price_amount,
      quantity.value,
      normalizeSkuId(selectedSku.value.id),
      selectedSku.value.sku_code,
      quantity.value,
    )
  })

  const selectedSkuWholesaleMemberPrice = computed(() => {
    if (!product.value || !selectedSku.value || !selectedSkuWholesalePrice.value) return null
    const skuId = normalizeSkuId(selectedSku.value.id)
    return getMemberPriceForSku(skuId, selectedSkuWholesalePrice.value)
  })

  const selectedSkuPromotionPrice = computed(() => {
    if (!selectedSku.value || !hasSkuPromotionPrice(selectedSku.value)) return null
    return getSkuPromotionPriceAmount(selectedSku.value)
  })

  const selectedSkuPromotionMemberPrice = computed(() => {
    if (!product.value || !selectedSku.value || selectedSkuPromotionPrice.value === null) return null
    const skuId = normalizeSkuId(selectedSku.value.id)
    return getMemberPriceForSku(skuId, selectedSkuPromotionPrice.value)
  })

  const selectedSkuPriceState = computed(() => {
    if (!selectedSku.value) return null
    return resolveSelectedSkuPriceState({
      basePrice: selectedSku.value.price_amount,
      promotionPrice: selectedSkuPromotionPrice.value,
      wholesalePrice: selectedSkuWholesalePrice.value,
      wholesaleMemberPrice: selectedSkuWholesaleMemberPrice.value,
      promotionMemberPrice: selectedSkuPromotionMemberPrice.value,
      hasBaseMemberPrice: hasMemberPrice.value,
    })
  })
  const hasSelectedSkuWholesalePrice = computed(() => selectedSkuPriceState.value?.hasWholesalePrice ?? false)
  const selectedSkuWholesaleFinalIsMember = computed(() => selectedSkuPriceState.value?.wholesaleFinalIsMember ?? false)
  const selectedSkuWholesaleFinalPrice = computed(() => selectedSkuPriceState.value?.wholesaleFinalPrice ?? null)
  const selectedSkuPromotionFinalIsMember = computed(() => selectedSkuPriceState.value?.promotionFinalIsMember ?? false)
  const selectedSkuPromotionFinalPrice = computed(() => selectedSkuPriceState.value?.promotionFinalPrice ?? null)
  const showSelectedSkuMemberBadge = computed(() => selectedSkuPriceState.value?.showMemberBadge ?? false)

  const formatWholesaleTier = (tier: any) => {
    return t('products.wholesaleTier', {
      count: Number(tier?.min_quantity || 0),
      price: formatPrice(tier?.unit_price, siteCurrency.value),
    })
  }

  const skuAvailableStock = (sku: any) => {
    return resolveProductSkuAvailableStock(product.value, sku)
  }

  const isSkuPurchasable = (sku: any) => {
    return isProductSkuPurchasable(skuAvailableStock(sku))
  }

  const formatSkuStockDisplay = (display: PublicStockDisplay) => {
    switch (display.kind) {
      case 'unlimited':
        return t('productDetail.skuStockUnlimited')
      case 'out':
        return t('productDetail.skuStockOut')
      case 'remaining':
        return t('productDetail.skuStockRemaining', { count: display.count })
      case 'low_stock':
        return t('productDetail.skuStockLow')
      case 'hidden':
        return t('productDetail.skuStockHidden')
      case 'range':
        return t('productDetail.skuStockRange', { min: display.min, max: display.max })
      case 'range_plus':
        return t('productDetail.skuStockRangePlus', { min: display.min })
      case 'in_stock':
      default:
        return t('productDetail.skuStockInStock')
    }
  }

  const skuStockText = (sku: any) => {
    const display = resolveSkuStockDisplay(product.value, sku)
    return formatSkuStockDisplay(display)
  }

  const skuStockBadgeClass = (sku: any) => {
    const display = resolveSkuStockDisplay(product.value, sku)
    if (display.kind === 'unlimited' || display.kind === 'hidden') return 'border-slate-200 text-slate-600 dark:border-slate-700 dark:text-slate-300'
    if (display.kind === 'out') return 'border-rose-200 bg-rose-50 text-rose-700 dark:border-rose-700 dark:bg-rose-950/30 dark:text-rose-300'
    if (display.kind === 'low_stock' || (display.kind === 'range' && display.max <= 5)) return 'border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-700 dark:bg-amber-950/30 dark:text-amber-300'
    return 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-700 dark:bg-emerald-950/30 dark:text-emerald-300'
  }

  const quantityEffectiveLimit = computed(() => {
    return resolveProductQuantityBounds(product.value, skuAvailableStock(selectedSku.value)).limit
  })

  const quantityEffectiveMin = computed(() => {
    return resolveProductQuantityBounds(product.value, skuAvailableStock(selectedSku.value)).minimum
  })

  const handleQuantityInput = (event: Event) => {
    const val = parseInt((event.target as HTMLInputElement).value, 10)
    const minimum = quantityEffectiveMin.value
    if (isNaN(val) || val < minimum) {
      quantity.value = minimum
    } else if (quantityEffectiveLimit.value !== null && val > quantityEffectiveLimit.value) {
      quantity.value = quantityEffectiveLimit.value
    } else {
      quantity.value = val
    }
  }

  const purchaseType = computed(() => product.value?.purchase_type || 'member')
  const requiresLogin = computed(() => purchaseType.value === 'member' && !userAuthStore.isAuthenticated)
  const productPurchaseState = computed(() => resolveProductPurchaseState({
    product: product.value,
    activeSkuCount: activeSkus.value.length,
    hasSelectedSku: Boolean(selectedSku.value),
    selectedSkuPurchasable: selectedSku.value ? isSkuPurchasable(selectedSku.value) : true,
    minimum: quantityEffectiveMin.value,
    limit: quantityEffectiveLimit.value,
  }))
  const requiresSKUSelection = computed(() => productPurchaseState.value.requiresSkuSelection)
  const stockBelowMinPurchase = computed(() => productPurchaseState.value.stockBelowMinimum)
  const canPurchase = computed(() => productPurchaseState.value.canPurchase)
  const cannotPurchaseReason = computed(() => {
    if (!product.value) return ''
    if (requiresLogin.value) return ''
    if (requiresSKUSelection.value) return t('productDetail.skuRequired')
    if (stockBelowMinPurchase.value) return t('productDetail.stockBelowMinPurchase', { count: quantityEffectiveMin.value })
    if (canPurchase.value) return ''
    return t('productDetail.stockUnavailable')
  })
  const categoryName = computed(() => {
    const category = product.value?.category?.name
    return category ? getLocalizedText(category) : ''
  })

  const images = computed(() => {
    if (!product.value?.images) return []
    let imageArray: string[] = []
    if (Array.isArray(product.value.images)) {
      imageArray = product.value.images
    } else if (product.value.images.images && Array.isArray(product.value.images.images)) {
      imageArray = product.value.images.images
    }
    return imageArray.map(img => getImageUrl(img))
  })

  const skuDisplayText = (sku: any) => {
    return buildSkuDisplayText({
      skuCode: sku?.sku_code,
      specValues: sku?.spec_values,
      fallback: t('productDetail.skuFallback'),
      locale: appStore.locale,
    })
  }

  const syncSelectedSku = () => {
    selectedSkuId.value = resolveSelectedProductSkuId({
      activeSkus: activeSkus.value,
      currentSkuId: selectedSkuId.value,
      preserveCurrent: true,
      isPurchasable: isSkuPurchasable,
    })
  }


  const buildItemPayload = (sku: any) => buildProductCheckoutItemSnapshot({
    product: product.value,
    sku,
    image: images.value[0],
    quantity: quantity.value,
  })


  const buyNow = () => {
    purchaseWarning.value = ''
    if (!canPurchase.value) return
    if (!product.value) return
    if (requiresLogin.value) {
      router.push(`/auth/login?redirect=${encodeURIComponent(route.fullPath)}`)
      return
    }

    const sku = selectedSku.value
    const available = skuAvailableStock(sku)
    const violation = resolveProductPurchaseViolation({ product: product.value, availableStock: available, requestedQuantity: quantity.value })
    if (violation) {
      purchaseWarning.value = violation.kind === 'stock'
        ? (violation.count > 0 ? t('productDetail.addCartStockExceeded', { count: violation.count }) : t('productDetail.stockUnavailable'))
        : t('productDetail.addCartLimitExceeded', { count: violation.count })
      return
    }

    buyNowStore.setItem(buildItemPayload(sku))
    router.push('/checkout')
  }

  // 移动端购买条价格展示
  const mobileBarShowMemberPrice = computed(() => {
    if (!selectedSku.value) return false
    if (hasSelectedSkuWholesalePrice.value) return selectedSkuWholesaleFinalIsMember.value
    if (hasSkuPromotionPrice(selectedSku.value)) return selectedSkuPromotionFinalIsMember.value
    return hasMemberPrice.value
  })
  const mobileBarMemberPriceDisplay = computed(() => {
    if (hasSelectedSkuWholesalePrice.value && selectedSkuWholesaleFinalIsMember.value) {
      return formatPrice(selectedSkuWholesaleFinalPrice.value, siteCurrency.value)
    }
    if (selectedSku.value && hasSkuPromotionPrice(selectedSku.value) && selectedSkuPromotionFinalIsMember.value) {
      return formatPrice(selectedSkuPromotionFinalPrice.value, siteCurrency.value)
    }
    if (!selectedSkuMemberPrice.value) return ''
    return formatPrice(selectedSkuMemberPrice.value, siteCurrency.value)
  })
  const mobileBarShowSkuPromotionPrice = computed(() => {
    if (mobileBarShowMemberPrice.value) return false
    if (hasSelectedSkuWholesalePrice.value) return false
    return !!selectedSku.value && hasSkuPromotionPrice(selectedSku.value)
  })
  const mobileBarSkuPromotionPriceDisplay = computed(() => {
    if (!selectedSku.value) return ''
    return formatPrice(getSkuPromotionPriceAmount(selectedSku.value), siteCurrency.value)
  })
  const mobileBarShowSkuPrice = computed(() => {
    if (mobileBarShowMemberPrice.value || mobileBarShowSkuPromotionPrice.value) return false
    return !!selectedSku.value
  })
  const mobileBarSkuPriceDisplay = computed(() => {
    if (!selectedSku.value) return ''
    return formatPrice(selectedSku.value.price_amount, siteCurrency.value)
  })
  const mobileBarShowProductPromotionPrice = computed(() => {
    if (selectedSku.value) return false
    return product.value ? hasPromotionPrice(product.value) : false
  })
  const mobileBarProductPromotionPriceDisplay = computed(() => {
    if (!product.value) return ''
    return formatPrice(getPromotionPriceAmount(product.value), siteCurrency.value)
  })
  const mobileBarProductPriceDisplay = computed(() => {
    if (!product.value) return ''
    return formatPrice(product.value.price_amount, siteCurrency.value)
  })

  const goLogin = () => {
    router.push(`/auth/login?redirect=${encodeURIComponent(route.fullPath)}`)
  }

  let disposed = false
  let productRequest = 0
  const loadProduct = async () => {
    if (disposed) return
    const request = ++productRequest
    const slug = route.params.slug as string
    const current = () => !disposed && request === productRequest && slug === route.params.slug
    loading.value = true
    try {
      const data = await takeProductDetailRequest(slug)
      if (!current()) return
      product.value = data
      if (images.value.length > 0) {
        currentImage.value = images.value[0] || ''
      }
      syncSelectedSku()
      loading.value = false
      await nextTick()
      if (current()) options.onLoaded?.()
    } catch (error) {
      if (!current()) return
      console.error('Failed to load product:', error)
      product.value = null
      selectedSkuId.value = 0
    } finally {
      if (current()) loading.value = false
    }
  }

  const debouncedLoadProduct = debounceAsync(loadProduct, 300)

  const canonicalUrl = computed(() => {
    if (!product.value?.slug) return ''
    const fromConfig = String(appStore.config?.brand?.site_url || '').trim().replace(/\/+$/, '')
    const base = fromConfig || window.location.origin.replace(/\/+$/, '')
    return `${base}/products/${product.value.slug}`
  })

  useHead({
    title: () => product.value ? getLocalizedText(product.value.title) : '',
    link: () => {
      if (!canonicalUrl.value) return []
      return [{ rel: 'canonical', href: canonicalUrl.value }]
    },
    meta: () => {
      if (!product.value) return []
      const seoMeta = product.value.seo_meta || {}
      const seoKeywords = getLocalizedText(seoMeta.keywords) || (typeof seoMeta.keywords === 'string' ? seoMeta.keywords : '')
      const seoDescription = getLocalizedText(seoMeta.description) || (typeof seoMeta.description === 'string' ? seoMeta.description : '')
      const tags = []

      if (seoKeywords) tags.push({ name: 'keywords', content: seoKeywords })
      if (seoDescription) tags.push({ name: 'description', content: seoDescription })

      tags.push({ property: 'og:type', content: 'product' })
      if (product.value.title) {
        tags.push({ property: 'og:title', content: getLocalizedText(product.value.title) })
      }
      if (seoDescription) {
        tags.push({ property: 'og:description', content: seoDescription })
      }
      if (images.value && images.value.length > 0) {
        tags.push({ property: 'og:image', content: images.value[0] })
      }
      if (canonicalUrl.value) {
        tags.push({ property: 'og:url', content: canonicalUrl.value })
      }

      tags.push({ name: 'twitter:card', content: 'summary_large_image' })
      if (product.value.title) {
        tags.push({ name: 'twitter:title', content: getLocalizedText(product.value.title) })
      }
      if (seoDescription) {
        tags.push({ name: 'twitter:description', content: seoDescription })
      }
      if (images.value && images.value.length > 0) {
        tags.push({ name: 'twitter:image', content: images.value[0] })
      }

      return tags
    },
    script: () => {
      if (!product.value) return []
      const title = getLocalizedText(product.value.title)
      const seoMeta = product.value.seo_meta || {}
      const description = getLocalizedText(seoMeta.description) || (typeof seoMeta.description === 'string' ? seoMeta.description : '')
      const priceAmount = product.value.price_amount || '0'
      const currency = siteCurrency.value || 'CNY'

      const jsonLd: Record<string, any> = {
        '@context': 'https://schema.org',
        '@type': 'Product',
        name: title,
        url: canonicalUrl.value || window.location.href,
        offers: {
          '@type': 'Offer',
          price: priceAmount,
          priceCurrency: currency,
          availability: product.value.stock_status === 'out_of_stock'
            ? 'https://schema.org/OutOfStock'
            : 'https://schema.org/InStock',
        },
      }
      if (description) jsonLd.description = description
      if (images.value.length > 0) jsonLd.image = images.value
      if (product.value.category?.name) {
        jsonLd.category = getLocalizedText(product.value.category.name)
      }

      return [{
        type: 'application/ld+json',
        innerHTML: JSON.stringify(jsonLd),
      }]
    },
  })

  watch(() => route.params.slug, () => {
    debouncedLoadProduct.cancel()
    product.value = null
    currentImage.value = ''
    selectedSkuId.value = 0
    quantity.value = 1
    purchaseWarning.value = ''
    void loadProduct()
  }, { flush: 'sync' })

  const initialProductRequest = loadProduct()

  onMounted(() => {
    void initialProductRequest
    ensureMemberLevels()
  })

  watch(userMemberLevelId, () => {
    ensureMemberLevels()
  })

  watch(
    () => selectedSkuId.value,
    () => {
      purchaseWarning.value = ''
      quantity.value = quantityEffectiveMin.value
    }
  )

  watch(quantityEffectiveMin, (minimum) => {
    if (minimum > quantity.value) {
      quantity.value = minimum
    }
  })

  watch(quantityEffectiveLimit, (limit) => {
    if (limit !== null && quantity.value > limit) {
      quantity.value = Math.max(quantityEffectiveMin.value, limit)
    }
  })

  onUnmounted(() => {
    disposed = true
    ++productRequest
    debouncedLoadProduct.cancel()
  })

  return {
    // 国际化/格式化（模板需要）
    getLocalizedText, siteCurrency, formatPrice,
    getPurchaseTypeLabel, getFulfillmentTypeLabel, getStockBadgeVariant, getStockStatusLabel,
    hasPromotionPrice, getPromotionPriceAmount, getPromotionSaveAmount,
    hasSkuPromotionPrice, getSkuPromotionPriceAmount, getSkuPromotionSaveAmount,
    hasPromotionRules, getPromotionRules, hasWholesalePrices, getWholesalePrices,
    formatPromotionRule, formatWholesaleTier, formatRelatedPostDate,
    normalizeSkuId,
    // 状态
    loading, product, relatedPosts, currentImage, selectedSkuId, quantity, purchaseWarning,
    activeSkus, selectedSku,
    // 价格计算
    selectedSkuMemberPrice, hasMemberPrice,
    hasSelectedSkuWholesalePrice, selectedSkuWholesaleFinalIsMember, selectedSkuWholesaleFinalPrice,
    selectedSkuWholesaleRules,
    selectedSkuPromotionPrice, selectedSkuPromotionFinalIsMember, selectedSkuPromotionFinalPrice,
    showSelectedSkuMemberBadge,
    // SKU / 库存 / 数量
    isSkuPurchasable, skuDisplayText, skuStockText, skuStockBadgeClass,
    quantityEffectiveLimit, quantityEffectiveMin, handleQuantityInput,
    // 购买能力
    purchaseType, requiresLogin, requiresSKUSelection, canPurchase, cannotPurchaseReason,
    categoryName, images,
    // 动作
    buyNow, goLogin, loadProduct,
    // 移动端购买条
    mobileBarShowMemberPrice, mobileBarMemberPriceDisplay,
    mobileBarShowSkuPromotionPrice, mobileBarSkuPromotionPriceDisplay,
    mobileBarShowSkuPrice, mobileBarSkuPriceDisplay,
    mobileBarShowProductPromotionPrice, mobileBarProductPromotionPriceDisplay, mobileBarProductPriceDisplay,
  }
}
