import { getBrowserStorage, readStorageItem, removeStorageItem, writeStorageItem, type BrowserStorage } from './browserStorage.ts'

const KEY = 'xshop-pending-checkout-order-v1'
const MAX_AGE_MS = 24 * 60 * 60 * 1000

type StoragePort = BrowserStorage

export function getCheckoutSessionStorage(): StoragePort | null {
  return getBrowserStorage('sessionStorage')
}

export function savePendingCheckoutOrder(storage: StoragePort | null, orderNo: unknown, guest: boolean, now = Date.now()) {
  if (!storage) return
  const order = String(orderNo || '').trim()
  if (!order || order.length > 128) return
  writeStorageItem(storage, KEY, JSON.stringify({ order, guest, created: now }))
}

export function readPendingCheckoutOrder(storage: StoragePort | null, now = Date.now()): { orderNo: string; guest: boolean } | null {
  if (!storage) return null
  const raw = readStorageItem(storage, KEY)
  if (!raw) return null
  try {
    const data = JSON.parse(raw)
    const age = now - Number(data.created)
    if (typeof data.order !== 'string' || !data.order || data.order.length > 128 || typeof data.guest !== 'boolean' || !Number.isFinite(age) || age < 0 || age > MAX_AGE_MS) throw Error('invalid')
    return { orderNo: data.order, guest: data.guest }
  } catch {
    removeStorageItem(storage, KEY)
    return null
  }
}
