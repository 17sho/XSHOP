import assert from 'node:assert/strict'
import fs from 'node:fs'
import test from 'node:test'
const read = (path: string) => fs.readFileSync(new URL(`../src/${path}`, import.meta.url), 'utf8')

test('API and gift panels retain original 450ms entry and success feedback without permanent transforms', () => {
  for (const [name, enter, success] of [['Api', 'api-panel-enter', 'new-secret-burst'], ['GiftCard', 'gift-card-panel-enter', 'gift-card-success-burst']]) {
    const source = read(`views/personal/${name}Panel.vue`)
    assert.ok(source.includes(`animation: ${enter} 0.45s ease backwards;`))
    assert.ok(source.includes(`animation: ${success} 0.45s ease backwards;`))
    assert.match(source, /transform: translateY\(10px\)/)
    assert.match(source, /transform: translateY\(8px\) scale\(0\.98\)/)
    assert.match(source, /transform: none;/)
    assert.doesNotMatch(source, /motion-success-feedback|ease both|forwards/)
  }
  assert.doesNotMatch(read('style.css'), /motion-success-feedback/)
})

test('global reduced-motion policy covers both themes, pseudo-elements and teleports without hiding loading status', () => {
  const css = read('style.css')
  assert.match(css, /@media \(prefers-reduced-motion: reduce\)\s*\{\s*\*,\s*\*::before,\s*\*::after\s*\{/)
  const policy = css.slice(css.indexOf('@media (prefers-reduced-motion: reduce)')).split('\n}')[0]!
  assert.match(policy, /animation: none !important/)
  assert.match(policy, /transition-duration: 0s !important/)
  assert.match(policy, /transition-delay: 0s !important/)
  assert.match(policy, /scroll-behavior: auto !important/)
  assert.doesNotMatch(policy, /display:|visibility:|opacity:|transform:/)
  assert.doesNotMatch(read('templates/vault/styles/vault.css'), /prefers-reduced-motion/)
  assert.match(read('components/BackToTop.vue'), /scrollPageToTop/)
  assert.doesNotMatch(read('components/BackToTop.vue'), /behavior: 'smooth'/)
})

test('compositing is not permanently preallocated and visibility transition has a single owner', () => {
  assert.doesNotMatch(read('templates/vault/components/VaultProductCard.vue'), /will-change-transform/)
  const backToTop = read('components/BackToTop.vue')
  assert.doesNotMatch(backToTop, /transition-all|enter-active-class|leave-active-class/)
  assert.match(backToTop, /<Transition name="back-to-top">/)
  assert.match(backToTop, /transition-none/)
  assert.match(backToTop, /transition: opacity 160ms ease-out, transform 160ms ease-out/)
  assert.doesNotMatch(read('components/product/ProductMobileBar.vue'), /backdrop-blur/)
})
