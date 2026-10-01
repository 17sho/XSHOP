<template>
  <Teleport to="body">
    <Transition name="quick-buy" :duration="transitionDuration(visible)" appear @before-enter="beforeEnter" @before-leave="beforeLeave">
      <div
        v-if="visible"
        data-overlay-root
        class="fixed inset-0 z-50 flex items-end justify-center bg-black/40 backdrop-blur-[2px] md:items-center md:p-6"
        @click.self="close"
      >
        <div
          ref="dialogRef"
          data-overlay-panel
          role="dialog"
          aria-modal="true"
          :aria-label="t('quickBuy.title')"
          tabindex="-1"
          class="w-full max-h-[85vh] flex flex-col rounded-t-2xl md:rounded-2xl bg-card backdrop-blur-xl text-card-foreground border-t md:border shadow-2xl md:max-w-md md:max-h-[75vh]"
        >
          <!-- Mobile drag handle -->
          <div class="md:hidden flex justify-center pt-2.5 pb-1 shrink-0">
            <div class="w-8 h-1 rounded-full bg-gray-300 dark:bg-gray-600" />
          </div>

          <!-- Desktop header bar -->
          <div class="hidden md:flex items-center justify-between px-5 pt-4 pb-0 shrink-0">
            <h2 class="text-sm font-semibold text-foreground">{{ t('quickBuy.title') }}</h2>
            <Button type="button" variant="ghost" size="icon" class="-mr-1 text-muted-foreground" @click="close">
              <X />
            </Button>
          </div>

          <!-- Scrollable content area -->
          <div class="overflow-y-auto overscroll-contain flex-1 px-4 md:px-5 md:pt-4">
            <!-- Product header -->
            <div class="flex gap-3.5 mb-4">
              <!-- Image -->
              <div
                class="w-[90px] h-[90px] md:w-[100px] md:h-[100px] rounded-xl md:rounded-2xl overflow-hidden shrink-0 border cursor-pointer"
                @click="goToDetail"
              >
                <img
                  v-if="productImage"
                  :src="productImage"
                  :alt="productTitle"
                  class="w-full h-full object-cover"
                />
                <div v-else class="w-full h-full flex items-center justify-center bg-muted">
                  <ImageIcon class="w-7 h-7 text-muted-foreground" :stroke-width="1.5" />
                </div>
              </div>

              <!-- Info -->
              <div class="flex-1 min-w-0 flex flex-col">
                <h3
                  class="text-sm md:text-[15px] font-semibold text-foreground line-clamp-2 leading-snug cursor-pointer hover:underline decoration-gray-300 dark:decoration-gray-600 underline-offset-2"
                  @click="goToDetail"
                >
                  {{ productTitle }}
                </h3>

                <div class="mt-1.5 flex flex-wrap items-center gap-1">
                  <Badge :variant="product.fulfillment_type === 'auto' ? 'info' : 'neutral'" size="xs">
                    {{ getFulfillmentTypeLabel(product.fulfillment_type) }}
                  </Badge>
                  <Badge :variant="getStockBadgeVariant(product.stock_status)" size="xs">
                    {{ getStockStatusLabel(product) }}
                  </Badge>
                  <Badge v-if="hasSelectedSkuWholesalePrice" variant="success" size="xs">
                    {{ t('products.wholesaleTag') }}
                  </Badge>
                  <Badge v-if="showSelectedSkuMemberBadge" variant="warning" size="xs">
                    {{ t('products.memberPriceTag') }}
                  </Badge>
                </div>

                <!-- Price -->
                <div class="mt-auto pt-1">
                  <template v-if="selectedSku && hasSelectedSkuWholesalePrice">
                    <span
                      class="text-lg md:text-xl font-bold"
                      :class="selectedSkuWholesaleFinalIsMember ? 'text-amber-600 dark:text-amber-300' : 'text-emerald-600 dark:text-emerald-400'"
                    >
                      {{ formatPrice(selectedSkuWholesaleFinalPrice!, siteCurrency) }}
                    </span>
                    <span class="ml-1.5 text-xs text-muted-foreground line-through">
                      {{ formatPrice(selectedSku.price_amount, siteCurrency) }}
                    </span>
                  </template>
                  <template v-else-if="selectedSku && hasSkuPromotionPrice(selectedSku)">
                    <span
                      class="text-lg md:text-xl font-bold"
                      :class="selectedSkuPromotionFinalIsMember ? 'text-amber-600 dark:text-amber-300' : 'text-rose-600 dark:text-rose-400'"
                    >
                      {{ formatPrice(selectedSkuPromotionFinalPrice!, siteCurrency) }}
                    </span>
                    <span class="ml-1.5 text-xs text-muted-foreground line-through">
                      {{ formatPrice(selectedSku.price_amount, siteCurrency) }}
                    </span>
                  </template>
                  <template v-else-if="selectedSku && hasMemberPrice">
                    <span class="text-lg md:text-xl font-bold text-amber-600 dark:text-amber-300">
                      {{ formatPrice(selectedSkuMemberPrice!, siteCurrency) }}
                    </span>
                    <span class="ml-1.5 text-xs text-muted-foreground line-through">
                      {{ formatPrice(selectedSku.price_amount, siteCurrency) }}
                    </span>
                  </template>
                  <template v-else-if="selectedSku">
                    <span class="text-lg md:text-xl font-bold text-primary">
                      {{ formatPrice(selectedSku.price_amount, siteCurrency) }}
                    </span>
                  </template>
                  <template v-else-if="hasPromotionPrice(product)">
                    <span class="text-lg md:text-xl font-bold text-rose-600 dark:text-rose-400">
                      {{ formatPrice(getPromotionPriceAmount(product), siteCurrency) }}
                    </span>
                    <span class="ml-1.5 text-xs text-muted-foreground line-through">
                      {{ formatPrice(product.price_amount, siteCurrency) }}
                    </span>
                  </template>
                  <template v-else>
                    <span class="text-lg md:text-xl font-bold text-primary">
                      {{ formatPrice(product.price_amount, siteCurrency) }}
                    </span>
                  </template>
                </div>
              </div>

              <!-- Mobile close -->
              <Button type="button" variant="ghost" size="icon" class="md:hidden self-start -mr-1 text-muted-foreground" @click="close">
                <X />
              </Button>
            </div>

            <!-- Divider -->
            <div class="h-px bg-border -mx-4 md:-mx-5 mb-4" />

            <!-- Product description -->
            <p v-if="productDescription" class="mb-4 text-xs leading-relaxed text-muted-foreground line-clamp-3">
              {{ productDescription }}
            </p>

            <!-- Promotion rules -->
            <div v-if="selectedSkuWholesaleRules.length" class="mb-4 rounded-lg border border-emerald-200 bg-emerald-50/50 px-3 py-2 dark:border-emerald-800/50 dark:bg-emerald-950/20">
              <div class="mb-1 text-[11px] font-semibold text-emerald-700 dark:text-emerald-300">
                {{ t('products.wholesaleRulesTitle') }}
              </div>
              <div class="flex flex-wrap gap-1">
                <Badge v-for="tier in selectedSkuWholesaleRules" :key="`${tier.sku_id || tier.sku_code || 'all'}-${tier.min_quantity}`" variant="success" size="xs" class="rounded-full">
                  {{ formatWholesaleTier(tier) }}
                </Badge>
              </div>
            </div>

            <div v-if="hasPromotionRules(product)" class="mb-4 rounded-lg border border-orange-200 dark:border-orange-800/50 bg-orange-50/50 dark:bg-orange-950/20 px-3 py-2">
              <div class="flex items-center gap-1 mb-1">
                <Tag class="w-3.5 h-3.5 text-orange-500 dark:text-orange-400 shrink-0" />
                <span class="text-[11px] font-semibold text-orange-700 dark:text-orange-300">
                  {{ t('products.promotionRulesTitle') }}
                </span>
              </div>
              <ul class="space-y-0.5">
                <li v-for="rule in getPromotionRules(product)" :key="rule.id" class="text-[11px] text-orange-600 dark:text-orange-300/90 flex items-center gap-1">
                  <span class="w-1 h-1 rounded-full bg-orange-400 dark:bg-orange-500 shrink-0"></span>
                  <span>{{ formatPromotionRule(rule) }}</span>
                </li>
              </ul>
            </div>

            <!-- SKU Selection -->
            <div v-if="activeSkus.length > 1" class="mb-4">
              <div class="mb-2 text-xs font-medium text-muted-foreground">
                {{ t('quickBuy.selectSku') }}
              </div>
              <div class="flex flex-wrap gap-2">
                <button
                  v-for="sku in activeSkus"
                  :key="sku.id"
                  type="button"
                  class="rounded-lg border px-3 py-1.5 text-[13px] transition-all"
                  :class="[
                    normalizeSkuId(sku.id) === selectedSkuId
                      ? 'border-primary/45 bg-primary/10 ring-1 ring-primary/30 font-semibold'
                      : 'bg-secondary text-secondary-foreground hover:bg-secondary/80 font-medium',
                    isSkuPurchasable(sku) ? 'cursor-pointer' : 'cursor-not-allowed opacity-45 border-dashed',
                  ]"
                  :disabled="!isSkuPurchasable(sku)"
                  @click="selectedSkuId = normalizeSkuId(sku.id)"
                >
                  <span>{{ skuDisplayText(sku) }}</span>
                  <span
                    v-if="!isSkuPurchasable(sku)"
                    class="ml-1 text-[10px] opacity-70"
                  >({{ t('productDetail.skuStockOut') }})</span>
                </button>
              </div>
            </div>

            <!-- Selected SKU stock info -->
            <div v-if="selectedSku" class="mb-4 flex items-center gap-2 text-xs">
              <span class="text-muted-foreground">{{ t('quickBuy.stock') }}:</span>
              <span
                class="inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[11px] font-medium"
                :class="skuStockBadgeClass(selectedSku)"
              >
                <span
                  class="w-1.5 h-1.5 rounded-full"
                  :class="skuStockDotClass(selectedSku)"
                />
                {{ skuStockText(selectedSku) }}
              </span>
            </div>

            <!-- Quantity -->
            <div class="mb-4 flex items-center gap-3">
              <span class="text-xs font-medium text-muted-foreground">{{ t('quickBuy.quantity') }}</span>
              <div class="flex items-center rounded-lg border overflow-hidden">
                <button
                  type="button"
                  class="w-9 h-9 flex items-center justify-center text-muted-foreground hover:bg-secondary transition-colors cursor-pointer disabled:cursor-not-allowed disabled:opacity-30"
                  :disabled="quantity <= effectiveMin"
                  @click="quantity = Math.max(effectiveMin, quantity - 1)"
                >
                  <Minus class="w-3.5 h-3.5" :stroke-width="2.5" />
                </button>
                <input
                  type="text"
                  inputmode="numeric"
                  class="w-12 h-9 text-center text-sm font-semibold text-foreground border-x bg-transparent outline-none tabular-nums [appearance:textfield] [&::-webkit-outer-spin-button]:appearance-none [&::-webkit-inner-spin-button]:appearance-none"
                  :value="quantity"
                  @change="handleQuantityInput($event)"
                  @keydown.enter.prevent="($event.target as HTMLInputElement)?.blur()"
                />
                <button
                  type="button"
                  class="w-9 h-9 flex items-center justify-center text-muted-foreground hover:bg-secondary transition-colors cursor-pointer disabled:cursor-not-allowed disabled:opacity-30"
                  :disabled="effectiveLimit !== null && quantity >= effectiveLimit"
                  @click="quantity = quantity + 1"
                >
                  <Plus class="w-3.5 h-3.5" :stroke-width="2.5" />
                </button>
              </div>
            </div>

            <!-- Warning -->
            <p v-if="stockBelowMinPurchase" class="mb-4 rounded-lg border border-warning/40 bg-warning/10 px-3 py-2 text-xs font-medium text-warning">
              {{ t('productDetail.stockBelowMinPurchase', { count: effectiveMin }) }}
            </p>
            <p v-else-if="purchaseWarning" class="mb-4 rounded-lg border border-warning/40 bg-warning/10 px-3 py-2 text-xs font-medium text-warning">
              {{ purchaseWarning }}
            </p>
          </div>

          <!-- Actions (sticky bottom) -->
          <div class="shrink-0 px-4 md:px-5 pt-3 pb-3 md:pb-5 border-t theme-safe-bottom">
            <Button
              v-if="requiresLogin"
              class="w-full py-3 h-auto min-h-[44px] rounded-xl text-sm font-semibold"
              @click="goLogin"
            >
              {{ t('quickBuy.loginToBuy') }}
            </Button>
            <div v-else-if="isSoldOut(product)" class="flex gap-3">
              <Button
                variant="secondary"
                class="flex-1 py-3 h-auto min-h-[44px] rounded-xl text-sm font-semibold"
                @click="goToDetail"
              >
                {{ t('quickBuy.viewDetail') }}
              </Button>
            </div>
            <div v-else class="flex gap-3">
              <Button
                :disabled="!canPurchase"
                class="flex-1 py-3 h-auto min-h-[44px] rounded-xl text-sm font-semibold"
                @click="handleBuyNow"
              >
                {{ t('quickBuy.buyNow') }}
              </Button>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>


