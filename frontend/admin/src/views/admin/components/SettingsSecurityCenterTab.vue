<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import { Switch } from '@/components/ui/switch'
import { notifyError, notifySuccess } from '@/utils/notify'
import { normalizeSecurityCenterSettings, securityCenterKeys as keys, type SecurityCenterConfig, type SecurityCenterSettingsPayload } from '@/utils/securityCenterConfig'

const { t } = useI18n()
const submitting = ref(false)
const loaded = ref(false)
const loadFailed = ref(false)
const ready = computed(() => loaded.value && !loadFailed.value && !submitting.value)
const form = reactive<SecurityCenterConfig>(normalizeSecurityCenterSettings({}))
let original: Record<string, unknown> = {}
let disposed = false

const load = async () => {
  try {
    const response = await adminAPI.getSettings({ key: 'security_center_config' })
    if (disposed) return
    const raw = response.data?.data
    if (raw != null && (typeof raw !== 'object' || Array.isArray(raw))) throw new Error('Invalid security center configuration')
    original = { ...(raw || {}) }
    Object.assign(form, normalizeSecurityCenterSettings(original))
  } catch {
    if (disposed) return
    loadFailed.value = true
    notifyError(t('admin.settings.securityCenter.loadFailed'))
  } finally { if (!disposed) loaded.value = true }
}
const save = async () => {
  if (disposed || !ready.value) return
  submitting.value = true
  const payload: SecurityCenterSettingsPayload = { key: 'security_center_config', value: { ...original, ...form } }
  try {
    await adminAPI.updateSettings(payload)
    if (disposed) return
    original = { ...payload.value }
    notifySuccess(t('admin.settings.alerts.saveSuccess'))
  } catch {
    if (!disposed) notifyError(t('admin.settings.alerts.saveFailed'))
  } finally { if (!disposed) submitting.value = false }
}
onMounted(load)
onBeforeUnmount(() => { disposed = true })
defineExpose({ save, submitting, loaded, loadFailed, ready })
</script>

<template>
  <section class="overflow-hidden rounded-2xl border border-border bg-card shadow-sm">
    <div class="border-b border-border bg-muted/30 px-5 py-4">
      <h3 class="font-semibold">{{ t('admin.settings.securityCenter.title') }}</h3>
      <p class="mt-1 text-xs text-muted-foreground">{{ t('admin.settings.securityCenter.subtitle') }}</p>
      <p class="mt-2 text-xs text-muted-foreground">{{ t('admin.settings.securityCenter.providerHint') }}</p>
    </div>
    <div v-if="loadFailed" class="p-5 text-sm text-destructive">{{ t('admin.settings.securityCenter.loadFailed') }}</div>
    <div v-else class="divide-y divide-border">
      <label v-for="key in keys" :key="key" :for="`security-center-${key}`" class="flex min-h-16 items-center justify-between gap-4 px-5 py-3">
        <span>
          <span class="block text-sm font-medium">{{ t(`admin.settings.securityCenter.sections.${key}`) }}</span>
          <span class="mt-0.5 block text-xs text-muted-foreground">{{ t(`admin.settings.securityCenter.hints.${key}`) }}</span>
        </span>
        <Switch :id="`security-center-${key}`" v-model="form[key]" :disabled="!ready" />
      </label>
    </div>
  </section>
</template>
