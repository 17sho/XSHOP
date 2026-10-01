import test from 'node:test'
import assert from 'node:assert/strict'
import { createPinia, setActivePinia, defineStore } from 'pinia'
import { evaluate, deferred } from './helpers/sourceRuntime.ts'
import { normalizePersonalCenterVisibility } from '../src/utils/personalCenterVisibility.ts'

const operations: Record<string, string> = {
  loadProfile: 'current', saveProfile: 'updateProfile', changeEmail: 'changeEmail', sendChangeEmailCode: 'sendChangeEmailCode',
  changePassword: 'changePassword', loadRecentOrders: 'orders', loadRecentLoginLogs: 'loginLogs',
  loadTelegramBinding: 'getTelegramBinding', bindTelegram: 'bindTelegram', bindTelegramMiniApp: 'bindTelegramMiniApp', unbindTelegram: 'unbindTelegram',
  loadGoogleBinding: 'getGoogleBinding', bindGoogle: 'bindGoogle', exchangeGoogleRedirectBind: 'googleRedirectBindExchange', unbindGoogle: 'unbindGoogle',
}
function setup() {
  setActivePinia(createPinia())
  const storage = new Map<string, string>()
  const localStorage = { getItem: (k: string) => storage.get(k) ?? null, setItem: (k: string, v: string) => storage.set(k, v), removeItem: (k: string) => storage.delete(k) }
  const auth = evaluate('src/stores/userAuth.ts', ['useUserAuthStore'], {
    defineStore, useRouter: () => ({ push() {} }), localStorage,
    readAuthStorage: localStorage.getItem, writeAuthStorage: localStorage.setItem, removeAuthStorage: localStorage.removeItem,
  }).useUserAuthStore()
  auth.acceptOAuthLogin({ token: 'A', user: { id: 1 } })
  const pending: ReturnType<typeof deferred>[] = []
  const request = () => { const d = deferred(); pending.push(d); return d.promise }
  const store = evaluate('src/stores/userProfile.ts', ['useUserProfileStore'], {
    defineStore, useUserAuthStore: () => auth, normalizePersonalCenterVisibility,
    userProfileAPI: new Proxy({}, { get: () => request }), userOrderAPI: { list: request }, memberLevelAPI: {},
  }).useUserProfileStore()
  return { auth, store, storage, pending }
}
for (const [operation] of Object.entries(operations)) {
  for (const reject of [false, true]) test(`FE03: ${operation} ignores old session ${reject ? 'error' : 'success'}, including same-token re-login`, async () => {
    const { auth, store, storage, pending } = setup()
    store.profile = { id: 1, nickname: 'A' }; store.recentOrders = [{ id: 1 }]
    const first = store[operation]({ nickname: 'A' })
    auth.logout(); auth.acceptOAuthLogin({ token: 'A', user: { id: 2 } })
    assert.equal(store.profile, null, 'private state clears synchronously on session change')
    assert.deepEqual(store.recentOrders, [])
    if (reject) pending[0]!.reject(new Error('old account error'))
    else pending[0]!.resolve({ data: { data: { id: 1, nickname: 'A' }, pagination: { total: 7 } } })
    assert.equal(await first, false)
    assert.equal(store.profile, null); assert.deepEqual(store.recentOrders, [])
    assert.equal(store.telegramBinding, null); assert.equal(store.googleBinding, null)
    assert.equal(store.profileError, ''); assert.equal(store.securityError, '')
    assert.equal(auth.user.id, 2); assert.equal(JSON.parse(storage.get('user_profile')!).id, 2)
  })
}
test('FE03: stale completion does not clear new session loading; current save persists', async () => {
  const { auth, store, storage, pending } = setup()
  const old = store.saveProfile({ nickname: 'A' })
  auth.logout(); auth.acceptOAuthLogin({ token: 'B', user: { id: 2 } })
  const fresh = store.saveProfile({ nickname: 'B' })
  pending[0]!.resolve({ data: { data: { id: 1 } } }); await old
  assert.equal(store.savingProfile, true)
  pending[1]!.resolve({ data: { data: { id: 2, nickname: 'B' } } }); assert.equal(await fresh, true)
  assert.equal(store.savingProfile, false); assert.equal(store.profile.nickname, 'B')
  assert.equal(JSON.parse(storage.get('user_profile')!).nickname, 'B')
})
