import assert from 'node:assert/strict'
import fs from 'node:fs'
import test from 'node:test'

const read = (path: string) => fs.readFileSync(new URL(path, import.meta.url), 'utf8')

test('shared personal center navigation restores configurable affiliate reseller and API entries', () => {
  const source = read('../src/composables/usePersonalCenter.ts')
  for (const section of ['affiliate', 'reseller', 'api']) assert.match(source, new RegExp(`key: '${section}'`))
})

test('both themes render restored affiliate and API panels while reseller uses the dedicated console', () => {
  for (const file of ['../src/views/PersonalCenter.vue', '../src/templates/vault/PersonalCenter.vue']) {
    const source = read(file)
    for (const panel of ['AffiliatePanel', 'ApiPanel']) assert.match(source, new RegExp(`<${panel}\\b`))
  }
  const router = read('../src/router/index.ts')
  assert.match(router, /path: '\/me\/reseller'[^\n]*redirect: '\/reseller'/)
})
