import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const quickBuySource = readFileSync(
  new URL('../src/components/ProductQuickBuy.vue', import.meta.url),
  'utf8',
)

const quickBuyTemplate = quickBuySource.match(/<template>([\s\S]*?)<\/template>/)?.[1]

test('quick buy overlay and panel animate when initially mounted as visible', () => {
  assert.ok(quickBuyTemplate, 'ProductQuickBuy template should exist')

  const transitionAttributes = [...quickBuyTemplate.matchAll(/<Transition\b([^>]*)>/g)].map(
    (match) => match[1],
  )

  assert.equal(transitionAttributes.length, 1, 'one transition must own overlay/panel visibility without nested lifecycle duplication')
  assert.match(quickBuySource, /data-overlay-root/)
  assert.match(quickBuySource, /data-overlay-panel/)
  assert.match(quickBuySource, /\.quick-buy-enter-active \[data-overlay-panel\] \{ transition:transform/)
  assert.match(quickBuySource, /\.quick-buy-leave-active \[data-overlay-panel\] \{ transition:transform/)
  transitionAttributes.forEach((attributes, index) => {
    assert.match(
      attributes,
      /(?:^|\s)appear(?:\s|$)/,
      `quick buy transition ${index + 1} should animate on initial render`,
    )
  })
})
