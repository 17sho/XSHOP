import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useUserAuthStore } from '../stores/userAuth'
import { useI18n } from 'vue-i18n'
import { debounceAsync } from '../utils/debounce'
import { useAppStore } from '../stores/app'
import { userAuthAPI, type CaptchaPayload } from '../api'
import ImageCaptcha from '../components/captcha/ImageCaptcha.vue'
import TurnstileCaptcha from '../components/captcha/TurnstileCaptcha.vue'

/**
 * 找回密码页共享逻辑（classic + vault 双模板共用）。
 * 完整保留原 views/auth/Forgot.vue 的行为，仅抽离为 composable。
 */
export function useForgot() {
  const router = useRouter()
  const userAuthStore = useUserAuthStore()
  const appStore = useAppStore()
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

  const emailVerificationEnabled = computed(() => appStore.config?.email_verification_enabled !== false)

  const email = ref('')
  const code = ref('')
  const newPassword = ref('')
  const error = ref('')
  const sending = ref(false)
  const countdown = ref(0)
  const captchaPayload = ref<CaptchaPayload>({})
  const turnstileToken = ref('')
  const imageCaptchaRef = ref<InstanceType<typeof ImageCaptcha> | null>(null)
  const turnstileRef = ref<InstanceType<typeof TurnstileCaptcha> | null>(null)
  let timer: number | undefined

  const captchaConfig = computed(() => appStore.config?.captcha || null)
  const captchaProvider = computed(() => String(captchaConfig.value?.provider || 'none'))
  const sendCodeCaptchaEnabled = computed(() => !!captchaConfig.value?.scenes?.reset_send_code && captchaProvider.value !== 'none')
  const turnstileSiteKey = computed(() => String(captchaConfig.value?.turnstile?.site_key || ''))

  const startCountdown = () => {
    countdown.value = 60
    timer = window.setInterval(() => {
      countdown.value -= 1
      if (countdown.value <= 0 && timer) {
        clearInterval(timer)
        timer = undefined
      }
    }, 1000)
  }

  const getCaptchaPayload = (): CaptchaPayload | undefined => {
    if (!sendCodeCaptchaEnabled.value) return undefined
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

  const performSendCode = async () => {
    if (disposed) return
    error.value = ''
    if (!email.value) {
      error.value = t('auth.forgot.errors.emailRequired')
      return
    }
    if (countdown.value > 0) return

    if (sendCodeCaptchaEnabled.value && captchaProvider.value === 'image') {
      if (!captchaPayload.value.captcha_id || !captchaPayload.value.captcha_code) {
        error.value = t('auth.common.captchaRequired')
        return
      }
    }
    if (sendCodeCaptchaEnabled.value && captchaProvider.value === 'turnstile') {
      if (!turnstileToken.value) {
        error.value = t('auth.common.captchaRequired')
        return
      }
    }

    sending.value = true
    const operation = beginAuthOperation()
    try {
      await userAuthAPI.sendVerifyCode({
        email: email.value,
        purpose: 'reset',
        captcha_payload: getCaptchaPayload(),
      })
      if (!operation.current()) return
      startCountdown()
    } catch (err: any) {
      if (!operation.current()) return
      error.value = err.message || t('auth.forgot.errors.sendCodeFailed')
      if (captchaProvider.value === 'image') {
        imageCaptchaRef.value?.refresh()
      }
      if (captchaProvider.value === 'turnstile') {
        turnstileRef.value?.reset()
        turnstileToken.value = ''
      }
    } finally {
      if (operation.current()) sending.value = false
      operation.finish()
    }
  }

  const performReset = async () => {
    if (disposed) return
    error.value = ''
    if (!email.value || !code.value || !newPassword.value) return
    const operation = beginAuthOperation()
    try {
      await userAuthAPI.forgotPassword({
        email: email.value,
        code: code.value,
        new_password: newPassword.value
      })
      if (!operation.current()) return
      router.push('/auth/login')
    } catch (err: any) {
      if (!operation.current()) return
      error.value = err.message || t('auth.forgot.errors.resetFailed')
    } finally {
      operation.finish()
    }
  }

  const handleSendCode = debounceAsync(performSendCode, 200)
  const handleReset = debounceAsync(performReset, 200)

  watch([() => userAuthStore.sessionGeneration, () => email.value], () => {
    releaseAuthLoading?.()
    authRequest++
    handleSendCode.cancel()
    handleReset.cancel()
    if (timer !== undefined) clearInterval(timer)
    timer = undefined
    countdown.value = 0
    sending.value = false
  }, { flush: 'sync' })

  onUnmounted(() => {
    releaseAuthLoading?.()
    disposed = true
    authRequest++
    if (timer !== undefined) clearInterval(timer)
    timer = undefined
    handleSendCode.cancel()
    handleReset.cancel()
  })

  onMounted(async () => {
    await appStore.loadConfig(true)
  })

  return {
    userAuthStore,
    brandSiteName,
    emailVerificationEnabled,
    email,
    code,
    newPassword,
    error,
    sending,
    countdown,
    captchaPayload,
    turnstileToken,
    imageCaptchaRef,
    turnstileRef,
    captchaProvider,
    sendCodeCaptchaEnabled,
    turnstileSiteKey,
    handleCaptchaConfigStale,
    handleSendCode,
    handleReset,
  }
}
