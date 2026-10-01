import assert from 'node:assert/strict'
import fs from 'node:fs'
import test from 'node:test'

const read = (path: string) => fs.readFileSync(new URL(`../${path}`, import.meta.url), 'utf8')
const registerLogic = read('src/composables/useRegister.ts')
const views = [read('src/views/auth/Register.vue'), read('src/templates/vault/auth/Register.vue')]

test('registration header matches login while configured brand metadata stays available', () => {
  assert.match(registerLogic, /brandLogo/)
  assert.match(registerLogic, /site_logo/)
  for (const view of views) {
    assert.doesNotMatch(view, /<img\s+v-if="brandLogo"/)
    assert.match(view, /{{ brandSiteName }}/)
    assert.match(view, /userAuthStore, brandSiteName,/)
    assert.doesNotMatch(view, /<p class="mt-3 [^"]*">{{ brandSiteName }}/)
  }
})

test('registration controls clear the mobile header and keep room for bottom navigation', () => {
  assert.match(views[0], /items-start[\s\S]*pb-28[\s\S]*pt-24[\s\S]*sm:items-center/)
  assert.match(views[1], /items-start[\s\S]*pb-28[\s\S]*pt-20[\s\S]*sm:items-center/)
})
