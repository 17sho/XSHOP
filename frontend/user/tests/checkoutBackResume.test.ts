import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { getCheckoutSessionStorage, savePendingCheckoutOrder, readPendingCheckoutOrder } from '../src/utils/pendingCheckoutOrder.ts'

const checkout = readFileSync(new URL('../src/composables/useCheckout.ts', import.meta.url), 'utf8')
const storage = () => {
  const values = new Map<string, string>()
  return { getItem: (key: string) => values.get(key) ?? null, setItem: (key: string, value: string) => { values.set(key, value) }, removeItem: (key: string) => { values.delete(key) } }
}

test('unpaid created order is recoverable on empty checkout, without restoring cart', () => {
  const store = storage()
  savePendingCheckoutOrder(store, 'DJ123', true, 1000)
  assert.deepEqual(readPendingCheckoutOrder(store, 1001), { orderNo: 'DJ123', guest: true })
  assert.equal(readPendingCheckoutOrder(store, 1000 + 24 * 60 * 60 * 1000 + 1), null)
  assert.match(checkout, /savePendingCheckoutOrder\(checkoutStorage, responseData\.order_no, !userAuthStore\.isAuthenticated\)/)
  const router = readFileSync(new URL('../src/router/index.ts', import.meta.url), 'utf8')
  assert.match(router, /readPendingCheckoutOrder\(getBrowserStorage\('sessionStorage'\)\)/)
})

test('storage failure is best effort after an order already exists', () => {
  const unavailable = {
    getItem: (_key: string): string | null => { throw Error('storage denied') },
    setItem: (_key: string, _value: string): void => { throw Error('storage denied') },
    removeItem: (_key: string): void => { throw Error('storage denied') },
  }
  assert.doesNotThrow(() => savePendingCheckoutOrder(unavailable, 'DJ123', true))
  assert.equal(readPendingCheckoutOrder(unavailable), null)
  const router = readFileSync(new URL('../src/router/index.ts', import.meta.url), 'utf8')
  assert.match(router, /readPendingCheckoutOrder\(getBrowserStorage\('sessionStorage'\)\)/)
})

test('invalid or missing pending order cannot redirect to a payment page', () => {
  const store = storage()
  assert.equal(readPendingCheckoutOrder(store), null)
  savePendingCheckoutOrder(store, '', false)
  assert.equal(readPendingCheckoutOrder(store), null)
})

test('member and guest pending orders retain their route identity through refresh', () => {
  const memberStore = storage()
  const guestStore = storage()
  savePendingCheckoutOrder(memberStore, ' MEMBER-1 ', false, 1000)
  savePendingCheckoutOrder(guestStore, 'GUEST-1', true, 1000)
  assert.deepEqual(readPendingCheckoutOrder(memberStore, 1000 + 24 * 60 * 60 * 1000), { orderNo: 'MEMBER-1', guest: false })
  assert.deepEqual(readPendingCheckoutOrder(guestStore, 1001), { orderNo: 'GUEST-1', guest: true })
})

test('Safari throwing sessionStorage getter cannot block checkout recovery', () => {
  const originalWindow = Object.getOwnPropertyDescriptor(globalThis, 'window')
  Object.defineProperty(globalThis, 'window', {
    configurable: true,
    value: Object.defineProperty({}, 'sessionStorage', { get: () => { throw Error('SecurityError') } }),
  })
  try {
    assert.equal(getCheckoutSessionStorage(), null)
  } finally {
    if (originalWindow) Object.defineProperty(globalThis, 'window', originalWindow)
    else Reflect.deleteProperty(globalThis, 'window')
  }
})

test('empty checkout resolves before mounting and preserves an existing pending order', () => {
  const router = readFileSync(new URL('../src/router/index.ts', import.meta.url), 'utf8')
  assert.match(router, /if \(to\.name === 'checkout'\) \{[\s\S]*?readPendingCheckoutOrder\(getBrowserStorage\('sessionStorage'\)\)[\s\S]*?return next\(\{ path: '\/pay'[\s\S]*?return next\(\{ path: '\/'[\s\S]*?\}/)
  assert.doesNotMatch(checkout.slice(checkout.indexOf('  onMounted(async () => {'), checkout.indexOf('  onUnmounted(() => {')), /router\.replace/)
})
