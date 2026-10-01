import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { createCheckoutRedirectIntent, consumeCheckoutRedirectIntent } from '../src/utils/checkoutRedirectIntent.ts'

const checkout = readFileSync(new URL('../src/composables/useCheckout.ts', import.meta.url), 'utf8')
const payment = readFileSync(new URL('../src/composables/usePayment.ts', import.meta.url), 'utf8')

test('checkout-created payment opens only once, and only for matching payment/order', () => {
  const store = new Map<string, string>()
  const storage = { getItem: (key: string) => store.get(key) ?? null, setItem: (key: string, val: string) => { store.set(key, val) }, removeItem: (key: string) => { store.delete(key) } }
  createCheckoutRedirectIntent(storage, 'DJ123', 42, 1000)
  assert.equal(consumeCheckoutRedirectIntent(storage, 'DJ123', 41, 1001), false)
  assert.equal(consumeCheckoutRedirectIntent(storage, 'DJ123', 42, 1002), false)
  createCheckoutRedirectIntent(storage, 'DJ123', 42, 1000)
  assert.equal(consumeCheckoutRedirectIntent(storage, 'DJ123', 42, 1001), true)
  assert.equal(consumeCheckoutRedirectIntent(storage, 'DJ123', 42, 1002), false)
  createCheckoutRedirectIntent(storage, 'DJ123', 42, 1000)
  assert.equal(consumeCheckoutRedirectIntent(storage, 'DJ123', 42, 121001), false)
})

test('redirect intent remains valid at TTL boundary and is consumed even on mismatch', () => {
  const store = new Map<string, string>()
  const storage = { getItem: (key: string) => store.get(key) ?? null, setItem: (key: string, val: string) => { store.set(key, val) }, removeItem: (key: string) => { store.delete(key) } }
  createCheckoutRedirectIntent(storage, 'DJ123', 42, 1000)
  assert.equal(consumeCheckoutRedirectIntent(storage, 'DJ123', 42, 61_000), true)
  createCheckoutRedirectIntent(storage, 'DJ123', 42, 1000)
  assert.equal(consumeCheckoutRedirectIntent(storage, 'OTHER', 42, 1001), false)
  assert.equal(consumeCheckoutRedirectIntent(storage, 'DJ123', 42, 1002), false)
})

test('payment redirect intent is best effort when browser storage is blocked', () => {
  const blocked = {
    getItem: (_key: string): string | null => { throw Error('storage denied') },
    setItem: (_key: string, _value: string): void => { throw Error('storage denied') },
    removeItem: (_key: string): void => { throw Error('storage denied') },
  }
  assert.doesNotThrow(() => createCheckoutRedirectIntent(blocked, 'DJ123', 42))
  assert.equal(consumeCheckoutRedirectIntent(blocked, 'DJ123', 42), false)
  const payment = readFileSync(new URL('../src/composables/usePayment.ts', import.meta.url), 'utf8')
  assert.match(payment, /consumeCheckoutRedirectIntent\(getBrowserStorage\('sessionStorage'\), orderNoResolved\.value, data\.payment_id\)/)
})

test('redirect intent does not auto-open when it cannot be consumed', () => {
  const raw = JSON.stringify({ order: 'DJ123', payment: 42, created: 1000 })
  const storage = {
    getItem: (_key: string) => raw,
    setItem: (_key: string, _value: string): void => {},
    removeItem: (_key: string): void => { throw Error('storage removal denied') },
  }

  assert.equal(consumeCheckoutRedirectIntent(storage, 'DJ123', 42, 1001), false)
  assert.equal(consumeCheckoutRedirectIntent(storage, 'DJ123', 42, 1002), false)
})

test('checkout passes the created payment identity to the next page; ordinary restores never reopen', () => {
  assert.match(checkout, /createCheckoutRedirectIntent\(checkoutStorage, responseData\.order_no, responseData\.payment_id/)
  assert.match(checkout, /const checkoutStorage = getBrowserStorage\('sessionStorage'\)/)
  assert.match(payment, /consumeCheckoutRedirectIntent\(getBrowserStorage\('sessionStorage'\), orderNoResolved\.value, data\.payment_id/)
  assert.match(payment, /if \(shouldAutoOpenPaymentLink\(data\)\) openPayLinkInCompatibleWindow\(true\)/)
  const restore = payment.match(/const loadLatestPayment = async \(\) => \{([\s\S]*?)\n  \}\n\n  const buildPayRouteQuery/)?.[1] || ''
  assert.match(restore, /consumeCheckoutRedirectIntent/)
})
