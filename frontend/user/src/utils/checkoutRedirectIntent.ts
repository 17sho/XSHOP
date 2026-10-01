import { readStorageItem, removeStorageItem, writeStorageItem, type BrowserStorage } from './browserStorage.ts'

const INTENT_KEY = 'checkout-created-payment-redirect-v1'
const MAX_AGE_MS = 60_000

type RedirectStorage = BrowserStorage

export const createCheckoutRedirectIntent = (storage: RedirectStorage | null, orderNo: unknown, paymentId: unknown, now = Date.now()) => {
  if (!storage) return
  const order = String(orderNo || '').trim()
  const payment = Number(paymentId)
  if (!order || !Number.isSafeInteger(payment) || payment <= 0) return
  writeStorageItem(storage, INTENT_KEY, JSON.stringify({ order, payment, created: now }))
}

export const consumeCheckoutRedirectIntent = (storage: RedirectStorage | null, orderNo: unknown, paymentId: unknown, now = Date.now()) => {
  if (!storage) return false
  const raw = readStorageItem(storage, INTENT_KEY)
  if (!raw) return false
  if (!removeStorageItem(storage, INTENT_KEY)) return false
  try {
    const intent = JSON.parse(raw)
    const age = now - Number(intent.created)
    return age >= 0 && age <= MAX_AGE_MS && intent.order === String(orderNo || '').trim() && intent.payment === Number(paymentId)
  } catch {
    return false
  }
}
