import assert from 'node:assert/strict'
import fs from 'node:fs'
import test from 'node:test'

const app = fs.readFileSync(new URL('../src/App.vue', import.meta.url), 'utf8')
const style = fs.readFileSync(new URL('../src/style.css', import.meta.url), 'utf8')
const card = fs.readFileSync(new URL('../src/components/ProductCard.vue', import.meta.url), 'utf8')

test('route changes use a short entering-page animation', () => {
  assert.equal((app.match(/<Transition name="page-fade" mode="out-in"[^>]*>/g) || []).length, 2)
  assert.match(app, /transition: opacity 200ms ease;/)
  assert.doesNotMatch(app, /fullPath|:key=|\.animate|motionController|transform/)
  // CSS blur is forbidden; HTMLElement.blur() deliberately releases retiring focus.
  assert.doesNotMatch(app, /filter\s*:[^;\n]*blur\(/)
  assert.match(app, /focused\.blur\(\)/)
})

test('product cards use a short staggered upward reveal', () => {
  assert.match(style, /theme-slide-up[^}]*250ms/s)
  assert.match(style, /translateY\(12px\)/)
  assert.match(card, /animationStep: 50/)
})

test('motion respects reduced-motion preference', () => {
  assert.match(style, /prefers-reduced-motion: reduce/)
})
