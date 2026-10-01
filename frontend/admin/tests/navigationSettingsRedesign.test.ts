import assert from 'node:assert/strict'
import fs from 'node:fs'
import test from 'node:test'

const read = (path: string) => fs.readFileSync(new URL(`../src/${path}`, import.meta.url), 'utf8')
const settings = read('views/admin/Settings.vue')
const navigation = read('views/admin/components/SettingsNavigationTab.vue')
const messages = read('i18n/index.ts')

test('settings shell keeps its existing language, save and tab navigation', () => {
  assert.doesNotMatch(settings, /settings-mobile-module-select/)
  assert.match(settings, /<TabsList class="h-auto flex-wrap gap-1">/)
  assert.doesNotMatch(settings, /v-if="currentTab !== 'navigation'"/)
})

test('navigation optimization stays inside the navigation tab', () => {
  assert.match(navigation, /navigation-settings-content/)
  assert.doesNotMatch(navigation, /navigation-settings-header|navigation-save-bar/)
  assert.match(settings, /<SettingsNavigationTab ref="navigationTabRef" :current-lang="currentLang"/)
})

test('built-in navigation rows have explicit state and full-size controls', () => {
  assert.match(navigation, /:id="`nav-\$\{key\}`"/)
  assert.match(navigation, /:for="`nav-\$\{key\}`"/)
  assert.match(navigation, /'notice'/)
  assert.doesNotMatch(navigation, /'blog'|'about'|builtin\.blog|builtin\.about/)
  assert.match(navigation, /'visible' : 'hidden'/)
})

test('custom navigation exposes capacity and an actionable empty state', () => {
  assert.match(navigation, /form\.customItems\.length[^\n]*MAX_CUSTOM_ITEMS/)
  assert.match(navigation, /custom\.emptyHint/)
  assert.match(navigation, /custom\.addFirst/)
})

test('navigation and homepage notices have distinct labels and descriptions in all locales', () => {
  const blocks = messages.match(/navigation:\s*\{[\s\S]*?\n\s*orderEmailTemplate:/g) || []
  assert.equal(blocks.length, 3)
  for (const [index, label] of ['导航通知', '導航通知', 'Navigation notifications'].entries()) {
    assert.ok(blocks[index]!.includes(`notice: '${label}'`))
    assert.match(blocks[index]!, /homepageNotice:/)
    assert.match(blocks[index]!, /homepageNoticeHint:/)
    assert.match(blocks[index]!, /noticeHint:/)
  }
  assert.match(navigation, /for="homepage-notice"/)
  assert.match(navigation, /id="homepage-notice" v-model="form.homepageNoticeEnabled"/)
})

test('notification label is consistent in every admin locale', () => {
  assert.match(messages, /notice: '导航通知'/)
  assert.match(messages, /notice: '導航通知'/)
  assert.match(messages, /notice: 'Navigation notifications'/)
  const navigationBlocks = messages.match(/navigation:\s*\{[\s\S]*?\n\s*orderEmailTemplate:/g) || []
  assert.equal(navigationBlocks.length, 3)
  for (const block of navigationBlocks) assert.doesNotMatch(block, /notice: '公告'/)
})

test('navigation redesign copy exists in every admin locale', () => {
  for (const key of ['editingLanguage', 'save', 'saving', 'visible', 'hidden', 'emptyHint', 'addFirst']) {
    assert.ok((messages.match(new RegExp(`${key}:`, 'g')) || []).length >= 3, `${key} must exist in all locales`)
  }
})
