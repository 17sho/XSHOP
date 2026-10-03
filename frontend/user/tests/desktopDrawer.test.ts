import test from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'
test('desktop menu links retain close and route behavior',()=>{const s=readFileSync('src/components/Navbar.vue','utf8');assert.match(s,/@click="closeDesktopMenu"/);assert.match(s,/data-desktop-navigation-trigger/);assert.match(s,/to="\/guest\/orders"/);assert.doesNotMatch(s,/hidden lg:flex items-center space-x-1/)})

