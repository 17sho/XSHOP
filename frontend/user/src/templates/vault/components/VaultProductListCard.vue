<template>
  <article
    class="vault-compact-row group min-w-0 flex items-center gap-3 rounded-lg border bg-card p-2.5 transition hover:border-hairline-strong hover:shadow-[var(--shadow-sm)] sm:gap-3.5 sm:p-3 theme-slide-up"
    :class="{ 'opacity-[0.74]': soldOut }"
    :style="{ animationDelay: `${index * 20}ms` }"
  >
    <RouterLink :to="`/products/${product.slug}`" class="flex min-w-0 flex-1 items-center gap-3 sm:gap-3.5">
      <span class="relative grid h-14 w-14 flex-none place-items-center overflow-hidden rounded-[11px] sm:h-16 sm:w-16" :class="coverClass">
        <img v-if="image" :src="image" :alt="title" loading="lazy" class="absolute inset-0 h-full w-full object-cover" />
        <Package v-else class="relative z-[1] h-6 w-6 text-white" />
        <span v-if="soldOut" class="absolute inset-0 grid place-items-center bg-black/55 px-1 text-center text-[10px] font-bold text-white">{{ t('products.stockStatus.outOfStock') }}</span>
      </span>

      <span class="flex min-w-0 flex-1 flex-col gap-1">
        <span class="flex min-w-0 items-center gap-1.5">
          <span v-if="categoryName" class="hidden max-w-[88px] flex-none truncate text-[11px] font-semibold uppercase tracking-wide text-muted-foreground sm:inline">{{ categoryName }}</span>
          <span v-if="categoryName" class="hidden flex-none text-muted-foreground/40 sm:inline">·</span>
          <span class="truncate text-sm font-bold text-foreground">{{ title }}</span>
        </span>
        <span class="flex min-w-0 flex-wrap items-center gap-1">
          <span class="inline-flex w-fit items-center rounded-full border border-primary/20 bg-primary/10 px-2 py-0.5 text-[11px] font-semibold text-primary">{{ deliveryLabel }}</span>
          <span class="inline-flex w-fit items-center rounded-full px-2 py-0.5 text-[10px] font-semibold" :class="stockBadgeClass">{{ stockLabel }}</span>
          <span v-if="priceSignal" class="hidden truncate rounded-full bg-[color:var(--gold-soft)] px-2 py-0.5 text-[10px] font-semibold text-[color:var(--gold-strong)] sm:inline">{{ priceSignal }}</span>
        </span>
      </span>
    </RouterLink>

    <span class="flex flex-none items-center gap-2 sm:gap-3">
      <RouterLink :to="`/products/${product.slug}`" class="text-right">
        <span class="block whitespace-nowrap text-sm font-extrabold tabular-nums text-foreground sm:text-base">{{ price }}</span>
        <span v-if="originalPrice" class="block text-[11px] font-semibold text-muted-foreground line-through">{{ originalPrice }}</span>
        <span v-else-if="priceSignal" class="block text-[9px] font-semibold text-[color:var(--gold-strong)] sm:hidden">{{ priceSignal }}</span>
      </RouterLink>
      <button
        v-if="!soldOut"
        type="button"
        class="grid h-9 w-9 flex-none place-items-center rounded-full bg-primary text-white transition hover:bg-primary/90 active:scale-90"
        :aria-label="t('products.quickBuyAria')"
        @click="$emit('quickBuy', product)"
      ><ShoppingCart class="h-4 w-4" /></button>
      <RouterLink :to="`/products/${product.slug}`" class="hidden flex-none sm:block" :aria-label="t('common.viewDetails')"><ChevronRight class="h-4 w-4 text-muted-foreground transition group-hover:translate-x-0.5 group-hover:text-foreground" /></RouterLink>
    </span>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { ChevronRight, Package, ShoppingCart } from 'lucide-vue-next'
import { getFirstImageUrl, getImageUrl } from '../../../utils/image'
import { useLocalized, useProductLabels } from '../../../composables/useProduct'

const props = withDefaults(defineProps<{ product: any; index?: number }>(), { index: 0 })
defineEmits<{ quickBuy: [product: any] }>()
const { t } = useI18n()
const { getLocalizedText, formatPrice, siteCurrency } = useLocalized()
const { isSoldOut, getFulfillmentTypeLabel, getStockStatusLabel, hasPromotionPrice, getPromotionPriceAmount, hasWholesalePrices, hasPromotionRules } = useProductLabels()
const covers = ['bg-[linear-gradient(135deg,#7b74f2,var(--red))]','bg-[linear-gradient(135deg,#1cc0bf,var(--teal))]','bg-[linear-gradient(135deg,#9b6cf5,var(--plum))]','bg-[linear-gradient(135deg,#f7bd4e,var(--gold))]','bg-[linear-gradient(135deg,#3a3950,var(--ink))]']
const coverClass = computed(() => covers[(props.index ?? 0) % covers.length])
const title = computed(() => getLocalizedText(props.product?.title))
const categoryName = computed(() => getLocalizedText(props.product?.category?.name))
const image = computed(() => getFirstImageUrl(props.product?.images) || (props.product?.category?.icon ? getImageUrl(props.product.category.icon) : ''))
const soldOut = computed(() => isSoldOut(props.product))
const stockLabel = computed(() => getStockStatusLabel(props.product))
const stockBadgeClass = computed(() => {
  if (soldOut.value) return 'bg-secondary text-muted-foreground'
  if (props.product?.stock_status === 'low_stock') return 'bg-[color:var(--gold-soft)] text-[color:var(--gold-strong)]'
  return 'bg-[color:var(--teal-soft)] text-[color:var(--teal-strong)]'
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
.vault-compact-row:active { transform:scale(.985); }
@media (prefers-reduced-motion: reduce) { .vault-compact-row,.vault-compact-row button { transition:none; } .vault-compact-row:active,.vault-compact-row button:active { transform:none; } }
</style>
