<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { notifyError, notifySuccess } from '@/utils/notify'

const { t } = useI18n()

const MAX_CUSTOM_ITEMS = 10
const supportedLanguages = ['zh-CN', 'zh-TW', 'en-US'] as const
type SupportedLanguage = (typeof supportedLanguages)[number]

const presetIcons = [
  { key: 'link', label: 'Link', path: 'M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1' },
  { key: 'document', label: 'Document', path: 'M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z' },
  { key: 'globe', label: 'Globe', path: 'M21 12a9 9 0 01-9 9m9-9a9 9 0 00-9-9m9 9H3m9 9a9 9 0 01-9-9m9 9c1.657 0 3-4.03 3-9s-1.343-9-3-9m0 18c-1.657 0-3-4.03-3-9s1.343-9 3-9' },
  { key: 'star', label: 'Star', path: 'M11.049 2.927c.3-.921 1.603-.921 1.902 0l1.519 4.674a1 1 0 00.95.69h4.915c.969 0 1.371 1.24.588 1.81l-3.976 2.888a1 1 0 00-.363 1.118l1.518 4.674c.3.922-.755 1.688-1.538 1.118l-3.976-2.888a1 1 0 00-1.176 0l-3.976 2.888c-.783.57-1.838-.197-1.538-1.118l1.518-4.674a1 1 0 00-.363-1.118l-3.976-2.888c-.784-.57-.38-1.81.588-1.81h4.914a1 1 0 00.951-.69l1.519-4.674z' },
  { key: 'heart', label: 'Heart', path: 'M4.318 6.318a4.5 4.5 0 000 6.364L12 20.364l7.682-7.682a4.5 4.5 0 00-6.364-6.364L12 7.636l-1.318-1.318a4.5 4.5 0 00-6.364 0z' },
  { key: 'chat', label: 'Chat', path: 'M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z' },
  { key: 'gift', label: 'Gift', path: 'M12 8v13m0-13V6a4 4 0 00-4-4 4 4 0 00-4 4v2h8zm0 0V6a4 4 0 014-4 4 4 0 014 4v2h-8zM5 8h14a1 1 0 011 1v3H4V9a1 1 0 011-1zm0 4h14v7a2 2 0 01-2 2H7a2 2 0 01-2-2v-7z' },
  { key: 'lightning', label: 'Lightning', path: 'M13 10V3L4 14h7v7l9-11h-7z' },
  { key: 'shield', label: 'Shield', path: 'M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z' },
  { key: 'book', label: 'Book', path: 'M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253' },
  { key: 'code', label: 'Code', path: 'M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4' },
  { key: 'phone', label: 'Phone', path: 'M3 5a2 2 0 012-2h3.28a1 1 0 01.948.684l1.498 4.493a1 1 0 01-.502 1.21l-2.257 1.13a11.042 11.042 0 005.516 5.516l1.13-2.257a1 1 0 011.21-.502l4.493 1.498a1 1 0 01.684.949V19a2 2 0 01-2 2h-1C9.716 21 3 14.284 3 6V5z' },
  { key: 'map', label: 'Map', path: 'M17.657 16.657L13.414 20.9a1.998 1.998 0 01-2.827 0l-4.244-4.243a8 8 0 1111.314 0z M15 11a3 3 0 11-6 0 3 3 0 016 0z' },
  { key: 'music', label: 'Music', path: 'M9 19V6l12-3v13M9 19c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zm12-3c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zM9 10l12-3' },
  { key: 'camera', label: 'Camera', path: 'M3 9a2 2 0 012-2h.93a2 2 0 001.664-.89l.812-1.22A2 2 0 0110.07 4h3.86a2 2 0 011.664.89l.812 1.22A2 2 0 0018.07 7H19a2 2 0 012 2v9a2 2 0 01-2 2H5a2 2 0 01-2-2V9z M15 13a3 3 0 11-6 0 3 3 0 016 0z' },
]

