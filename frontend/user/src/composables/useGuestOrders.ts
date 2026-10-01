import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { guestOrderAPI } from '../api'
import { orderStatusVariant, orderStatusLabel } from '../utils/status'
import { debounceAsync } from '../utils/debounce'
import { amountToCents } from '../utils/money'
import { clearGuestOrderAuth, loadGuestOrderAuth, saveGuestOrderAuth } from '../utils/guestOrderAuth'

type LookupMode = 'browser' | 'credentials'

/** 游客订单查询/列表逻辑（classic + vault 共用）。 */
export function useGuestOrders() {
  const { t } = useI18n()
  const activeTab = ref<LookupMode>('browser')
  const savedAuth = ref({ email: '', order_password: '' })
  const email = ref('')
  const orderPassword = ref('')
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
    const generation = ++requestGeneration
    loading.value = true
    error.value = ''
    try {
      const response = await guestOrderAPI.list({
        email: email.value,
        order_password: orderPassword.value,
        page,
        page_size: pagination.value.page_size,
      })
      if (generation !== requestGeneration) return
      applyResponse(response)
    } catch (err: any) {
      if (generation !== requestGeneration) return
      orders.value = []
      error.value = err.message || t('guestOrders.errors.searchFailed')
    } finally {
      if (generation === requestGeneration) loading.value = false
    }
  }
  const debouncedLoadOrders = debounceAsync(loadCredentialOrders, 300)
  const searchByCredentials = async () => {
    // The submitted query owns the UI immediately, not when its debounce fires.
    resetResults()
    if (!email.value || !orderPassword.value) {
      error.value = t('guestOrders.errors.missing')
      return
    }
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
    loadSavedAuth()
    void loadBrowserOrders(1)
  })
  onUnmounted(invalidateRequests)

  return {
    activeTab, setActiveTab, savedAuth, email, orderPassword, loading, error, orders, pagination,
    hasSavedAuth, clearSaved, handleSearch, loadBrowserOrders, searchByCredentials,
    emptyMessage, changePage, statusLabel, statusVariant, statusPillClass, formatMoney,
    formatDiscountMoney, hasDiscountAmount, hasDiscount, formatDate,
  }
}
