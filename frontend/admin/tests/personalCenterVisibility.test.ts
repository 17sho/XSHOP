import assert from 'node:assert/strict'
import fs from 'node:fs'
import test from 'node:test'

const read = (path: string) => fs.readFileSync(new URL(`../${path}`, import.meta.url), 'utf8')
const settings = read('src/views/admin/Settings.vue')
const settingsTab = read('src/views/admin/components/SettingsPersonalCenterVisibilityTab.vue')
const userDetail = read('src/views/admin/UserDetail.vue')
const types = read('src/api/types.ts')
const i18n = read('src/i18n/index.ts')

test('settings expose six global personal-center visibility defaults', () => {
  assert.match(settings, /SettingsPersonalCenterVisibilityTab/)
  assert.match(settings, /personalCenterVisibility/)
  assert.match(settingsTab, /personal_center_visibility_config/)
  assert.match(settingsTab, /loadFailed/)
  assert.match(settingsTab, /if \(!loaded\.value \|\| loadFailed\.value\) return/)
  assert.match(settingsTab, /defineExpose\(\{ save, submitting, loaded, loadFailed \}\)/)
  assert.match(settings, /currentTab === 'personal_center_visibility'[^\n]*!personalCenterVisibilityTabRef\?\.loaded[^\n]*personalCenterVisibilityTabRef\?\.loadFailed/)
  for (const key of ['overview', 'orders', 'wallet', 'affiliate', 'reseller', 'gift_cards', 'security', 'api', 'profile']) {
    assert.match(settingsTab, new RegExp(key))
  }
})

test('user detail supports inherit show and hide with a sparse override payload', () => {
  assert.match(userDetail, /personal_center_visibility_overrides/)
  assert.match(userDetail, /personal_center_visibility_effective/)
  assert.match(userDetail, /inherit/)
  assert.match(userDetail, /visibilityOverridesPayload/)
  assert.match(userDetail, /adminAPI\.updateUser/)
  assert.match(types, /PersonalCenterVisibilityOverrides/)
})

test('personal-center visibility controls are localized in every admin locale', () => {
  const occurrences = (i18n.match(/personalCenterVisibility:/g) || []).length
  assert.equal(occurrences, 9)
  for (const label of ['inherit', 'show', 'hide']) assert.match(i18n, new RegExp(`${label}:`))
})
