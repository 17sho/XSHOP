import test from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'
test('desktop menu uses same popover as language switch without sidebar',()=>{
 const s=readFileSync('src/components/Navbar.vue','utf8')
 assert.match(s,/<Popover v-model:open="desktopOpen">/)
 assert.match(s,/data-desktop-navigation-menu/)
 assert.doesNotMatch(s,/<dialog|showModal|desktop-nav-drawer/)
})
