import test from 'node:test'
import assert from 'node:assert/strict'
import {
  buildProductCheckoutItemSnapshot,
  findSelectedProductSku,
  isProductSkuPurchasable,
  normalizeOptionalPurchaseLimit,
  resolveActiveProductSkus,
  resolveProductPurchaseState,
  resolveProductPurchaseViolation,
  resolveProductQuantityBounds,
  resolveProductSkuAvailableStock,
  resolveSelectedProductSkuId,

  resolveSelectedSkuPriceState,
  shouldEnforceProductSkuStock,
} from '../src/utils/productPurchase.ts'

const quantityCases = [
  { name: 'defaults missing bounds to one and unlimited', min: undefined, max: undefined, stock: null, expected: { minimum: 1, limit: null } },
  { name: 'normalizes fractional product bounds', min: '2.9', max: '9.8', stock: null, expected: { minimum: 2, limit: 9 } },
  { name: 'uses exact stock when lower than product maximum', min: 2, max: 9, stock: 4, expected: { minimum: 2, limit: 4 } },
  { name: 'uses product maximum when lower than exact stock', min: 2, max: 3, stock: 8, expected: { minimum: 2, limit: 3 } },
  { name: 'retains zero stock as an effective limit', min: 2, max: undefined, stock: 0, expected: { minimum: 2, limit: 0 } },
]

test('product quantity bounds preserve purchase minimum and maximum rules', () => {
  for (const row of quantityCases) {
    assert.deepEqual(
      resolveProductQuantityBounds({ min_purchase_quantity: row.min, max_purchase_quantity: row.max }, row.stock),
      row.expected,
      row.name,
    )
  }
})

test('optional purchase limits reject non-positive and invalid values', () => {
  for (const [input, expected] of [[undefined, null], [null, null], ['', null], [0, null], [-1, null], ['bad', null], ['3.9', 3]] as const) {
    assert.equal(normalizeOptionalPurchaseLimit(input), expected, String(input))
  }
})

const stockCases = [
  { name: 'manual finite stock', product: { fulfillment_type: 'manual' }, sku: { manual_stock_total: 7 }, enforced: true, available: 7 },
  { name: 'manual unlimited stock', product: { fulfillment_type: 'manual' }, sku: { manual_stock_total: -1 }, enforced: false, available: null },
  { name: 'auto stock', product: { fulfillment_type: 'auto' }, sku: { auto_stock_available: 5 }, enforced: true, available: 5 },
  { name: 'upstream stock', product: { fulfillment_type: 'upstream' }, sku: { upstream_stock: 6 }, enforced: true, available: 6 },
  { name: 'sku-hidden stock', product: { fulfillment_type: 'manual' }, sku: { manual_stock_total: 7, stock_quantity_hidden: true }, enforced: false, available: null },
  { name: 'product-hidden stock', product: { fulfillment_type: 'auto', stock_quantity_hidden: true }, sku: { auto_stock_available: 5 }, enforced: false, available: null },
]

test('stock enforcement keeps fulfillment and hidden-stock semantics', () => {
  for (const row of stockCases) {
    assert.equal(shouldEnforceProductSkuStock(row.product, row.sku), row.enforced, row.name)
    assert.equal(resolveProductSkuAvailableStock(row.product, row.sku), row.available, row.name)
  }
})

test('checkout item snapshot normalizes the shared product and sku fields', () => {
  const wholesalePrices = [{ sku_id: 12, min_quantity: 3, unit_price: '11.00' }]
  const paymentChannelIds = [4, 9]
  const snapshot = buildProductCheckoutItemSnapshot({
    product: {
      id: 21,
      slug: 'sample',
      title: { en: 'Sample' },
      price_amount: '15.00',
      wholesale_prices: wholesalePrices,
      min_purchase_quantity: '2.9',
      max_purchase_quantity: '8.7',
      purchase_type: 'member',
      fulfillment_type: 'manual',
      manual_form_schema: { fields: [] },
      payment_channel_ids: paymentChannelIds,
      stock_display_mode: 'range',
      stock_quantity_hidden: true,
    },
    sku: {
      id: '12',
      sku_code: 88,
      spec_values: { Size: 'L' },
      price_amount: '12.00',
      manual_stock_total: '-1',
      manual_stock_locked: '2.9',
      manual_stock_sold: -4,
      auto_stock_available: '5.8',
      upstream_stock: '9.2',
      stock_status: 'in_stock',
      stock_display: 'range_6_10',
      stock_range_min: '6.8',
      stock_range_max: 10,
    },
    image: '/sample.webp',
    quantity: 2,
  })

  assert.deepEqual(snapshot, {
    productId: 21,
    skuId: 12,
    skuCode: '88',
    skuSpecValues: { Size: 'L' },
    skuManualStockTotal: -1,
    skuManualStockLocked: 2,
    skuManualStockSold: 0,
    skuAutoStockAvailable: 5,
    skuUpstreamStock: 9,
    skuStockStatus: 'in_stock',
    skuStockDisplayMode: 'range',
    skuStockDisplay: 'range_6_10',
    skuStockRangeMin: 6,
    skuStockRangeMax: 10,
    skuStockQuantityHidden: true,
    skuStockEnforced: false,
    slug: 'sample',
    title: { en: 'Sample' },
    priceAmount: '12.00',
    wholesalePrices,
    image: '/sample.webp',
    minPurchaseQuantity: 2,
    maxPurchaseQuantity: 8,
    purchaseType: 'member',
    fulfillmentType: 'manual',
    manualFormSchema: { fields: [] },
    paymentChannelIds,
    quantity: 2,
  })
})

