import test from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'
test('original card quick buy is wired to the catalog dialog',()=>{
 const card=readFileSync(new URL('../src/components/ProductCard.vue',import.meta.url),'utf8')
 const page=readFileSync(new URL('../src/views/Products.vue',import.meta.url),'utf8')
 assert.match(card, /\$emit\('quickBuy', product\)/)
 assert.match(page, /@quick-buy="openQuickBuy"/)
 assert.match(page, /<ProductQuickBuy/)
})
