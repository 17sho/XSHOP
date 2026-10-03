import test from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'
test('horizontal list restores original quick purchase rather than navigating',()=>{
 const component=readFileSync(new URL('../src/components/ProductListCard.vue',import.meta.url),'utf8')
 const page=readFileSync(new URL('../src/views/Products.vue',import.meta.url),'utf8')
 assert.match(component,/\$emit\('quickBuy', product\)/)
 assert.match(component,/products\.quickBuy/)
 assert.match(page,/<ProductListCard[^>]+@quick-buy="openQuickBuy"/)
})