test('checkout item snapshot preserves product fallbacks and optional-field omission', () => {
  const snapshot = buildProductCheckoutItemSnapshot({
    product: { id: 3, slug: 'fallback', title: 'Fallback', price_amount: '7.00' },
    sku: {},
    image: undefined,
    quantity: 1,
  })

  assert.equal(snapshot.priceAmount, '7.00')
  assert.equal(snapshot.image, undefined)
  assert.equal(snapshot.minPurchaseQuantity, undefined)
  assert.equal(snapshot.maxPurchaseQuantity, undefined)
  assert.equal(snapshot.wholesalePrices, undefined)
  assert.equal(snapshot.paymentChannelIds, undefined)
  assert.deepEqual(snapshot.manualFormSchema, {})
})

test('active sku selection filters inactive rows and matches normalized ids', () => {
  const activeSku = { id: '12', is_active: 1 }
  const rows = resolveActiveProductSkus({
    skus: [activeSku, { id: 13, is_active: false }, null],
  })

  assert.deepEqual(rows, [activeSku])
  assert.equal(findSelectedProductSku(rows, 12), activeSku)
  assert.equal(findSelectedProductSku(rows, 0), null)
  assert.equal(findSelectedProductSku(rows, 99), null)
  assert.deepEqual(resolveActiveProductSkus({ skus: 'invalid' }), [])
})

const selectedSkuIdCases: Array<{
  name: string
  rows: Array<{ id: number | string }>
  current: number
  preserve: boolean
  purchasable: number[]
  expected: number
}> = [
  { name: 'empty rows clear selection', rows: [], current: 9, preserve: true, purchasable: [] as number[], expected: 0 },
  { name: 'single row is always selected', rows: [{ id: '7' }], current: 9, preserve: true, purchasable: [], expected: 7 },
  { name: 'detail preserves a current active selection', rows: [{ id: 7 }, { id: 8 }], current: 8, preserve: true, purchasable: [7], expected: 8 },
  { name: 'detail replaces a stale selection with first purchasable', rows: [{ id: 7 }, { id: 8 }], current: 9, preserve: true, purchasable: [8], expected: 8 },
  { name: 'detail does not preserve zero through an invalid sku id', rows: [{ id: 'invalid' }, { id: 8 }], current: 0, preserve: true, purchasable: [8], expected: 8 },
  { name: 'detail does not preserve an invalid current id through an invalid sku id', rows: [{ id: 'invalid' }, { id: 8 }], current: 'invalid' as any, preserve: true, purchasable: [8], expected: 8 },
  { name: 'quick buy resets to first purchasable', rows: [{ id: 7 }, { id: 8 }], current: 8, preserve: false, purchasable: [7, 8], expected: 7 },
  { name: 'falls back to first row when every sku is unavailable', rows: [{ id: 7 }, { id: 8 }], current: 0, preserve: false, purchasable: [], expected: 7 },
]

test('selected sku resolution preserves detail and quick-buy selection policies', () => {
  for (const row of selectedSkuIdCases) {
    assert.equal(resolveSelectedProductSkuId({
      activeSkus: row.rows,
      currentSkuId: row.current,
      preserveCurrent: row.preserve,
      isPurchasable: sku => row.purchasable.includes(Number(sku.id)),
    }), row.expected, row.name)
  }
})

const priceStateCases = [
  {
    name: 'wholesale beats promotion and uses its member result',
    input: { basePrice: 20, promotionPrice: '15.00', wholesalePrice: '12.00', wholesaleMemberPrice: 10, promotionMemberPrice: 13, hasBaseMemberPrice: true },
    expected: { hasWholesalePrice: true, wholesaleFinalPrice: 10, wholesaleFinalIsMember: true, promotionFinalPrice: 13, promotionFinalIsMember: true, showMemberBadge: true },
  },
  {
    name: 'wholesale must beat the promotion comparison price',
    input: { basePrice: 20, promotionPrice: 15, wholesalePrice: 16, wholesaleMemberPrice: 14, promotionMemberPrice: null, hasBaseMemberPrice: true },
    expected: { hasWholesalePrice: false, wholesaleFinalPrice: null, wholesaleFinalIsMember: false, promotionFinalPrice: 15, promotionFinalIsMember: false, showMemberBadge: false },
  },
  {
    name: 'promotion member price controls badge when wholesale is absent',
    input: { basePrice: 20, promotionPrice: 15, wholesalePrice: null, wholesaleMemberPrice: null, promotionMemberPrice: 13, hasBaseMemberPrice: true },
    expected: { hasWholesalePrice: false, wholesaleFinalPrice: null, wholesaleFinalIsMember: false, promotionFinalPrice: 13, promotionFinalIsMember: true, showMemberBadge: true },
  },
  {
    name: 'base member flag controls badge when no promotion applies',
    input: { basePrice: 20, promotionPrice: null, wholesalePrice: null, wholesaleMemberPrice: null, promotionMemberPrice: null, hasBaseMemberPrice: true },
    expected: { hasWholesalePrice: false, wholesaleFinalPrice: null, wholesaleFinalIsMember: false, promotionFinalPrice: null, promotionFinalIsMember: false, showMemberBadge: true },
  },
]

