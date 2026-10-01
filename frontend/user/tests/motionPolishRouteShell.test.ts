import test from 'node:test'
import assert from 'node:assert/strict'
import { fixture, settle, vue, pass, empty } from './helpers/motionPolishRouteHarness.ts'

for (const theme of ['classic', 'vault']) test(`${theme}: actual PersonalCenter shell remains still with fixed descendants and query drafts`, async () => {
  let loads = 0, mounts = 0
  const store = vue.reactive({
    displayName: 'Synthetic fixture', profile: { email: 'fixture@example.invalid' }, memberLevels: [], currentLevel: null,
    personalCenterVisibility: { overview: true, profile: true, security: true, wallet: true, orders: false },
    ensureProfileLoaded: async () => { loads++; return true }, loadMemberLevels: async () => {},
  })
  const Panel = { setup() { mounts++; const draft = vue.ref('draft'); return () => vue.h('div', [vue.h('input', { value: draft.value, onInput: (e: any) => { draft.value = e.target.value } }), vue.h('div', { style: 'position:fixed', 'data-fixed-dialog': '' }, 'Dialog')]) } }
  const mocks: Record<string, any> = {
    'vue-i18n': { useI18n: () => ({ t: (s: string) => s, locale: vue.ref('en-US') }) },
    '../stores/userProfile': { useUserProfileStore: () => store },
    '../../utils/image': { getImageUrl: (s: string) => s },
    '../components/shared/StatCard.vue': { default: pass },
  }
  for (const name of ['Profile', 'Security', 'Orders', 'Wallet', 'GiftCard', 'Affiliate', 'Api']) {
    mocks[`./personal/${name}Panel.vue`] = { default: Panel }
    mocks[`../../views/personal/${name}Panel.vue`] = { default: Panel }
  }
  const h = await fixture(theme, mocks, load => {
    const Personal = load(theme === 'classic' ? 'views/PersonalCenter.vue' : 'templates/vault/PersonalCenter.vue').default
    return [{ path: '/', component: empty }, { path: '/me/:section?', component: Personal, props: (route: any) => ({ section: route.params.section || 'profile' }) }]
  })
  try {
    await h.go('/me/profile'); await settle()
    const header = h.host.querySelector('header'), aside = h.host.querySelector('aside')
    const content = h.host.querySelector('section')!
    assert.ok(content.querySelector('[data-fixed-dialog]'))
    assert.equal(content.contains(header), false); assert.equal(content.contains(aside), false)
    const input = h.host.querySelector('input')!
    input.value = 'retained draft'; input.dispatchEvent(new window.Event('input', { bubbles: true })); await settle()
    await h.go('/me/profile?tab=preserved')
    assert.equal(h.events.length, 0); assert.equal(h.host.querySelector('input'), input); assert.equal(input.value, 'retained draft')
    assert.equal(mounts, 1); assert.equal(loads, 1)
    await h.go('/me/security')
    assert.equal(h.events.length, 0)
    assert.equal(h.host.querySelector('header'), header); assert.equal(h.host.querySelector('aside'), aside)
    assert.equal(loads, 1)
    assert.equal(h.host.querySelector('section'), content)
    for (let ancestor = content.querySelector('[data-fixed-dialog]')!.parentElement; ancestor; ancestor = ancestor.parentElement) {
      assert.ok(['', 'none'].includes(window.getComputedStyle(ancestor).transform))
    }
  } finally { h.unmount() }
})
