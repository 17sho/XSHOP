import assert from 'node:assert/strict'
import fs from 'node:fs'
import test from 'node:test'

const settings = fs.readFileSync(new URL('../src/views/admin/Settings.vue', import.meta.url), 'utf8')
const i18n = fs.readFileSync(new URL('../src/i18n/index.ts', import.meta.url), 'utf8')

test('template settings expose the new product catalog card/list layout', () => {
  assert.match(settings, /product_catalog_layout/)
  assert.match(settings, /value="card"/)
  assert.match(settings, /value="list"/)
  assert.doesNotMatch(settings, /template_mode/)
  assert.match(i18n, /catalogLayoutTitle/)
  assert.match(i18n, /catalogCardMode/)
  assert.match(i18n, /catalogListMode/)
})
