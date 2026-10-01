<template>
  <div
    class="social-google-button relative h-10 w-full overflow-hidden rounded-[inherit]"
    :class="[
      shape === 'pill' ? 'rounded-full' : 'rounded-md',
      { 'pointer-events-none opacity-60': disabled },
    ]"
    :aria-disabled="disabled"
    :inert="disabled"
  >
    <button
      type="button"
      class="social-google-visual absolute inset-0 z-10 flex h-10 w-full items-center justify-center gap-3 rounded-[inherit] border border-[#5f6368] bg-[#202124] font-semibold text-[#e8eaed] hover:bg-[#303134]"
      aria-hidden="true"
      :disabled="disabled || loading"
      @click="focusNativeGoogleButton"
    >
      <svg class="h-5 w-5" viewBox="0 0 48 48" aria-hidden="true">
        <path fill="#EA4335" d="M24 9.5c3.54 0 6.71 1.22 9.21 3.6l6.85-6.85C35.9 2.38 30.47 0 24 0 14.62 0 6.51 5.38 2.56 13.22l7.98 6.19C12.43 13.72 17.74 9.5 24 9.5z" />
        <path fill="#4285F4" d="M46.98 24.55c0-1.57-.15-3.09-.38-4.55H24v9.02h12.94c-.58 2.96-2.26 5.48-4.78 7.18l7.73 6c4.51-4.18 7.09-10.36 7.09-17.65z" />
        <path fill="#FBBC05" d="M10.53 28.59c-.48-1.45-.76-2.99-.76-4.59s.27-3.14.76-4.59l-7.98-6.19C.92 16.46 0 20.12 0 24c0 3.88.92 7.54 2.56 10.78l7.97-6.19z" />
        <path fill="#34A853" d="M24 48c6.48 0 11.93-2.13 15.89-5.81l-7.73-6c-2.15 1.45-4.92 2.3-8.16 2.3-6.26 0-11.57-4.22-13.47-9.91l-7.98 6.19C6.51 42.62 14.62 48 24 48z" />
      </svg>
      <span>Google</span>
    </button>
    <div
      ref="buttonContainerRef"
      class="social-google-native pointer-events-none absolute inset-0 z-0 flex h-10 w-full justify-center opacity-0 [&_div.S9gUrf-YoZ4jf]:w-full [&_div.S9gUrf-YoZ4jf>div]:w-full [&_div[role=button]]:!w-full [&_div[role=button]]:!max-w-none"
    ></div>
    <div
      v-if="loading"
      class="absolute inset-0 z-20 flex items-center justify-center rounded-[inherit] bg-[#202124] text-[#e8eaed]"
      role="status"
    >
      <span class="text-xs">{{ loadingLabel }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import {
  initializeGoogleIdentity,
  renderGoogleIdentityButton,
  resolveGoogleButtonWidth,
} from '../../utils/googleIdentity'
import {
  acceptGoogleRedirectPreparedIntent,
  createGoogleRedirectIntentRefreshScheduler,
  type GoogleRedirectPreparedIntent,
} from '../../utils/googleRedirect'
import type { GoogleAccountsID } from '../../types/google-identity'

const props = withDefaults(defineProps<{
  clientId: string
  locale?: string
  shape?: 'rectangular' | 'pill'
  text?: 'signin_with' | 'continue_with'
  disabled?: boolean
  loadingLabel?: string
  uxMode?: 'popup' | 'redirect'
  loginUri?: string
  prepareRedirect?: () => Promise<GoogleRedirectPreparedIntent>
}>(), {
  locale: '',
  shape: 'rectangular',
  text: 'signin_with',
  disabled: false,
  loadingLabel: 'Loading Google sign-in',
  uxMode: 'popup',
  loginUri: '',
})

const emit = defineEmits<{
  credential: [credential: string]
  error: [error: Error]
}>()

const buttonContainerRef = ref<HTMLDivElement | null>(null)
const loading = ref(true)
const isCustomVisualShell = true
let accountsID: GoogleAccountsID | null = null
let resizeObserver: ResizeObserver | null = null
let initializeVersion = 0
let lastRenderedWidth = 0
let active = false
let redirectState = ''
let redirectPreparationPromise: Promise<GoogleRedirectPreparedIntent> | null = null

const redirectIntentScheduler = createGoogleRedirectIntentRefreshScheduler(
  (callback, delay) => window.setTimeout(callback, delay),
  (handle) => window.clearTimeout(handle as number),
)

const scheduleRedirectIntentRefresh = (version: number, issuedAt: number) => {
  redirectIntentScheduler.clear()
  if (props.uxMode !== 'redirect') return
  redirectIntentScheduler.schedule(issuedAt, () => {
    if (!active || version !== initializeVersion) return
    void initializeButton()
  })
}

const prepareRedirectIntent = (): Promise<GoogleRedirectPreparedIntent> => {
  if (redirectPreparationPromise) return redirectPreparationPromise
  if (!props.prepareRedirect) {
    return Promise.reject(new Error('Google redirect intent preparation is unavailable'))
  }
  redirectPreparationPromise = props.prepareRedirect().finally(() => {
    redirectPreparationPromise = null
  })
  return redirectPreparationPromise
}

const renderButton = () => {
  const container = buttonContainerRef.value
  if (!container || !accountsID || !active) return

  const width = resolveGoogleButtonWidth(container.getBoundingClientRect().width)
  try {
    renderGoogleIdentityButton({
      accountsID,
      container,
      locale: props.locale,
      shape: props.shape,
      text: props.text,
      width,
      state: redirectState,
    })
    lastRenderedWidth = width || 0
  } catch (error) {
    if (!isCustomVisualShell) {
      emit('error', error instanceof Error ? error : new Error(String(error)))
    }
  }
}

const scheduleRender = () => {
  void nextTick(renderButton)
}

const focusNativeGoogleButton = () => {
  if (props.disabled || loading.value) return
  const nativeButton = buttonContainerRef.value?.querySelector<HTMLElement>('[role="button"]')
  nativeButton?.focus()
  nativeButton?.click?.()
}

const initializeButton = async () => {
  const clientID = props.clientId.trim()
  const version = ++initializeVersion
  redirectIntentScheduler.clear()
  accountsID = null
  redirectState = ''
  lastRenderedWidth = 0
  buttonContainerRef.value?.replaceChildren()
  if (clientID === '') {
    loading.value = false
    return
  }

  loading.value = true
  try {
    let redirectIntentIssuedAt = 0
    if (props.uxMode === 'redirect') {
      const preparedIntent = acceptGoogleRedirectPreparedIntent(
        await prepareRedirectIntent(),
        version,
        initializeVersion,
      )
      if (!active || !preparedIntent) return
      redirectState = preparedIntent.state
      redirectIntentIssuedAt = preparedIntent.issuedAt
    }

    const initializedAccountsID = await initializeGoogleIdentity({
      clientID,
      onCredential: (credential) => {
        if (active && version === initializeVersion && !props.disabled) {
          emit('credential', credential)
        }
      },
      onInvalidCredential: () => {
        if (active && version === initializeVersion) {
          emit('error', new Error('Google credential is missing'))
        }
      },
      uxMode: props.uxMode,
      loginUri: props.loginUri,
    })
    if (!active || version !== initializeVersion) return
    accountsID = initializedAccountsID
    scheduleRender()
    if (redirectIntentIssuedAt > 0) {
      scheduleRedirectIntentRefresh(version, redirectIntentIssuedAt)
    }
  } catch (error) {
    if (!active || version !== initializeVersion) return
    if (!isCustomVisualShell) {
      emit('error', error instanceof Error ? error : new Error(String(error)))
    }
  } finally {
    if (active && version === initializeVersion) {
      loading.value = false
    }
  }
}

onMounted(() => {
  active = true
  void initializeButton()
  if (typeof ResizeObserver !== 'undefined' && buttonContainerRef.value) {
    resizeObserver = new ResizeObserver((entries) => {
      const width = resolveGoogleButtonWidth(entries[0]?.contentRect.width)
      if (!width || Math.abs(width - lastRenderedWidth) < 8) return
      scheduleRender()
    })
    resizeObserver.observe(buttonContainerRef.value)
  }
})

watch(
  () => [props.clientId, props.uxMode, props.loginUri],
  () => { void initializeButton() },
)

watch(
  () => [props.locale, props.shape, props.text],
  scheduleRender,
)

onBeforeUnmount(() => {
  active = false
  initializeVersion += 1
  resizeObserver?.disconnect()
  resizeObserver = null
  redirectIntentScheduler.clear()
  accountsID?.cancel?.()
  accountsID = null
  buttonContainerRef.value?.replaceChildren()
})
</script>
