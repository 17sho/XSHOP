import test from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'
test('category refresh shows a spinning indicator in both themes without unlocking stale products',()=>{
 for(const p of ['src/views/Products.vue','src/templates/vault/Products.vue']){
 const s=readFileSync(p,'utf8');assert.match(s,/data-category-loading-region/);assert.match(s,/v-show="!\(loading \|\| catalogStale\) \|\| !!loadError"/);assert.match(s,/h-10 w-10/);assert.match(s,/data-category-refresh-spinner/);assert.match(s,/animate-spin/);assert.match(s,/:inert="catalogStale/);
 }
})
