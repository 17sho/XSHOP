import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import QRCode from 'qrcode'
import { walletAPI } from '../api/wallet'
import { useTelegramMiniAppStore } from '../stores/telegramMiniApp'
import { copyText } from '../utils/clipboard'
import { resolvePaymentLinkNavigationTarget, resolvePaymentPresentationMode, shouldAutoOpenPaymentLink } from '../utils/paymentResumePolicy'
import { hasActualCustomerSurcharge } from '../utils/customerFee'
import type { BadgeTone } from '../utils/status'

/**
 * 充值订单详情逻辑（classic + vault 共用，含二维码渲染与轮询）。
 */
export function useRechargeOrderDetail() {
  const { t } = useI18n()
  const route = useRoute()
  const telegramMiniAppStore = useTelegramMiniAppStore()

  const loading = ref(true)
  const checkingPayment = ref(false)
  const recharge = ref<any>(null)
  const payment = ref<any>(null)
  const pollTimer = ref<number | null>(null)
  const walletAddressCopied = ref(false)
  const walletAddressCopiedTimer = ref<number | null>(null)
  const qrImageUrl = ref('')
  const qrRenderVersion = ref(0)

  const rechargeNo = computed(() => String(route.params.recharge_no || '').trim())

  const isPending = computed(() => {
    const status = String(recharge.value?.status || '').toLowerCase()
    return status === 'pending' || status === 'initiated'
  })

  const payLink = computed(() => String(payment.value?.pay_url || '').trim())
  const interactionMode = computed(() => String(payment.value?.interaction_mode || '').trim().toLowerCase())
  const paymentPresentationMode = computed(() => resolvePaymentPresentationMode(interactionMode.value))
  const isTelegramMiniApp = computed(() => telegramMiniAppStore.isMiniApp && telegramMiniAppStore.isReady)
  const showTelegramPayHint = computed(() => isTelegramMiniApp.value && Boolean(payLink.value))

  const qrCodeContent = computed(() => String(payment.value?.qr_code || '').trim())
  const qrFallbackContent = computed(() => {
    if (paymentPresentationMode.value === 'redirect') return ''
    if (qrCodeContent.value) return ''
    return payLink.value
  })
  const qrDisplayContent = computed(() => qrCodeContent.value || qrFallbackContent.value)
  const qrUsingPayLinkFallback = computed(() => Boolean(!qrCodeContent.value && qrFallbackContent.value))
  const showQRCode = computed(() => paymentPresentationMode.value === 'qr' && Boolean(qrImageUrl.value))
  const cryptoWalletAddress = computed(() => String(payment.value?.wallet_address || '').trim())
  const cryptoChainAmount = computed(() => String(payment.value?.chain_amount || '').trim())
  const cryptoChain = computed(() => String(payment.value?.chain || '').trim())
  const cryptoTokenID = computed(() => String(payment.value?.token_id || '').trim())
  const cryptoTokenLabel = computed(() => {
    const tokenID = cryptoTokenID.value
    if (!tokenID) return ''
    const parts = tokenID.split('-').filter(Boolean)
    return String(parts[parts.length - 1] || tokenID).toUpperCase()
  })
  const cryptoTokenDetail = computed(() => {
    if (!cryptoTokenID.value) return ''
    return cryptoTokenID.value.toUpperCase() === cryptoTokenLabel.value ? '' : cryptoTokenID.value
  })
  const formatCryptoChain = (value: string) => {
    const normalized = value.trim().toLowerCase()
    const labels: Record<string, string> = {
      tron: 'TRON',
      trc20: 'TRON',
      base: 'Base',
      ethereum: 'Ethereum',
      eth: 'Ethereum',
      bsc: 'BNB Smart Chain',
      polygon: 'Polygon',
    }
    return labels[normalized] || value
  }
  const cryptoPaymentDetails = computed(() => {
    const details: Array<{ key: string; label: string; value: string; detail?: string }> = []
    if (cryptoTokenLabel.value) {
      details.push({ key: 'token', label: t('payment.cryptoToken'), value: cryptoTokenLabel.value, detail: cryptoTokenDetail.value })
    }
    if (cryptoChain.value) {
      details.push({ key: 'chain', label: t('payment.cryptoChain'), value: formatCryptoChain(cryptoChain.value) })
    }
    if (cryptoChainAmount.value) {
      details.push({ key: 'amount', label: t('payment.cryptoAmount'), value: cryptoChainAmount.value })
    }
    if (cryptoWalletAddress.value) {
      details.push({ key: 'wallet_address', label: t('payment.walletAddress'), value: cryptoWalletAddress.value })
    }
    return details
  })
  const hasCryptoPaymentDetails = computed(() => cryptoPaymentDetails.value.length > 0)
  const customerFeeApplied = computed(() => {
    return hasActualCustomerSurcharge({
      fee_policy: payment.value?.fee_policy,
      fee_amount: recharge.value?.fee_amount,
    })
  })

  const rechargeStatusText = (status?: string) => {
    const normalized = String(status || '').toLowerCase()
    const key = `personalCenter.wallet.rechargeStatus.${normalized}`
    const translated = t(key)
    if (translated === key) return normalized || '-'
    return translated
  }

  const rechargeStatusVariant = (status?: string): BadgeTone => {
    const normalized = String(status || '').toLowerCase()
    if (normalized === 'success') return 'success'
    if (normalized === 'failed' || normalized === 'expired') return 'danger'
    return 'warning'
  }

  // vault 模板：BadgeTone → vault pill 类名（classic 不使用）
  const rechargeStatusPillClass = (status?: string) => {
    const variant = rechargeStatusVariant(status)
    if (variant === 'success') return 'pill-done'
    if (variant === 'danger') return 'pill-sale'
    return 'pill-low'
  }

  const formatMoney = (amount?: string, currency?: string) => {
    if (amount === null || amount === undefined || amount === '') return '-'
    if (currency === null || currency === undefined || currency === '') return String(amount)
    return `${amount} ${currency}`
  }

  const formatDate = (raw?: string) => {
    if (!raw) return ''
    const date = new Date(raw)
    if (Number.isNaN(date.getTime())) return raw
    return date.toLocaleString()
  }

  const syncPayload = (payload: any) => {
    recharge.value = payload?.recharge || recharge.value
    const paymentData = payload?.payment || (payload?.payment_id != null ? {
      id: payload.payment_id,
      provider_type: payload.provider_type,
      channel_type: payload.channel_type,
      interaction_mode: payload.interaction_mode,
      pay_url: payload.pay_url,
      qr_code: payload.qr_code,
      expires_at: payload.expires_at,
      status: payload.status,
      fee_policy: payload.fee_policy,
    } : undefined)
    if (paymentData) {
      payment.value = paymentData
    }
  }

  let disposed = false
  let generation = 0
  let readVersion = 0
  const context = () => {
    const version = generation
    const no = rechargeNo.value
    return () => !disposed && version === generation && no === rechargeNo.value
  }
  const loadDetail = async () => {
    if (disposed || !rechargeNo.value) return
    const current = context()
    const request = ++readVersion
    loading.value = true
    try {
      const response = await walletAPI.rechargeDetail(rechargeNo.value)
      if (!current() || request !== readVersion) return
      const payload = response.data.data || {}
      syncPayload(payload)
    } catch {
      if (!current() || request !== readVersion) return
      recharge.value = null
    } finally {
      if (current() && request === readVersion) loading.value = false
    }
  }

  const refreshStatus = async (silent = false) => {
    if (disposed || !rechargeNo.value) return
    const current = context()
    const request = ++readVersion
    try {
      const response = await walletAPI.rechargeDetail(rechargeNo.value)
      if (!current() || request !== readVersion) return
      const payload = response.data.data || {}
      syncPayload(payload)

      const status = String(recharge.value?.status || '').toLowerCase()
      if (status === 'success' || status === 'failed' || status === 'expired') {
        stopPolling()
      } else {
        startPolling()
      }
    } catch (err: any) {
      if (current() && request === readVersion && !silent) {
        console.error('Failed to refresh recharge status:', err)
      }
    } finally {
      if (current() && request === readVersion) loading.value = false
    }
  }

  const captures = new Set<number>()
  const checkPayment = async () => {
    if (disposed) return
    const current = context()
    const paymentID = Number(payment.value?.id || payment.value?.payment_id || 0)
    if (!Number.isFinite(paymentID) || paymentID <= 0) return
    if (captures.has(paymentID)) return
    captures.add(paymentID)
    checkingPayment.value = true
    try {
      const response = await walletAPI.captureRechargePayment(paymentID)
      if (!current()) return
      const payload = response.data.data || {}
      syncPayload(payload)
      await refreshStatus(true)
    } catch (err: any) {
      if (current()) console.error('Failed to check payment:', err)
    } finally {
      captures.delete(paymentID)
      if (current()) checkingPayment.value = false
    }
  }

  const startPolling = () => {
    if (disposed || !isPending.value || pollTimer.value) return
    const current = context()
    pollTimer.value = window.setInterval(() => {
      if (!current()) return
      void refreshStatus(true)
    }, 5000)
  }

  const stopPolling = () => {
    if (pollTimer.value) {
      clearInterval(pollTimer.value)
      pollTimer.value = null
    }
  }

  const openPayLinkInCompatibleWindow = (automatic: boolean) => {
    if (!payLink.value) return
    if (isTelegramMiniApp.value) {
      try {
        window.Telegram?.WebApp?.openLink?.(payLink.value)
      } catch {
        window.open(payLink.value, '_blank')
      }
    } else if (resolvePaymentLinkNavigationTarget(automatic) === 'current-tab') {
      window.location.assign(payLink.value)
    } else {
      window.open(payLink.value, '_blank')
    }
  }

  const handleOpenPayLink = () => {
    openPayLinkInCompatibleWindow(false)
  }

  let walletCopyVersion = 0
  const clearCopyFeedback = () => {
    ++walletCopyVersion
    walletAddressCopied.value = false
    if (walletAddressCopiedTimer.value) window.clearTimeout(walletAddressCopiedTimer.value)
    walletAddressCopiedTimer.value = null
  }

  const handleCopyWalletAddress = async () => {
    if (disposed || !cryptoWalletAddress.value) return
    const active = context()
    const version = ++walletCopyVersion
    const current = () => active() && version === walletCopyVersion
    walletAddressCopied.value = false
    if (walletAddressCopiedTimer.value) window.clearTimeout(walletAddressCopiedTimer.value)
    walletAddressCopiedTimer.value = null
    try {
      await copyText(cryptoWalletAddress.value)
      if (!current()) return
      walletAddressCopied.value = true
      if (walletAddressCopiedTimer.value) {
        window.clearTimeout(walletAddressCopiedTimer.value)
      }
      walletAddressCopiedTimer.value = window.setTimeout(() => {
        if (!current()) return
        walletAddressCopied.value = false
        walletAddressCopiedTimer.value = null
      }, 1500)
    } catch {
      if (current()) window.alert(t('payment.copyFailed'))
    }
  }

  const renderQRCodeImage = async () => {
    const qr = qrDisplayContent.value
    const currentVersion = qrRenderVersion.value + 1
    qrRenderVersion.value = currentVersion
    if (!qr) {
      qrImageUrl.value = ''
      return
    }
    if (qr.startsWith('data:image/')) {
      qrImageUrl.value = qr
      return
    }
    const isImageURL = /^https?:\/\/.+\.(png|jpe?g|gif|webp|svg)(\?.*)?$/i.test(qr)
    if (isImageURL) {
      qrImageUrl.value = qr
      return
    }
    try {
      const dataURL = await QRCode.toDataURL(qr, {
        width: 240,
        margin: 1,
        errorCorrectionLevel: 'M',
      })
      if (currentVersion !== qrRenderVersion.value) return
      qrImageUrl.value = dataURL
    } catch {
      if (currentVersion !== qrRenderVersion.value) return
      qrImageUrl.value = ''
    }
  }

  watch(() => qrDisplayContent.value, () => { void renderQRCodeImage() }, { immediate: true })

  const enterDetail = async () => {
    const current = context()
    await loadDetail()
    if (!current()) return
    if (isPending.value) {
      startPolling()
      // 自动跳转类支付使用当前标签页，避免异步加载后被浏览器拦截为弹窗。
      if (shouldAutoOpenPaymentLink(payment.value)) {
        openPayLinkInCompatibleWindow(true)
      }
    }
  }

  onMounted(() => { void enterDetail() })

  watch(rechargeNo, () => {
    ++generation
    ++readVersion
    ++qrRenderVersion.value
    stopPolling()
    recharge.value = null
    payment.value = null
    qrImageUrl.value = ''
    checkingPayment.value = false
    clearCopyFeedback()
    loading.value = false
    void enterDetail()
  }, { flush: 'sync' })

  onUnmounted(() => {
    disposed = true
    ++generation
    ++qrRenderVersion.value
    stopPolling()
    clearCopyFeedback()
  })

  return {
    loading,
    checkingPayment,
    recharge,
    payment,
    walletAddressCopied,
    qrImageUrl,
    isPending,
    payLink,
    showTelegramPayHint,
    qrUsingPayLinkFallback,
    showQRCode,
    cryptoWalletAddress,
    cryptoPaymentDetails,
    hasCryptoPaymentDetails,
    customerFeeApplied,
    rechargeStatusText,
    rechargeStatusVariant,
    rechargeStatusPillClass,
    formatMoney,
    formatDate,
    loadDetail,
    checkPayment,
    handleOpenPayLink,
    handleCopyWalletAddress,
  }
}