<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useOverlayMotionLifecycle } from '../composables/overlayMotionLifecycle'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '../stores/app'
import { useBuyNowStore } from '../stores/buyNow'
import { useUserAuthStore } from '../stores/userAuth'
import { useUserProfileStore } from '../stores/userProfile'
import { getFirstImageUrl, getImageUrl } from '../utils/image'
import { normalizeSkuId, buildSkuDisplayText } from '../utils/sku'
import { resolveSkuStockDisplay, type PublicStockDisplay } from '../utils/publicStock'
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
import { useLocalized, useProductLabels } from '../composables/useProduct'

import { X, Image as ImageIcon, Tag, Minus, Plus } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'

const props = defineProps<{
  product: any
  visible: boolean
}>()

const emit = defineEmits<{
  'update:visible': [value: boolean]
}>()

const router = useRouter()
const route = useRoute()
const { t } = useI18n()
const appStore = useAppStore()
const buyNowStore = useBuyNowStore()
const userAuthStore = useUserAuthStore()
const userProfileStore = useUserProfileStore()

const { getLocalizedText, siteCurrency, formatPrice } = useLocalized()
const {
  getFulfillmentTypeLabel,
  getStockBadgeVariant,
  getStockStatusLabel,
  isSoldOut,
  hasPromotionPrice,
  getPromotionPriceAmount,
  hasSkuPromotionPrice,
  getSkuPromotionPriceAmount,
  hasPromotionRules,
  getPromotionRules,
  getWholesalePrices,
  resolveWholesalePriceAmount,
  resolveMemberPriceAmount,
} = useProductLabels()