test('selected sku price state preserves wholesale promotion and member precedence', () => {
  for (const row of priceStateCases) {
    assert.deepEqual(resolveSelectedSkuPriceState(row.input), row.expected, row.name)
  }
})

test('sku purchasability treats hidden or unlimited stock as available', () => {
  for (const [available, expected] of [[null, true], [1, true], [0, false], [-1, false]] as const) {
    assert.equal(isProductSkuPurchasable(available), expected, String(available))
  }
})

const purchaseStateCases = [
  { name: 'missing product', input: { product: null, activeSkuCount: 1, hasSelectedSku: true, selectedSkuPurchasable: true, minimum: 1, limit: null }, expected: { requiresSkuSelection: false, stockBelowMinimum: false, canPurchase: false } },
  { name: 'multiple skus require selection', input: { product: {}, activeSkuCount: 2, hasSelectedSku: false, selectedSkuPurchasable: true, minimum: 1, limit: null }, expected: { requiresSkuSelection: true, stockBelowMinimum: false, canPurchase: false } },
  { name: 'stock below minimum', input: { product: {}, activeSkuCount: 1, hasSelectedSku: true, selectedSkuPurchasable: true, minimum: 3, limit: 2 }, expected: { requiresSkuSelection: false, stockBelowMinimum: true, canPurchase: false } },
  { name: 'sold out product', input: { product: { is_sold_out: true }, activeSkuCount: 1, hasSelectedSku: true, selectedSkuPurchasable: true, minimum: 1, limit: null }, expected: { requiresSkuSelection: false, stockBelowMinimum: false, canPurchase: false } },
  { name: 'out of stock status', input: { product: { stock_status: 'out_of_stock' }, activeSkuCount: 1, hasSelectedSku: true, selectedSkuPurchasable: true, minimum: 1, limit: null }, expected: { requiresSkuSelection: false, stockBelowMinimum: false, canPurchase: false } },
  { name: 'selected sku unavailable', input: { product: {}, activeSkuCount: 1, hasSelectedSku: true, selectedSkuPurchasable: false, minimum: 1, limit: 4 }, expected: { requiresSkuSelection: false, stockBelowMinimum: false, canPurchase: false } },
  { name: 'purchasable product', input: { product: {}, activeSkuCount: 1, hasSelectedSku: true, selectedSkuPurchasable: true, minimum: 1, limit: 4 }, expected: { requiresSkuSelection: false, stockBelowMinimum: false, canPurchase: true } },
]

test('product purchase state preserves sku stock and quantity gates', () => {
  for (const row of purchaseStateCases) {
    assert.deepEqual(resolveProductPurchaseState(row.input), row.expected, row.name)
  }
})


const purchaseViolationCases = [
  { name: 'within limit', input: { product: { max_purchase_quantity: 5 }, availableStock: 8, requestedQuantity: 5 }, expected: null },
  { name: 'product limit exceeded', input: { product: { max_purchase_quantity: 5 }, availableStock: 8, requestedQuantity: 6 }, expected: { kind: 'limit', count: 5 } },
  { name: 'stock exceeded', input: { product: { max_purchase_quantity: 8 }, availableStock: 5, requestedQuantity: 6 }, expected: { kind: 'stock', count: 5 } },
  { name: 'equal stock and product limit keeps stock priority', input: { product: { max_purchase_quantity: 5 }, availableStock: 5, requestedQuantity: 6 }, expected: { kind: 'stock', count: 5 } },
  { name: 'zero stock is unavailable stock violation', input: { product: {}, availableStock: 0, requestedQuantity: 1 }, expected: { kind: 'stock', count: 0 } },
  { name: 'hidden stock only enforces product limit', input: { product: { max_purchase_quantity: 5 }, availableStock: null, requestedQuantity: 6 }, expected: { kind: 'limit', count: 5 } },
  { name: 'unlimited quantity has no violation', input: { product: {}, availableStock: null, requestedQuantity: 999 }, expected: null },
]

test('purchase violation preserves stock over product-limit warning priority', () => {
  for (const row of purchaseViolationCases) {
    assert.deepEqual(resolveProductPurchaseViolation(row.input), row.expected, row.name)
  }
})
