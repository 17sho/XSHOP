import test from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'
import {siteIconLinks} from '../src/utils/siteIcon.ts'
test('configured website icon independently controls browser icons',()=>{
 const links=siteIconLinks({brand:{site_icon:'/uploads/custom.png',site_logo:'/other.svg'}})
 assert.equal(links.length,3)
 assert.ok(links.every(x=>x.href==='/uploads/custom.png'))
 assert.deepEqual(siteIconLinks(null),[])
 assert.ok(siteIconLinks({brand:{site_icon:'javascript:alert(1)'}}).every(x=>x.href.startsWith('/favicon')||x.href.startsWith('/apple')))
 assert.equal(siteIconLinks({brand:{}})[0]?.href,'/favicon-v5.svg')
})
test('no competing static default icons or manifest',()=>{
 const html=readFileSync('index.html','utf8')
 assert.doesNotMatch(html,/<link rel="(?:icon|shortcut icon|apple-touch-icon|manifest)"/)
 assert.match(readFileSync('src/stores/app.ts','utf8'),/siteIconLinks\(config.value\)/)
})
