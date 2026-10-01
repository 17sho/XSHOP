import test from 'node:test'
import assert from 'node:assert/strict'
import { reactive, nextTick } from 'vue'
import { runtime, deferred } from './helpers/motion17BRuntime.ts'
function setup() {
  const auth = reactive({ sessionGeneration: 0, user: { id: 1 } })
  const pending = deferred()
  const store = reactive({ profile: { id: 1, nickname: 'original', locale: 'zh-CN', authorized: true } as any, profileError: '', saveProfile: () => pending.promise })
  const r = runtime('src/views/personal/ProfilePanel.vue', ['profileForm', 'profileAlert', 'handleSaveProfile'], { useI18n: () => ({ t: (s: string) => s }), useUserProfileStore: () => store, useUserAuthStore: () => auth })
  return { r, store, auth, pending }
}
for (const action of ['account', 'dispose']) test(`profile save completion after ${action} cannot show stale alert`, async () => {
  const { r, store, auth, pending } = setup()
  r.profileForm.nickname = 'draft'; const save = r.handleSaveProfile()
  if (action === 'account') { auth.sessionGeneration++; store.profile = { id: 2, nickname: 'second', locale: 'en-US' } }
  else r.dispose()
  pending.resolve(true); await save
  assert.equal(r.profileAlert.value, null); r.dispose()
})
test('successful profile save adopts server normalization and becomes clean; edits during save survive', async () => {
  const { r, store, pending } = setup()
  r.profileForm.nickname = '  draft  '; const save = r.handleSaveProfile()
  r.profileForm.locale = 'en-US'
  store.profile = { id: 1, nickname: 'draft', locale: 'zh-CN' }
  pending.resolve(true); await save
  assert.equal(r.profileForm.nickname, 'draft'); assert.equal(r.profileForm.locale, 'en-US')
  store.profile = { id: 1, nickname: 'updated remotely', locale: 'zh-TW' }; await nextTick()
  assert.equal(r.profileForm.nickname, 'updated remotely'); assert.equal(r.profileForm.locale, 'en-US')
  assert.equal(r.profileAlert.value.level, 'success'); r.dispose()
})
test('failed profile save preserves unsaved fields and shows current error', async () => {
  const { r, store, pending } = setup()
  r.profileForm.nickname = 'unsaved'; r.profileForm.locale = 'en-US'
  const save = r.handleSaveProfile(); store.profileError = 'fixture failure'; pending.resolve(false); await save
  assert.deepEqual({ ...r.profileForm }, { nickname: 'unsaved', locale: 'en-US' })
  assert.equal(r.profileAlert.value.level, 'error'); assert.equal(r.profileAlert.value.message, 'fixture failure')
  store.profile = { id: 2, nickname: 'new account', locale: 'zh-CN' }; await nextTick()
  assert.equal(r.profileAlert.value, null); assert.equal(r.profileForm.nickname, 'new account'); r.dispose()
})
test('profile query/background refresh keeps dirty nickname/locale without stopping authorization refresh', async () => {
  const { r, store } = setup()
  r.profileForm.nickname = 'draft'; r.profileForm.locale = 'en-US'
  store.profile = { id: 1, nickname: 'server', locale: 'zh-TW', authorized: false }; await nextTick()
  assert.equal(r.profileForm.nickname, 'draft'); assert.equal(r.profileForm.locale, 'en-US')
  assert.equal(store.profile.authorized, false)
  r.dispose()
})
test('clean fields follow profile refresh; actual account/null resets draft', async () => {
  const { r, store, auth } = setup()
  r.profileForm.nickname = 'draft'
  store.profile = { id: 1, nickname: 'server', locale: 'en-US' }; await nextTick()
  assert.equal(r.profileForm.nickname, 'draft'); assert.equal(r.profileForm.locale, 'en-US')
  auth.sessionGeneration++; store.profile = null; await nextTick()
  assert.deepEqual({ ...r.profileForm }, { nickname: '', locale: 'zh-CN' })
  store.profile = { id: 2, nickname: 'second', locale: 'zh-TW' }; await nextTick()
  assert.deepEqual({ ...r.profileForm }, { nickname: 'second', locale: 'zh-TW' }); r.dispose()
})
