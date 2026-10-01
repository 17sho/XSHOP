import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { guestOrderAPI } from '../api'
import { debounceAsync } from '../utils/debounce'
import { clearGuestOrderAuth, loadGuestOrderAuth, saveGuestOrderAuth } from '../utils/guestOrderAuth'
import { resolveGuestOrderDetailViewState } from '../utils/guestOrderDetailState'
import { useOrderDisplayHelpers } from './useOrderDisplayHelpers'
import { useConfirmDialog } from './useConfirmDialog'
import { toast } from './useToast'

/**
 * 游客订单详情逻辑（classic + vault 共用）。
 */
export function useGuestOrderDetail() {
  const route = useRoute()
  const router = useRouter()
  const { t } = useI18n()
  const { confirm: showConfirm } = useConfirmDialog()

  const loading = ref(true)
  const order = ref<any>(null)
  const authError = ref('')
  const auth = ref({
    email: '',
    order_password: '',
  })
  const fulfillmentDownloading = ref(false)

  const helpers = useOrderDisplayHelpers(order)

  const handleDownloadFulfillment = async (orderNo: string) => {
    if (disposed || fulfillmentDownloading.value) return
    const version = ++downloadVersion
    const no = route.params.order_no
    const credentials = JSON.stringify(auth.value)
    const ownsDownload = () => version === downloadVersion
    const current = () => !disposed && ownsDownload() && no === route.params.order_no && credentials === JSON.stringify(auth.value)
    fulfillmentDownloading.value = true
    try {
      const res = await guestOrderAPI.downloadFulfillment(orderNo, {
        email: auth.value.email,
        order_password: auth.value.order_password,
      })
      if (!current()) return
      const blob = new Blob([res.data], { type: 'text/plain; charset=utf-8' })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `fulfillment-${orderNo}.txt`
      a.click()
      URL.revokeObjectURL(url)
    } catch {} finally {
      if (ownsDownload()) fulfillmentDownloading.value = false
    }
  }

  const loadSavedAuth = () => {
    auth.value = loadGuestOrderAuth()
  }

  const hasAuth = computed(() => Boolean(auth.value.email && auth.value.order_password))
  const showAuthForm = computed(() => !hasAuth.value || authError.value !== '')
  const viewState = computed(() => resolveGuestOrderDetailViewState({
    loading: loading.value,
    order: order.value,
    showAuthForm: showAuthForm.value,
  }))

  let downloadVersion = 0
  let disposed = false
  let requestVersion = 0
  const operationContext = () => {
    const version = requestVersion
    const no = route.params.order_no
    const credentials = JSON.stringify(auth.value)
    return () => !disposed && version === requestVersion && no === route.params.order_no && credentials === JSON.stringify(auth.value)
  }
  const loadOrder = async () => {
    if (disposed) return
    const version = ++requestVersion
    const orderNo = String(route.params.order_no || '').trim()
    const current = () => !disposed && version === requestVersion && orderNo === String(route.params.order_no || '').trim()
    loading.value = true
    try {
      if (!hasAuth.value) {
        order.value = null
        authError.value = t('guestOrderDetail.authRequired')
        return
      }
      const response = await guestOrderAPI.detail(orderNo, {
        email: auth.value.email,
        order_password: auth.value.order_password,
      })
      if (!current()) return
      order.value = response.data.data
      authError.value = ''
    } catch (error) {
      if (!current()) return
      order.value = null
      authError.value = t('guestOrderDetail.authInvalid')
    } finally {
      if (current()) loading.value = false
    }
  }

  const debouncedLoadOrder = debounceAsync(loadOrder, 300)

  const persistAuth = () => {
    saveGuestOrderAuth({
      email: auth.value.email,
      order_password: auth.value.order_password,
    })
  }

  const handleAuthSubmit = async () => {
    authError.value = ''
    if (!hasAuth.value) {
      authError.value = t('guestOrderDetail.authRequired')
      return
    }
    persistAuth()
    loading.value = true
    await debouncedLoadOrder()
  }

  const clearAuth = () => {
    ++downloadVersion
    ++requestVersion
    debouncedLoadOrder.cancel()
    loading.value = false
    fulfillmentDownloading.value = false
    clearGuestOrderAuth()
    auth.value = { email: '', order_password: '' }
    order.value = null
    authError.value = t('guestOrderDetail.authRequired')
  }

  const cancelOrder = async () => {
    if (disposed) return
    const current = operationContext()
    if (!order.value || order.value.status !== 'pending_payment') return
    const confirmed = await showConfirm({
      title: t('orderDetail.cancel'),
      message: t('orderDetail.cancelConfirm'),
      confirmText: t('common.confirm'),
      cancelText: t('common.cancel'),
      variant: 'danger',
    })
    if (!confirmed || !current()) return
    try {
      await guestOrderAPI.cancel(order.value.order_no, auth.value)
      if (current()) await debouncedLoadOrder()
    } catch {
      if (current()) toast.error(t('orderDetail.cancelFailed'))
    }
  }

  onMounted(() => {
    if (!route.params.order_no) {
      router.push('/guest/orders')
      return
    }
    loadSavedAuth()
    loadOrder()
  })

  watch(() => route.params.order_no, () => {
    ++downloadVersion
    ++requestVersion
    debouncedLoadOrder.cancel()
    order.value = null
    authError.value = ''
    fulfillmentDownloading.value = false
    if (route.params.order_no) void loadOrder()
    else loading.value = false
  }, { flush: 'sync' })

  onUnmounted(() => {
    disposed = true
    ++downloadVersion
    fulfillmentDownloading.value = false
    ++requestVersion
    debouncedLoadOrder.cancel()
  })

  return {
    loading,
    order,
    authError,
    auth,
    showAuthForm,
    viewState,
    handleAuthSubmit,
    clearAuth,
    cancelOrder,
    fulfillmentDownloading,
    handleDownloadFulfillment,
    ...helpers,
  }
}
