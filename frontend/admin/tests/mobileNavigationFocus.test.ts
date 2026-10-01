import assert from 'node:assert/strict'
import fs from 'node:fs'
import test from 'node:test'

const layout = fs.readFileSync(new URL('../src/layouts/AdminLayout.vue', import.meta.url), 'utf8')

test('mobile navigation is an explicit overlay and does not request search autofocus', () => {
  const mobileBlock = layout.match(/<!-- Mobile sidebar \(Sheet\) -->([\s\S]*?)<!-- Mobile sidebar end -->/)?.[1] ?? ''
  assert.match(mobileBlock, /<aside\b[^>]*v-show="mobileNavOpen"/)
  assert.match(mobileBlock, /:inert="mobileNavOpen \? undefined : true"/)
  assert.match(mobileBlock, /:aria-hidden="!mobileNavOpen"/)
  assert.match(mobileBlock, /<aside\b[^>]*aria-label="Mobile navigation"/)
  assert.match(mobileBlock, /@click="mobileNavOpen = false"/)
  assert.doesNotMatch(mobileBlock, /autofocus|auto-focus/)
})
