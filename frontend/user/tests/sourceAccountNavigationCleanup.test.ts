import assert from 'node:assert/strict'
import fs from 'node:fs'
import test from 'node:test'

const read = (path: string) => fs.readFileSync(new URL(path, import.meta.url), 'utf8')

const router = read('../src/router/index.ts')
const app = read('../src/App.vue')
const navbar = read('../src/components/Navbar.vue')
const mobileNav = read('../src/components/MobileBottomNav.vue')
const navConfig = read('../src/composables/useNavConfig.ts')
const vaultLayout = read('../src/templates/vault/layout/VaultLayout.vue')
const security = read('../src/views/personal/SecurityPanel.vue')
const register = read('../src/composables/useRegister.ts')
const classicPersonalCenter = read('../src/views/PersonalCenter.vue')
const vaultPersonalCenter = read('../src/templates/vault/PersonalCenter.vue')
const enUS = read('../src/i18n/locales/en-US.json')
const zhCN = read('../src/i18n/locales/zh-CN.json')
const zhTW = read('../src/i18n/locales/zh-TW.json')

const routeBlock = (path: string) => {
  const escaped = path.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const match = router.match(new RegExp(`\\{\\s*path: '${escaped}',[\\s\\S]*?\\n\\s*\\},`))
  assert.ok(match, `route ${path} should exist`)
  return match[0]
}

test('root renders products directly and removed destinations are absent from storefront navigation', () => {
  const root = routeBlock('/')
  assert.match(root, /name: 'products'/)
  assert.match(root, /component: Products/)
  assert.doesNotMatch(root, /redirect:/)

  assert.doesNotMatch(app, /<Footer\b|import Footer from/)
  assert.doesNotMatch(navbar, /to="\/cart"|ShoppingCart|cartCount|useCartStore/)
  assert.doesNotMatch(mobileNav, /\/cart|\/products|ShoppingCart|cartCount|useCartStore/)
  assert.doesNotMatch(navConfig, /blog: \{|about: \{|key: 'products'/)
  assert.doesNotMatch(vaultLayout, /to="\/(?:cart|products)"|<footer\b|\/blog|\/about|ShoppingCart/)
})

test('advanced account URLs are restored and reseller forwards to the dedicated console', () => {
  for (const path of ['/me/api', '/me/affiliate', '/me/reseller']) {
    assert.match(router, new RegExp(`path: ['"]${path.replaceAll('/', '\\/')}['"]`))
  }
  assert.match(router, /path: '\/reseller'/)
  assert.match(router, /path: '\/me\/reseller'[^\n]*redirect: '\/reseller'/)
})

test('classic and vault personal centers render restored advanced account panels', () => {
  for (const source of [classicPersonalCenter, vaultPersonalCenter]) {
    assert.match(source, /AffiliatePanel/)
    assert.match(source, /ApiPanel/)
  }
})

test('security restores original identity and email controls before history, password and 2FA', () => {
  const template = security.slice(0, security.indexOf('<script'))
  const sections = ['TelegramBindingSection', 'GoogleBindingSection', 'EmailChangeForm', 'LoginHistorySection', 'PasswordChangeForm', 'TwoFactorSection']
  const positions = sections.map(name => {
    assert.match(security, new RegExp(`<${name}\\b`))
    return template.indexOf(`<${name}`)
  })
  assert.ok(positions.every((position, i) => i === 0 || position > positions[i - 1]), 'original panel order')
  assert.doesNotMatch(template, /LogOut|navbar\.logout/)
  assert.match(security, /useUserAuthStore\(\)/)
  const expected = ['Change email with double verification codes', '通过双验证码流程完成邮箱换绑', '透過雙驗證碼流程完成信箱更換']
  ;[enUS, zhCN, zhTW].forEach((messages, index) => {
    const parsed = JSON.parse(messages)
    assert.equal(parsed.personalCenter.security.subtitle, expected[index])
    assert.ok(parsed.personalCenter.security.subtitleBindOnly)
  })
})

test('registration verification remains backend-controlled through shared native registration logic', () => {
  assert.match(register, /emailVerificationEnabled = computed\(\(\) => appStore\.config\?\.email_verification_enabled !== false\)/)
  assert.match(register, /userAuthAPI\.sendVerifyCode/)
  assert.match(register, /if \(!operation\.current\(\)\) return/)
  assert.match(register, /purpose: 'register'/)
  assert.match(register, /code: emailVerificationEnabled\.value \? code\.value : ''/)
})

test('route changes avoid duplicate interactive page trees', () => {
  assert.equal((app.match(/<Transition name="page-fade" mode="out-in"[^>]*>/g) || []).length, 2)
  assert.doesNotMatch(app, /:key="[^"]*fullPath/)
  assert.doesNotMatch(app, /:key="routeRenderKey\(route\)"/)
  assert.doesNotMatch(app, /auth-route-page|auth-page-enter/)

})
