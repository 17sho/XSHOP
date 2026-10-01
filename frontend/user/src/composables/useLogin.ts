import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useUserAuthStore } from '../stores/userAuth'
import { useI18n } from 'vue-i18n'
import { debounceAsync } from '../utils/debounce'
import { useAppStore } from '../stores/app'
import { useTelegramMiniAppStore } from '../stores/telegramMiniApp'
import { buildTelegramMiniAppEntryLink, isTelegramUrlEnvironment, openTelegramCompatibleLink } from '../utils/telegramMiniApp'
import { userAuthAPI } from '../api'
import type { CaptchaPayload, TelegramAuthPayload } from '../api'
import {
  canShowGoogleIdentityButton,
  detectGoogleIdentityUXMode,
  hasThirdPartyLoginOption,
} from '../utils/googleIdentity'
import {
  createGoogleRedirectIntent,
  createGoogleRedirectPreparedIntent,
  getGoogleRedirectSessionStorage,
  shouldResumeGoogleRedirect2FA,
  storeGoogleRedirectIntent,
  tryBuildGoogleRedirectCredentialCallbackURL,
} from '../utils/googleRedirect'
import ImageCaptcha from '../components/captcha/ImageCaptcha.vue'
import TurnstileCaptcha from '../components/captcha/TurnstileCaptcha.vue'
import { useFormValidation } from './useFormValidation'

/**
 * 用户登录页共享逻辑（classic + vault 双模板共用）。
 * 完整保留原 views/auth/Login.vue 的行为，仅抽离为 composable。
 */
