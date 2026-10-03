import assert from 'node:assert/strict'
import fs from 'node:fs'
import test from 'node:test'

const read = (path: string) => fs.readFileSync(new URL(`../${path}`, import.meta.url), 'utf8')

test('order lookup keeps only browser and credential modes', () => {
  const api = read('src/api/order.ts')
  const composable = read('src/composables/useGuestOrders.ts')
  const classic = read('src/views/GuestOrders.vue')
  const vault = read('src/templates/vault/GuestOrders.vue')

  assert.match(api, /browserOrders/)
  assert.match(api, /\/guest\/orders\/browser/)
  assert.match(composable, /activeTab/)
  assert.match(composable, /loadBrowserOrders/)
  assert.match(composable, /searchByCredentials/)
  assert.doesNotMatch(composable, /searchByOrderNo/)
  assert.match(classic, /guestOrders\.tabs\.browser/)
  assert.match(classic, /guestOrders\.tabs\.credentials/)
  assert.doesNotMatch(classic, /guestOrders\.tabs\.orderNo/)
  assert.match(vault, /import GuestOrders from '\.\.\/\.\.\/views\/GuestOrders\.vue'/)
  assert.match(vault, /<GuestOrders\s*\/>/)
  assert.doesNotMatch(vault, /useGuestOrders\(\)/)
})

test('mobile navigation keeps product, order lookup and account', () => {
  const mobile = read('src/components/MobileBottomNav.vue')
  assert.match(mobile, /\/guest\/orders/)
  assert.match(mobile, /ClipboardList/)
  assert.match(mobile, /bottomNav\.orders/)
})

test('static document carries Chinese brand metadata without runtime patches', () => {
  const html = read('index.html')
  assert.match(html, /<html lang="zh-CN">/)
  assert.match(html, /<title>XSHOP<\/title>/)
  assert.doesNotMatch(html, /<link rel="icon"/)
  assert.match(read("src/stores/app.ts"), /siteIconLinks\(config.value\)/)
  assert.doesNotMatch(html, /MutationObserver|XMLHttpRequest/)
})

test('notice list opens notice details through the shared detail view', () => {
  const postList = read('src/composables/usePostList.ts')
  assert.match(postList, /type === 'notice' \? `\/notice\/\$\{slug\}` : `\/blog\/\$\{slug\}`/)
  const router = read('src/router/index.ts')
  assert.match(router, /path: '\/notice\/:slug'/)
  const detail = read('src/composables/useBlogDetail.ts')
  assert.match(detail, /route\.name === 'notice-detail' \? '\/notice' : '\/blog'/)
})

test('notice-facing labels use notification and history wording', () => {
  const zhCN = JSON.parse(read('src/i18n/locales/zh-CN.json'))
  const zhTW = JSON.parse(read('src/i18n/locales/zh-TW.json'))
  const enUS = JSON.parse(read('src/i18n/locales/en-US.json'))
  assert.equal(zhCN.nav.notice, '通知')
  assert.equal(zhCN.notice.title, '历史通知')
  assert.equal(zhCN.blogDetail.backToNotice, '返回历史通知')
  assert.equal(zhTW.nav.notice, '通知')
  assert.equal(zhTW.notice.title, '歷史通知')
  assert.equal(enUS.nav.notice, 'Notifications')
  assert.equal(enUS.notice.title, 'Notification History')
  for (const page of [read('src/views/Notice.vue'), read('src/templates/vault/Notice.vue')]) {
    assert.match(page, /notice\.title/)
  }
})
