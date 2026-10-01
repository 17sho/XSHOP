import { onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { userOrderAPI } from '../api'
import { debounceAsync } from '../utils/debounce'
import { useConfirmDialog } from './useConfirmDialog'
import { toast } from './useToast'
import { useOrderDisplayHelpers } from './useOrderDisplayHelpers'

/**
 * 已登录用户订单详情逻辑（classic + vault 共用）。
 */
export function useOrderDetail() {
  const route = useRoute()
  const router = useRouter()
  const { confirm: showConfirm } = useConfirmDialog()
  const { t } = useI18n()

  const loading = ref(true)
  const order = ref<any>(null)
  const fulfillmentDownloading = ref(false)

  const helpers = useOrderDisplayHelpers(order)

  const handleDownloadFulfillment = async (orderNo: string) => {
    if (disposed || fulfillmentDownloading.value) return
    const version = ++downloadVersion
    const no = route.params.order_no
    const ownsDownload = () => version === downloadVersion
    const current = () => !disposed && ownsDownload() && no === route.params.order_no
    fulfillmentDownloading.value = true
    try {
      const res = await userOrderAPI.downloadFulfillment(orderNo)
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

  let downloadVersion = 0
  let disposed = false
  let requestVersion = 0
  const operationContext = () => {
    const version = requestVersion
    const no = route.params.order_no
    return () => !disposed && version === requestVersion && no === route.params.order_no
  }
  const loadOrder = async () => {
    if (disposed) return
    const version = ++requestVersion
    const orderNo = String(route.params.order_no || '').trim()
    const current = () => !disposed && version === requestVersion && orderNo === String(route.params.order_no || '').trim()
    loading.value = true
    try {
      const response = await userOrderAPI.detail(orderNo)
      if (!current()) return
      order.value = response.data.data
    } catch (error) {
      if (!current()) return
      order.value = null
    } finally {
      if (current()) loading.value = false
    }
  }

  const debouncedLoadOrder = debounceAsync(loadOrder, 300)

  const cancelOrder = async () => {
    if (disposed) return
    const current = operationContext()
    if (!order.value) return
    const confirmed = await showConfirm({
      title: t('orderDetail.cancel'),
      message: t('orderDetail.cancelConfirm'),
      confirmText: t('common.confirm'),
      cancelText: t('common.cancel'),
      variant: 'danger',
    })
    if (!confirmed || !current()) return
    try {
      await userOrderAPI.cancel(order.value.order_no)
      if (current()) await debouncedLoadOrder()
    } catch {
      if (current()) toast.error(t('orderDetail.cancelFailed'))
    }
  }

  onMounted(() => {
    if (!route.params.order_no) {
      router.push('/me/orders')
      return
    }
    loadOrder()
  })

  watch(() => route.params.order_no, () => {
    ++downloadVersion
    ++requestVersion
    debouncedLoadOrder.cancel()
    order.value = null
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
    debouncedLoadOrder,
    cancelOrder,
    fulfillmentDownloading,
    handleDownloadFulfillment,
    ...helpers,
  }
}