const selectedSkuId = ref(0)
const quantity = ref(1)
const purchaseWarning = ref('')

const productTitle = computed(() => getLocalizedText(props.product?.title))
const productDescription = computed(() => getLocalizedText(props.product?.description))

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

const productImage = computed(() => {
  const p = props.product
  if (!p) return ''
  if (p.images) {
    const url = getFirstImageUrl(p.images)
    if (url) return url
  }
  if (p.category?.icon) return getImageUrl(p.category.icon)
  return ''
})

const activeSkus = computed(() => {
  return resolveActiveProductSkus(props.product)
})

const selectedSku = computed(() => {
  return findSelectedProductSku(activeSkus.value, selectedSkuId.value)
})

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
  const price = resolveMemberPriceAmount(props.product, skuId, basePrice, userMemberLevelId.value, currentMemberDiscountRate.value)
  return price === null ? null : Number(price)
}

const selectedSkuMemberPrice = computed(() => {
  if (!selectedSku.value) return null
  return getMemberPriceForSku(normalizeSkuId(selectedSku.value.id), selectedSku.value.price_amount)
})

const hasMemberPrice = computed(() => {
  if (!selectedSku.value || selectedSkuMemberPrice.value === null) return false
  return selectedSkuMemberPrice.value < Number(selectedSku.value.price_amount || 0)
})

