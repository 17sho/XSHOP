export interface CheckoutItem {
  productId: number
  skuId?: number
  skuCode?: string
  skuSpecValues?: Record<string, any>
  skuManualStockTotal?: number
  skuManualStockLocked?: number
  skuManualStockSold?: number
  skuAutoStockAvailable?: number
  skuUpstreamStock?: number
  skuStockStatus?: string
  skuStockDisplayMode?: string
  skuStockDisplay?: string
  skuStockRangeMin?: number
  skuStockRangeMax?: number
  skuStockQuantityHidden?: boolean
  skuStockEnforced?: boolean
  skuStockSnapshotAt?: string
  slug: string
  title: any
  priceAmount: string
  wholesalePrices?: Array<{ sku_id?: number; sku_code?: string; min_quantity: number; unit_price: string | number }>
  image?: string
  quantity: number
  minPurchaseQuantity?: number
  maxPurchaseQuantity?: number
  purchaseType?: string
  fulfillmentType?: string
  manualFormSchema?: any
  paymentChannelIds?: number[]
}

const normalizeOptionalLimitNumber = (value: unknown): number | undefined => {
  if (value === undefined || value === null || value === '') return undefined
  const numberValue = Number(value)
  if (!Number.isFinite(numberValue)) return undefined
  const integerValue = Math.floor(numberValue)
  return integerValue > 0 ? integerValue : undefined
}

export const checkoutItemPurchaseLimit = (item: Pick<CheckoutItem, 'maxPurchaseQuantity'>) =>
  normalizeOptionalLimitNumber(item.maxPurchaseQuantity) ?? null

export const checkoutItemPurchaseMin = (item: Pick<CheckoutItem, 'minPurchaseQuantity'>) =>
  normalizeOptionalLimitNumber(item.minPurchaseQuantity) ?? 1
