import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { guestOrderAPI } from '../api'
import { orderStatusVariant, orderStatusLabel } from '../utils/status'
import { debounceAsync } from '../utils/debounce'
import { amountToCents } from '../utils/money'
import { clearGuestOrderAuth, loadGuestOrderAuth, saveGuestOrderAuth } from '../utils/guestOrderAuth'
import { useAppStore } from '../stores/app'
import { useFormValidation } from './useFormValidation'
import type { CaptchaPayload } from '../api'

type LookupMode = 'browser' | 'credentials'

/** 游客订单查询/列表逻辑（classic + vault 共用）。 */
export function useGuestOrders() {
  const { t } = useI18n()
  const appStore = useAppStore()
  const { emailRule } = useFormValidation(['email'])
  // Reuse the guest scene: it protects creation and email/password lookup.
  const captchaProvider = computed(() => String(appStore.config?.captcha?.provider || 'none'))
  const captchaEnabled = computed(() => !!appStore.config?.captcha?.scenes?.guest_create_order && captchaProvider.value !== 'none')
  const turnstileSiteKey = computed(() => String(appStore.config?.captcha?.turnstile?.site_key || ''))
  const captchaPayload = ref<CaptchaPayload>({})
  const turnstileToken = ref('')
  const imageCaptchaRef = ref<{ refresh: () => Promise<void> } | null>(null)
  const turnstileRef = ref<{ reset: () => void } | null>(null)
  const resetCaptcha = () => {
    captchaPayload.value = {}; turnstileToken.value = ''
    if (captchaProvider.value === 'image') void imageCaptchaRef.value?.refresh()
    else turnstileRef.value?.reset()
  }
  const handleCaptchaConfigStale = async () => { await appStore.loadConfig(true); resetCaptcha() }
  const getCaptchaPayload = (): CaptchaPayload | undefined => !captchaEnabled.value ? undefined : captchaProvider.value === 'image' ? { ...captchaPayload.value } : { turnstile_token: turnstileToken.value }
  const validateLookup = () => {
    if (!email.value.trim() || !orderPassword.value.trim()) { error.value = t('guestOrders.errors.missing'); return false }
    if (emailRule()(email.value.trim())) { error.value = t('error.email_invalid'); return false }
    if (captchaEnabled.value && !(captchaProvider.value === 'image' ? captchaPayload.value.captcha_id && captchaPayload.value.captcha_code : captchaProvider.value === 'turnstile' && turnstileToken.value)) {
      error.value = t('auth.common.captchaRequired'); return false
    }
    return true
  }
  const activeTab = ref<LookupMode>('browser')
  const savedAuth = ref({ email: '', order_password: '' })
  const email = ref('')
  const orderPassword = ref('')
  const orderNo = ref('')
  let submittedOrderNo = ''
  const loading = ref(false)
  const error = ref('')
  const orders = ref<any[]>([])
  const pagination = ref({ page: 1, page_size: 20, total: 0, total_page: 1 })

  const loadSavedAuth = () => {
    savedAuth.value = loadGuestOrderAuth()
    email.value = savedAuth.value.email
    orderPassword.value = savedAuth.value.order_password
  }
  const hasSavedAuth = computed(() => Boolean(savedAuth.value.email || savedAuth.value.order_password))
  const persistAuth = () => {
    const payload = { email: email.value, order_password: orderPassword.value }
    saveGuestOrderAuth(payload)
    savedAuth.value = payload
  }
  let requestGeneration = 0
  const invalidateRequests = () => {
    requestGeneration++
    debouncedLoadOrders.cancel()
    loading.value = false
  }
  const resetResults = () => {
    invalidateRequests()
    orders.value = []
    error.value = ''
    pagination.value = { page: 1, page_size: 20, total: 0, total_page: 1 }
  }
  const clearSaved = () => {
    clearGuestOrderAuth()
    savedAuth.value = { email: '', order_password: '' }
    email.value = ''
    orderPassword.value = ''
    orderNo.value = ''
    submittedOrderNo = ''
    resetResults()
  }
  const applyResponse = (response: any) => {
    orders.value = response.data.data || []
    pagination.value = response.data.pagination || pagination.value
  }
  const loadBrowserOrders = async (page = 1) => {
    const generation = ++requestGeneration
    loading.value = true
    error.value = ''
    try {
      const response = await guestOrderAPI.browserOrders({ page, page_size: pagination.value.page_size })
      if (generation !== requestGeneration) return
      applyResponse(response)
    } catch (err: any) {
      if (generation !== requestGeneration) return
      orders.value = []
      error.value = err.message || t('guestOrders.errors.browserFailed')
    } finally {
      if (generation === requestGeneration) loading.value = false
    }
  }
  const loadCredentialOrders = async (page: number) => {
    if (!validateLookup()) return
    const generation = ++requestGeneration
    loading.value = true
    error.value = ''
    try {
      const response = await guestOrderAPI.list({
        email: email.value,
        order_password: orderPassword.value,
        ...(submittedOrderNo ? { order_no: submittedOrderNo } : {}),
        page,
        page_size: pagination.value.page_size,
        ...(captchaEnabled.value ? { captcha_payload: getCaptchaPayload() } : {}),
      })
      if (generation !== requestGeneration) return
      applyResponse(response)
    } catch (err: any) {
      if (generation !== requestGeneration) return
      orders.value = []
      error.value = err.message || t('guestOrders.errors.searchFailed')
    } finally {
      if (generation === requestGeneration) { loading.value = false; resetCaptcha() }
    }
  }
  const debouncedLoadOrders = debounceAsync(loadCredentialOrders, 300)
  const searchByCredentials = async () => {
    // The submitted query owns the UI immediately, not when its debounce fires.
    resetResults()
    if (!validateLookup()) return
    email.value = email.value.trim().toLowerCase()
    submittedOrderNo = orderNo.value.trim()
    persistAuth()
    await debouncedLoadOrders(1)
  }
  const handleSearch = searchByCredentials
  const setActiveTab = (tab: LookupMode) => {
    activeTab.value = tab
    resetResults()
    if (tab === 'browser') void loadBrowserOrders(1)
  }
  const emptyMessage = computed(() => activeTab.value === 'browser'
    ? t('guestOrders.browserEmpty')
    : t('guestOrders.empty'))
  const changePage = (page: number) => {
    if (page < 1 || page > pagination.value.total_page) return
    if (activeTab.value === 'browser') void loadBrowserOrders(page)
    else debouncedLoadOrders(page)
  }
  const statusLabel = (status: string) => orderStatusLabel(t, status)
  const statusVariant = (status: string) => orderStatusVariant(status)
  const statusPillClass = (status?: string) => ({ success: 'pill-done', warning: 'pill-low', danger: 'pill-sale', info: 'pill-stock', accent: 'pill-sale', neutral: 'pill-out' })[orderStatusVariant(status)] || 'pill-out'
  const formatMoney = (amount?: string, currency?: string) => amount == null || amount === '' ? '-' : currency ? `${amount} ${currency}` : String(amount)
  const hasDiscountAmount = (amount?: string) => {
    if (amount == null || amount === '') return false
    const cents = amountToCents(amount)
    return cents !== null && cents > 0
  }
  const formatDiscountMoney = (amount?: string, currency?: string) => hasDiscountAmount(amount) ? `-${formatMoney(amount, currency)}` : formatMoney(amount, currency)
  const hasDiscount = (order: any) => Boolean(order && (hasDiscountAmount(order.discount_amount) || hasDiscountAmount(order.promotion_discount_amount)))
  const formatDate = (raw?: string) => {
    if (!raw) return ''
    const date = new Date(raw)
    return Number.isNaN(date.getTime()) ? raw : date.toLocaleString()
  }

  onMounted(() => {
    void appStore.loadConfig()
    loadSavedAuth()
    void loadBrowserOrders(1)
  })
  onUnmounted(invalidateRequests)

  return {
    captchaEnabled, captchaProvider, captchaPayload, turnstileToken, turnstileSiteKey, imageCaptchaRef, turnstileRef, handleCaptchaConfigStale,
    activeTab, setActiveTab, savedAuth, email, orderPassword, orderNo, loading, error, orders, pagination,
    hasSavedAuth, clearSaved, handleSearch, loadBrowserOrders, searchByCredentials,
    emptyMessage, changePage, statusLabel, statusVariant, statusPillClass, formatMoney,
    formatDiscountMoney, hasDiscountAmount, hasDiscount, formatDate,
  }
}
