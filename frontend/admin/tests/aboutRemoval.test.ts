import assert from 'node:assert/strict'
import { existsSync, readFileSync } from 'node:fs'
import test from 'node:test'

const adminSource = (path: string) => readFileSync(new URL(`../src/${path}`, import.meta.url), 'utf8')
const userSource = (path: string) => readFileSync(new URL(`../../user/src/${path}`, import.meta.url), 'utf8')
const repoSource = (path: string) => readFileSync(new URL(`../../../${path}`, import.meta.url), 'utf8')

test('about page and its settings surface are fully removed while legal and contact remain', () => {
  const settings = adminSource('views/admin/Settings.vue')
  const adminMessages = adminSource('i18n/index.ts')
  const router = userSource('router/index.ts')
  const footer = userSource('components/Footer.vue')
  const sitemap = repoSource('internal/modules/sitemap/application/service.go')
  const normalizer = repoSource('internal/modules/settings/application/site_normalize.go')

  assert.doesNotMatch(settings, /settings\.tabs\.about|value=["']about["']|value:\s*['"]about['"]|form\.about|data\.about|settings\.about/)
  assert.match(settings, /settings\.tabs\.legal/)
  assert.match(settings, /contact:\s*form\.contact/)
  assert.doesNotMatch(adminMessages, /关于我们配置|關於我們配置|About Page Settings|配置前台 About 页面内容|配置前台 About 頁面內容|Configure front-end About page content/)

  assert.doesNotMatch(router, /path:\s*['"]\/about['"]|name:\s*['"]about['"]|About\.vue/)
  assert.doesNotMatch(footer, /\/about|nav\.about|builtin\.about/)
  assert.equal(existsSync(new URL('../../user/src/views/About.vue', import.meta.url)), false)
  assert.equal(existsSync(new URL('../../user/src/templates/vault/About.vue', import.meta.url)), false)
  assert.equal(existsSync(new URL('../../user/src/composables/useAbout.ts', import.meta.url)), false)

  assert.doesNotMatch(sitemap, /["']\/about["']/)
  assert.doesNotMatch(normalizer, /normalized\[["']about["']\]|normalizeSiteAbout/)
  assert.match(normalizer, /normalized\[["']contact["']\]/)
  assert.match(normalizer, /normalized\[["']legal["']\]/)
})
