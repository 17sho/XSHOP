import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import {
  getBrowserStorage,
  readStorageItem,
  removeStorageItem,
  writeStorageItem,
} from '../src/utils/browserStorage.ts'

const originalWindow = Object.getOwnPropertyDescriptor(globalThis, 'window')

test.afterEach(() => {
  if (originalWindow) Object.defineProperty(globalThis, 'window', originalWindow)
  else Reflect.deleteProperty(globalThis, 'window')
})

test('Safari storage property access failures return no storage', () => {
  Object.defineProperty(globalThis, 'window', {
    configurable: true,
    value: Object.defineProperties({}, {
      sessionStorage: { get: () => { throw Error('SecurityError') } },
      localStorage: { get: () => { throw Error('SecurityError') } },
    }),
  })

  assert.equal(getBrowserStorage('sessionStorage'), null)
  assert.equal(getBrowserStorage('localStorage'), null)
})

test('Safari storage method failures are contained by every operation', () => {
  const blocked = {
    getItem: (_key: string): string | null => { throw Error('SecurityError') },
    setItem: (_key: string, _value: string): void => { throw Error('QuotaExceededError') },
    removeItem: (_key: string): void => { throw Error('SecurityError') },
  }

  assert.equal(readStorageItem(blocked, 'key'), null)
  assert.equal(writeStorageItem(blocked, 'key', 'value'), false)
  assert.equal(removeStorageItem(blocked, 'key'), false)
})

test('checkout recovery and buy-now persistence share the safe storage boundary', () => {
  const pending = readFileSync(new URL('../src/utils/pendingCheckoutOrder.ts', import.meta.url), 'utf8')
  const redirect = readFileSync(new URL('../src/utils/checkoutRedirectIntent.ts', import.meta.url), 'utf8')
  const buyNow = readFileSync(new URL('../src/utils/buyNowPersistence.ts', import.meta.url), 'utf8')
  const store = readFileSync(new URL('../src/stores/buyNow.ts', import.meta.url), 'utf8')

  assert.match(pending, /from '.\/browserStorage(?:\.ts)?'/)
  assert.match(redirect, /from '.\/browserStorage(?:\.ts)?'/)
  assert.match(buyNow, /from '.\/browserStorage(?:\.ts)?'/)
  assert.match(store, /getBrowserStorage\('sessionStorage'\)/)
  assert.match(store, /getBrowserStorage\('localStorage'\)/)
  assert.doesNotMatch(store, /const safeStorage/)
})
