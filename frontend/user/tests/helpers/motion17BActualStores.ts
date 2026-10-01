import { reactive } from 'vue'
import { runtime, deferred } from './motion17BRuntime.ts'
import { evaluate } from './sourceRuntime.ts'
import * as browserStorage from '../../src/utils/browserStorage.ts'
import { debounceAsync } from '../../src/utils/debounce.ts'

// Real auth/profile/storage bodies; only transport and browser boundaries are inert.
Object.assign(globalThis, { fetch: () => { throw new Error('network forbidden') } })
export function setup(mode = 'normal', name = 'Login') {
  const values = new Map<string,string>(); const scripts:any[] = []; const calls:any[] = []; const pushes:any[] = []
  const denied = () => { throw new DOMException('Denied', 'SecurityError') }
  const storage = { getItem: (k:string) => mode === 'methods' ? denied() : values.get(k) ?? null,
    setItem: (k:string,v:string) => mode === 'methods' || mode === 'writes' ? denied() : values.set(k,v),
    removeItem: (k:string) => mode === 'methods' || mode === 'writes' ? denied() : values.delete(k) }
  const win:any = { setTimeout, clearTimeout, setInterval, clearInterval, addEventListener(){}, removeEventListener(){}, open: () => ({}), location: { origin: 'https://fixture.invalid', href: 'original' } }
  Object.defineProperty(win, 'localStorage', { get: mode === 'getter' ? denied : () => storage })
  Object.assign(globalThis, { window: win, sessionStorage: storage, document: { createElement: () => { const s:any = { setAttribute(){} }; scripts.push(s); return s } } })
  const boundary = evaluate('src/utils/authStorage.ts', ['readAuthStorage','writeAuthStorage','removeAuthStorage'], browserStorage)
  const request = (payload:any) => { const d = deferred(); calls.push({ ...d, payload }); return d.promise }
  const api = Object.fromEntries(['login','register','verify2FA','sendVerifyCode','forgotPassword','googleLogin','telegramLogin','telegramMiniAppLogin','telegramOidcStart','googleRedirectIntent'].map(k => [k, request]))
  const auth = evaluate('src/stores/userAuth.ts', ['useUserAuthStore'], { ...boundary, userAuthAPI: api, useRouter: () => ({ push(){} }), defineStore: (_:string, fn:Function) => () => reactive(fn()) }).useUserAuthStore()
  const profileRuntime = runtime('src/stores/userProfile.ts', ['useUserProfileStore'], { defineStore: (_:string, fn:Function) => () => reactive(fn()), useUserAuthStore: () => auth, userProfileAPI: { updateProfile: request, current: request } })
  const profile = profileRuntime.run(() => profileRuntime.useUserProfileStore())
  const app = reactive({ config: { email_verification_enabled: false, telegram_auth: { enabled: false, bot_username: 'fixture_bot', mode: 'widget' } }, loadConfig: async () => {} })
  const mini = reactive({ isMiniApp: false, isReady: false, initData: '' })
  const r = runtime(`src/composables/use${name}.ts`, [`use${name}`], {
    useUserAuthStore: () => auth, userAuthAPI: api, useAppStore: () => app,
    useRouter: () => ({ push: (v:any) => pushes.push(v), replace(){} }), useRoute: () => ({ query: {} }),
    useTelegramMiniAppStore: () => mini, useI18n: () => ({ t: (v:string) => v }), debounceAsync,
    useFormValidation: () => ({ addRule(){}, requiredRule(){}, emailRule(){}, minLengthRule(){}, validateAll: () => true }),
    shouldResumeGoogleRedirect2FA: () => false, detectGoogleIdentityUXMode: () => 'popup', isTelegramUrlEnvironment: () => false,
    canShowGoogleIdentityButton: () => false, hasThirdPartyLoginOption: () => false,
    createGoogleRedirectPreparedIntent: () => ({ issuedAt: 1 }), createGoogleRedirectIntent: () => ({}), getGoogleRedirectSessionStorage: () => storage, storeGoogleRedirectIntent(){},
  })
  const g = r.run(() => r[`use${name}`]()); g.email.value = 'fixture@example.invalid'; g.password.value = 'inert-fixture'; if (g.agreed) g.agreed.value = true
  return { auth, profile, g, r, calls, values, boundary, app, scripts, pushes, mini, dispose: () => { r.dispose(); profileRuntime.dispose() } }
}
