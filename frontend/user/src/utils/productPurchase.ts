import type { CheckoutItem } from '../types/checkout.ts'
import { resolveSkuAvailableStock } from './publicStock.ts'
import { normalizeSkuId } from './sku.ts'

export const resolveActiveProductSkus = (product: any): any[] => {
  const rows = Array.isArray(product?.skus) ? product.skus : []
  return rows.filter((sku: any) => Boolean(sku?.is_active))
}

export const findSelectedProductSku = (activeSkus: any[], selectedSkuId: unknown): any | null => {
  const normalizedId = normalizeSkuId(selectedSkuId)
  if (normalizedId <= 0) return null
  return activeSkus.find((sku: any) => normalizeSkuId(sku?.id) === normalizedId) || null
}

export const resolveSelectedProductSkuId = ({
  activeSkus,
  currentSkuId,
  preserveCurrent,
  isPurchasable,
}: {
  activeSkus: any[]
  currentSkuId: unknown
  preserveCurrent: boolean
  isPurchasable: (sku: any) => boolean
}): number => {
  if (activeSkus.length === 0) return 0
  if (activeSkus.length === 1) return normalizeSkuId(activeSkus[0]?.id)
  const normalizedCurrent = normalizeSkuId(currentSkuId)
  if (preserveCurrent && normalizedCurrent > 0 && activeSkus.some((sku: any) => normalizeSkuId(sku?.id) === normalizedCurrent)) {
    return normalizedCurrent
  }
  const firstPurchasable = activeSkus.find(isPurchasable)
  return normalizeSkuId((firstPurchasable || activeSkus[0])?.id)
}

export interface SelectedSkuPriceInput {
  basePrice: unknown
  promotionPrice: unknown | null
  wholesalePrice: unknown | null
  wholesaleMemberPrice: number | null
  promotionMemberPrice: number | null
  hasBaseMemberPrice: boolean
}

export const resolveSelectedSkuPriceState = ({
  basePrice,
  promotionPrice,
  wholesalePrice,
  wholesaleMemberPrice,
  promotionMemberPrice,
  hasBaseMemberPrice,
}: SelectedSkuPriceInput) => {
  const comparisonPrice = promotionPrice !== null ? Number(promotionPrice) : Number(basePrice || 0)
  const hasWholesalePrice = wholesalePrice !== null && Boolean(wholesalePrice) && Number(wholesalePrice) < comparisonPrice
  const wholesaleFinalIsMember = hasWholesalePrice && wholesaleMemberPrice !== null
  const promotionFinalIsMember = promotionPrice !== null && promotionMemberPrice !== null
  return {
    hasWholesalePrice,
    wholesaleFinalPrice: hasWholesalePrice
      ? (wholesaleMemberPrice !== null ? wholesaleMemberPrice : wholesalePrice)
      : null,
    wholesaleFinalIsMember,
    promotionFinalPrice: promotionPrice === null
      ? null
      : (promotionMemberPrice !== null ? promotionMemberPrice : promotionPrice),
    promotionFinalIsMember,
    showMemberBadge: hasWholesalePrice
      ? wholesaleFinalIsMember
      : promotionPrice !== null ? promotionFinalIsMember : hasBaseMemberPrice,
  }
}

export const normalizeStockNumber = (value: unknown): number => {
  const numberValue = Number(value)
  if (!Number.isFinite(numberValue)) return 0
  return Math.max(Math.floor(numberValue), 0)
}

export const normalizeManualStockTotal = (value: unknown): number => {
  const numberValue = Number(value)
  if (!Number.isFinite(numberValue)) return 0
  const integerValue = Math.floor(numberValue)
  if (integerValue === -1) return -1
  return Math.max(integerValue, 0)
}

export const normalizeOptionalPurchaseLimit = (value: unknown): number | null => {
  const numberValue = Number(value)
  if (!Number.isFinite(numberValue)) return null
  const integerValue = Math.floor(numberValue)
  return integerValue > 0 ? integerValue : null
}

export const shouldEnforceProductSkuStock = (product: any, sku: any): boolean => {
  if (!sku) return false
  if (sku?.stock_quantity_hidden === true || product?.stock_quantity_hidden === true) return false
  if (product?.fulfillment_type === 'auto' || product?.fulfillment_type === 'upstream') return true
  if (product?.fulfillment_type !== 'manual') return false
  return normalizeManualStockTotal(sku?.manual_stock_total) !== -1
}

export const resolveProductSkuAvailableStock = (product: any, sku: any): number | null => {
  if (!sku) return 0
  if (!shouldEnforceProductSkuStock(product, sku) && !sku?.stock_quantity_hidden) return null
  return resolveSkuAvailableStock(product, sku)
}

