import test from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'
test('both navigation themes hide fallback branding until config is available',()=>{
 for(const file of ['src/components/Navbar.vue','src/templates/vault/layout/VaultLayout.vue']){
 const source=readFileSync(file,'utf8')
 assert.match(source,/v-if="!appStore.config"[^>]*data-brand-placeholder/)
 assert.match(source,/v-else-if="brand(?:Site)?Logo"/)
 }
})
