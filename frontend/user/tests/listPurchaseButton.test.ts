import test from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'
test('list purchase action is a visible text button with sold-out protection',()=>{
 const s=readFileSync(new URL('../src/components/ProductListCard.vue',import.meta.url),'utf8')
 assert.match(s,/t\('products.quickBuy'\)/)
 assert.match(s,/:disabled="isSoldOut\(product\)"/)
 assert.match(s,/@click.stop="\$emit\('quickBuy', product\)"/)
 assert.doesNotMatch(s,/<ShoppingCart/)
})
