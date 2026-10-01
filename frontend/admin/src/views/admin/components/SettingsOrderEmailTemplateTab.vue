<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { onBeforeRouteLeave } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { Switch } from '@/components/ui/switch'
import { notifyError, notifySuccess } from '@/utils/notify'
import { getImageUrl } from '@/utils/image'
import { orderEmailSceneKeys } from '@/utils/orderEmailTemplates'

const { t } = useI18n()

const supportedLanguages = ['zh-CN', 'zh-TW', 'en-US'] as const
type SupportedLanguage = (typeof supportedLanguages)[number]
type EditorField = 'subject' | 'body'
type PreviewMode = 'desktop' | 'mobile'
type OrderEmailLocalizedTemplate = { subject: string; body: string; custom_html: string; custom_html_enabled: boolean }
type OrderEmailSceneTemplate = Record<SupportedLanguage, OrderEmailLocalizedTemplate>
const moduleKeys = ['header', 'order_details', 'items', 'delivery', 'instructions', 'message', 'notice', 'footer'] as const
type ModuleKey = (typeof moduleKeys)[number]
type OrderEmailModules = Record<ModuleKey, boolean>
const createDefaultModules = (): OrderEmailModules => ({ header: true, order_details: true, items: true, delivery: true, instructions: true, message: true, notice: true, footer: true })

interface OrderEmailTemplateData {
  templates: Record<(typeof orderEmailSceneKeys)[number], OrderEmailSceneTemplate>
  guest_tip: Record<SupportedLanguage, string>
  modules: OrderEmailModules
}

interface OrderEmailBrand {
  site_name?: string
  site_url?: string
  site_logo?: string
  site_icon?: string
}

const sceneKeys = orderEmailSceneKeys
type SceneKey = (typeof sceneKeys)[number]

const props = defineProps<{
  loaded: boolean
  data: OrderEmailTemplateData
  currentLang: SupportedLanguage
  brand?: OrderEmailBrand
  currency?: string
}>()

const emit = defineEmits<{ saved: [] }>()

const submitting = ref(false)
const currentScene = ref<SceneKey>('default')
const previewMode = ref<PreviewMode>('desktop')
const focusedField = ref<EditorField>('body')
const subjectInput = ref<HTMLInputElement | null>(null)
const bodyInput = ref<HTMLTextAreaElement | null>(null)
const advancedInput = ref<HTMLTextAreaElement | null>(null)

const createLocalizedTemplate = (): OrderEmailLocalizedTemplate => ({ subject: '', body: '', custom_html: '', custom_html_enabled: false })
const createSceneTemplate = (): OrderEmailSceneTemplate => ({
  'zh-CN': createLocalizedTemplate(),
  'zh-TW': createLocalizedTemplate(),
  'en-US': createLocalizedTemplate(),
})
const deepCloneTemplate = (src?: OrderEmailSceneTemplate): OrderEmailSceneTemplate => {
  const result = createSceneTemplate()
  supportedLanguages.forEach((lang) => {
    result[lang].subject = src?.[lang]?.subject || ''
    result[lang].body = src?.[lang]?.body || ''
    result[lang].custom_html = src?.[lang]?.custom_html || ''
    result[lang].custom_html_enabled = src?.[lang]?.custom_html_enabled === true
  })
  return result
}

const form = reactive<OrderEmailTemplateData>({
  templates: {
    default: createSceneTemplate(),
    paid: createSceneTemplate(),
    delivered: createSceneTemplate(),
    delivered_with_content: createSceneTemplate(),
    refunded: createSceneTemplate(),
    partially_refunded: createSceneTemplate(),
  },
  guest_tip: { 'zh-CN': '', 'zh-TW': '', 'en-US': '' },
  modules: createDefaultModules(),
})

const serialize = (data: OrderEmailTemplateData) => JSON.stringify(data)
const savedSnapshot = ref('')
const syncFromProps = () => {
  sceneKeys.forEach((key) => {
    form.templates[key] = deepCloneTemplate(props.data.templates[key])
  })
  supportedLanguages.forEach((lang) => {
    form.guest_tip[lang] = props.data.guest_tip[lang] || ''
  })
  moduleKeys.forEach((key) => { form.modules[key] = props.data.modules?.[key] !== false })
  savedSnapshot.value = serialize(form)
}
syncFromProps()
watch(() => props.data, syncFromProps, { deep: true })

