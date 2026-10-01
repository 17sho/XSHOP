import assert from 'node:assert/strict'
import test from 'node:test'
import {
  readCachedHomepageAd,
  writeCachedHomepageAd,
} from '../src/utils/homepageAdCache.ts'

const storage = () => {
  const values = new Map<string, string>()
  return {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => { values.set(key, value) },
    removeItem: (key: string) => { values.delete(key) },
  }
}

test('homepage ad cache is scoped by host and survives a refresh in the same tab', () => {
  const session = storage()
  const ad = { title: { 'zh-CN': '站点公告' }, content: { 'zh-CN': '内容' } }
  writeCachedHomepageAd(session, 'shop.example.com', ad)
  assert.deepEqual(readCachedHomepageAd(session, 'shop.example.com'), ad)
  assert.equal(readCachedHomepageAd(session, 'tenant.example.com'), null)
})

test('invalid or removed cached homepage ads are not rendered', () => {
  const session = storage()
  session.setItem('dujiao:homepage-ad:shop.example.com', '{broken')
  assert.equal(readCachedHomepageAd(session, 'shop.example.com'), null)
  writeCachedHomepageAd(session, 'shop.example.com', null)
  assert.equal(readCachedHomepageAd(session, 'shop.example.com'), null)
})