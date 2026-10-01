import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
test('order cards have one transition utility while preserving lift, border and shadow hover', () => {
  const source = fs.readFileSync(new URL('../src/views/personal/OrdersPanel.vue', import.meta.url), 'utf8')
  const cards = [...source.matchAll(/class="([^"]*hover:-translate-y-0\.5[^"]*)"/g)].map(m => m[1]!)
  assert.equal(cards.length, 2)
  for (const card of cards) {
    assert.equal(card.split(/\s+/).filter(c => c === 'transition' || c.startsWith('transition-')).length, 1)
    assert.match(card, /hover:border-primary\/30/); assert.match(card, /hover:shadow-md/)
  }
})
