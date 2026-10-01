import test from 'node:test'
import assert from 'node:assert/strict'
import { runtime } from './helpers/motion17BRuntime.ts'
import { setup } from './helpers/motion17BActualStores.ts'

function mountProfile(s: ReturnType<typeof setup>) {
  s.auth.acceptOAuthLogin({ token: 'inert-token', user: { id: 1 } })
  s.profile.profile = { id: 1, nickname: 'original', locale: 'zh-CN', authorized: true }
  return runtime('src/views/personal/ProfilePanel.vue', ['profileForm', 'handleSaveProfile', 'profileAlert'], {
    useUserProfileStore: () => s.profile, useUserAuthStore: () => s.auth, useI18n: () => ({ t: (v: string) => v }),
  })
}

for (const field of ['nickname', 'locale']) test(`actual store: ${field} edited away and back to submitted is still a newer edit`, async () => {
  const s = setup(); const panel = mountProfile(s)
  try {
    const submitted = field === 'nickname' ? 'submitted' : 'en-US'
    panel.profileForm[field] = submitted
    const pending = panel.handleSaveProfile()
    panel.profileForm[field] = field === 'nickname' ? 'intermediate' : 'zh-CN'
    panel.profileForm[field] = submitted
    s.calls[0].resolve({ data: { data: { id: 1, nickname: 'normalized', locale: 'zh-TW' } } })
    await pending
    assert.equal(panel.profileForm[field], submitted)
    assert.equal(s.profile.profile[field], field === 'nickname' ? 'normalized' : 'zh-TW')
  } finally { panel.dispose(); s.dispose() }
})

test('actual store: forced clean refresh hydrates, dirty field survives, and visibility data updates', async () => {
  const s = setup(); const panel = mountProfile(s)
  try {
    panel.profileForm.nickname = 'unsaved'
    const pending = s.profile.ensureProfileLoaded(true)
    assert.equal(s.calls.length, 1)
    s.calls[0].resolve({ data: { data: { id: 1, nickname: 'remote', locale: 'en-US', authorized: false } } })
    assert.equal(await pending, true)
    assert.deepEqual({ ...panel.profileForm }, { nickname: 'unsaved', locale: 'en-US' })
    assert.equal(s.auth.user.nickname, 'remote')
    assert.equal(s.profile.profile.authorized, false)
    assert.equal(s.profile.loadingProfile, false)
  } finally { panel.dispose(); s.dispose() }
})

test('actual store: failure keeps newer edits, releases busy and allows a normalized retry', async () => {
  const s = setup(); const panel = mountProfile(s)
  try {
    panel.profileForm.nickname = 'submitted'
    const pending = panel.handleSaveProfile()
    panel.profileForm.nickname = 'original'
    panel.profileForm.locale = 'en-US'
    s.calls[0].reject(new Error('fixture failure')); await pending
    assert.deepEqual({ ...panel.profileForm }, { nickname: 'original', locale: 'en-US' })
    assert.equal(panel.profileAlert.value.level, 'error')
    assert.equal(panel.profileAlert.value.message, 'fixture failure')
    assert.equal(s.profile.savingProfile, false)
    const retry = panel.handleSaveProfile()
    assert.deepEqual(s.calls[1].payload, { nickname: 'original', locale: 'en-US' })
    s.calls[1].resolve({ data: { data: { id: 1, nickname: 'normalized retry', locale: 'zh-TW' } } }); await retry
    assert.deepEqual({ ...panel.profileForm }, { nickname: 'normalized retry', locale: 'zh-TW' })
    assert.equal(panel.profileAlert.value.level, 'success')
  } finally { panel.dispose(); s.dispose() }
})

