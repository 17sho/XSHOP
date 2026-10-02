import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
test('navbar uses configured site logo rather than a fixed release brand', () => {
 const s = readFileSync(new URL('../src/components/Navbar.vue', import.meta.url), 'utf8')
 assert.match(s, /:src="brandSiteLogo"/)
 assert.match(s, /config\?\.brand\?\.site_logo/)
 assert.match(s, /v-else/)
})
