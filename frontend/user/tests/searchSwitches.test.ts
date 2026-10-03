import test from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'
test('page and navigation search have independent settings and shared query behavior',()=>{
 const read=(p:string)=>readFileSync(p,'utf8');const admin=read('../admin/src/views/admin/Settings.vue');assert.match(admin,/form.product_search_enabled/);assert.match(admin,/form.nav_search_enabled/);
 for(const p of ['src/views/Products.vue','src/templates/vault/Products.vue'])assert.match(read(p),/product_search_enabled/);
 for(const p of ['src/components/Navbar.vue','src/templates/vault/layout/VaultLayout.vue'])assert.match(read(p),/NavigationSearch/);
 assert.match(read('src/components/NavigationSearch.vue'),/nav_search_enabled/);assert.match(read('src/composables/useProductList.ts'),/route.query\??.search/);
})