for (const leave of ['account', 'logout', 'dispose']) test(`actual store: pending save after ${leave} cannot restore old draft or stale alert`, async () => {
  const s = setup(); const panel = mountProfile(s)
  try {
    panel.profileForm.nickname = 'submitted'
    const pending = panel.handleSaveProfile()
    panel.profileForm.nickname = 'original'
    if (leave === 'dispose') panel.dispose()
    else {
      s.auth.setToken(leave === 'logout' ? '' : 'second-inert-token')
      assert.equal(s.profile.profile, null)
      assert.deepEqual({ ...panel.profileForm }, { nickname: '', locale: 'zh-CN' })
      if (leave === 'account') s.profile.profile = { id: 2, nickname: 'second', locale: 'en-US' }
    }
    const expected = { ...panel.profileForm }
    s.calls[0].resolve({ data: { data: { id: 1, nickname: 'obsolete normalized', locale: 'zh-TW' } } }); await pending
    assert.deepEqual({ ...panel.profileForm }, expected)
    assert.equal(panel.profileAlert.value, null)
    assert.equal(s.profile.savingProfile, false)
    if (leave !== 'dispose') assert.equal(s.profile.profile?.id, leave === 'account' ? 2 : undefined)
  } finally { panel.dispose(); s.dispose() }
})

for (const field of ['nickname','locale']) test(`COUNTEREXAMPLE: edit ${field} back to original during pending save must survive response`, async () => {
  const s = setup(); let panel:any
  try {
    s.auth.acceptOAuthLogin({ token: 'inert-token', user: { id: 1 } })
    s.profile.profile = { id: 1, nickname: 'original', locale: 'zh-CN' }
    panel = runtime('src/views/personal/ProfilePanel.vue', ['profileForm','handleSaveProfile','profileAlert'], { useUserProfileStore: () => s.profile, useUserAuthStore: () => s.auth, useI18n: () => ({ t: (v:string) => v }) })
    const original = panel.profileForm[field]; const submitted = field === 'nickname' ? 'submitted' : 'en-US'
    panel.profileForm[field] = submitted; const pending = panel.handleSaveProfile()
    panel.profileForm[field] = original
    s.calls[0].resolve({ data: { data: { id: 1, nickname: 'original', locale: 'zh-CN', [field]: submitted } } }); await pending
    assert.equal(panel.profileForm[field], original, 'the user changed this field after submitting; server response must not erase that edit')
    assert.equal(s.profile.profile[field], submitted, 'store still accepts authoritative response')
    assert.equal(s.profile.savingProfile, false)
    assert.equal(panel.profileAlert.value.level, 'success')
    const refresh = s.profile.ensureProfileLoaded(true)
    s.calls[1].resolve({ data: { data: { id: 1, nickname: 'refreshed', locale: 'zh-TW', authorized: false } } })
    assert.equal(await refresh, true)
    assert.equal(panel.profileForm[field], original, 'reverted field remains unsaved after later refresh too')
    assert.equal(s.profile.profile.authorized, false)
  } finally { panel?.dispose(); s.dispose() }
})

for (const field of ['nickname', 'locale']) test(`actual store: unchanged ${field} adopts normalization while other clean field hydrates`, async () => {
  const s = setup(); let panel:any
  try {
    s.auth.acceptOAuthLogin({ token: 'inert-token', user: { id: 1 } })
    s.profile.profile = { id: 1, nickname: 'original', locale: 'zh-CN', authorized: true }
    panel = runtime('src/views/personal/ProfilePanel.vue', ['profileForm','handleSaveProfile','profileAlert'], { useUserProfileStore: () => s.profile, useUserAuthStore: () => s.auth, useI18n: () => ({ t: (v:string) => v }) })
    panel.profileForm[field] = field === 'nickname' ? ' submitted ' : 'en-US'
    const pending = panel.handleSaveProfile()
    s.calls[0].resolve({ data: { data: { id: 1, nickname: 'normalized', locale: 'zh-TW', authorized: false } } }); await pending
    assert.deepEqual({ ...panel.profileForm }, { nickname: 'normalized', locale: 'zh-TW' })
    assert.equal(s.profile.profile.authorized, false)
    assert.equal(panel.profileAlert.value.level, 'success')
    s.profile.profile = { id: 1, nickname: 'refreshed', locale: 'en-US', authorized: true }
    assert.deepEqual({ ...panel.profileForm }, { nickname: 'refreshed', locale: 'en-US' })
    assert.equal(s.profile.profile.authorized, true)
  } finally { panel?.dispose(); s.dispose() }
})
