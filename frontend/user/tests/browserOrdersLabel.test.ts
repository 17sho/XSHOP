import test from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'
test('guest browser orders use browser terminology in Chinese locales',()=>{
 for(const [locale,label] of [['zh-CN','浏览器订单'],['zh-TW','瀏覽器訂單']]){
 const s=readFileSync(`src/i18n/locales/${locale}.json`,'utf8');assert.ok(s.includes(`"browser": "${label}"`));assert.doesNotMatch(s,/本机订单|本機訂單/);
 }
})
