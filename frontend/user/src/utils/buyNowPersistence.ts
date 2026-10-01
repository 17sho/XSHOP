import type { CheckoutItem } from '../types/checkout'
import { readStorageItem, removeStorageItem, writeStorageItem, type BrowserStorage } from './browserStorage.ts'

export interface BuyNowStorage extends BrowserStorage {}

export const BUY_NOW_ITEM_STORAGE_KEY = 'dujiao:buy-now-item:v1'
export const DURABLE_BUY_NOW_ITEM_STORAGE_KEY = 'dujiao:buy-now-draft:v1'
const DRAFT_MAX_AGE_MS = 7 * 24 * 60 * 60 * 1000

const validBuyNowItem = (value: unknown): value is CheckoutItem => {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const item = value as Partial<CheckoutItem>
  return Number.isFinite(Number(item.productId))
    && Number(item.productId) > 0
    && typeof item.slug === 'string'
    && item.slug.trim() !== ''
    && typeof item.priceAmount === 'string'
    && Number.isFinite(Number(item.quantity))
    && Number(item.quantity) > 0
}

export const readBuyNowItem = (
  storage: BuyNowStorage | null | undefined,
): CheckoutItem | null => {
  if (!storage) return null
  try {
    const raw = readStorageItem(storage, BUY_NOW_ITEM_STORAGE_KEY)
    if (!raw) return null
    const parsed = JSON.parse(raw)
    if (!validBuyNowItem(parsed)) {
      removeStorageItem(storage, BUY_NOW_ITEM_STORAGE_KEY)
      return null
    }
    return parsed
  } catch {
    removeStorageItem(storage, BUY_NOW_ITEM_STORAGE_KEY)
    return null
  }
}

export const writeBuyNowItem = (
  storage: BuyNowStorage | null | undefined,
  item: CheckoutItem | null | undefined,
): void => {
  if (!storage) return
  try {
    if (!validBuyNowItem(item)) {
      removeStorageItem(storage, BUY_NOW_ITEM_STORAGE_KEY)
      return
    }
    writeStorageItem(storage, BUY_NOW_ITEM_STORAGE_KEY, JSON.stringify(item))
  } catch {
    // Checkout still works in memory if Safari denies session storage.
  }
}

// Persist only the product snapshot; never guest credentials, captcha, or payment details.
export const readDurableBuyNowItem = (storage: BuyNowStorage | null | undefined, now = Date.now()): CheckoutItem | null => {
  if (!storage) return null
  try {
    const raw = readStorageItem(storage, DURABLE_BUY_NOW_ITEM_STORAGE_KEY)
    if (!raw) return null
    const draft = JSON.parse(raw)
    const age = now - Number(draft?.created)
    if (!Number.isFinite(age) || age < 0 || age > DRAFT_MAX_AGE_MS || !validBuyNowItem(draft?.item)) {
      removeStorageItem(storage, DURABLE_BUY_NOW_ITEM_STORAGE_KEY)
      return null
    }
    return draft.item
  } catch {
    removeStorageItem(storage, DURABLE_BUY_NOW_ITEM_STORAGE_KEY)
    return null
  }
}

export const writeDurableBuyNowItem = (storage: BuyNowStorage | null | undefined, item: CheckoutItem | null | undefined, now = Date.now()): void => {
  if (!storage) return
  try {
    if (!validBuyNowItem(item)) {
      removeStorageItem(storage, DURABLE_BUY_NOW_ITEM_STORAGE_KEY)
      return
    }
    writeStorageItem(storage, DURABLE_BUY_NOW_ITEM_STORAGE_KEY, JSON.stringify({ item, created: now }))
  } catch {
    // Checkout remains usable if persistent storage is unavailable.
  }
}