export const isProductSkuPurchasable = (availableStock: number | null): boolean =>
  availableStock === null || availableStock > 0

export const resolveProductQuantityBounds = (
  product: any,
  availableStock: number | null,
): { minimum: number; limit: number | null } => {
  const minimum = normalizeOptionalPurchaseLimit(product?.min_purchase_quantity) ?? 1
  const productLimit = normalizeOptionalPurchaseLimit(product?.max_purchase_quantity)
  const limit = availableStock === null
    ? productLimit
    : productLimit === null ? availableStock : Math.min(productLimit, availableStock)
  return { minimum, limit }
}

export type ProductPurchaseViolation = { kind: 'stock' | 'limit'; count: number }

export const resolveProductPurchaseViolation = ({
  product,
  availableStock,
  requestedQuantity,
}: {
  product: any
  availableStock: number | null
  requestedQuantity: number
}): ProductPurchaseViolation | null => {
  const productLimit = normalizeOptionalPurchaseLimit(product?.max_purchase_quantity)
  const limit = resolveProductQuantityBounds(product, availableStock).limit
  if (limit === null || requestedQuantity <= limit) return null
  const stockSetsLimit = availableStock !== null
    && limit === availableStock
    && (productLimit === null || availableStock <= productLimit)
  return { kind: stockSetsLimit ? 'stock' : 'limit', count: limit }
}

export const resolveProductPurchaseState = ({
  product,
  activeSkuCount,
  hasSelectedSku,
  selectedSkuPurchasable,
  minimum,
  limit,
}: {
  product: any
  activeSkuCount: number
  hasSelectedSku: boolean
  selectedSkuPurchasable: boolean
  minimum: number
  limit: number | null
}) => {
  const requiresSkuSelection = activeSkuCount > 1 && !hasSelectedSku
  const stockBelowMinimum = limit !== null && limit < minimum
  const canPurchase = Boolean(product)
    && activeSkuCount > 0
    && !product?.is_sold_out
    && !requiresSkuSelection
    && product?.stock_status !== 'out_of_stock'
    && (!hasSelectedSku || selectedSkuPurchasable)
    && !stockBelowMinimum
  return { requiresSkuSelection, stockBelowMinimum, canPurchase }
}

export const buildProductCheckoutItemSnapshot = ({
  product,
  sku,
  image,
  quantity,
}: {
  product: any
  sku: any
  image?: string
  quantity: number
}): CheckoutItem => ({
  productId: product.id,
  skuId: normalizeSkuId(sku?.id),
  skuCode: String(sku?.sku_code || ''),
  skuSpecValues: (sku?.spec_values && typeof sku.spec_values === 'object') ? sku.spec_values : undefined,
  skuManualStockTotal: normalizeManualStockTotal(sku?.manual_stock_total),
  skuManualStockLocked: normalizeStockNumber(sku?.manual_stock_locked),
  skuManualStockSold: normalizeStockNumber(sku?.manual_stock_sold),
  skuAutoStockAvailable: normalizeStockNumber(sku?.auto_stock_available),
  skuUpstreamStock: normalizeManualStockTotal(sku?.upstream_stock),
  skuStockStatus: String(sku?.stock_status || ''),
  skuStockDisplayMode: String(sku?.stock_display_mode || product?.stock_display_mode || ''),
  skuStockDisplay: String(sku?.stock_display || ''),
  skuStockRangeMin: normalizeStockNumber(sku?.stock_range_min) || undefined,
  skuStockRangeMax: normalizeStockNumber(sku?.stock_range_max) || undefined,
  skuStockQuantityHidden: Boolean(sku?.stock_quantity_hidden || product?.stock_quantity_hidden),
  skuStockEnforced: shouldEnforceProductSkuStock(product, sku),
  slug: product.slug,
  title: product.title,
  priceAmount: String(sku?.price_amount || product.price_amount || '0.00'),
  wholesalePrices: Array.isArray(product.wholesale_prices) ? product.wholesale_prices : undefined,
  image,
  minPurchaseQuantity: normalizeOptionalPurchaseLimit(product.min_purchase_quantity) ?? undefined,
  maxPurchaseQuantity: normalizeOptionalPurchaseLimit(product.max_purchase_quantity) ?? undefined,
  purchaseType: product.purchase_type,
  fulfillmentType: product.fulfillment_type,
  manualFormSchema: product.manual_form_schema || {},
  paymentChannelIds: Array.isArray(product.payment_channel_ids) && product.payment_channel_ids.length > 0
    ? product.payment_channel_ids
    : undefined,
  quantity,
})
