import assert from 'node:assert/strict'
import fs from 'node:fs'
import test from 'node:test'
const read = (path: string) => fs.readFileSync(new URL(`../src/${path}`, import.meta.url), 'utf8')

test('upstream out-in route opacity has no fullPath identity or transformed ancestor', () => {
  const app = read('App.vue')
  assert.equal((app.match(/<Transition name="page-fade" mode="out-in"[^>]*>/g) || []).length, 2)
  assert.match(app, /transition: opacity 200ms ease;/)
  assert.doesNotMatch(app, /fullPath|:key=|transform|motionController|playContentRouteEnter|data-route-motion/)
  for (const path of ['views/PersonalCenter.vue', 'templates/vault/PersonalCenter.vue', 'views/reseller/ResellerConsoleLayout.vue']) {
    assert.doesNotMatch(read(path), /data-route-motion|playContentRouteEnter|v-motion-change/)
  }
})

test('same-path forms remain unkeyed and site sections retain their mounted drafts', () => {
  for (const path of ['views/auth/Login.vue', 'templates/vault/auth/Login.vue', 'views/GuestOrders.vue', 'components/reseller/ResellerSiteConfigPanel.vue']) {
    assert.doesNotMatch(read(path), /v-motion-change|vMotionChange|<form[^>]*:key|<Transition/)
  }
  assert.equal((read('components/reseller/ResellerSiteConfigPanel.vue').match(/v-show="activeSection/g) || []).length, 4)
  assert.match(read('templates/vault/GuestOrders.vue'), /import GuestOrders from '\.\.\/\.\.\/views\/GuestOrders.vue'/)
})
