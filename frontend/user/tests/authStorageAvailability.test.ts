import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import { evaluate } from './helpers/sourceRuntime.ts'
import * as browserStorage from '../src/utils/browserStorage.ts'

function setup(mode: 'normal' | 'getter' | 'methods' | 'writes') {
  const values = new Map<string, string>()
  const denied = () => { throw new DOMException('Denied', 'SecurityError') }
  const storage = {
    getItem: (key: string) => mode === 'methods' ? denied() : values.get(key) ?? null,
    setItem: (key: string, value: string) => mode === 'methods' || mode === 'writes' ? denied() : values.set(key, value),
    removeItem: (key: string) => mode === 'methods' || mode === 'writes' ? denied() : values.delete(key),
  }
  const window = { location: { href: '' } }
  Object.defineProperty(window, 'localStorage', { get: mode === 'getter' ? denied : () => storage })
  Object.assign(globalThis, { window })
  const boundary = fs.existsSync(new URL('../src/utils/authStorage.ts', import.meta.url))
    ? evaluate('src/utils/authStorage.ts', ['readAuthStorage', 'writeAuthStorage', 'removeAuthStorage'], browserStorage) : {}
  const globals = { ...boundary, useRouter: () => ({ push() {} }), defineStore: (_id: string, setup: Function) => setup }
  Object.defineProperty(globalThis, 'localStorage', { configurable: true, get: mode === 'getter' ? denied : () => storage })
  const store = () => evaluate('src/stores/userAuth.ts', ['useUserAuthStore'], globals).useUserAuthStore()
  return { store, values, boundary }
}
for (const mode of ['getter', 'methods', 'writes', 'normal'] as const) test(`FE05: ${mode} storage supports bootstrap, login, authenticated client and logout`, async () => {
  const { store, values, boundary } = setup(mode)
  const auth = store()
  assert.equal(auth.token.value, '')
  auth.acceptOAuthLogin({ token: 'fixture-token', user: { id: 7 } })
  assert.equal(auth.token.value, 'fixture-token')
  const requests: any[] = []
  const { userApi } = evaluate('src/api/client.ts', ['userApi'], {
    ...boundary, i18n: { global: { t: (s: string) => s, locale: 'en-US' } }, isPublicAuthEndpoint: () => false,
    fetch: async (_url: string, opts: any) => { requests.push(opts); return { ok: true, json: async () => ({ status_code: 0 }) } },
  })
  await userApi.get('/me')
  assert.equal(requests[0].headers.Authorization, 'Bearer fixture-token')
  if (mode === 'normal') {
    assert.equal(values.get('user_token'), 'fixture-token')
    assert.equal(store().user.value.id, 7)
  }
  auth.logout(); await userApi.get('/me')
  assert.equal(requests[1].headers.Authorization, undefined)
  assert.equal(auth.user.value, null)
  if (mode === 'normal') assert.equal(values.has('user_token'), false)
})