const currentTemplate = computed(() => form.templates[currentScene.value][props.currentLang])
const moduleVisibilityBindings = computed(() => ({
  header: form.modules.header, order_details: form.modules.order_details, items: form.modules.items,
  delivery: form.modules.delivery, instructions: form.modules.instructions, message: form.modules.message,
  notice: form.modules.notice, footer: form.modules.footer,
}))
const hasUnsavedChanges = computed(() => serialize(form) !== savedSnapshot.value)
const sceneIsDirty = (scene: SceneKey) => supportedLanguages.some((lang) => {
  const local = form.templates[scene][lang]
  const original = (JSON.parse(savedSnapshot.value) as OrderEmailTemplateData).templates[scene]?.[lang]
  return local.subject !== (original?.subject || '') || local.body !== (original?.body || '') || local.custom_html !== (original?.custom_html || '') || local.custom_html_enabled !== (original?.custom_html_enabled === true)
})
const sceneIsComplete = (scene: SceneKey) => supportedLanguages.every((lang) => {
  const template = form.templates[scene][lang]
  return template.subject.trim() !== '' && template.body.trim() !== ''
})

const templateVariables = [
  'order_no', 'status', 'amount', 'refund_amount', 'refund_reason', 'currency',
  'fulfillment_info', 'instructions', 'site_name', 'site_url',
] as const
const allowedVariables = new Set<string>(templateVariables)
const variablePattern = /{{\s*([^{}]+?)\s*}}/g
const findUnknownVariables = (value: string) => {
  const unknown = new Set<string>()
  for (const match of value.matchAll(variablePattern)) {
    const key = (match[1] || '').trim()
    if (!allowedVariables.has(key)) unknown.add(key)
  }
  return [...unknown]
}
const unknownVariables = computed(() => {
  const unknown = new Set<string>()
  sceneKeys.forEach((scene) => supportedLanguages.forEach((lang) => {
    findUnknownVariables(form.templates[scene][lang].subject).forEach((key) => unknown.add(key))
    findUnknownVariables(form.templates[scene][lang].body).forEach((key) => unknown.add(key))
    findUnknownVariables(form.templates[scene][lang].custom_html).forEach((key) => unknown.add(key))
  }))
  supportedLanguages.forEach((lang) => findUnknownVariables(form.guest_tip[lang]).forEach((key) => unknown.add(key)))
  return [...unknown]
})
const emptySubjects = computed(() => sceneKeys.flatMap((scene) => supportedLanguages
  .filter((lang) => !form.templates[scene][lang].subject.trim())
  .map((lang) => `${t(`admin.settings.orderEmailTemplate.scenes.${scene}`)} (${lang})`)))
