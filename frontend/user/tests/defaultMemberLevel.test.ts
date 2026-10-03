import {test} from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'

test('unassigned existing account resolves default before next level',()=>{
 const s=readFileSync(new URL('../src/stores/userProfile.ts',import.meta.url),'utf8')
 const expression=s.match(/const currentLevel = computed\(\(\) => \{([\s\S]*?)\n    \}\)/)![1]
 const levels=[{id:1,is_default:true,sort_order:0},{id:2,is_default:false,sort_order:10}]
 const resolve=new Function('profile','memberLevels',expression)
 assert.equal(resolve({value:{member_level_id:0}},{value:levels})?.id,1)
 assert.equal(resolve({value:{member_level_id:2}},{value:levels})?.id,2)
 assert.equal(resolve({value:{member_level_id:99}},{value:levels}),null)
})
