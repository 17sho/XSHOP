<template>
  <div class="space-y-6">
    <div class="rounded-2xl border bg-card p-7 shadow-sm">
      <PanelHeading
        :title="t('personalCenter.security.title')"
        :description="!canManageEmail ? t('personalCenter.security.subtitleReadOnly') : requiresOldEmailCode ? t('personalCenter.security.subtitle') : t('personalCenter.security.subtitleBindOnly')"
        :icon="ShieldCheck"
      >
        <template #actions>
          <Badge variant="accent" size="sm">{{ t('personalCenter.tabs.security') }}</Badge>
        </template>
      </PanelHeading>

      <Alert v-if="securityAlert" class="mb-5" :variant="pageAlertVariant(securityAlert.level)" :class="pageAlertToneClass(securityAlert.level)">
        <AlertDescription>{{ securityAlert.message }}</AlertDescription>
      </Alert>

      <TelegramBindingSection
        v-if="features.telegram_binding"
        :telegram-enabled="telegramEnabled"
        :telegram-bound="telegramBound"
        :loading-telegram-binding="userProfileStore.loadingTelegramBinding"
        :avatar-url="userProfileStore.telegramBinding?.avatar_url || ''"
        :telegram-display-name="telegramDisplayName"
        :provider-user-id="userProfileStore.telegramBinding?.provider_user_id || '-'"
        :formatted-auth-at="formatDate(userProfileStore.telegramBinding?.auth_at) || '-'"
        :unbinding-telegram="userProfileStore.unbindingTelegram"
        :can-unbind-telegram="canUnbindTelegram"
        :show-telegram-mini-app-entry="showTelegramMiniAppEntry"
        :show-mini-app-bind-action="showMiniAppBindAction"
        :show-telegram-widget="showTelegramWidget"
        :show-telegram-oidc-bind="showTelegramOidcBind"
        :binding-telegram="userProfileStore.bindingTelegram"
        :mini-app-init-data="miniAppInitData"
        ref="telegramSectionRef"
        @unbind="handleUnbindTelegram"
        @mini-app-bind="handleTelegramMiniAppBind"
        @open-mini-app-entry="openTelegramMiniAppEntry"
        @oidc-bind="startTelegramOidcBind"
      />

      <GoogleBindingSection
        v-if="features.google_binding"
        :key="googleGeneration"
        :google-enabled="googleAuthorizationEnabled"
        :google-bound="googleBound"
        :loading-google-binding="userProfileStore.loadingGoogleBinding"
        :client-id="googleClientID"
        :locale="googleButtonLocale"
        :avatar-url="userProfileStore.googleBinding?.avatar_url || ''"
        :google-display-name="googleDisplayName"
        :email="userProfileStore.googleBinding?.email || userProfileStore.googleBinding?.username || ''"
        :provider-user-id="userProfileStore.googleBinding?.provider_user_id || '-'"
        :formatted-auth-at="formatDate(userProfileStore.googleBinding?.auth_at) || '-'"
        :binding-google="userProfileStore.bindingGoogle"
        :unbinding-google="userProfileStore.unbindingGoogle"
        :can-unbind-google="canUnbindGoogle"
        :is-telegram-mini-app="isTelegramMiniApp"
        :google-ux-mode="googleIdentityUXMode"
        :google-login-uri="googleRedirectLoginURI"
        :prepare-redirect="prepareGoogleRedirectBind"
        @credential="googleCredentialHandler"
        @script-error="googleErrorHandler"
        @unbind="handleUnbindGoogle"
      />

      <EmailChangeForm
        v-if="canManageEmail"
        :current-email-display="currentEmailDisplay"
        :requires-old-email-code="requiresOldEmailCode"
        v-model:new-email="securityForm.newEmail"
        v-model:old-code="securityForm.oldCode"
        v-model:new-code="securityForm.newCode"
        :sending-code="userProfileStore.sendingCode"
        :old-code-cooldown="oldCodeCooldown"
        :new-code-cooldown="newCodeCooldown"
        :changing-email="userProfileStore.changingEmail"
        @submit="handleChangeEmail"
        @send-old-code="handleSendOldCode"
        @send-new-code="handleSendNewCode"
      />
      <p v-else-if="userProfileStore.profile?.email" class="text-sm text-muted-foreground">
        {{ t('personalCenter.security.currentEmailLabel') }}：{{ userProfileStore.profile.email }}
      </p>
    </div>

    <LoginHistorySection
      v-if="features.login_history"
      :loading="userProfileStore.loadingLoginLogs"
      :logs="userProfileStore.recentLoginLogs"
    />

    <PasswordChangeForm
      v-if="canManagePassword"
      :requires-old-password="requiresOldPassword"
      v-model:old-password="passwordForm.oldPassword"
      v-model:new-password="passwordForm.newPassword"
      v-model:confirm-password="passwordForm.confirmPassword"
      :changing-password="userProfileStore.changingPassword"
      @submit="handleChangePassword"
    />

    <TwoFactorSection v-if="hasPasswordCapability" :allow-setup="canSetupTwoFactor" :setup-authority="getTwoFactorSetupAuthority" />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { ShieldCheck } from 'lucide-vue-next'
