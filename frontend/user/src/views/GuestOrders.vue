<template>
  <div class="min-h-screen bg-background px-4 pb-16 pt-24 text-foreground">
    <div class="mx-auto max-w-[960px]">
      <section class="rounded-2xl border bg-card p-5 shadow-sm sm:p-7">
        <div class="border-l-4 border-primary pl-4">
          <h1 class="flex items-center gap-2 text-2xl font-black sm:text-3xl"><ClipboardList class="h-7 w-7" />{{ t('guestOrders.title') }}</h1>
          <p class="mt-1 text-sm text-muted-foreground">{{ t('guestOrders.subtitle') }}</p>
        </div>
        <div class="mt-6 grid grid-cols-2 gap-1 rounded-xl bg-secondary p-1">
          <button v-for="tab in tabs" :key="tab.key" type="button" class="min-h-11 rounded-lg px-2 text-xs font-semibold transition sm:text-sm" :class="activeTab === tab.key ? 'bg-card text-primary shadow-sm' : 'text-muted-foreground'" @click="setActiveTab(tab.key)">{{ tab.label }}</button>
        </div>
        <div>
        <div v-if="activeTab === 'credentials'" class="mt-6 grid gap-3 sm:grid-cols-[1fr_1fr_auto]">
          <Input v-model="email" type="email" class="h-11" :placeholder="t('guestOrders.emailPlaceholder')" />
          <Input v-model="orderPassword" type="password" class="h-11" :placeholder="t('guestOrders.passwordPlaceholder')" />
          <Input v-model="orderNo" type="text" class="h-11 sm:col-span-3" :aria-label="t('guestOrders.orderNoPlaceholder')" :placeholder="t('guestOrders.orderNoPlaceholder')" />
          <div v-if="captchaEnabled" class="min-w-0 sm:col-span-3">
            <ImageCaptcha v-if="captchaProvider === 'image'" :ref="(el: any) => imageCaptchaRef = el" v-model="captchaPayload" :disabled="loading" @config-stale="handleCaptchaConfigStale" />
            <TurnstileCaptcha v-else-if="captchaProvider === 'turnstile'" :ref="(el: any) => turnstileRef = el" v-model="turnstileToken" :site-key="turnstileSiteKey" />
          </div>
          <Button class="h-11 sm:col-span-3" :disabled="loading" @click="searchByCredentials"><Search />{{ t('guestOrders.search') }}</Button>
        </div>
        <Alert v-if="error" variant="destructive" class="mt-4"><AlertDescription>{{ error }}</AlertDescription></Alert>
        <div v-if="orders.length === 0 && !loading" class="mt-6 flex min-h-52 flex-col items-center justify-center rounded-xl border border-dashed text-center text-muted-foreground">
          <ClipboardList class="mb-3 h-10 w-10 opacity-50" /><p class="font-semibold">{{ emptyMessage }}</p>
          <p v-if="activeTab === 'browser'" class="mt-1 text-xs">{{ t('guestOrders.browserEmptyHint') }}</p>
        </div>
        <div v-else class="mt-6 space-y-3">
          <div v-for="order in orders" :key="order.order_no" class="flex flex-col justify-between gap-4 rounded-xl border p-4 sm:flex-row sm:items-center">
            <div><div class="text-xs text-muted-foreground">{{ t('orders.orderNo') }}：{{ order.order_no }}</div><div class="mt-1 text-lg font-bold">{{ formatMoney(order.total_amount, order.currency) }}</div><div class="mt-1 text-xs text-muted-foreground">{{ formatDate(order.created_at) }}</div></div>
            <div class="flex items-center gap-2"><Badge :variant="statusVariant(order.status)">{{ statusLabel(order.status) }}</Badge><RouterLink :to="{ name: 'guest-order-detail', params: { order_no: order.order_no } }" :class="buttonVariants({ variant: 'outline', size: 'sm' })">{{ t('guestOrders.viewDetails') }}</RouterLink></div>
          </div>
        </div>
        <PaginationNav :current-page="pagination.page" :total-pages="pagination.total_page" :loading="loading" @change-page="changePage" />
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { ClipboardList, Search } from 'lucide-vue-next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button, buttonVariants } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import PaginationNav from '../components/PaginationNav.vue'
import { useGuestOrders } from '../composables/useGuestOrders'
import ImageCaptcha from '../components/captcha/ImageCaptcha.vue'
import TurnstileCaptcha from '../components/captcha/TurnstileCaptcha.vue'

const { t } = useI18n()
const tabs = computed(() => [
  { key: 'browser' as const, label: t('guestOrders.tabs.browser') },
  { key: 'credentials' as const, label: t('guestOrders.tabs.credentials') },
])
const { activeTab, setActiveTab, email, orderPassword, orderNo, loading, error, orders, pagination,
  captchaEnabled, captchaProvider, captchaPayload, turnstileToken, turnstileSiteKey, imageCaptchaRef, turnstileRef, handleCaptchaConfigStale,
  searchByCredentials, emptyMessage, changePage, statusLabel, statusVariant,
  formatMoney, formatDate } = useGuestOrders()
</script>
