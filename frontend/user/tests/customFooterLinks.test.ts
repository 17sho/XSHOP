import test from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'
test('minimal custom footer is conditional and wired to both themes',()=>{const c=readFileSync('src/components/CustomFooterLinks.vue','utf8');assert.match(c,/v-if="links.length"/);assert.match(c,/footer_links/);assert.ok(c.includes('https?'));assert.match(readFileSync('src/App.vue','utf8'),/<CustomFooterLinks/);assert.doesNotMatch(readFileSync('../admin/src/views/admin/Settings.vue','utf8'),/v-model="form.contact.telegram"/);})