interface CustomNavItem {
  id: number
  title: Record<SupportedLanguage, string>
  link_type: 'internal' | 'external'
  url: string
  target: '_self' | '_blank'
  sort_order: number
  enabled: boolean
  icon: string
}

const props = defineProps<{
  currentLang: SupportedLanguage
}>()

const emit = defineEmits<{
  saved: []
}>()

const submitting = ref(false)
const loaded = ref(false)
const loadFailed = ref(false)

const form = reactive({
  homepageNoticeEnabled: true,
  builtin: {
    notice: true,
  },
  customItems: [] as CustomNavItem[],
})

const createEmptyItem = (): CustomNavItem => ({
  id: Date.now(),
  title: { 'zh-CN': '', 'zh-TW': '', 'en-US': '' },
  link_type: 'internal',
  url: '',
  target: '_self',
  sort_order: 0,
  enabled: true,
  icon: 'link',
})

const addItem = () => {
  if (form.customItems.length >= MAX_CUSTOM_ITEMS) return
  form.customItems.push(createEmptyItem())
}

const removeItem = (index: number) => {
  form.customItems.splice(index, 1)
}

const fetchNavConfig = async () => {
  loaded.value = false
  loadFailed.value = false
  try {
    const res = await adminAPI.getSettings({ key: 'nav_config' })
    const data = res.data?.data
    if (!data || typeof data !== 'object' || Array.isArray(data)) throw new Error('Invalid settings response')
    if (data && typeof data === 'object') {
      const d = data as Record<string, unknown>
      form.homepageNoticeEnabled = d.homepage_notice_enabled !== false
      if (d.builtin && typeof d.builtin === 'object') {
        const b = d.builtin as Record<string, boolean>
        form.builtin.notice = b.notice !== false
      }
      if (Array.isArray(d.custom_items)) {
        form.customItems = (d.custom_items as Array<Record<string, unknown>>).map((item) => ({
          id: (item.id as number) || Date.now(),
          title: {
            'zh-CN': ((item.title as Record<string, string>)?.['zh-CN']) || '',
            'zh-TW': ((item.title as Record<string, string>)?.['zh-TW']) || '',
            'en-US': ((item.title as Record<string, string>)?.['en-US']) || '',
          },
          link_type: item.link_type === 'external' ? 'external' : 'internal',
          url: (item.url as string) || '',
          target: item.target === '_blank' ? '_blank' : '_self',
          sort_order: (item.sort_order as number) || 0,
          enabled: item.enabled !== false,
          icon: (item.icon as string) || 'link',
        }))
      }
    }
    loaded.value = true
  } catch {
    loadFailed.value = true
    notifyError(t('admin.settings.alerts.loadFailed'))
  }
}

const save = async () => {
  if (!loaded.value || loadFailed.value || submitting.value) return
  submitting.value = true
  try {
    const payload = {
      key: 'nav_config',
      value: {
        homepage_notice_enabled: form.homepageNoticeEnabled,
        builtin: { ...form.builtin },
        custom_items: form.customItems.map((item) => ({
          id: item.id,
          title: { ...item.title },
          link_type: item.link_type,
          url: item.url,
          target: item.target,
          sort_order: item.sort_order,
          enabled: item.enabled,
          icon: item.icon,
        })),
      },
    }
    await adminAPI.updateSettings(payload)
    notifySuccess(t('admin.settings.alerts.saveSuccess'))
    emit('saved')
  } catch {
    notifyError(t('admin.settings.alerts.saveFailed'))
  } finally {
    submitting.value = false
  }
}

onMounted(() => {
  fetchNavConfig()
})

defineExpose({ save, submitting, loaded, loadFailed })
</script>

