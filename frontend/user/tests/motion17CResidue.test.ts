import assert from 'node:assert/strict'
import fs from 'node:fs'
import test from 'node:test'
const root = new URL('../src/', import.meta.url)
const read = (path: string) => fs.readFileSync(new URL(path, root), 'utf8')
const retired = ['components/Loading.vue']

test('proven unmounted implementations and unused bounce utility are physically absent', () => {
  for (const path of retired) assert.equal(fs.existsSync(new URL(path, root)), false, path)
  assert.doesNotMatch(read('style.css'), /theme-bounce-in/)
})

test('retained authentication and alternate theme capabilities still have their genuine entry points', () => {
  for (const name of ['TelegramBindingSection', 'GoogleBindingSection', 'EmailChangeForm', 'LoginHistorySection', 'PasswordChangeForm', 'TwoFactorSection']) {
    assert.ok(read('views/personal/SecurityPanel.vue').includes(`<${name}`))
    assert.ok(fs.existsSync(new URL(`components/security/${name}.vue`, root)))
  }
  assert.match(read('views/auth/Login.vue'), /<GoogleIdentityButton/)
  assert.match(read('router/index.ts'), /GoogleCallback|google-callback/)
  assert.match(read('router/index.ts'), /TelegramCallback|telegram-callback/)
  assert.match(read('templates/registry.ts'), /import.meta.glob\('\.\/vault\/\*\*\/\*\.vue'\)/)
  for (const path of ['templates/vault/ProductDetail.vue', 'templates/vault/components/VaultProductMobileBar.vue', 'templates/vault/components/VaultBannerHero.vue']) assert.ok(fs.existsSync(new URL(path, root)))
})