import { pageAlertVariant, pageAlertToneClass, type PageAlert } from '../../utils/alerts'
import PanelHeading from '../../components/shared/PanelHeading.vue'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { userProfileAPI } from '../../api/user'
import type { TelegramAuthPayload } from '../../api'
import { useAppStore } from '../../stores/app'
import { useTelegramMiniAppStore } from '../../stores/telegramMiniApp'
import { useUserProfileStore } from '../../stores/userProfile'
import { useUserAuthStore } from '../../stores/userAuth'
import { buildTelegramMiniAppEntryLink, isTelegramUrlEnvironment, openTelegramCompatibleLink } from '../../utils/telegramMiniApp'
import { normalizeSecurityCenterConfig, type SecurityCenterKey } from '../../utils/securityCenterConfig'
import { canUnbindExternalIdentity } from '../../utils/externalIdentity'
import { detectGoogleIdentityUXMode } from '../../utils/googleIdentity'
import { createGoogleRedirectIntent, createGoogleRedirectPreparedIntent, getGoogleRedirectSessionStorage, storeGoogleRedirectIntent, tryBuildGoogleRedirectCredentialCallbackURL } from '../../utils/googleRedirect'
import TelegramBindingSection from '../../components/security/TelegramBindingSection.vue'
import GoogleBindingSection from '../../components/security/GoogleBindingSection.vue'
import EmailChangeForm from '../../components/security/EmailChangeForm.vue'
import LoginHistorySection from '../../components/security/LoginHistorySection.vue'
import PasswordChangeForm from '../../components/security/PasswordChangeForm.vue'
import TwoFactorSection from '../../components/security/TwoFactorSection.vue'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const telegramMiniAppStore = useTelegramMiniAppStore()
const userProfileStore = useUserProfileStore()
const userAuthStore = useUserAuthStore()
const securityForm = reactive({ newEmail: '', oldCode: '', newCode: '' })
const passwordForm = reactive({ oldPassword: '', newPassword: '', confirmPassword: '' })
const securityAlert = ref<PageAlert | null>(null)
const oldCodeCooldown = ref(0)
const newCodeCooldown = ref(0)
let emailDraftGeneration = 0
let emailTargetGeneration = 0
watch(securityForm, () => { emailDraftGeneration++ }, { flush: 'sync' })
watch(() => securityForm.newEmail, () => { emailTargetGeneration++; newCodeCooldown.value = 0 }, { flush: 'sync' })
const telegramSectionRef = ref<InstanceType<typeof TelegramBindingSection> | null>(null)
let cooldownTimer: number | null = null
let disposed = false
let generation = 0
let telegramRenderVersion = 0
let installedTelegramCallback: ((raw: unknown) => void) | null = null
let installedTelegramCallbackName = ''
const telegramCallbackName = '__dujiaoSecurityTelegramBind'
const telegramGeneration = ref(0)
const googleGeneration = ref(0)