export function useLogin() {
  const router = useRouter()
  const route = useRoute()
  const userAuthStore = useUserAuthStore()
  const appStore = useAppStore()
  const telegramMiniAppStore = useTelegramMiniAppStore()
  const { t } = useI18n()

  let disposed = false
  let authRequest = 0
  let releaseAuthLoading: (() => void) | undefined
  // The server may already have acted. Only discard obsolete client commits;
  // never try to roll back a completed authentication/email operation.
  const beginAuthOperation = () => {
    const request = ++authRequest
    const session = userAuthStore.sessionGeneration
    const identity = email.value
    const current = () => !disposed && request === authRequest &&
      session === userAuthStore.sessionGeneration && identity === email.value
    userAuthStore.loading = true
    let ownsLoading = true
    const release = () => {
      if (ownsLoading && request === authRequest) userAuthStore.loading = false
      ownsLoading = false
    }
    const finish = () => { if (current()) release() }
    releaseAuthLoading = release
    return { current, finish }
  }

  const brandSiteName = computed(() => {
    const siteName = String(appStore.config?.brand?.site_name || '').trim()
    return siteName !== '' ? siteName : 'Dujiao-Next'
  })

  const email = ref('')
  const password = ref('')
  const showPassword = ref(false)
  const rememberMe = ref(true)

  const resumeGoogleRedirect2FAOnMount = shouldResumeGoogleRedirect2FA(
    route.query.google2fa,
    userAuthStore.challengeToken,
  )
  const step = ref<'password' | 'totp'>(
    resumeGoogleRedirect2FAOnMount ? 'totp' : 'password',
  )
  const totpMode = ref<'code' | 'recovery'>('code')
  const totpCode = ref('')
  const recoveryCode = ref('')
  const challengeRemainingSeconds = ref(0)
  let challengeTimer: ReturnType<typeof setInterval> | null = null

  const formValidation = useFormValidation(['email', 'password'])
  formValidation.addRule('email', formValidation.requiredRule('formValidation.emailRequired'))
  formValidation.addRule('email', formValidation.emailRule())
  formValidation.addRule('password', formValidation.requiredRule('formValidation.passwordRequired'))
  const error = ref('')
  const info = ref('')
  const captchaPayload = ref<CaptchaPayload>({})
  const turnstileToken = ref('')
  const imageCaptchaRef = ref<InstanceType<typeof ImageCaptcha> | null>(null)
  const turnstileRef = ref<InstanceType<typeof TurnstileCaptcha> | null>(null)
  const telegramWidgetRef = ref<HTMLDivElement | null>(null)

  const captchaConfig = computed(() => appStore.config?.captcha || null)
  const captchaProvider = computed(() => String(captchaConfig.value?.provider || 'none'))
  const loginCaptchaEnabled = computed(() => !!captchaConfig.value?.scenes?.login && captchaProvider.value !== 'none')
  const turnstileSiteKey = computed(() => String(captchaConfig.value?.turnstile?.site_key || ''))
  const telegramConfig = computed(() => appStore.config?.telegram_auth || null)
  const telegramBotUsername = computed(() => String(telegramConfig.value?.bot_username || '').trim())
  const telegramMiniAppURL = computed(() => String(telegramConfig.value?.mini_app_url || '').trim())
  const telegramEnabled = computed(() => !!telegramConfig.value?.enabled && telegramBotUsername.value !== '')
  const telegramLoginMode = computed(() => String(telegramConfig.value?.mode || '').trim())
  const isWidgetMode = computed(() => telegramLoginMode.value === 'widget' || (telegramLoginMode.value === '' && telegramEnabled.value))
  const googleConfig = computed(() => appStore.config?.google_auth || null)
  const googleClientID = computed(() => String(googleConfig.value?.client_id || '').trim())
  const googleEnabled = computed(() => !!googleConfig.value?.enabled && googleClientID.value !== '')
  const githubConfig = computed(() => appStore.config?.github_auth || null)
  const showGitHubLogin = computed(() => !!githubConfig.value?.enabled && String(githubConfig.value?.client_id || '').trim() !== '')
  let githubLoginRequest: number | null = null
  const startGitHubLogin = () => {
    if (disposed) return
    githubLoginRequest = authRequest
    const popup = window.open('/api/v1/auth/github', 'dujiao-github-oauth', 'popup,width=640,height=720')
    if (!popup) window.location.assign('/api/v1/auth/github')
  }
  const handleGitHubMessage = (event: MessageEvent) => {
    if (disposed || githubLoginRequest === null || githubLoginRequest !== authRequest) return
    if (event.origin !== window.location.origin || event.data?.type !== 'dujiao:github-oauth') return
    githubLoginRequest = null
    const data = event.data.data || {}
    if (data.error) { error.value = t('auth.login.loginFailed'); return }
    userAuthStore.acceptOAuthLogin(data)
    if (data.requires_totp) step.value = 'totp'
    else void router.push(String(route.query.redirect || '/'))
  }
  const googleButtonLocale = computed(() => String(appStore.locale || '').trim())
  const googleIdentityUXMode = detectGoogleIdentityUXMode()
  const googleRedirectLoginURI = googleIdentityUXMode === 'redirect'
    ? tryBuildGoogleRedirectCredentialCallbackURL()
    : ''
  const googleRedirectAvailable = googleIdentityUXMode === 'popup' || googleRedirectLoginURI !== ''
  const registrationEnabled = computed(() => appStore.config?.registration_enabled !== false)
  const emailVerificationEnabled = computed(() => appStore.config?.email_verification_enabled !== false)
  const isTelegramUrlEnv = isTelegramUrlEnvironment()
  const isTelegramMiniApp = computed(() => (telegramMiniAppStore.isMiniApp && telegramMiniAppStore.isReady) || isTelegramUrlEnv)
  const miniAppInitData = computed(() => String(telegramMiniAppStore.initData || '').trim())
  const showTelegramWidget = computed(() => isWidgetMode.value && telegramEnabled.value && !isTelegramMiniApp.value)
  const showTelegramOidc = computed(() => telegramLoginMode.value === 'oidc' && telegramEnabled.value && !isTelegramMiniApp.value)
  const showMiniAppLoginHint = computed(() => isTelegramMiniApp.value)
  const showGoogleLogin = computed(() => canShowGoogleIdentityButton(
    googleEnabled.value,
    googleClientID.value,
    isTelegramMiniApp.value,
  ) && googleRedirectAvailable)
  const showThirdPartyLogin = computed(() => hasThirdPartyLoginOption(
    showTelegramWidget.value,
    showTelegramOidc.value,
    showMiniAppLoginHint.value,
    showGoogleLogin.value || showGitHubLogin.value,
  ))
  const telegramMiniAppEntryLink = computed(() => buildTelegramMiniAppEntryLink(telegramBotUsername.value, telegramMiniAppURL.value))
  const showTelegramMiniAppEntry = computed(() => !isTelegramMiniApp.value && telegramMiniAppEntryLink.value !== '')
  const telegramCallbackName = '__dujiaoUserTelegramLogin'
  const miniAppLoginAttempted = ref(false)
  const attemptingMiniAppLogin = ref(false)

  const getCaptchaPayload = (): CaptchaPayload | undefined => {
    if (!loginCaptchaEnabled.value) return undefined
    if (captchaProvider.value === 'image') {
      return {
        captcha_id: captchaPayload.value.captcha_id || '',
        captcha_code: captchaPayload.value.captcha_code || '',
      }
    }
    if (captchaProvider.value === 'turnstile') {
      return {
        turnstile_token: turnstileToken.value,
      }
    }
    return undefined
  }

  const handleCaptchaConfigStale = async () => {
    const request = authRequest
    await appStore.loadConfig(true)
    if (disposed || request !== authRequest) return
    captchaPayload.value = {}
    turnstileToken.value = ''
  }

  const redirectAfterLogin = () => {
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/me/orders'
    return router.push(redirect)
  }

  const openTelegramMiniAppEntry = () => {
    if (telegramMiniAppEntryLink.value === '') return
    openTelegramCompatibleLink(telegramMiniAppEntryLink.value)
  }

  const performLogin = async () => {
    if (disposed) return
    error.value = ''
    if (!formValidation.validateAll({ email: email.value, password: password.value })) return

    if (loginCaptchaEnabled.value && captchaProvider.value === 'image') {
      if (!captchaPayload.value.captcha_id || !captchaPayload.value.captcha_code) {
        error.value = t('auth.common.captchaRequired')
        return
      }
    }

    if (loginCaptchaEnabled.value && captchaProvider.value === 'turnstile') {
      if (!turnstileToken.value) {
        error.value = t('auth.common.captchaRequired')
        return
      }
    }

    const operation = beginAuthOperation()
    try {
      const response = await userAuthAPI.login({
        email: email.value,
        password: password.value,
        remember_me: rememberMe.value,
        captcha_payload: getCaptchaPayload(),
      })
      if (!operation.current()) return
      operation.finish()
      const data = response.data.data
      userAuthStore.clearChallenge()
      userAuthStore.acceptOAuthLogin(data?.requires_totp ? {
        requires_totp: true,
        challenge_token: data.challenge_token,
        challenge_expires_at: data.challenge_expires_at,
      } : data)
      if (data?.requires_totp) {
        enter2FAStep()
        return
      }
      await redirectAfterLogin()
    } catch (err: any) {
      if (!operation.current()) return
      error.value = err.message || t('auth.login.error')
      if (captchaProvider.value === 'image') {
        imageCaptchaRef.value?.refresh()
      }
      if (captchaProvider.value === 'turnstile') {
        turnstileRef.value?.reset()
        turnstileToken.value = ''
      }
    } finally {
      operation.finish()
    }
  }

  const handleLogin = debounceAsync(performLogin, 200)

  const stopChallengeCountdown = () => {
    if (challengeTimer) {
      clearInterval(challengeTimer)
      challengeTimer = null
    }
  }

  const startChallengeCountdown = () => {
    stopChallengeCountdown()
    const tick = () => {
      const expiresAt = userAuthStore.challengeExpiresAt
      if (!expiresAt) {
        challengeRemainingSeconds.value = 0
        stopChallengeCountdown()
        return
      }
      const diff = Math.max(0, Math.floor((new Date(expiresAt).getTime() - Date.now()) / 1000))
      challengeRemainingSeconds.value = diff
      if (diff <= 0) {
        stopChallengeCountdown()
        cancel2FA()
        error.value = t('auth.login.totp.expired')
      }
    }
    tick()
    if (challengeRemainingSeconds.value > 0 && !disposed) challengeTimer = setInterval(tick, 1000)
  }

  const cancel2FA = () => {
    handleVerify2FA.cancel()
    releaseAuthLoading?.()
    authRequest++
    stopChallengeCountdown()
    userAuthStore.clearChallenge()
    step.value = 'password'
    totpCode.value = ''
    recoveryCode.value = ''
    challengeRemainingSeconds.value = 0
  }

  const performVerify2FA = async () => {
    if (disposed) return
    error.value = ''
    if (totpMode.value === 'code') {
      const code = totpCode.value.trim()
      if (code === '') {
        error.value = t('auth.login.totp.codeRequired')
        return
      }
      const operation = beginAuthOperation()
      const challenge = userAuthStore.challengeToken
      try {
        if (!challenge) throw new Error('challenge_token_missing')
        const response = await userAuthAPI.verify2FA({ challenge_token: challenge, code })
        if (!operation.current() || challenge !== userAuthStore.challengeToken) return
        operation.finish()
        userAuthStore.clearChallenge()
        userAuthStore.acceptOAuthLogin(response.data.data)
        stopChallengeCountdown()
        await redirectAfterLogin()
      } catch (err: any) {
        if (!operation.current() || challenge !== userAuthStore.challengeToken) return
        error.value = err.message || t('auth.login.totp.verifyFailed')
        totpCode.value = ''
      } finally {
        operation.finish()
      }
      return
    }
    const rc = recoveryCode.value.trim()
    if (rc === '') {
      error.value = t('auth.login.totp.recoveryRequired')
      return
    }
    const operation = beginAuthOperation()
    const challenge = userAuthStore.challengeToken
    try {
      if (!challenge) throw new Error('challenge_token_missing')
      const response = await userAuthAPI.verify2FA({ challenge_token: challenge, recovery_code: rc })
      if (!operation.current() || challenge !== userAuthStore.challengeToken) return
      operation.finish()
      userAuthStore.clearChallenge()
      userAuthStore.acceptOAuthLogin(response.data.data)
      stopChallengeCountdown()
      await redirectAfterLogin()
    } catch (err: any) {
      if (!operation.current() || challenge !== userAuthStore.challengeToken) return
      error.value = err.message || t('auth.login.totp.verifyFailed')
      recoveryCode.value = ''
    } finally {
      operation.finish()
    }
  }

  const handleVerify2FA = debounceAsync(performVerify2FA, 200)

  const buildTelegramPayload = (raw: any): TelegramAuthPayload | null => {
    const id = Number(raw?.id)
    const authDate = Number(raw?.auth_date)
    const hash = String(raw?.hash || '').trim()
    if (!Number.isFinite(id) || id <= 0 || !Number.isFinite(authDate) || authDate <= 0 || hash === '') {
      return null
    }
    return {
      id,
      first_name: String(raw?.first_name || '').trim(),
      last_name: String(raw?.last_name || '').trim(),
      username: String(raw?.username || '').trim(),
      photo_url: String(raw?.photo_url || '').trim(),
      auth_date: authDate,
      hash,
    }
  }

  const enter2FAStep = () => {
    step.value = 'totp'
    totpMode.value = 'code'
    totpCode.value = ''
    recoveryCode.value = ''
    startChallengeCountdown()
  }

  const handleTelegramAuth = async (raw: any) => {
    if (disposed) return
    error.value = ''
    const payload = buildTelegramPayload(raw)
    if (!payload) {
      error.value = t('auth.login.telegramInvalidPayload')
      return
    }
    const operation = beginAuthOperation()
    try {
      const response = await userAuthAPI.telegramLogin(payload)
      if (!operation.current()) return
      operation.finish()
      const data = response.data.data
      userAuthStore.clearChallenge()
      userAuthStore.acceptOAuthLogin(data?.requires_totp ? {
        requires_totp: true,
        challenge_token: data.challenge_token,
        challenge_expires_at: data.challenge_expires_at,
      } : data)
      if (data?.requires_totp) {
        enter2FAStep()
        return
      }
      await redirectAfterLogin()
    } catch (err: any) {
      if (!operation.current()) return
      error.value = err.message || t('auth.login.telegramLoginFailed')
    } finally {
      operation.finish()
    }
  }

  const handleGoogleCredential = async (credential: string) => {
    if (disposed || userAuthStore.loading) return
    error.value = ''
    const normalizedCredential = String(credential || '').trim()
    if (normalizedCredential === '') {
      error.value = t('auth.login.googleInvalidCredential')
      return
    }

    const operation = beginAuthOperation()
    try {
      const response = await userAuthAPI.googleLogin({ credential: normalizedCredential })
      if (!operation.current()) return
      operation.finish()
      const data = response.data.data
      userAuthStore.clearChallenge()
      userAuthStore.acceptOAuthLogin(data?.requires_totp ? {
        requires_totp: true,
        challenge_token: data.challenge_token,
        challenge_expires_at: data.challenge_expires_at,
      } : data)
      if (data?.requires_totp) {
        enter2FAStep()
        return
      }
      await redirectAfterLogin()
    } catch (err: any) {
      if (!operation.current()) return
      error.value = err?.message || t('auth.login.googleLoginFailed')
    } finally {
      operation.finish()
    }
  }

  const handleGoogleScriptError = () => {
    if (disposed) return
    error.value = t('auth.login.googleWidgetLoadFailed')
  }

  // Safari restores the login page from its back/forward cache after a user
  // cancels Google's redirect flow. GIS can emit a late teardown error during
  // that restore even though the button is available again; do not leave that
  // stale, provider-only notice on an otherwise usable login form.
  const handlePageShow = (event: PageTransitionEvent) => {
    if (event.persisted && error.value === t('auth.login.googleWidgetLoadFailed')) {
      error.value = ''
    }
  }

  const prepareGoogleRedirectLogin = async () => {
    if (disposed) throw new Error('auth_page_inactive')
    const request = authRequest
    const response = await userAuthAPI.googleRedirectIntent()
    if (disposed || request !== authRequest) throw new Error('auth_page_inactive')
    const preparedIntent = createGoogleRedirectPreparedIntent(response.data.data)
    if (!preparedIntent) {
      throw new Error('Google redirect state is invalid')
    }
    const intent = createGoogleRedirectIntent(
      'login',
      route.query.redirect,
      preparedIntent.issuedAt,
    )
    storeGoogleRedirectIntent(getGoogleRedirectSessionStorage(), intent)
    return preparedIntent
  }

  const tryTelegramMiniAppLogin = async () => {
    if (disposed || !isTelegramMiniApp.value || miniAppInitData.value === '' || miniAppLoginAttempted.value || attemptingMiniAppLogin.value) {
      return
    }

    miniAppLoginAttempted.value = true
    attemptingMiniAppLogin.value = true
    error.value = ''

    const operation = beginAuthOperation()
    try {
      const response = await userAuthAPI.telegramMiniAppLogin({ init_data: miniAppInitData.value })
      if (!operation.current()) return
      attemptingMiniAppLogin.value = false
      operation.finish()
      const data = response.data.data
      userAuthStore.clearChallenge()
      userAuthStore.acceptOAuthLogin(data?.requires_totp ? {
        requires_totp: true,
        challenge_token: data.challenge_token,
        challenge_expires_at: data.challenge_expires_at,
      } : data)
      if (data?.requires_totp) {
        enter2FAStep()
        return
      }
      await redirectAfterLogin()
    } catch (err: any) {
      if (!operation.current()) return
      error.value = err.message || t('auth.login.telegramLoginFailed')
    } finally {
      if (operation.current()) attemptingMiniAppLogin.value = false
      operation.finish()
    }
  }

  let telegramWidgetGeneration = 0
  let telegramWidgetScript: HTMLScriptElement | null = null
  const clearTelegramWidget = () => {
    telegramWidgetGeneration++
    if (telegramWidgetScript) {
      telegramWidgetScript.onerror = null
      telegramWidgetScript.onload = null
      telegramWidgetScript = null
    }
    if (telegramWidgetRef.value) {
      telegramWidgetRef.value.innerHTML = ''
    }
  }

  const startTelegramOidc = async () => {
    if (disposed) return
    const request = authRequest
    error.value = ''
    try {
      sessionStorage.removeItem('tg_oidc_intent')
      const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : ''
      if (redirect) {
        sessionStorage.setItem('tg_oidc_redirect', redirect)
      } else {
        sessionStorage.removeItem('tg_oidc_redirect')
      }
      const res = await userAuthAPI.telegramOidcStart()
      if (disposed || request !== authRequest) return
      const url = String(res?.data?.data?.auth_url || '')
      if (!url) {
        error.value = t('auth.login.telegramLoginFailed')
        return
      }
      window.location.href = url
    } catch (err: any) {
      if (disposed || request !== authRequest) return
      error.value = err?.message || t('auth.login.telegramLoginFailed')
    }
  }

  const renderTelegramWidget = () => {
    if (disposed) return
    if (telegramLoginMode.value === 'oidc') {
      clearTelegramWidget()
      return
    }
    if (!showTelegramWidget.value || !telegramWidgetRef.value) {
      clearTelegramWidget()
      return
    }
    clearTelegramWidget()
    const generation = telegramWidgetGeneration
    const session = userAuthStore.sessionGeneration
    const botUsername = telegramBotUsername.value
    const script = document.createElement('script')
    telegramWidgetScript = script
    script.async = true
    script.src = 'https://telegram.org/js/telegram-widget.js?22'
    script.setAttribute('data-telegram-login', telegramBotUsername.value)
    script.setAttribute('data-size', 'large')
    script.setAttribute('data-userpic', 'false')
    script.setAttribute('data-request-access', 'write')
    script.setAttribute('data-onauth', `${telegramCallbackName}(user)`)
    script.onerror = () => {
      // A detached handler may already be queued; clearing its property alone
      // cannot revoke it. Capture this render, not the latest global callback.
      if (disposed || generation !== telegramWidgetGeneration ||
        session !== userAuthStore.sessionGeneration ||
        botUsername !== telegramBotUsername.value || !showTelegramWidget.value) return
      error.value = t('auth.login.telegramWidgetLoadFailed')
    }
    telegramWidgetRef.value.appendChild(script)
  }

  onMounted(async () => {
    window.addEventListener('message', handleGitHubMessage)
    window.addEventListener('pageshow', handlePageShow)
    const session = userAuthStore.sessionGeneration
    await appStore.loadConfig(true)
    if (disposed || session !== userAuthStore.sessionGeneration) return
    const win = window as Window & Record<string, any>
    win[telegramCallbackName] = handleTelegramAuth
    renderTelegramWidget()

    const resumeTelegram2FA = route.query.tg2fa === '1' && userAuthStore.challengeToken
    const resumeGoogle2FA = shouldResumeGoogleRedirect2FA(
      route.query.google2fa,
      userAuthStore.challengeToken,
    )
    if (resumeTelegram2FA || resumeGoogle2FA) {
      enter2FAStep()
      const nextQuery = { ...route.query }
      delete nextQuery.tg2fa
      delete nextQuery.google2fa
      router.replace({ path: route.path, query: nextQuery })
    }

    const reason = typeof route.query.reason === 'string' ? route.query.reason : ''
    if (reason === 'password_changed') {
      info.value = t('auth.login.passwordChangedTip')
      const nextQuery = { ...route.query }
      delete nextQuery.reason
      router.replace({ path: route.path, query: nextQuery })
    }

    await tryTelegramMiniAppLogin()
  })

  watch([showTelegramWidget, telegramBotUsername], () => {
    renderTelegramWidget()
  })

  watch([isTelegramMiniApp, miniAppInitData], () => {
    releaseAuthLoading?.()
    authRequest++
    miniAppLoginAttempted.value = false
    attemptingMiniAppLogin.value = false
    void tryTelegramMiniAppLogin()
  }, { flush: 'sync' })

  watch([() => userAuthStore.sessionGeneration, () => email.value], ([session, identity], [oldSession, oldIdentity]) => {
    if (session === oldSession && identity !== oldIdentity && step.value === 'totp') cancel2FA()
    releaseAuthLoading?.()
    authRequest++
    handleLogin.cancel()
    handleVerify2FA.cancel()
    stopChallengeCountdown()
  }, { flush: 'sync' })

  onUnmounted(() => {
    releaseAuthLoading?.()
    disposed = true
    authRequest++
    handleLogin.cancel()
    handleVerify2FA.cancel()
    window.removeEventListener('message', handleGitHubMessage)
    window.removeEventListener('pageshow', handlePageShow)
    const win = window as Window & Record<string, any>
    delete win[telegramCallbackName]
    clearTelegramWidget()
    stopChallengeCountdown()
  })

  return {
    // store
    userAuthStore,
    // brand
    brandSiteName,
    // credentials
    email,
    password,
    showPassword,
    rememberMe,
    // 2FA
    step,
    totpMode,
    totpCode,
    recoveryCode,
    challengeRemainingSeconds,
    handleVerify2FA,
    cancel2FA,
    // messages
    error,
    info,
    // validation
    formValidation,
    // captcha
    loginCaptchaEnabled,
    captchaProvider,
    captchaPayload,
    turnstileToken,
    turnstileSiteKey,
    imageCaptchaRef,
    turnstileRef,
    handleCaptchaConfigStale,
    // flags
    registrationEnabled,
    emailVerificationEnabled,
    // telegram
    showTelegramWidget,
    telegramWidgetRef,
    showTelegramOidc,
    startTelegramOidc,
    showMiniAppLoginHint,
    attemptingMiniAppLogin,
    showTelegramMiniAppEntry,
    openTelegramMiniAppEntry,
    // google
    googleClientID,
    googleButtonLocale,
    googleIdentityUXMode,
    googleRedirectLoginURI,
    prepareGoogleRedirectLogin,
    showGoogleLogin,
    showGitHubLogin,
    startGitHubLogin,
    showThirdPartyLogin,
    handleGoogleCredential,
    handleGoogleScriptError,
    // actions
    handleLogin,
  }
}
