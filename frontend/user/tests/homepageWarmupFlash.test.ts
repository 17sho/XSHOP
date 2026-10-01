import assert from 'node:assert/strict'
import { readFileSync, readdirSync } from 'node:fs'
import test from 'node:test'

const router = readFileSync(new URL('../src/router/index.ts', import.meta.url), 'utf8')
const app = readFileSync(new URL('../src/App.vue', import.meta.url), 'utf8')

test('homepage immediately warms the two mobile navigation destinations', () => {
  assert.match(router, /export const warmupPrimaryNavigationRoutes/)
  assert.match(router, /Promise\.allSettled\(\[guestOrdersViewLoader\(\),\s*loginViewLoader\(\)\]\)/)
  assert.doesNotMatch(router, /Promise\.allSettled\([^)]*personalCenterViewLoader/s)
  assert.doesNotMatch(router, /if \(router\.currentRoute\.value\.name === 'products'\) return/)
})

test('classic routes use original unkeyed exit-then-enter fading without custom route WAAPI', () => {
  const classic = app.split('<!-- classic 模板')[1]?.split('</template>')[0]
  assert.ok(classic)
  assert.match(classic, /<RouterView v-slot="\{ Component \}">[\s\S]*<Transition name="page-fade" mode="out-in"[^>]*>[\s\S]*<component :is="Component"/)
  assert.match(app, /transition: opacity 200ms ease/)
  assert.doesNotMatch(classic, /:key=|playRouteEnter|routePage/)
  assert.doesNotMatch(app, /\.animate\(/)
})

test('built lazy routes import the same entry that index.html loads', () => {
  const dist = new URL('../dist/', import.meta.url)
  const html = readFileSync(new URL('index.html', dist), 'utf8')
  const entry = html.match(/src="\/assets\/(index-[^"/]+\.js)"/)?.[1]
  assert.ok(entry, 'index.html must refer to a bundled entry')
  for (const prefix of ['GuestOrders-', 'Login-', 'PersonalCenter-']) {
    const file = readdirSync(new URL('assets/', dist)).find(name => name.startsWith(prefix) && name.endsWith('.js'))
    assert.ok(file, `${prefix} lazy chunk must exist`)
    const chunk = readFileSync(new URL(`assets/${file}`, dist), 'utf8')
    assert.ok(chunk.includes(`./${entry}`), `${file} must import ${entry}, not an older app entry`)
  }
})
