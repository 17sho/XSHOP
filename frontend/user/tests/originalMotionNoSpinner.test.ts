import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
const root = new URL('../src/', import.meta.url)
const read = (file: string) => fs.readFileSync(new URL(file, root), 'utf8')
function sources(dir: URL): string[] {
  return fs.readdirSync(dir, { withFileTypes: true }).flatMap(entry => {
    const url = new URL(entry.name + (entry.isDirectory() ? '/' : ''), dir)
    return entry.isDirectory() ? sources(url) : /\.(vue|css|ts)$/.test(entry.name) ? [url.pathname] : []
  })
}
test('only category refresh restores spinning loader markup', () => {
  const offenders = sources(root).filter(file => /animate-spin|@keyframes\s+spin\b|animation:\s*spin\b/.test(fs.readFileSync(file, 'utf8')))
  assert.deepEqual(offenders.map(file => path.relative(root.pathname, file)).sort(), ['templates/vault/Products.vue', 'views/Products.vue'])
})
test('pagination preserves pending disablement and readable status without a ring', () => {
  const source = read('components/PaginationNav.vue')
  assert.doesNotMatch(source, /Loader2/)
  assert.match(source, /v-if="loading"[^>]*>[\s\S]*?t\('common.loading'\)/)
  assert.match(source, /currentPage === 1 \|\| loading/)
  assert.match(source, /currentPage === totalPages \|\| loading/)
  assert.match(source, /aria-live="polite"/)
})
test('OAuth callbacks retain processing labels, never substitute static loading circles', () => {
  const google = read('components/auth/GoogleIdentityButton.vue')
  assert.doesNotMatch(google, /border-t-transparent/)
  assert.match(google, /<span class="text-xs">\{\{ loadingLabel \}\}<\/span>/)
  for (const provider of ['Google', 'Telegram']) {
    const source = read(`templates/vault/auth/${provider}Callback.vue`)
    assert.match(source, /v-if="loading"/)
    assert.match(source, /processing/)
    assert.doesNotMatch(source, /border-t-primary|border-t-transparent/)
  }
})
test('legal and upload pending views retain visible non-spinning feedback', () => {
  for (const name of ['views/Legal.vue', 'templates/vault/Legal.vue', 'components/reseller/ResellerImageField.vue', 'components/reseller/ResellerRichText.vue']) {
    const source = read(name)
    assert.doesNotMatch(source, /Loader2/)
    assert.match(source, /t\('common.loading'\)/)
  }
  assert.match(read('components/reseller/ResellerImageField.vue'), /:disabled="uploading"/)
  assert.match(read('components/reseller/ResellerRichText.vue'), /:disabled="uploading"/)
})
