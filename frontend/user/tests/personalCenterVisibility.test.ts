import assert from 'node:assert/strict'
import fs from 'node:fs'
import test from 'node:test'

const read = (path: string) => fs.readFileSync(new URL(`../${path}`, import.meta.url), 'utf8')
const helper = read('src/utils/personalCenterVisibility.ts')
const personal = read('src/composables/usePersonalCenter.ts')
const router = read('src/router/index.ts')
const classic = read('src/views/PersonalCenter.vue')
const vault = read('src/templates/vault/PersonalCenter.vue')
const types = read('src/api/types.ts')

test('personal-center visibility normalizes six defaults and resolves safe fallbacks', () => {
  for (const key of ['overview', 'orders', 'wallet', 'affiliate', 'reseller', 'gift_cards', 'security', 'api', 'profile']) {
    assert.match(helper, new RegExp(key))
  }
  assert.match(helper, /DEFAULT_PERSONAL_CENTER_VISIBILITY/)
  assert.match(helper, /firstVisiblePersonalCenterPath/)
  assert.match(helper, /:\s*'\/'/)
  assert.match(helper, /gift_cards:\s*'giftCard'/)
  assert.match(types, /personal_center_visibility/)
})

test('all personal routes carry visibility metadata and hidden direct routes are guarded from fresh profile data', () => {
  for (const key of ['overview', 'orders', 'wallet', 'affiliate', 'reseller', 'gift_cards', 'security', 'api', 'profile']) {
    assert.match(router, new RegExp(`personalSection:\\s*'${key}'`))
  }
  assert.match(router, /path: '\/reseller'[\s\S]*?personalSection: 'reseller'/)
  assert.ok(router.indexOf("to.meta.resellerConsole && !appStore.canAccessResellerConsole") < router.indexOf('to.meta.personalSection'))
  assert.match(router, /ensureProfileLoaded/)
  assert.match(router, /resolvePersonalCenterFallback/)
  assert.doesNotMatch(router, /localStorage[^\n]*personal_center_visibility/)
})

test('classic and vault share filtered sections and hide order overview data when orders are hidden', () => {
  assert.match(personal, /visibleSectionItems = computed/)
  assert.match(personal, /personalCenterVisibility/)
  assert.match(personal, /canViewOrders/)
  assert.match(personal, /if \(canViewOrders\.value\)/)
  for (const source of [classic, vault]) {
    assert.match(source, /visibleSectionItems/)
    assert.match(source, /v-if="canViewOrders"/)
  }
})
