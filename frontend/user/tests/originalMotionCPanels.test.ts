import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import postcss from 'postcss'
import { fixture, settle, vue, pass, empty } from './helpers/motionPolishRouteHarness.ts'
const read = (path: string) => readFileSync(new URL(`../src/${path}`, import.meta.url), 'utf8')
const css = (path: string) => postcss.parse(read(path).match(/<style scoped>([\s\S]*?)<\/style>/)?.[1] || '')
const rule = (tree: postcss.Root, selector: string) => { let value = ''; tree.walkRules(selector, r => { value = r.toString() }); return value }
const frames = (tree: postcss.Root, name: string) => { let value = ''; tree.walkAtRules('keyframes', r => { if (r.params === name) value = r.toString() }); return value }

for (const [name, panel, success, burst] of [
  ['Api', 'api-panel-enter', 'new-secret-burst', 'new-secret-burst'],
  ['GiftCard', 'gift-card-panel-enter', 'success-burst', 'gift-card-success-burst'],
]) test(`${name}: original 450ms ease panel and success, clean final transform`, () => {
  const file = `views/personal/${name}Panel.vue`, source = read(file), tree = css(file)
  assert.match(source.split('\n')[1], new RegExp(panel))
  assert.match(rule(tree, '.' + panel), new RegExp(`animation: ${panel} 0\\.45s ease backwards`))
  assert.match(rule(tree, '.' + success), new RegExp(`animation: ${burst} 0\\.45s ease backwards`))
  assert.match(frames(tree, panel), /opacity:\s*0;\s*transform:\s*translateY\(10px\)/)
  assert.match(frames(tree, burst), /opacity:\s*0;\s*transform:\s*translateY\(8px\) scale\(0\.98\)/)
  for (const name of [panel, burst]) {
    assert.match(frames(tree, name), /opacity:\s*1;\s*transform:\s*none/)
    assert.doesNotMatch(rule(tree, '.' + name), /\bboth\b|\bforwards\b/)
  }
  assert.doesNotMatch(source, /motion-success-feedback/)
  assert.match(read('style.css'), /prefers-reduced-motion: reduce[\s\S]*animation: none !important/)
})

test('theme-slide-up keeps upstream 250ms curve/Y12, no perpetual containing block or unused bounce', () => {
  const global = read('style.css')
  assert.match(global, /\.theme-slide-up\s*\{\s*animation: theme-slide-up 250ms var\(--ui-ease-out\) backwards/)
  assert.match(global, /--ui-ease-out: cubic-bezier\(0\.16, 1, 0\.3, 1\)/)
  assert.match(global, /@keyframes theme-slide-up\s*\{\s*from \{ opacity: 0; transform: translateY\(12px\); \}\s*to \{ opacity: 1; transform: none;/)
  assert.doesNotMatch(global, /motion-success-feedback|theme-bounce-in/)
})

Object.defineProperty(globalThis, 'localStorage', { configurable: true, value: window.localStorage })
for (const theme of ['classic', 'vault']) test(`${theme}: actual Api/Gift section has only its local owner; no global replay on same route component`, async () => {
  let mutations = 0
  const forbidden = () => { mutations++; throw Error('Business writes forbidden') }
  const store = vue.reactive({ displayName: 'Fixture', profile: { email: 'fixture@example.invalid' }, memberLevels: [], currentLevel: null,
    personalCenterVisibility: { overview: true, profile: true, security: true, wallet: true, orders: false, api: true, giftCard: true },
    ensureProfileLoaded: async () => true, loadMemberLevels: async () => {} })
  const mocks: Record<string, any> = {
    'vue-i18n': { useI18n: () => ({ t: (s: string) => s, locale: vue.ref('en-US') }) },
    '../stores/userProfile': { useUserProfileStore: () => store },
    '../../stores/app': { useAppStore: () => ({ config: {}, loadConfig: async () => {} }) },
    '../../api': { apiCredentialAPI: { getMy: async () => ({ data: { data: { status: 'none' } } }), apply: forbidden, update: forbidden, regenerate: forbidden }, giftCardAPI: { redeem: forbidden } },
    '../../utils/image': { getImageUrl: (s: string) => s },
    '@/components/ui/input': { Input: 'input' }, '@/components/ui/label': { Label: 'label' },
    '../../components/captcha/ImageCaptcha.vue': { default: empty }, '../../components/captcha/TurnstileCaptcha.vue': { default: empty },
    '../components/shared/StatCard.vue': { default: pass },
  }
  for (const name of ['Profile', 'Security', 'Orders', 'Wallet', 'Affiliate']) for (const prefix of ['./personal', '../../views/personal']) mocks[`${prefix}/${name}Panel.vue`] = { default: empty }
  const h = await fixture(theme, mocks, load => {
    const Personal = load(theme === 'classic' ? 'views/PersonalCenter.vue' : 'templates/vault/PersonalCenter.vue').default
    return [{ path: '/', component: empty }, { path: '/me/:section?', component: Personal, props: (r: any) => ({ section: r.params.section }) }]
  })
  try {
    await h.go('/me/api'); await settle(); await new Promise(resolve => setTimeout(resolve, 35))
    const header = h.host.querySelector('header'), aside = h.host.querySelector('aside')
    assert.equal(h.host.querySelectorAll('.api-panel-enter').length, 1)
    await h.go('/me/giftCard'); await settle()
    const panel = h.host.querySelector('.gift-card-panel-enter')!
    assert.ok(panel, 'actual GiftCard panel owns its upstream entrance')
    assert.equal(h.host.querySelector('header'), header); assert.equal(h.host.querySelector('aside'), aside)
    assert.equal(h.host.querySelector('.page-fade-enter-active'), null, 'same PersonalCenter route component must not replay global transition')
    assert.equal(h.events.length, 0, 'custom WAAPI owner removed, not layered over local animation')
    const input = panel.querySelector('input')!
    input.value = 'fixture draft'; input.dispatchEvent(new window.Event('input', { bubbles: true }))
    await h.go('/me/giftCard?draft=preserved'); await settle()
    assert.equal(h.host.querySelector('.gift-card-panel-enter'), panel)
    assert.equal(panel.querySelector('input'), input)
    assert.equal(mutations, 0)
  } finally { h.unmount(); localStorage.clear() }
})
