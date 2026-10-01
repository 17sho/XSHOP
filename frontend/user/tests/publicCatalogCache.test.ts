import assert from 'node:assert/strict'
import test from 'node:test'
import {
  readPublicCatalogCache,
  writePublicCatalogCache,
} from '../src/utils/publicCatalogCache.ts'

const storage = () => {
  const values = new Map<string, string>()
  return {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => { values.set(key, value) },
    removeItem: (key: string) => { values.delete(key) },
  }
}

test('catalog cache restores matching host and first-page content', () => {
  const session = storage()
  const value = { products: [{ id: 1 }], categories: [{ id: 2 }], totalPages: 3 }
  writePublicCatalogCache(session, 'store.example', value, 1_000)
  assert.deepEqual(readPublicCatalogCache(session, 'store.example', 1_200), value)
  assert.equal(readPublicCatalogCache(session, 'other.example', 1_200), null)
})

test('catalog cache expires and rejects malformed content', () => {
  const session = storage()
  writePublicCatalogCache(session, 'store.example', { products: [], categories: [], totalPages: 0 }, 1_000)
  assert.equal(readPublicCatalogCache(session, 'store.example', 301_001), null)
  session.setItem('dujiao:public-catalog:store.example', '{broken')
  assert.equal(readPublicCatalogCache(session, 'store.example', 1_100), null)
})
