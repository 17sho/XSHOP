<template>
  <article
    class="compact-product-row group relative flex flex-row items-center overflow-hidden rounded-xl border bg-card transition-all theme-slide-up"
    :class="soldOut ? 'border-destructive/30 opacity-85 grayscale-[0.25] saturate-50' : 'hover:-translate-y-0.5 hover:border-primary/30 hover:shadow-md'"
    :style="{ animationDelay: `${index * 20}ms` }"
  >
    <button type="button" class="flex min-w-0 flex-1 items-center text-left" @click="$emit('open', product.slug)">
      <span class="relative m-1.5 h-11 w-11 flex-none overflow-hidden rounded-lg sm:m-2.5 sm:h-16 sm:w-16">
        <img v-if="image" :src="image" :alt="title" loading="lazy" class="h-full w-full object-cover transition-transform duration-500" :class="soldOut ? 'brightness-75 grayscale' : 'group-hover:scale-110'" />
        <span v-else class="grid h-full w-full place-items-center bg-muted text-muted-foreground"><Package class="h-5 w-5" /></span>
        <span v-if="soldOut" class="absolute inset-0 grid place-items-center bg-black/50 px-1 text-center text-[10px] font-bold text-white">{{ t('products.stockStatus.outOfStock') }}</span>
      </span>

      <span class="flex min-w-0 flex-1 flex-col justify-center gap-0.5 py-1.5 pr-0.5 sm:py-2 sm:pr-1">
        <span class="flex min-w-0 items-center gap-1.5">
          <span v-if="categoryName" class="hidden max-w-[80px] flex-none truncate text-[11px] uppercase tracking-wider text-muted-foreground sm:inline">{{ categoryName }}</span>
          <span v-if="categoryName" class="hidden flex-none text-[11px] text-muted-foreground opacity-30 sm:inline">·</span>
          <span class="truncate text-xs font-semibold text-foreground sm:text-sm">{{ title }}</span>
        </span>
        <span class="flex min-w-0 items-center gap-1">
          <span class="inline-flex min-h-5 items-center rounded-md border border-primary/25 bg-primary/10 px-1.5 text-[10px] font-bold text-primary">{{ deliveryLabel }}</span>
          <span class="inline-flex min-h-5 items-center rounded-md px-1.5 text-[10px] font-bold" :class="stockBadgeClass">{{ stockLabel }}</span>
          <span v-if="priceSignal" class="hidden min-h-5 items-center rounded-md bg-secondary px-1.5 text-[10px] font-bold text-primary sm:inline-flex">{{ priceSignal }}</span>
        </span>
      </span>
    </button>

    <span class="flex flex-none items-center gap-1 pr-1.5 sm:gap-3 sm:pr-4">
      <button type="button" class="flex flex-col items-end text-right" @click="$emit('open', product.slug)">
        <span class="whitespace-nowrap text-xs font-bold sm:text-sm" :class="originalPrice ? 'text-destructive' : 'text-foreground'">{{ price }}</span>
        <span v-if="originalPrice" class="text-[10px] text-muted-foreground line-through">{{ originalPrice }}</span>
        <span v-else-if="priceSignal" class="text-[9px] font-semibold text-primary sm:hidden">{{ priceSignal }}</span>
      </button>
      <button
        type="button"
        class="grid h-7 w-7 flex-none place-items-center rounded-md border bg-background text-foreground transition active:scale-90 disabled:cursor-not-allowed disabled:opacity-40 sm:h-8 sm:w-8"
        :aria-label="t('common.viewDetails')"
        :disabled="soldOut"
        @click="$emit('purchase', product.slug)"
      ><ArrowRight class="h-4 w-4" /></button>
      <button type="button" class="hidden flex-none sm:block" :aria-label="t('common.viewDetails')" @click="$emit('open', product.slug)"><ChevronRight class="h-4 w-4 text-muted-foreground transition-transform group-hover:translate-x-0.5 group-hover:text-foreground" /></button>
    </span>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { ArrowRight, ChevronRight, Package } from 'lucide-vue-next'
import { getFirstImageUrl, getImageUrl } from '../utils/image'
import { useLocalized, useProductLabels } from '../composables/useProduct'

const props = withDefaults(defineProps<{ product: any; index?: number }>(), { index: 0 })
defineEmits<{ open: [slug: string]; purchase: [slug: string] }>()
const { t } = useI18n()
const { getLocalizedText, formatPrice, siteCurrency } = useLocalized()
const { isSoldOut, getFulfillmentTypeLabel, getStockStatusLabel, hasPromotionPrice, getPromotionPriceAmount, hasWholesalePrices, hasPromotionRules } = useProductLabels()
const title = computed(() => getLocalizedText(props.product?.title))
const categoryName = computed(() => getLocalizedText(props.product?.category?.name))
const image = computed(() => getFirstImageUrl(props.product?.images) || (props.product?.category?.icon ? getImageUrl(props.product.category.icon) : ''))
const soldOut = computed(() => isSoldOut(props.product))
const stockLabel = computed(() => getStockStatusLabel(props.product))
const stockBadgeClass = computed(() => {
  if (soldOut.value) return 'bg-destructive text-white'
  if (props.product?.stock_status === 'low_stock') return 'bg-amber-100 text-amber-700 dark:bg-amber-950/45 dark:text-amber-300'
  return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950/45 dark:text-emerald-300'
})
const deliveryLabel = computed(() => getFulfillmentTypeLabel(props.product?.fulfillment_type))
const price = computed(() => formatPrice(hasPromotionPrice(props.product) ? getPromotionPriceAmount(props.product) : props.product?.price_amount, siteCurrency.value))
const originalPrice = computed(() => hasPromotionPrice(props.product) ? formatPrice(props.product?.price_amount, siteCurrency.value) : '')
const priceSignal = computed(() => {
  if (hasPromotionPrice(props.product)) return t('products.promotionTag')
  if (hasWholesalePrices(props.product)) return t('products.wholesaleTag')
  if (hasPromotionRules(props.product)) return t('products.promotionBadge')
  return ''
})
</script>

<style scoped>
.compact-product-row:active { transform:scale(.985); }
@media (prefers-reduced-motion: reduce) { .compact-product-row,.compact-product-row button { transition:none; } .compact-product-row:active,.compact-product-row button:active { transform:none; } }
</style>
