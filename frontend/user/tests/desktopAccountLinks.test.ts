import test from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'
test('guest lookup and account stay outside desktop popover',()=>{const s=readFileSync('src/components/Navbar.vue','utf8');const menu=s.split('<PopoverContent data-desktop-navigation-menu')[1]?.split('</PopoverContent>')[0]||'';assert.doesNotMatch(menu,/guest\/orders|auth\/login|personalCenter|logout/);assert.match(s,/data-desktop-account-actions/);assert.match(s,/to="\/guest\/orders"/)})