const features = computed(() => normalizeSecurityCenterConfig(appStore.config))
const featureGenerations = reactive<Record<SecurityCenterKey, number>>({ telegram_binding: 0, google_binding: 0, email_change: 0, password_change: 0, two_factor: 0, login_history: 0 })
type Provider = 'telegram' | 'google'
const captureAuthority = (provider?: Provider, feature?: SecurityCenterKey) => {
  const key = feature || (provider ? `${provider}_binding` as SecurityCenterKey : undefined)
  const policyGeneration = key ? featureGenerations[key] : 0
  const owner = generation
  const session = userAuthStore.sessionGeneration
  const token = userAuthStore.token
  const providerGeneration = provider === 'telegram' ? telegramGeneration.value : googleGeneration.value
  return () => !disposed && !!token && userAuthStore.token === token
    && owner === generation && session === userAuthStore.sessionGeneration
    && (!key || (features.value[key] && policyGeneration === featureGenerations[key]))
    && (!provider || providerGeneration === (provider === 'telegram' ? telegramGeneration.value : googleGeneration.value))
}
const alive = () => !disposed && !!userAuthStore.token
const alert = (level: PageAlert['level'], key: string) => { securityAlert.value = { level, message: t(`personalCenter.security.${key}`) } }

const telegramConfig = computed(() => appStore.config?.telegram_auth || null)
const telegramBotUsername = computed(() => String(telegramConfig.value?.bot_username || '').trim())
const telegramMiniAppURL = computed(() => String(telegramConfig.value?.mini_app_url || '').trim())
const telegramLoginMode = computed(() => String(telegramConfig.value?.mode || '').trim())
const telegramEnabled = computed(() => features.value.telegram_binding && ['widget', 'oidc'].includes(telegramLoginMode.value) && !!telegramConfig.value?.enabled && telegramBotUsername.value !== '')
const telegramBound = computed(() => !!userProfileStore.telegramBinding?.bound)
const googleConfig = computed(() => appStore.config?.google_auth || null)
const googleClientID = computed(() => String(googleConfig.value?.client_id || '').trim())
const googleEnabled = computed(() => features.value.google_binding && !!googleConfig.value?.enabled && googleClientID.value !== '')
const googleButtonLocale = computed(() => String(appStore.locale || '').trim())
const googleBound = computed(() => !!userProfileStore.googleBinding?.bound)
const googleIdentityUXMode = detectGoogleIdentityUXMode()
const googleRedirectLoginURI = googleIdentityUXMode === 'redirect' ? tryBuildGoogleRedirectCredentialCallbackURL() : ''
const googleAuthorizationEnabled = computed(() => googleEnabled.value && (googleIdentityUXMode === 'popup' || googleRedirectLoginURI !== ''))
const isTelegramUrlEnv = isTelegramUrlEnvironment()
const isTelegramMiniApp = computed(() => (telegramMiniAppStore.isMiniApp && telegramMiniAppStore.isReady) || isTelegramUrlEnv)
const miniAppInitData = computed(() => String(telegramMiniAppStore.initData || '').trim())
const showMiniAppBindAction = computed(() => telegramEnabled.value && !telegramBound.value && isTelegramMiniApp.value)
const showTelegramWidget = computed(() => telegramLoginMode.value === 'widget' && telegramEnabled.value && !telegramBound.value && !isTelegramMiniApp.value)
const showTelegramOidcBind = computed(() => telegramLoginMode.value === 'oidc' && telegramEnabled.value && !telegramBound.value && !isTelegramMiniApp.value)
const telegramMiniAppEntryLink = computed(() => buildTelegramMiniAppEntryLink(telegramBotUsername.value, telegramMiniAppURL.value))
const showTelegramMiniAppEntry = computed(() => features.value.telegram_binding && telegramEnabled.value && !isTelegramMiniApp.value && telegramMiniAppEntryLink.value !== '')
const emailChangeMode = computed(() => userProfileStore.profile?.email_change_mode || '')
const canManageEmail = computed(() => features.value.email_change && ['bind_only', 'change_with_old_and_new'].includes(emailChangeMode.value))
const requiresOldEmailCode = computed(() => emailChangeMode.value === 'change_with_old_and_new')
const passwordChangeMode = computed(() => userProfileStore.profile?.password_change_mode || '')
const hasPasswordCapability = computed(() => requiresOldEmailCode.value && ['change_with_old', 'set_without_old'].includes(passwordChangeMode.value))
const canManagePassword = computed(() => features.value.password_change && hasPasswordCapability.value)
const canSetupTwoFactor = computed(() => features.value.two_factor && hasPasswordCapability.value)
// Read the live source and its sync generation, not a render-delivered snapshot.
const getTwoFactorSetupAuthority = (): Readonly<{ allowed: boolean; generation: number }> => ({
  allowed: !disposed && canSetupTwoFactor.value,
  generation: featureGenerations.two_factor,
})
const requiresOldPassword = computed(() => passwordChangeMode.value === 'change_with_old')
// Missing backend capability data must never enable identity unbinding.
const canUnbindTelegram = computed(() => features.value.telegram_binding && canUnbindExternalIdentity(userProfileStore.telegramBinding))
const canUnbindGoogle = computed(() => features.value.google_binding && canUnbindExternalIdentity(userProfileStore.googleBinding))
const currentEmailDisplay = computed(() => requiresOldEmailCode.value ? userProfileStore.profile?.email || '' : t('personalCenter.security.bindOnlyEmailDisplay'))
const telegramDisplayName = computed(() => userProfileStore.telegramBinding?.username ? `@${userProfileStore.telegramBinding.username}` : t('personalCenter.security.telegramDisplayFallback'))
const googleDisplayName = computed(() => {
  const binding = userProfileStore.googleBinding
  return String(binding?.display_name || binding?.email || binding?.username || '').trim() || t('personalCenter.security.googleDisplayFallback')
})
const formatDate = (raw?: string | null) => {
  if (!raw) return ''
  const date = new Date(raw)
  return Number.isNaN(date.getTime()) ? raw : date.toLocaleString()
}

