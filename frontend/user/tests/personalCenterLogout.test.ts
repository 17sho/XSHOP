import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

test('personal center exposes logout without desktop-only visibility', () => {
 const source = readFileSync(new URL('../src/views/PersonalCenter.vue', import.meta.url), 'utf8')
 assert.match(source, /<Button[^>]*data-testid="personal-logout"[^>]*@click="userAuthStore\.logout\(\)"[^>]*>/)
 assert.match(source, /t\('navbar.logout'\)/)
 assert.match(source, /useUserAuthStore/)
 const button = source.match(/<Button[^>]*data-testid="personal-logout"[^>]*>/)![0]
 assert.doesNotMatch(button, /hidden|lg:|md:/)
})
