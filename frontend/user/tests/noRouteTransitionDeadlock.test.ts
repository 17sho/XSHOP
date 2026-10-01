import assert from 'node:assert/strict'
import fs from 'node:fs'
import test from 'node:test'

const app = fs.readFileSync(new URL('../src/App.vue', import.meta.url), 'utf8')

test('route changes mount one interactive page tree at a time', () => {
  assert.equal((app.match(/<Transition name="page-fade" mode="out-in"[^>]*>/g) || []).length, 2)
  assert.doesNotMatch(app, /:key="[^"]*fullPath/)
  assert.doesNotMatch(app, /:key="routeRenderKey\(route\)"/)

})
