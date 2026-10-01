import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('../src/i18n/index.ts', import.meta.url), 'utf8')

test('admin content navigation calls notices notifications', () => {
  assert.doesNotMatch(source, /announcementList:\s*['"]公告列表['"]/)
  assert.doesNotMatch(source, /announcementList:\s*['"]Announcements?['"]/)
  assert.match(source, /announcementList:\s*['"]通知['"]/)
  assert.match(source, /announcementList:\s*['"]Notifications['"]/)
})

test('admin notification page is labeled notification history', () => {
  assert.match(source, /posts:\s*\{[\s\S]*?title:\s*['"]历史通知['"]/)
  assert.match(source, /posts:\s*\{[\s\S]*?title:\s*['"]歷史通知['"]/)
  assert.match(source, /posts:\s*\{[\s\S]*?title:\s*['"]Notification History['"]/)
})
