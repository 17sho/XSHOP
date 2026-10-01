import assert from 'node:assert/strict'
import test from 'node:test'
import {
  readBuyNowItem,
  writeBuyNowItem,
  readDurableBuyNowItem,
  writeDurableBuyNowItem,
} from '../src/utils/buyNowPersistence.ts'

const storage = () => {
  const values = new Map<string, string>()
  return {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => { values.set(key, value) },
    removeItem: (key: string) => { values.delete(key) },
  }
}

test('buy-now checkout item survives a page refresh in the same tab', () => {
  const session = storage()
  const item = { productId: 7, slug: 'mailbox', title: { 'zh-CN': '邮箱' }, priceAmount: '9.90', quantity: 1 }
  writeBuyNowItem(session, item)
  assert.deepEqual(readBuyNowItem(session), item)
})

test('clearing or corrupting the buy-now checkout removes stale state', () => {
  const session = storage()
  writeBuyNowItem(session, null)
  assert.equal(readBuyNowItem(session), null)
  session.setItem('dujiao:buy-now-item:v1', '{broken')
  assert.equal(readBuyNowItem(session), null)
})

test('unsubmitted buy-now item survives a closed tab but expires after seven days', () => {
  const local = storage()
  const item = { productId: 7, slug: 'mailbox', title: { 'zh-CN': '邮箱' }, priceAmount: '9.90', quantity: 1 }
  writeDurableBuyNowItem(local, item, 1000)
  assert.deepEqual(readDurableBuyNowItem(local, 1000 + 2 * 24 * 60 * 60 * 1000), item)
  assert.equal(readDurableBuyNowItem(local, 1000 + 8 * 24 * 60 * 60 * 1000), null)
})

test('Safari storage method failures keep buy-now checkout usable in memory', () => {
  const blocked = {
    getItem: (_key: string): string | null => { throw Error('SecurityError') },
    setItem: (_key: string, _value: string): void => { throw Error('QuotaExceededError') },
    removeItem: (_key: string): void => { throw Error('SecurityError') },
  }
  const item = { productId: 7, slug: 'mailbox', title: {}, priceAmount: '9.90', quantity: 1 }
  assert.doesNotThrow(() => writeBuyNowItem(blocked, item))
  assert.doesNotThrow(() => writeDurableBuyNowItem(blocked, item))
  assert.equal(readBuyNowItem(blocked), null)
  assert.equal(readDurableBuyNowItem(blocked), null)
})

test('durable buy-now draft is valid at the seven-day TTL boundary', () => {
  const local = storage()
  const item = { productId: 7, slug: 'mailbox', title: {}, priceAmount: '9.90', quantity: 1 }
  writeDurableBuyNowItem(local, item, 1000)
  assert.deepEqual(readDurableBuyNowItem(local, 1000 + 7 * 24 * 60 * 60 * 1000), item)
})

test('completed buy-now order removes persistent draft so reopening cannot resubmit', () => {
  const local = storage()
  const item = { productId: 7, slug: 'mailbox', title: {}, priceAmount: '9.90', quantity: 1 }
  writeDurableBuyNowItem(local, item, 1000)
  writeDurableBuyNowItem(local, null, 1001)
  assert.equal(readDurableBuyNowItem(local, 1002), null)
})