const validationErrors = computed(() => {
  const errors: string[] = []
  if (emptySubjects.value.length) errors.push(t('admin.settings.orderEmailTemplate.validation.emptySubject', { items: emptySubjects.value.join(', ') }))
  if (unknownVariables.value.length) errors.push(t('admin.settings.orderEmailTemplate.validation.unknownVariables', { variables: unknownVariables.value.join(', ') }))
  sceneKeys.forEach((scene) => supportedLanguages.forEach((lang) => {
    const html = form.templates[scene][lang].custom_html
    if (new Blob([html]).size > 200 * 1024) errors.push(t('admin.settings.orderEmailTemplate.validation.htmlTooLarge', { scene, lang }))
    if (/<\/?(?:script|style|link|iframe|object|embed|form|base|svg|math)\b/i.test(html) || /\son[a-z]+\s*=/i.test(html) || /(?:javascript:|vbscript:|data:|file:|blob:|url\s*\(|image-set\s*\(|@import|expression\s*\(|behavior\s*:|-moz-binding)/i.test(html)) errors.push(t('admin.settings.orderEmailTemplate.validation.unsafeHtml', { scene, lang }))
  }))
  return errors
})

const previewBrandName = computed(() => props.brand?.site_name?.trim() || t('admin.settings.orderEmailTemplate.preview.fallbackBrand'))
const previewSiteURL = computed(() => {
  const value = props.brand?.site_url?.trim() || ''
  if (!value) return ''
  try {
    const parsed = new URL(value)
    return (parsed.protocol === 'http:' || parsed.protocol === 'https:') && !parsed.username && !parsed.password ? value : ''
  } catch {
    return ''
  }
})
const safeBrandLogo = computed(() => {
  const source = props.brand?.site_logo?.trim() || props.brand?.site_icon?.trim() || ''
  if (!source || source.startsWith('//')) return ''
  if (/^[a-z][a-z\d+.-]*:/i.test(source) && !/^https?:\/\//i.test(source)) return ''
  if (/^https?:\/\//i.test(source)) {
    try {
      const parsed = new URL(source)
      if (!parsed.hostname || parsed.username || parsed.password) return ''
    } catch {
      return ''
    }
  }
  return getImageUrl(source)
})
const previewCurrency = computed(() => props.currency?.trim().toUpperCase() || 'CNY')
const previewStatusKey = computed(() => currentScene.value === 'delivered_with_content' ? 'delivered' : currentScene.value)
const previewStatusTitle = computed(() => t(`admin.settings.orderEmailTemplate.preview.status.${previewStatusKey.value}.title`))
const previewStatusDescription = computed(() => t(`admin.settings.orderEmailTemplate.preview.status.${previewStatusKey.value}.description`))
const isRefundPreview = computed(() => currentScene.value === 'refunded' || currentScene.value === 'partially_refunded')
const isPaidPreview = computed(() => currentScene.value !== 'default')
const hasDeliveryPreview = computed(() => currentScene.value === 'delivered_with_content')
const sampleData = computed<Record<(typeof templateVariables)[number], string>>(() => ({
  order_no: 'DJ-20260930-1042',
  status: previewStatusTitle.value,
  amount: '129.00',
  refund_amount: '29.00',
  refund_reason: t('admin.settings.orderEmailTemplate.preview.sampleRefundReason'),
  currency: previewCurrency.value,
  fulfillment_info: 'License: DEMO-8K2P-4Q7M',
  instructions: t('admin.settings.orderEmailTemplate.preview.sampleInstructions'),
  site_name: previewBrandName.value,
  site_url: previewSiteURL.value,
}))
const renderPreview = (value: string) => value.replace(variablePattern, (token, rawKey: string) => {
  const key = rawKey.trim() as (typeof templateVariables)[number]
  return sampleData.value[key] ?? token
})
const previewSubject = computed(() => renderPreview(currentTemplate.value.subject) || t('admin.settings.orderEmailTemplate.preview.emptySubject'))
const previewBody = computed(() => renderPreview(currentTemplate.value.body) || t('admin.settings.orderEmailTemplate.preview.emptyBody'))
const previewGuestTip = computed(() => renderPreview(form.guest_tip[props.currentLang]))
const advancedPreviewHTML = computed(() => renderPreview(currentTemplate.value.custom_html))
const currentHTMLBytes = computed(() => new Blob([currentTemplate.value.custom_html]).size)

const rememberFocus = (field: EditorField) => { focusedField.value = field }
const insertVariable = async (key: string) => {
  const field = focusedField.value
  const element = field === 'subject' ? subjectInput.value : bodyInput.value
  const value = currentTemplate.value[field]
  const start = element?.selectionStart ?? value.length
  const end = element?.selectionEnd ?? start
  const token = `{{${key}}}`
  currentTemplate.value[field] = `${value.slice(0, start)}${token}${value.slice(end)}`
  await nextTick()
  const target = field === 'subject' ? subjectInput.value : bodyInput.value
  target?.focus()
  target?.setSelectionRange(start + token.length, start + token.length)
}

const resetCurrentTemplate = () => {
  const original = (JSON.parse(savedSnapshot.value) as OrderEmailTemplateData).templates[currentScene.value]?.[props.currentLang]
  currentTemplate.value.subject = original?.subject || ''
  currentTemplate.value.body = original?.body || ''
}
const resetCurrentAdvancedHTML = () => {
  const original = (JSON.parse(savedSnapshot.value) as OrderEmailTemplateData).templates[currentScene.value]?.[props.currentLang]
  currentTemplate.value.custom_html = original?.custom_html || ''
  currentTemplate.value.custom_html_enabled = original?.custom_html_enabled === true
}
const insertAdvancedVariable = async (key: string) => {
  const value = currentTemplate.value.custom_html
  const start = advancedInput.value?.selectionStart ?? value.length
  const end = advancedInput.value?.selectionEnd ?? start
  const token = `{{${key}}}`
  currentTemplate.value.custom_html = `${value.slice(0, start)}${token}${value.slice(end)}`
  await nextTick()
  advancedInput.value?.focus()
  advancedInput.value?.setSelectionRange(start + token.length, start + token.length)
}

const notifyErrorIfNeeded = (err: unknown, fallback: string) => {
  const known = err as Error & { __notified?: boolean }
  if (known?.__notified) return
  notifyError(known?.message || fallback)
}

const save = async () => {
  if (!props.loaded || submitting.value) return
  if (validationErrors.value.length) return notifyError(validationErrors.value.join('\n'))
  submitting.value = true
  try {
    void moduleVisibilityBindings.value
    const payload: OrderEmailTemplateData = JSON.parse(serialize({ ...form, modules: { ...form.modules } }))
    const response = await adminAPI.updateOrderEmailTemplateSettings(payload as unknown as Record<string, unknown>)
    const saved = response.data?.data as OrderEmailTemplateData | undefined
    if (!saved?.templates || !sceneKeys.every(scene => supportedLanguages.every(lang => {
      const entry = saved.templates[scene]?.[lang]
      return entry && typeof entry.subject === 'string' && typeof entry.body === 'string' && typeof entry.custom_html === 'string' && typeof entry.custom_html_enabled === 'boolean'
    })) || !saved.guest_tip || !supportedLanguages.every(lang => typeof saved.guest_tip[lang] === 'string')) throw new Error(t('admin.settings.alerts.saveFailed'))
    saved.modules = { ...createDefaultModules(), ...saved.modules }
    for (const scene of sceneKeys) for (const lang of supportedLanguages) {
      for (const field of ['subject', 'body', 'custom_html', 'custom_html_enabled'] as const) {
        if (form.templates[scene][lang][field] === payload.templates[scene][lang][field]) Object.assign(form.templates[scene][lang], { [field]: saved.templates[scene][lang][field] })
      }
    }
    for (const lang of supportedLanguages) if (form.guest_tip[lang] === payload.guest_tip[lang]) form.guest_tip[lang] = saved.guest_tip[lang]
    for (const key of moduleKeys) if (form.modules[key] === payload.modules[key]) form.modules[key] = saved.modules[key]
    savedSnapshot.value = serialize({
      templates: Object.fromEntries(sceneKeys.map(scene => [scene, deepCloneTemplate(saved.templates[scene])])),
      guest_tip: Object.fromEntries(supportedLanguages.map(lang => [lang, saved.guest_tip[lang]])),
      modules: Object.fromEntries(moduleKeys.map(key => [key, saved.modules[key]])),
    } as OrderEmailTemplateData)
    notifySuccess(t('admin.settings.alerts.saveSuccess'))
    emit('saved')
  } catch (err) {
    notifyErrorIfNeeded(err, t('admin.settings.alerts.saveFailed'))
  } finally {
    submitting.value = false
  }
}

const warnBeforeUnload = (event: BeforeUnloadEvent) => {
  if (!hasUnsavedChanges.value && !submitting.value) return
  event.preventDefault()
  event.returnValue = ''
}
onMounted(() => window.addEventListener('beforeunload', warnBeforeUnload))
onBeforeUnmount(() => window.removeEventListener('beforeunload', warnBeforeUnload))

onBeforeRouteLeave(() => (!hasUnsavedChanges.value && !submitting.value) || window.confirm(t('admin.settings.registrationEmailTemplate.leaveConfirm')))
defineExpose({ save, submitting, hasUnsavedChanges })
</script>

<template>
  <fieldset :disabled="!loaded" class="min-w-0 overflow-hidden rounded-xl border border-border bg-card">
    <header class="flex flex-col gap-3 border-b border-border bg-muted/30 px-4 py-4 sm:flex-row sm:items-center sm:justify-between sm:px-6">
      <div>
        <div class="flex items-center gap-2">
          <h2 class="text-lg font-semibold">{{ t('admin.settings.orderEmailTemplate.title') }}</h2>
          <span v-if="hasUnsavedChanges" class="rounded-full bg-amber-100 px-2 py-0.5 text-[11px] font-medium text-amber-800 dark:bg-amber-950 dark:text-amber-300">
            {{ t('admin.settings.orderEmailTemplate.unsaved') }}
          </span>
        </div>
        <p class="mt-1 text-xs text-muted-foreground">{{ t('admin.settings.orderEmailTemplate.subtitle') }}</p>
      </div>
      <Button type="button" variant="outline" size="sm" :disabled="!sceneIsDirty(currentScene)" @click="resetCurrentTemplate">
        {{ t('admin.settings.orderEmailTemplate.resetCurrent') }}
      </Button>
    </header>

    <div class="grid min-h-[640px] grid-cols-1 xl:grid-cols-[220px_minmax(360px,1fr)_minmax(340px,0.9fr)]">
      <nav data-testid="order-email-scene-list" class="border-b border-border bg-muted/15 p-3 xl:border-b-0 xl:border-r">
        <p class="mb-2 px-2 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">{{ t('admin.settings.orderEmailTemplate.scene') }}</p>
        <div class="grid grid-cols-2 gap-1 sm:grid-cols-3 xl:grid-cols-1">
          <button
            v-for="scene in sceneKeys"
            :key="scene"
            type="button"
            class="flex min-w-0 items-center gap-2 rounded-lg px-3 py-2.5 text-left text-sm transition-colors"
            :class="currentScene === scene ? 'bg-primary text-primary-foreground' : 'hover:bg-muted'"
            @click="currentScene = scene"
          >
            <span class="h-2 w-2 shrink-0 rounded-full" :class="sceneIsComplete(scene) ? 'bg-emerald-500' : 'bg-amber-500'" :title="sceneIsComplete(scene) ? t('admin.settings.orderEmailTemplate.complete') : t('admin.settings.orderEmailTemplate.incomplete')"></span>
            <span class="truncate">{{ t(`admin.settings.orderEmailTemplate.scenes.${scene}`) }}</span>
            <span v-if="sceneIsDirty(scene)" class="ml-auto h-1.5 w-1.5 shrink-0 rounded-full bg-blue-500" :title="t('admin.settings.orderEmailTemplate.unsaved')"></span>
          </button>
        </div>
      </nav>

      <main class="space-y-5 border-b border-border p-4 sm:p-6 xl:border-b-0 xl:border-r">
        <div>
          <h3 class="font-semibold">{{ t(`admin.settings.orderEmailTemplate.scenes.${currentScene}`) }}</h3>
          <p class="text-xs text-muted-foreground">{{ t('admin.settings.orderEmailTemplate.editingLanguage', { language: currentLang }) }}</p>
        </div>

        <div class="space-y-2">
          <label for="order-email-subject" class="text-xs font-medium">{{ t('admin.settings.orderEmailTemplate.subject') }}</label>
          <input id="order-email-subject" ref="subjectInput" v-model="currentTemplate.subject" class="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring" @focus="rememberFocus('subject')" />
          <p v-if="!currentTemplate.subject.trim()" class="text-xs text-destructive">{{ t('admin.settings.orderEmailTemplate.validation.subjectRequired') }}</p>
        </div>

        <div class="space-y-2">
          <label for="order-email-body" class="text-xs font-medium">{{ t('admin.settings.orderEmailTemplate.body') }}</label>
          <textarea id="order-email-body" ref="bodyInput" v-model="currentTemplate.body" rows="13" class="flex w-full rounded-md border border-input bg-background px-3 py-2 font-mono text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring" @focus="rememberFocus('body')"></textarea>
        </div>

        <section class="rounded-lg border border-blue-200 bg-blue-50/70 p-3 dark:border-blue-900 dark:bg-blue-950/30">
          <h4 class="text-xs font-semibold text-blue-900 dark:text-blue-200">{{ t('admin.settings.orderEmailTemplate.variables') }}</h4>
          <p class="mb-2 text-[11px] text-blue-700 dark:text-blue-400">{{ t('admin.settings.orderEmailTemplate.variableInsertHint') }}</p>
          <div class="flex flex-wrap gap-1.5">
            <button v-for="key in templateVariables" :key="key" type="button" class="rounded-md border border-blue-200 bg-white px-2 py-1 font-mono text-[11px] text-blue-700 hover:bg-blue-100 dark:border-blue-800 dark:bg-blue-950 dark:text-blue-300" :title="t(`admin.settings.orderEmailTemplate.variableList.${key}`)" @mousedown.prevent @click="insertVariable(key)">
              <span v-text="'{{' + key + '}}'"></span>
            </button>
          </div>
        </section>

        <div v-if="unknownVariables.length" role="alert" class="rounded-lg border border-destructive/40 bg-destructive/10 p-3 text-xs text-destructive">
          {{ t('admin.settings.orderEmailTemplate.validation.unknownVariables', { variables: unknownVariables.join(', ') }) }}
        </div>

        <section data-testid="order-email-guest-copy" class="space-y-2 rounded-lg border border-dashed border-border bg-muted/20 p-4">
          <div>
            <h4 class="text-sm font-medium">{{ t('admin.settings.orderEmailTemplate.guestTip') }}</h4>
            <p class="text-xs text-muted-foreground">{{ t('admin.settings.orderEmailTemplate.guestTipDesc') }}</p>
          </div>
          <Textarea v-model="form.guest_tip[currentLang]" rows="3" class="font-mono text-sm" />
        </section>

        <section data-testid="order-email-module-selector" class="rounded-lg border border-border p-4">
          <h4 class="text-sm font-semibold">{{ t('admin.settings.orderEmailTemplate.modules.title') }}</h4>
          <p class="mb-3 text-xs text-muted-foreground">{{ t('admin.settings.orderEmailTemplate.modules.description') }}</p>
          <div class="grid grid-cols-2 gap-3">
            <label v-for="key in moduleKeys" :key="key" class="flex items-center gap-2 text-sm">
              <Switch v-model="form.modules[key]" />
              <span>{{ t(`admin.settings.orderEmailTemplate.modules.${key}`) }}</span>
            </label>
          </div>
        </section>

        <details data-testid="order-email-advanced" class="rounded-lg border border-amber-300 bg-amber-50/40 p-4 dark:border-amber-900 dark:bg-amber-950/20">
          <summary class="cursor-pointer text-sm font-semibold">{{ t('admin.settings.orderEmailTemplate.advanced.title') }}</summary>
          <div class="mt-4 space-y-3">
            <p class="text-xs text-amber-800 dark:text-amber-300">{{ t('admin.settings.orderEmailTemplate.advanced.warning') }}</p>
            <label class="flex items-center gap-2 text-sm"><Switch v-model="currentTemplate.custom_html_enabled" />{{ t('admin.settings.orderEmailTemplate.advanced.enable') }}</label>
            <div class="flex flex-wrap gap-1.5">
              <button v-for="key in templateVariables" :key="key" type="button" class="rounded border bg-background px-2 py-1 font-mono text-[11px]" @click="insertAdvancedVariable(key)"><span v-text="'{{' + key + '}}'"></span></button>
            </div>
            <textarea ref="advancedInput" v-model="currentTemplate.custom_html" rows="14" class="flex w-full rounded-md border border-input bg-background px-3 py-2 font-mono text-xs" spellcheck="false"></textarea>
            <div class="flex items-center justify-between">
              <span class="text-xs text-muted-foreground">{{ currentHTMLBytes }} / 204800 bytes</span>
              <Button type="button" size="sm" variant="outline" @click="resetCurrentAdvancedHTML">{{ t('admin.settings.orderEmailTemplate.advanced.reset') }}</Button>
            </div>
          </div>
        </details>
      </main>

      <aside class="bg-muted/20 p-4 sm:p-6">
        <div class="mb-4 flex items-center justify-between gap-3">
          <div>
            <h3 class="text-sm font-semibold">{{ t('admin.settings.orderEmailTemplate.preview.title') }}</h3>
            <p class="text-[11px] text-muted-foreground">{{ t('admin.settings.orderEmailTemplate.preview.synthetic') }}</p>
          </div>
          <div class="flex rounded-lg border bg-background p-0.5">
            <button type="button" class="rounded-md px-2.5 py-1 text-xs" :class="previewMode === 'desktop' ? 'bg-primary text-primary-foreground' : 'text-muted-foreground'" @click="previewMode = 'desktop'">{{ t('admin.settings.orderEmailTemplate.preview.desktop') }}</button>
            <button type="button" class="rounded-md px-2.5 py-1 text-xs" :class="previewMode === 'mobile' ? 'bg-primary text-primary-foreground' : 'text-muted-foreground'" @click="previewMode = 'mobile'">{{ t('admin.settings.orderEmailTemplate.preview.mobile') }}</button>
          </div>
        </div>

        <div data-testid="order-email-preview" class="mx-auto transition-[max-width]" :class="previewMode === 'mobile' ? 'max-w-[360px]' : 'max-w-[620px]'">
          <div class="mb-3 rounded-lg border bg-white px-4 py-3 text-slate-900 shadow-sm">
            <p class="text-[10px] font-medium uppercase tracking-wider text-slate-400">{{ t('admin.settings.orderEmailTemplate.subject') }}</p>
            <p class="mt-1 break-words text-sm font-semibold">{{ previewSubject }}</p>
          </div>
          <iframe v-if="currentTemplate.custom_html_enabled" sandbox="" :srcdoc="advancedPreviewHTML" class="min-h-[640px] w-full rounded-[18px] border bg-white" :title="t('admin.settings.orderEmailTemplate.advanced.previewTitle')"></iframe>
          <div v-else class="overflow-hidden rounded-[18px] bg-white font-sans text-[#162b47] shadow-sm">
            <header v-if="form.modules.header" data-testid="order-email-brand-header" class="bg-[#1766ba] px-7 py-8 text-center text-white">
              <img v-if="safeBrandLogo" :src="safeBrandLogo" alt="" class="mx-auto mb-3 h-[54px] w-[54px] object-contain" />
              <div class="text-[17px] font-bold">{{ previewBrandName }}</div>
              <div class="mx-auto mb-2 mt-3.5 text-xs tracking-[2px]">{{ t('admin.settings.orderEmailTemplate.preview.orderUpdate') }}</div>
              <h1 class="m-0 text-[27px] font-bold leading-[1.4] text-white">{{ previewStatusTitle }}</h1>
              <p class="mb-0 mt-2.5 text-sm text-[#e5f1ff]">{{ previewStatusDescription }}</p>
            </header>

            <div class="px-7 py-6">
              <p class="mb-2 mt-0 font-bold">{{ t('admin.settings.orderEmailTemplate.preview.hello') }}</p>
              <p class="mb-6 mt-0 text-sm leading-[1.7] text-[#52657d]">{{ previewStatusDescription }}</p>

              <section v-if="form.modules.order_details" data-testid="order-email-order-details">
                <h4 class="mb-2.5 mt-[18px] inline-block rounded-2xl bg-[#e9f3ff] px-[13px] py-1.5 text-[13px] font-bold text-[#145caa]">{{ t('admin.settings.orderEmailTemplate.preview.sections.orderDetails') }}</h4>
                <dl class="m-0 overflow-hidden rounded-xl border border-[#dfebf7] bg-[#f5f9fe] text-[13px]">
                  <div class="grid grid-cols-[110px_1fr] border-b border-[#dfebf7]"><dt class="p-2.5 text-[#59708c]">{{ t('admin.settings.orderEmailTemplate.preview.fields.orderNumber') }}</dt><dd class="m-0 break-all p-2.5 text-sm">{{ sampleData.order_no }}</dd></div>
                  <div class="grid grid-cols-[110px_1fr] border-b border-[#dfebf7]"><dt class="p-2.5 text-[#59708c]">{{ t('admin.settings.orderEmailTemplate.preview.fields.status') }}</dt><dd class="m-0 p-2.5 text-sm">{{ previewStatusTitle }}</dd></div>
                  <div class="grid grid-cols-[110px_1fr] border-b border-[#dfebf7]"><dt class="p-2.5 text-[#59708c]">{{ t('admin.settings.orderEmailTemplate.preview.fields.created') }}</dt><dd class="m-0 p-2.5 text-sm">2026-09-30 10:42</dd></div>
                  <div v-if="isPaidPreview" class="grid grid-cols-[110px_1fr] border-b border-[#dfebf7]"><dt class="p-2.5 text-[#59708c]">{{ t('admin.settings.orderEmailTemplate.preview.fields.paid') }}</dt><dd class="m-0 p-2.5 text-sm">2026-09-30 10:43</dd></div>
                  <div class="grid grid-cols-[110px_1fr] border-b border-[#dfebf7]"><dt class="p-2.5 text-[#59708c]">{{ t('admin.settings.orderEmailTemplate.preview.fields.amount') }}</dt><dd class="m-0 p-2.5 text-sm">{{ sampleData.amount }} {{ sampleData.currency }}</dd></div>
                  <template v-if="isRefundPreview">
                    <div class="grid grid-cols-[110px_1fr] border-b border-[#dfebf7]"><dt class="p-2.5 text-[#59708c]">{{ t('admin.settings.orderEmailTemplate.preview.fields.refundAmount') }}</dt><dd class="m-0 p-2.5 text-sm">{{ sampleData.refund_amount }} {{ sampleData.currency }}</dd></div>
                    <div class="grid grid-cols-[110px_1fr]"><dt class="p-2.5 text-[#59708c]">{{ t('admin.settings.orderEmailTemplate.preview.fields.refundReason') }}</dt><dd class="m-0 p-2.5 text-sm">{{ sampleData.refund_reason }}</dd></div>
                  </template>
                </dl>
              </section>

              <section v-if="form.modules.items" data-testid="order-email-items">
                <h4 class="mb-2.5 mt-[18px] inline-block rounded-2xl bg-[#e9f3ff] px-[13px] py-1.5 text-[13px] font-bold text-[#145caa]">{{ t('admin.settings.orderEmailTemplate.preview.sections.items') }}</h4>
                <dl class="m-0 overflow-hidden rounded-xl border border-[#dfebf7] bg-[#f5f9fe] text-[13px]">
                  <div v-for="field in ['childOrder', 'product', 'specification', 'unitPrice', 'quantity']" :key="field" class="grid grid-cols-[110px_1fr] border-b border-[#dfebf7] last:border-b-0">
                    <dt class="p-2.5 text-[#59708c]">{{ t(`admin.settings.orderEmailTemplate.preview.fields.${field}`) }}</dt>
                    <dd class="m-0 break-words p-2.5 text-sm">{{ t(`admin.settings.orderEmailTemplate.preview.sampleItem.${field}`, { currency: sampleData.currency }) }}</dd>
                  </div>
                </dl>
              </section>

              <section v-if="form.modules.delivery && hasDeliveryPreview" data-testid="order-email-delivery">
                <h4 class="mb-2.5 mt-[18px] inline-block rounded-2xl bg-[#e9f3ff] px-[13px] py-1.5 text-[13px] font-bold text-[#145caa]">{{ t('admin.settings.orderEmailTemplate.preview.sections.delivery') }}</h4>
                <p class="text-[13px] font-bold">{{ t('admin.settings.orderEmailTemplate.preview.fields.childOrder') }}: DJ-20260930-1042-1</p>
                <div class="whitespace-pre-wrap break-words rounded-xl bg-[#17395f] p-4 font-mono text-[13px] text-white">{{ sampleData.fulfillment_info }}</div>
              </section>

              <section v-if="form.modules.instructions && hasDeliveryPreview">
                <h4 class="mb-2.5 mt-[18px] inline-block rounded-2xl bg-[#e9f3ff] px-[13px] py-1.5 text-[13px] font-bold text-[#145caa]">{{ t('admin.settings.orderEmailTemplate.preview.sections.instructions') }}</h4>
                <p class="whitespace-pre-wrap break-words text-[13px] leading-[1.7]">{{ sampleData.instructions }}</p>
              </section>

              <section v-if="form.modules.message" data-testid="order-email-message">
                <h4 class="mb-2.5 mt-[18px] inline-block rounded-2xl bg-[#e9f3ff] px-[13px] py-1.5 text-[13px] font-bold text-[#145caa]">{{ t('admin.settings.orderEmailTemplate.preview.sections.message') }}</h4>
                <p class="whitespace-pre-wrap break-words text-[13px] leading-[1.7] text-[#465c75]">{{ previewBody }}</p>
              </section>

              <section v-if="form.modules.notice" data-testid="order-email-notice">
                <h4 class="mb-2.5 mt-[18px] inline-block rounded-2xl bg-[#e9f3ff] px-[13px] py-1.5 text-[13px] font-bold text-[#145caa]">{{ t('admin.settings.orderEmailTemplate.preview.sections.notice') }}</h4>
                <div class="rounded-xl border border-[#b7d6f6] bg-[#f5f9fe] p-3.5 text-[13px] leading-[1.7] text-[#465c75]">
                  <p v-if="previewGuestTip" class="m-0 whitespace-pre-wrap break-words">{{ previewGuestTip }}</p>
                  <p class="m-0">{{ t('admin.settings.orderEmailTemplate.preview.guestNotice') }}</p>
                  <p class="m-0">{{ t('admin.settings.orderEmailTemplate.preview.safetyNotice') }}</p>
                </div>
              </section>

              <p class="mb-0 mt-6 border-t border-dashed border-[#b7d6f6] pt-5 text-sm leading-[1.8]">
                {{ t('admin.settings.orderEmailTemplate.preview.signoff') }}<br><strong>{{ previewBrandName }} {{ t('admin.settings.orderEmailTemplate.preview.team') }}</strong>
              </p>
              <p v-if="previewSiteURL" class="mb-0 mt-2.5 break-all text-xs"><a :href="previewSiteURL" class="text-[#2367a9] no-underline">{{ previewSiteURL }}</a></p>
            </div>

            <footer v-if="form.modules.footer" data-testid="order-email-footer" class="bg-[#17395f] px-5 py-[22px] text-center text-xs text-[#cbdcf0]">
              {{ previewBrandName }}<br>{{ t('admin.settings.orderEmailTemplate.preview.footerNotice') }}
            </footer>
          </div>
        </div>
      </aside>
    </div>
  </fieldset>
</template>
