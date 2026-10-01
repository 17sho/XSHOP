<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import { Switch } from '@/components/ui/switch'
import { notifyError, notifySuccess } from '@/utils/notify'

const { t } = useI18n()
const submitting = ref(false)
const loaded = ref(false)
const loadFailed = ref(false)
const keys = ['overview', 'orders', 'wallet', 'affiliate', 'reseller', 'gift_cards', 'security', 'api', 'profile'] as const
const form = reactive<Record<(typeof keys)[number], boolean>>({
  overview: true, orders: true, wallet: true, affiliate: true, reseller: true, gift_cards: true, security: true, api: true, profile: true,
})

const load = async () => {
  try {
    const response = await adminAPI.getSettings({ key: 'personal_center_visibility_config' })
    const data = response.data?.data as Record<string, unknown> | undefined
    keys.forEach((key) => { form[key] = typeof data?.[key] === 'boolean' ? data[key] as boolean : true })
  } catch {
    loadFailed.value = true
    notifyError(t('admin.settings.personalCenterVisibility.loadFailed'))
  } finally { loaded.value = true }
}

const save = async () => {
  if (!loaded.value || loadFailed.value) return
  submitting.value = true
  try {
    await adminAPI.updateSettings({ key: 'personal_center_visibility_config', value: { ...form } })
    notifySuccess(t('admin.settings.alerts.saveSuccess'))
  } catch {
    notifyError(t('admin.settings.alerts.saveFailed'))
  } finally { submitting.value = false }
}

onMounted(load)
defineExpose({ save, submitting, loaded, loadFailed })
</script>

<template>
  <section v-if="loaded" class="overflow-hidden rounded-2xl border border-border bg-card shadow-sm">
    <div class="border-b border-border bg-muted/30 px-5 py-4">
      <h3 class="font-semibold">{{ t('admin.settings.personalCenterVisibility.title') }}</h3>
      <p class="mt-1 text-xs text-muted-foreground">{{ t('admin.settings.personalCenterVisibility.subtitle') }}</p>
    </div>
    <div v-if="loadFailed" class="p-5 text-sm text-destructive">{{ t('admin.settings.personalCenterVisibility.loadFailed') }}</div>
    <div v-else class="divide-y divide-border">
      <label v-for="key in keys" :key="key" :for="`personal-center-${key}`" class="flex min-h-16 cursor-pointer items-center justify-between gap-4 px-5 py-3">
        <span>
          <span class="block text-sm font-medium">{{ t(`admin.settings.personalCenterVisibility.sections.${key}`) }}</span>
          <span class="mt-0.5 block text-xs text-muted-foreground">{{ t(form[key] ? 'admin.settings.personalCenterVisibility.visible' : 'admin.settings.personalCenterVisibility.hidden') }}</span>
        </span>
        <Switch :id="`personal-center-${key}`" v-model="form[key]" />
      </label>
    </div>
  </section>
</template>
