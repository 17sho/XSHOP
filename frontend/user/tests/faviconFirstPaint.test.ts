import test from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'
test('initial document declares neutral icon before configuration to block implicit favicon fallback',()=>{
 const html=readFileSync('index.html','utf8')
 assert.match(html,/Website icon links are inserted by the server/)
 assert.doesNotMatch(html,/<link[^>]+href="\/favicon/)
})