<template>
  <div v-if="loadFailed" role="alert" class="rounded-lg border border-destructive/40 p-3 text-sm">
    {{ t('admin.settings.alerts.loadFailed') }}
    <button type="button" class="ml-3 underline" @click="fetchNavConfig">{{ t('admin.common.retry') }}</button>
  </div>
  <div v-if="loaded" class="navigation-settings-content space-y-5 md:space-y-6">

    <section class="overflow-hidden rounded-2xl border border-border bg-card shadow-sm">
      <div class="border-b border-border bg-muted/30 px-4 py-4 sm:px-5">
        <h3 class="font-semibold">{{ t('admin.settings.navigation.builtin.title') }}</h3>
        <p class="mt-1 text-xs text-muted-foreground">{{ t('admin.settings.navigation.builtin.subtitle') }}</p>
      </div>
      <div class="divide-y divide-border">
        <label v-for="key in (['notice'] as const)" :key="key" :for="`nav-${key}`" class="flex min-h-16 cursor-pointer items-center justify-between gap-4 px-4 py-3 sm:px-5">
          <span class="min-w-0">
            <span class="block text-sm font-medium">{{ t(`admin.settings.navigation.builtin.${key}`) }}</span>
            <span class="mt-0.5 block text-xs text-muted-foreground">{{ t('admin.settings.navigation.builtin.noticeHint') }}</span>
            <span class="mt-0.5 block text-xs text-muted-foreground">{{ t(`admin.settings.navigation.builtin.${form.builtin[key] ? 'visible' : 'hidden'}`) }}</span>
          </span>
          <Switch :id="`nav-${key}`" v-model="form.builtin[key]" />
        </label>
        <label for="homepage-notice" class="flex min-h-16 cursor-pointer items-center justify-between gap-4 px-4 py-3 sm:px-5">
          <span class="min-w-0">
            <span class="block text-sm font-medium">{{ t('admin.settings.navigation.builtin.homepageNotice') }}</span>
            <span class="mt-0.5 block text-xs text-muted-foreground">{{ t('admin.settings.navigation.builtin.homepageNoticeHint') }}</span>
            <span class="mt-0.5 block text-xs text-muted-foreground">{{ t(`admin.settings.navigation.builtin.${form.homepageNoticeEnabled ? 'visible' : 'hidden'}`) }}</span>
          </span>
          <Switch id="homepage-notice" v-model="form.homepageNoticeEnabled" />
        </label>
      </div>
    </section>

    <section class="overflow-hidden rounded-2xl border border-border bg-card shadow-sm">
      <div class="border-b border-border bg-muted/30 px-4 py-4 sm:px-5">
        <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <div class="flex items-center gap-2">
              <h3 class="font-semibold">{{ t('admin.settings.navigation.custom.title') }}</h3>
              <span class="rounded-full bg-secondary px-2 py-0.5 text-xs font-semibold">{{ form.customItems.length }} / {{ MAX_CUSTOM_ITEMS }}</span>
            </div>
            <p class="mt-1 text-xs text-muted-foreground">{{ t('admin.settings.navigation.custom.subtitle', { max: MAX_CUSTOM_ITEMS }) }}</p>
          </div>
          <button type="button" class="min-h-10 rounded-lg bg-primary px-4 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50" :disabled="form.customItems.length >= MAX_CUSTOM_ITEMS" @click="addItem">
            {{ form.customItems.length >= MAX_CUSTOM_ITEMS ? t('admin.settings.navigation.custom.maxReached') : t('admin.settings.navigation.custom.add') }}
          </button>
        </div>
      </div>

      <div v-if="form.customItems.length === 0" class="flex flex-col items-center px-5 py-10 text-center">
        <span class="grid h-12 w-12 place-items-center rounded-full bg-secondary text-xl">＋</span>
        <p class="mt-3 text-sm font-medium">{{ t('admin.settings.navigation.custom.empty') }}</p>
        <p class="mt-1 max-w-sm text-xs leading-5 text-muted-foreground">{{ t('admin.settings.navigation.custom.emptyHint') }}</p>
        <button type="button" class="mt-4 min-h-10 rounded-lg border border-border bg-background px-4 text-sm font-medium" @click="addItem">{{ t('admin.settings.navigation.custom.addFirst') }}</button>
      </div>

      <div v-else class="divide-y divide-border">
        <article v-for="(item, index) in form.customItems" :key="item.id" class="space-y-4 px-4 py-5 sm:px-5">
          <div class="flex items-center justify-between gap-3">
            <span class="rounded-full bg-secondary px-2.5 py-1 text-xs font-semibold">#{{ index + 1 }}</span>
            <div class="flex items-center gap-3">
              <label :for="`custom-nav-${item.id}`" class="flex min-h-10 cursor-pointer items-center gap-2 text-xs">
                <span>{{ t('admin.settings.navigation.custom.fields.enabled') }}</span>
                <Switch :id="`custom-nav-${item.id}`" v-model="item.enabled" />
              </label>
              <button type="button" class="min-h-10 px-2 text-xs font-medium text-destructive hover:underline" @click="removeItem(index)">{{ t('admin.settings.navigation.custom.delete') }}</button>
            </div>
          </div>

          <div>
            <label class="mb-2 block text-xs font-medium text-muted-foreground">{{ t('admin.settings.navigation.custom.fields.icon') }}</label>
            <div class="flex flex-wrap gap-2">
              <button v-for="preset in presetIcons" :key="preset.key" type="button" class="flex h-10 w-10 items-center justify-center rounded-lg border transition-colors" :class="item.icon === preset.key ? 'border-primary bg-primary/10 text-primary' : 'border-input bg-background text-muted-foreground hover:border-primary/50'" :title="preset.label" @click="item.icon = preset.key">
                <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.75" :d="preset.path" /></svg>
              </button>
            </div>
          </div>

          <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div>
              <label class="mb-1.5 block text-xs font-medium text-muted-foreground">{{ t('admin.settings.navigation.custom.fields.title') }} ({{ props.currentLang }})</label>
              <Input v-model="item.title[props.currentLang]" :placeholder="t('admin.settings.navigation.custom.fields.title')" />
            </div>
            <div>
              <label class="mb-1.5 block text-xs font-medium text-muted-foreground">{{ t('admin.settings.navigation.custom.fields.linkType') }}</label>
              <Select v-model="item.link_type"><SelectTrigger class="h-10"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="internal">{{ t('admin.settings.navigation.custom.fields.linkTypeInternal') }}</SelectItem><SelectItem value="external">{{ t('admin.settings.navigation.custom.fields.linkTypeExternal') }}</SelectItem></SelectContent></Select>
            </div>
            <div>
              <label class="mb-1.5 block text-xs font-medium text-muted-foreground">{{ t('admin.settings.navigation.custom.fields.url') }}</label>
              <Input v-model="item.url" :placeholder="item.link_type === 'internal' ? t('admin.settings.navigation.custom.fields.urlPlaceholderInternal') : t('admin.settings.navigation.custom.fields.urlPlaceholderExternal')" />
            </div>
            <div>
              <label class="mb-1.5 block text-xs font-medium text-muted-foreground">{{ t('admin.settings.navigation.custom.fields.target') }}</label>
              <Select v-model="item.target"><SelectTrigger class="h-10"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="_self">{{ t('admin.settings.navigation.custom.fields.targetSelf') }}</SelectItem><SelectItem value="_blank">{{ t('admin.settings.navigation.custom.fields.targetBlank') }}</SelectItem></SelectContent></Select>
            </div>
            <div>
              <label class="mb-1.5 block text-xs font-medium text-muted-foreground">{{ t('admin.settings.navigation.custom.fields.sortOrder') }}</label>
              <Input v-model.number="item.sort_order" type="number" placeholder="0" />
            </div>
          </div>
        </article>
      </div>
    </section>

  </div>
</template>
