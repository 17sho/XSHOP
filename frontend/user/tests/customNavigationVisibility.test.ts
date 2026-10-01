import assert from 'node:assert/strict'
import fs from 'node:fs'
import test from 'node:test'

const read = (path: string) => fs.readFileSync(new URL(`../${path}`, import.meta.url), 'utf8')

const navbar = read('src/components/Navbar.vue')
const vaultLayout = read('src/templates/vault/layout/VaultLayout.vue')
const navConfig = read('src/composables/useNavConfig.ts')

test('classic mobile header exposes configured secondary navigation', () => {
  assert.match(navbar, /secondaryNavItems/)
  assert.match(navbar, /v-if="mobileMenuItems\.length"/)
  assert.match(navbar, /v-for="item in mobileMenuItems"/)
  assert.match(navbar, /item\.type === 'route'/)
  assert.match(navbar, /:href="item\.path"/)
})

test('both active storefront templates consume the shared custom navigation contract', () => {
  assert.match(navConfig, /customNavItems/)
  assert.match(navConfig, /secondaryNavItems/)
  assert.match(navbar, /secondaryNavItems/)
  assert.match(vaultLayout, /secondaryNavItems/)
})
