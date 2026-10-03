import test from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync} from 'node:fs'
test('upgrade details and rollback are collapsed while errors stay visible',()=>{
 const s=readFileSync('src/components/XshopVersionBadge.vue','utf8');const t=s.split('<template>')[1];
 assert.match(t, /详情与回退/);
 const details=t.slice(t.indexOf('<details'),t.indexOf('</details>'));
 assert.match(details,/status.phase/);assert.match(details,/status.message/);assert.match(details,/@click="rollback"/);
 assert.doesNotMatch(t,/仅超级管理员可操作/);assert.doesNotMatch(t,/回退仅替换程序，不回滚数据库；会短暂重启。仅在后端/);
 assert.match(t,/status.state === 'failed'/);assert.match(t,/role="alert"/);
 assert.match(t,/status.version !== \(status.current_version \|\| version\)/);
 assert.match(s,/window.location.reload/);assert.match(s,/installFence/);
})