const selectedSkuWholesaleRules = computed(() => {
  if (!props.product || !selectedSku.value) return []
  return getWholesalePrices(
    props.product,
    normalizeSkuId(selectedSku.value.id),
    selectedSku.value.sku_code,
  )
})

const selectedSkuWholesalePrice = computed(() => {
  if (!props.product || !selectedSku.value) return null
  return resolveWholesalePriceAmount(
    props.product,
    selectedSku.value.price_amount,
    quantity.value,
    normalizeSkuId(selectedSku.value.id),
    selectedSku.value.sku_code,
    quantity.value,
  )
})

const selectedSkuWholesaleMemberPrice = computed(() => {
  if (!selectedSku.value || !selectedSkuWholesalePrice.value) return null
  return getMemberPriceForSku(normalizeSkuId(selectedSku.value.id), selectedSkuWholesalePrice.value)
})

const selectedSkuPromotionPrice = computed(() => {
  if (!selectedSku.value || !hasSkuPromotionPrice(selectedSku.value)) return null
  return getSkuPromotionPriceAmount(selectedSku.value)
})

const selectedSkuPromotionMemberPrice = computed(() => {
  if (!selectedSku.value || selectedSkuPromotionPrice.value === null) return null
  return getMemberPriceForSku(normalizeSkuId(selectedSku.value.id), selectedSkuPromotionPrice.value)
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

const formatWholesaleTier = (tier: any) => t('products.wholesaleTier', {
  count: Number(tier?.min_quantity || 0),
  price: formatPrice(tier?.unit_price, siteCurrency.value),
})

const effectiveMin = computed(() => {
  return resolveProductQuantityBounds(props.product, skuAvailableStock(selectedSku.value)).minimum
})

const skuAvailableStock = (sku: any) => {
  return resolveProductSkuAvailableStock(props.product, sku)
}

const isSkuPurchasable = (sku: any) => {
  return isProductSkuPurchasable(skuAvailableStock(sku))
}

const syncSelectedSku = () => {
  selectedSkuId.value = resolveSelectedProductSkuId({
    activeSkus: activeSkus.value,
    currentSkuId: selectedSkuId.value,
    preserveCurrent: false,
    isPurchasable: isSkuPurchasable,
  })
}

// Reset state when product changes or drawer opens
watch(() => [props.product, props.visible], () => {
  if (props.visible && props.product) {
    purchaseWarning.value = ''
    quantity.value = effectiveMin.value
    syncSelectedSku()
    ensureMemberLevels()
  }
}, { immediate: true })

const skuStockText = (sku: any) => {
  const display = resolveSkuStockDisplay(props.product, sku)
  return formatSkuStockDisplay(display)
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

const skuStockBadgeClass = (sku: any) => {
  const display = resolveSkuStockDisplay(props.product, sku)
  if (display.kind === 'unlimited' || display.kind === 'hidden') return 'border-slate-200 text-slate-600 dark:border-slate-700 dark:text-slate-300'
  if (display.kind === 'out') return 'border-rose-200 bg-rose-50 text-rose-700 dark:border-rose-700 dark:bg-rose-950/30 dark:text-rose-300'
  if (display.kind === 'low_stock' || (display.kind === 'range' && display.max <= 5)) return 'border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-700 dark:bg-amber-950/30 dark:text-amber-300'
  return 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-700 dark:bg-emerald-950/30 dark:text-emerald-300'
}

const skuStockDotClass = (sku: any) => {
  const display = resolveSkuStockDisplay(props.product, sku)
  if (display.kind === 'unlimited' || display.kind === 'hidden') return 'bg-slate-400 dark:bg-slate-500'
  if (display.kind === 'out') return 'bg-rose-500 dark:bg-rose-400'
  if (display.kind === 'low_stock' || (display.kind === 'range' && display.max <= 5)) return 'bg-amber-500 dark:bg-amber-400'
  return 'bg-emerald-500 dark:bg-emerald-400'
}

const skuDisplayText = (sku: any) => buildSkuDisplayText({
  skuCode: sku?.sku_code,
  specValues: sku?.spec_values,
  fallback: t('productDetail.skuFallback'),
  locale: appStore.locale,
})

const effectiveLimit = computed(() => {
  return resolveProductQuantityBounds(props.product, skuAvailableStock(selectedSku.value)).limit
})

const purchaseType = computed(() => props.product?.purchase_type || 'member')
const requiresLogin = computed(() => purchaseType.value === 'member' && !userAuthStore.isAuthenticated)
const productPurchaseState = computed(() => resolveProductPurchaseState({
  product: props.product,
  activeSkuCount: activeSkus.value.length,
  hasSelectedSku: Boolean(selectedSku.value),
  selectedSkuPurchasable: selectedSku.value ? isSkuPurchasable(selectedSku.value) : true,
  minimum: effectiveMin.value,
  limit: effectiveLimit.value,
}))
const stockBelowMinPurchase = computed(() => productPurchaseState.value.stockBelowMinimum)
const canPurchase = computed(() => productPurchaseState.value.canPurchase)


const handleQuantityInput = (event: Event) => {
  const input = event.target as HTMLInputElement
  const parsed = Math.floor(Number(input.value))
  const minimum = effectiveMin.value
  if (!Number.isFinite(parsed) || parsed < minimum) {
    quantity.value = minimum
    input.value = String(minimum)
    return
  }
  const limit = effectiveLimit.value
  if (limit !== null && parsed > limit) {
    quantity.value = limit
    input.value = String(limit)
    return
  }
  quantity.value = parsed
}

const close = () => {
  emit('update:visible', false)
}
// Vue must wait for the full panel budget: CSS reversal can end the
// independently eased root early. Sample on each render/interaction, not once
// at setup. The visible argument tracks ref-driven visibility outside the
// Transition slot too. Vue remains the only completion/cancellation timer owner.
const transitionDuration = (_visible: boolean) => {
  if (typeof window !== 'undefined' && window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) return 0
  // The backdrop (300/200) outlasts both responsive panel variants.
  return { enter: 300, leave: 200 }
}

const dialogRef = ref<HTMLElement | null>(null)
const { beforeEnter, beforeLeave } = useOverlayMotionLifecycle(() => props.visible, dialogRef, close, 50)

const handleBuyNow = () => {
  purchaseWarning.value = ''
  if (!canPurchase.value) return
  if (!props.product) return

  const sku = selectedSku.value
  const available = skuAvailableStock(sku)
  const violation = resolveProductPurchaseViolation({ product: props.product, availableStock: available, requestedQuantity: quantity.value })
  if (violation) {
    purchaseWarning.value = violation.kind === 'stock'
      ? (violation.count > 0 ? t('productDetail.addCartStockExceeded', { count: violation.count }) : t('productDetail.stockUnavailable'))
      : t('productDetail.addCartLimitExceeded', { count: violation.count })
    return
  }

  buyNowStore.setItem(buildProductCheckoutItemSnapshot({
    product: props.product,
    sku,
    image: getProductImages()[0] || '',
    quantity: quantity.value,
  }))
  close()
  router.push('/checkout')
}

const goLogin = () => {
  close()
  router.push(`/auth/login?redirect=${encodeURIComponent(route.fullPath)}`)
}

const goToDetail = () => {
  close()
  router.push(`/products/${props.product?.slug}`)
}

const getProductImages = () => {
  if (!props.product?.images) return []
  let imageArray: string[] = []
  if (Array.isArray(props.product.images)) {
    imageArray = props.product.images
  } else if (props.product.images.images && Array.isArray(props.product.images.images)) {
    imageArray = props.product.images.images
  }
  return imageArray.map((img: string) => getImageUrl(img))
}
</script>

<style scoped>
/* Fade only the painted backdrop, not the solid mobile sheet. The explicit
   Vue duration above retains both independent curves through interrupted leave. */
.quick-buy-enter-active { transition:background-color .3s ease-out,backdrop-filter .3s ease-out; }
.quick-buy-leave-active { transition:background-color .2s ease-in,backdrop-filter .2s ease-in; }
.quick-buy-enter-active [data-overlay-panel] { transition:transform .28s cubic-bezier(.32,.72,0,1); }
.quick-buy-leave-active [data-overlay-panel] { transition:transform .2s ease-in; }
.quick-buy-enter-from,.quick-buy-leave-to { background-color:rgb(0 0 0 / 0); backdrop-filter:blur(0px); }
.quick-buy-enter-from [data-overlay-panel],.quick-buy-leave-to [data-overlay-panel] { transform:translateY(100%); }
@media (min-width:768px) {
  .quick-buy-enter-active [data-overlay-panel] { transition:transform .2s ease-out,opacity .2s ease-out; }
  .quick-buy-leave-active [data-overlay-panel] { transition:transform .15s ease-in,opacity .15s ease-in; }
  .quick-buy-enter-from [data-overlay-panel] { transform:scale(.96); opacity:0; }
  .quick-buy-leave-to [data-overlay-panel] { transform:scale(.96); opacity:0; }
}
@media (prefers-reduced-motion: reduce) {
  .quick-buy-enter-active,.quick-buy-leave-active,.quick-buy-enter-active [data-overlay-panel],.quick-buy-leave-active [data-overlay-panel] { transition:none; }
  .quick-buy-enter-from [data-overlay-panel],.quick-buy-leave-to [data-overlay-panel] { transform:none; }
}
</style>