// Store actions fence sessions, but not their initiating component's lifetime.
// Use the same API adapters and store state, with authority checked BEFORE every
// commit. Do not await a store mutation that can already commit after disposal.
type BusyKey = 'sendingCode' | 'changingEmail' | 'changingPassword' | 'loadingLoginLogs' | 'loadingTelegramBinding' | 'loadingGoogleBinding' | 'bindingTelegram' | 'bindingGoogle' | 'unbindingTelegram' | 'unbindingGoogle'
const pending = new Map<BusyKey, object>()
const request = async <T,>(busy: BusyKey, run: () => Promise<T>, commit?: (data: T) => void, provider?: Provider, feature?: SecurityCenterKey): Promise<boolean> => {
  const isCurrent = captureAuthority(provider, feature)
  if (!isCurrent() || userProfileStore[busy]) return false
  const owner = {}
  pending.set(busy, owner)
  userProfileStore[busy] = true
  userProfileStore.securityError = ''
  try {
    const response = await run()
    if (!isCurrent() || pending.get(busy) !== owner) return false
    commit?.(response)
    return true
  } catch (error) {
    if (!isCurrent() || pending.get(busy) !== owner) return false
    userProfileStore.securityError = error instanceof Error ? error.message : t('personalCenter.security.externalIdentityRefreshFailed')
    return false
  } finally {
    if (pending.get(busy) === owner) {
      pending.delete(busy)
      userProfileStore[busy] = false
    }
  }
}
const releasePending = () => {
  for (const busy of pending.keys()) userProfileStore[busy] = false
  pending.clear()
}
const loadTelegramBinding = () => request('loadingTelegramBinding', () => userProfileAPI.getTelegramBinding(), r => { userProfileStore.telegramBinding = r.data.data || { bound: false } }, 'telegram')
const loadGoogleBinding = () => request('loadingGoogleBinding', () => userProfileAPI.getGoogleBinding(), r => { userProfileStore.googleBinding = r.data.data || { bound: false } }, 'google')
const finishExternalIdentityMutation = async (successKey: string, isCurrent: () => boolean) => {
  if (!isCurrent()) return
  const loaded = await Promise.all([
    features.value.telegram_binding ? loadTelegramBinding() : Promise.resolve(true),
    features.value.google_binding ? loadGoogleBinding() : Promise.resolve(true),
  ])
  if (!isCurrent()) return
  alert(loaded.every(Boolean) ? 'success' : 'warning', loaded.every(Boolean) ? successKey : 'externalIdentityRefreshFailed')
}
const clearCooldown = () => {
  if (cooldownTimer !== null) window.clearInterval(cooldownTimer)
  cooldownTimer = null
  oldCodeCooldown.value = 0
  newCodeCooldown.value = 0
}
const startCooldown = (kind: 'old' | 'new') => {
  if (!alive()) return
  if (kind === 'old') oldCodeCooldown.value = 60
  else newCodeCooldown.value = 60
  if (cooldownTimer !== null) return
  const isCurrent = captureAuthority()
  cooldownTimer = window.setInterval(() => {
    if (!isCurrent()) return
    oldCodeCooldown.value = Math.max(0, oldCodeCooldown.value - 1)
    newCodeCooldown.value = Math.max(0, newCodeCooldown.value - 1)
    if (!oldCodeCooldown.value && !newCodeCooldown.value) clearCooldown()
  }, 1000)
}
const sendCode = async (kind: 'old' | 'new') => {
  const isCurrent = captureAuthority(undefined, 'email_change')
  if (!isCurrent() || !canManageEmail.value || userProfileStore.sendingCode || (kind === 'old' ? oldCodeCooldown.value : newCodeCooldown.value) > 0) return
  securityAlert.value = null
  if (kind === 'old' && !requiresOldEmailCode.value) { alert('warning', 'bindOnlyOldCodeDisabled'); return }
  const newEmail = securityForm.newEmail.trim()
  const targetGeneration = emailTargetGeneration
  if (kind === 'new' && !newEmail) { alert('warning', 'newEmailRequired'); return }
  const ok = await request('sendingCode', () => userProfileAPI.sendChangeEmailCode({ kind, ...(kind === 'new' ? { new_email: newEmail } : {}) }), undefined, undefined, 'email_change')
  if (!isCurrent() || (kind === 'new' && targetGeneration !== emailTargetGeneration)) return
  if (!ok) { securityAlert.value = { level: 'error', message: userProfileStore.securityError || t('personalCenter.security.sendCodeFailed') }; return }
  startCooldown(kind)
  alert('success', kind === 'old' ? 'sendOldCodeSuccess' : 'sendNewCodeSuccess')
}
const handleSendOldCode = () => sendCode('old')
const handleSendNewCode = () => sendCode('new')
const handleChangeEmail = async () => {
  const isCurrent = captureAuthority(undefined, 'email_change')
  if (!isCurrent() || !canManageEmail.value || userProfileStore.changingEmail) return
  securityAlert.value = null
  const needsOld = requiresOldEmailCode.value
  const draftGeneration = emailDraftGeneration
  const payload = { new_email: securityForm.newEmail.trim(), new_code: securityForm.newCode.trim(), ...(needsOld ? { old_code: securityForm.oldCode.trim() } : {}) }
  if (!payload.new_email || !payload.new_code || (needsOld && !payload.old_code)) { alert('warning', needsOld ? 'changeEmailRequired' : 'bindEmailRequired'); return }
  const ok = await request('changingEmail', () => userProfileAPI.changeEmail(payload), r => {
    userProfileStore.profile = r.data.data
    userAuthStore.syncUserProfile(r.data.data)
  }, undefined, 'email_change')
  if (!isCurrent()) return
  if (!ok) { securityAlert.value = { level: 'error', message: userProfileStore.securityError || t('personalCenter.security.changeEmailFailed') }; return }
  if (draftGeneration === emailDraftGeneration) Object.assign(securityForm, { newEmail: '', oldCode: '', newCode: '' })
  clearCooldown()
  alert('success', needsOld ? 'changeEmailSuccess' : 'bindEmailSuccess')
}
const handleChangePassword = async () => {
  const isCurrent = captureAuthority(undefined, 'password_change')
  if (!isCurrent() || !canManagePassword.value || userProfileStore.changingPassword) return
  securityAlert.value = null
  const oldPassword = passwordForm.oldPassword.trim()
  const newPassword = passwordForm.newPassword.trim()
  const confirmPassword = passwordForm.confirmPassword.trim()
  const needsOld = requiresOldPassword.value
  if (!newPassword || !confirmPassword || (needsOld && !oldPassword)) { alert('warning', needsOld ? 'changePasswordRequired' : 'setPasswordRequired'); return }
  if (newPassword !== confirmPassword) { alert('warning', 'passwordMismatch'); return }
  const ok = await request('changingPassword', () => userProfileAPI.changePassword({ new_password: newPassword, ...(needsOld ? { old_password: oldPassword } : {}) }), undefined, undefined, 'password_change')
  if (!isCurrent()) return
  if (!ok) { securityAlert.value = { level: 'error', message: userProfileStore.securityError || t('personalCenter.security.changePasswordFailed') }; return }
  Object.assign(passwordForm, { oldPassword: '', newPassword: '', confirmPassword: '' })
  alert('success', needsOld ? 'changePasswordSuccess' : 'setPasswordSuccess')
  userAuthStore.logout('/auth/login?reason=password_changed')
}
const buildTelegramPayload = (raw: any): TelegramAuthPayload | null => {
  const id = Number(raw?.id), authDate = Number(raw?.auth_date), hash = String(raw?.hash || '').trim()
  if (!Number.isFinite(id) || id <= 0 || !Number.isFinite(authDate) || authDate <= 0 || !hash) return null
  return { id, auth_date: authDate, hash, first_name: String(raw?.first_name || '').trim(), last_name: String(raw?.last_name || '').trim(), username: String(raw?.username || '').trim(), photo_url: String(raw?.photo_url || '').trim() }
}
const handleTelegramBind = async (raw: unknown) => {
  const isCurrent = captureAuthority('telegram')
  if (!isCurrent() || !showTelegramWidget.value || userProfileStore.bindingTelegram) return
  securityAlert.value = null
  const payload = buildTelegramPayload(raw)
  if (!payload) { alert('warning', 'telegramInvalidPayload'); return }
  const ok = await request('bindingTelegram', () => userProfileAPI.bindTelegram(payload), r => { userProfileStore.telegramBinding = r.data.data || { bound: true } }, 'telegram')
  if (!isCurrent()) return
  if (!ok) { securityAlert.value = { level: 'error', message: userProfileStore.securityError || t('personalCenter.security.telegramBindFailed') }; return }
  await finishExternalIdentityMutation('telegramBindSuccess', isCurrent)
}
const handleTelegramMiniAppBind = async () => {
  const isCurrent = captureAuthority('telegram')
  if (!isCurrent() || !showMiniAppBindAction.value || userProfileStore.bindingTelegram) return
  securityAlert.value = null
  if (!miniAppInitData.value) { alert('warning', 'telegramMiniAppInitDataMissing'); return }
  const ok = await request('bindingTelegram', () => userProfileAPI.bindTelegramMiniApp({ init_data: miniAppInitData.value }), r => { userProfileStore.telegramBinding = r.data.data || { bound: true } }, 'telegram')
  if (!isCurrent()) return
  if (!ok) { securityAlert.value = { level: 'error', message: userProfileStore.securityError || t('personalCenter.security.telegramBindFailed') }; return }
  await finishExternalIdentityMutation('telegramBindSuccess', isCurrent)
}
const handleGoogleBind = async (credential: string) => {
  const isCurrent = captureAuthority('google')
  if (!isCurrent() || !googleAuthorizationEnabled.value || googleBound.value || isTelegramMiniApp.value || userProfileStore.bindingGoogle) return
  securityAlert.value = null
  const normalized = String(credential || '').trim()
  if (!normalized) { alert('warning', 'googleInvalidCredential'); return }
  const ok = await request('bindingGoogle', () => userProfileAPI.bindGoogle({ credential: normalized }), r => { userProfileStore.googleBinding = r.data.data || { bound: true } }, 'google')
  if (!isCurrent()) return
  if (!ok) { securityAlert.value = { level: 'error', message: userProfileStore.securityError || t('personalCenter.security.googleBindFailed') }; return }
  await finishExternalIdentityMutation('googleBindSuccess', isCurrent)
}
// Render-captured callbacks reject events retained by an old SDK even before
// Vue flushes a client/session A -> B -> A replacement.
const googleCredentialHandler = computed(() => {
  const version = googleGeneration.value
  const isCurrent = captureAuthority('google')
  return (credential: string) => { if (version === googleGeneration.value && isCurrent()) void handleGoogleBind(credential) }
})
const handleGoogleScriptError = () => { if (alive() && googleAuthorizationEnabled.value) alert('error', 'googleWidgetLoadFailed') }
const googleErrorHandler = computed(() => {
  const version = googleGeneration.value
  const isCurrent = captureAuthority('google')
  return () => { if (version === googleGeneration.value && isCurrent()) handleGoogleScriptError() }
})
const unbindIdentity = async (provider: Provider) => {
  const isCurrent = captureAuthority(provider)
  const busy = provider === 'telegram' ? 'unbindingTelegram' : 'unbindingGoogle'
  if (!isCurrent() || userProfileStore[busy]) return
  securityAlert.value = null
  if (!(provider === 'telegram' ? canUnbindTelegram.value : canUnbindGoogle.value)) { alert('warning', `${provider}UnbindDisabledTip`); return }
  const ok = await request(busy, () => provider === 'telegram' ? userProfileAPI.unbindTelegram() : userProfileAPI.unbindGoogle(), () => {
    if (provider === 'telegram') userProfileStore.telegramBinding = { bound: false }
    else userProfileStore.googleBinding = { bound: false }
  }, provider)
  if (!isCurrent()) return
  if (!ok) { securityAlert.value = { level: 'error', message: userProfileStore.securityError || t(`personalCenter.security.${provider}UnbindFailed`) }; return }
  await finishExternalIdentityMutation(`${provider}UnbindSuccess`, isCurrent)
}
const handleUnbindTelegram = () => unbindIdentity('telegram')
const handleUnbindGoogle = () => unbindIdentity('google')
const openTelegramMiniAppEntry = () => { if (alive() && showTelegramMiniAppEntry.value) openTelegramCompatibleLink(telegramMiniAppEntryLink.value) }
const startTelegramOidcBind = async () => {
  const isCurrent = captureAuthority('telegram')
  if (!isCurrent() || !showTelegramOidcBind.value || userProfileStore.bindingTelegram) return
  securityAlert.value = null
  await request('bindingTelegram', () => userProfileAPI.telegramOidcBindStart(), r => {
    if (!isCurrent()) return
    const url = String(r?.data?.data?.auth_url || '')
    if (!url) { alert('error', 'telegramOidcBindFailed'); return }
    sessionStorage.setItem('tg_oidc_intent', 'bind')
    window.location.href = url
  }, 'telegram')
  if (isCurrent() && userProfileStore.securityError) securityAlert.value = { level: 'error', message: userProfileStore.securityError }
}
let googleRedirectVersion = 0
const prepareGoogleRedirectBind = async () => {
  const isCurrent = captureAuthority('google')
  if (!isCurrent() || !googleAuthorizationEnabled.value || googleBound.value || isTelegramMiniApp.value) throw new Error('Google binding is unavailable')
  const version = ++googleRedirectVersion
  const response = await userProfileAPI.googleRedirectBindIntent()
  if (!isCurrent() || version !== googleRedirectVersion) throw new Error('Google binding request is stale')
  const intent = createGoogleRedirectPreparedIntent(response.data.data)
  if (!intent) throw new Error('Google redirect state is invalid')
  storeGoogleRedirectIntent(getGoogleRedirectSessionStorage(), createGoogleRedirectIntent('bind', '/me/security', intent.issuedAt))
  return intent
}
const clearTelegramWidget = () => {
  telegramRenderVersion++
  const win = window as Window & Record<string, any>
  if (win[telegramCallbackName] === installedTelegramCallback) delete win[telegramCallbackName]
  if (installedTelegramCallbackName && win[installedTelegramCallbackName] === installedTelegramCallback) delete win[installedTelegramCallbackName]
  installedTelegramCallbackName = ''
  installedTelegramCallback = null
  telegramSectionRef.value?.telegramWidgetRef?.replaceChildren()
}
const renderTelegramWidget = () => {
  clearTelegramWidget()
  const widget = telegramSectionRef.value?.telegramWidgetRef
  if (!alive() || !showTelegramWidget.value || !widget) return
  const isCurrent = captureAuthority('telegram')
  const version = telegramRenderVersion
  const valid = () => isCurrent() && version === telegramRenderVersion && showTelegramWidget.value
  const callback = (raw: unknown) => { if (valid()) return handleTelegramBind(raw) }
  const win = window as Window & Record<string, any>
  // The SDK parses data-onauth as an expression and resolves names later. A
  // stable global name would let an old widget call the replacement owner.
  const sequenceKey = '__dujiaoSecurityTelegramWidgetSequence'
  const sequence = Number(win[sequenceKey] || 0) + 1
  win[sequenceKey] = sequence
  installedTelegramCallbackName = `${telegramCallbackName}_${sequence}`
  installedTelegramCallback = callback
  win[installedTelegramCallbackName] = callback
  win[telegramCallbackName] = callback
  const script = document.createElement('script')
  script.async = true
  script.src = 'https://telegram.org/js/telegram-widget.js?22'
  script.setAttribute('data-telegram-login', telegramBotUsername.value)
  script.setAttribute('data-size', 'large')
  script.setAttribute('data-userpic', 'false')
  script.setAttribute('data-request-access', 'write')
  const lookup = `window[${JSON.stringify(installedTelegramCallbackName)}]`
  script.setAttribute('data-onauth', `${lookup} && ${lookup}(user)`)
  script.onerror = () => { if (valid()) alert('error', 'telegramWidgetLoadFailed') }
  widget.appendChild(script)
}
const invalidate = () => {
  generation++
  telegramGeneration.value++
  googleGeneration.value++
  googleRedirectVersion++
  releasePending()
  clearCooldown()
  clearTelegramWidget()
  Object.assign(securityForm, { newEmail: '', oldCode: '', newCode: '' })
  Object.assign(passwordForm, { oldPassword: '', newPassword: '', confirmPassword: '' })
  securityAlert.value = null
  void nextTick(() => { if (alive()) renderTelegramWidget() })
}
watch(() => [userAuthStore.sessionGeneration, userAuthStore.token], invalidate, { flush: 'sync' })
watch([() => features.value.telegram_binding, telegramEnabled, telegramBotUsername, telegramLoginMode, isTelegramMiniApp, miniAppInitData], () => {
  telegramGeneration.value++
  clearTelegramWidget()
  void nextTick(() => { if (alive()) renderTelegramWidget() })
}, { flush: 'sync' })
watch([() => features.value.google_binding, googleAuthorizationEnabled, googleClientID, isTelegramMiniApp], () => { googleGeneration.value++ }, { flush: 'sync' })
watch([showTelegramWidget, () => userProfileStore.loadingTelegramBinding], () => { void nextTick(() => { if (alive()) renderTelegramWidget() }) })
const loadLoginHistory = () => request('loadingLoginLogs', () => userProfileAPI.loginLogs({ page: 1, page_size: 10 }), r => { userProfileStore.recentLoginLogs = Array.isArray(r.data.data) ? r.data.data : [] }, undefined, 'login_history')
for (const key of Object.keys(featureGenerations) as SecurityCenterKey[]) {
  watch(() => features.value[key], (enabled) => {
    featureGenerations[key]++
    if (!enabled) {
      const owned: BusyKey[] = key === 'telegram_binding' ? ['loadingTelegramBinding', 'bindingTelegram', 'unbindingTelegram']
        : key === 'google_binding' ? ['loadingGoogleBinding', 'bindingGoogle', 'unbindingGoogle']
        : key === 'email_change' ? ['sendingCode', 'changingEmail']
        : key === 'password_change' ? ['changingPassword'] : key === 'login_history' ? ['loadingLoginLogs'] : []
      for (const busy of owned) { if (pending.has(busy)) { pending.delete(busy); userProfileStore[busy] = false } }
      if (key === 'email_change') clearCooldown()
    } else if (alive()) {
      if (key === 'login_history') void loadLoginHistory()
      if (key === 'telegram_binding') void loadTelegramBinding()
      if (key === 'google_binding') void loadGoogleBinding()
    }
  }, { flush: 'sync' })
}
onMounted(async () => {
  const isCurrent = captureAuthority()
  const callbackRoute = route.fullPath
  await appStore.loadConfig()
  if (!isCurrent()) return
  await Promise.all([
    loadLoginHistory(),
    loadTelegramBinding(), loadGoogleBinding(),
  ])
  if (!isCurrent()) return
  renderTelegramWidget()
  const marker = route.query.tgBound === '1' ? 'tgBound' : route.query.googleBound === '1' ? 'googleBound' : null
  if (!marker) return
  await finishExternalIdentityMutation(marker === 'tgBound' ? 'telegramBoundOk' : 'googleBindSuccess', isCurrent)
  if (!isCurrent() || route.fullPath !== callbackRoute) return
  const query = { ...route.query }
  delete query[marker]
  void router.replace({ path: route.path, query })
})
onBeforeUnmount(() => { disposed = true; invalidate() })
</script>
