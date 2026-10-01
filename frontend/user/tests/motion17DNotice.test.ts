import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { loader } from './helpers/motion17DHarness.ts'

test('latest-notice cache validates host, age, shape and stores only public summary fields', () => {
  const cache = loader()('utils/catalogNoticeCache.ts')
  const storage = window.sessionStorage; storage.clear()
  const notice = { id: 1, slug: 'fixture-notice', title: { 'zh-CN': '公告' }, summary: { 'en-US': 'Fixture' }, private: 'must-not-persist' }
  cache.writeCachedCatalogNotices(storage, 'fixture.example.invalid', [notice], 1000)
  assert.deepEqual(cache.readCachedCatalogNotices(storage, 'fixture.example.invalid', 1001), [{ id: 1, slug: 'fixture-notice', title: notice.title, summary: notice.summary }])
  assert.deepEqual(cache.readCachedCatalogNotices(storage, 'another.example.invalid', 1001), [])
  assert.deepEqual(cache.readCachedCatalogNotices(storage, 'fixture.example.invalid', 301001), [])
  cache.writeCachedCatalogNotices(storage, 'fixture.example.invalid', [{ ...notice, slug: '../evil' }], 1000)
  assert.deepEqual(cache.readCachedCatalogNotices(storage, 'fixture.example.invalid', 1001), [])
  assert.deepEqual(cache.readCachedCatalogNotices({ getItem() { throw Error() } }, 'fixture.example.invalid'), [])
})

test('cold notices stay below the catalog and warm notices occupy only their validated cached footprint', () => {
  const source = readFileSync(new URL('../src/views/Products.vue', import.meta.url), 'utf8')
  assert.match(source, /readCachedCatalogNotices\(noticeStorage, noticeHost\)/)
  assert.match(source, /const noticesAboveCatalog = latestNotices.value.length > 0/)
  assert.match(source, /'notices-cold': !noticesAboveCatalog/)
  assert.match(source, /\.notices-cold\s*\{\s*order:1/)
  assert.match(source, /if \(noticesDisposed \|\| !homepageNoticeEnabled.value \|\| generation !== noticeGeneration\) return/)
  assert.doesNotMatch(source, /category-switch-loading/, 'retired replacement spinner must not retain dead scoped CSS')
})

test('classic hero fade has scoped enter/leave and reduced-motion styles', () => {
  const source = readFileSync(new URL('../src/views/Products.vue', import.meta.url), 'utf8')
  assert.match(source, /\.banner-fade-enter-active[\s\S]*transition:\s*opacity/)
  assert.match(source, /\.banner-fade-leave-active/)
  assert.match(source, /prefers-reduced-motion[\s\S]*banner-fade-enter-active[\s\S]*transition:none/)
})
