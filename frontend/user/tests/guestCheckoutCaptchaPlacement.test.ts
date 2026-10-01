import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const checkoutPaths = ['../src/views/Checkout.vue', '../src/templates/vault/Checkout.vue']
const componentPath = '../src/components/checkout/GuestCheckoutCaptcha.vue'

for (const path of checkoutPaths) {
  test(`shared guest captcha stays beside order submission in ${path}`, () => {
    const source = readFileSync(new URL(path, import.meta.url), 'utf8')
    const template = source.slice(0, source.indexOf('<script setup'))
    const summary = template.indexOf("t('checkout.submitTitle')")
    const captcha = template.indexOf('<GuestCheckoutCaptcha')
    const submit = template.indexOf("t('checkout.submitButton')")

    assert.ok(summary >= 0)
    assert.ok(captcha > summary, 'captcha should not be stranded above the summary on mobile')
    assert.ok(captcha < submit)
    assert.doesNotMatch(template, /<ImageCaptcha|<TurnstileCaptcha/)
    assert.match(source, /import GuestCheckoutCaptcha from/)
  })
}

test('shared guest captcha preserves provider visibility, bindings, and stale-config event', () => {
  const source = readFileSync(new URL(componentPath, import.meta.url), 'utf8')

  assert.match(source, /v-if="enabled"/)
  assert.match(source, /v-if="provider === 'image'"/)
  assert.match(source, /v-else-if="provider === 'turnstile'"/)
  assert.match(source, /v-model="imagePayload"/)
  assert.match(source, /:disabled="disabled"/)
  assert.match(source, /@config-stale="emit\('config-stale'\)"/)
  assert.match(source, /v-model="turnstileToken"/)
  assert.match(source, /:site-key="turnstileSiteKey"/)
})

test('shared guest captcha keeps classic and vault presentation variants', () => {
  const source = readFileSync(new URL(componentPath, import.meta.url), 'utf8')

  assert.match(source, /variant: 'classic' \| 'vault'/)
  assert.match(source, /rounded-xl border bg-card p-4/)
  assert.match(source, /rounded-sm border bg-secondary p-3\.5/)
})
