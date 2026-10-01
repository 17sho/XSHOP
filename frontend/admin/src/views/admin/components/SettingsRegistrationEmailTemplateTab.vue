<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { onBeforeRouteLeave } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import { Button } from '@/components/ui/button'
import { notifyError, notifySuccess } from '@/utils/notify'
import { registrationLanguages, registrationVariables, validateRegistrationTemplates, type RegistrationEmailSettings, type RegistrationLanguage, verificationScenes, verificationTemplates, type VerificationScene } from '@/utils/registrationEmailTemplates'

import { renderRegistrationEmailPreview, type RegistrationBrand } from '@/utils/registrationEmailPreview'

const props = defineProps<{
  currentLang: RegistrationLanguage
  expireMinutes: number
  brand?: RegistrationBrand
  scene?: VerificationScene
  requireScenes?: boolean
}>()
const { t } = useI18n()
const text = (key: string) => t(`admin.settings.registrationEmailTemplate.${key}`)
const loaded = ref(false)
const loadFailed = ref(false)
const loading = ref(false)
const submitting = ref(false)
const form = ref<RegistrationEmailSettings | null>(null)
const savedSnapshot = ref('')
const currentScene = computed(() => props.scene || 'registration')
const currentTemplate = computed(() => form.value && verificationTemplates(form.value, currentScene.value)?.[props.currentLang])
const validationErrors = computed(() => form.value ? verificationScenes.flatMap(scene => {
  const templates = verificationTemplates(form.value!, scene)
  return templates ? validateRegistrationTemplates({ templates }).map(error => ({ ...error, scene })) : []
}) : [])
const reportError = (error: unknown, fallback: string) => {
  if (!(error as { __notified?: boolean })?.__notified) notifyError(fallback)
}
function readSettings(raw: unknown): RegistrationEmailSettings {
  const data = raw as RegistrationEmailSettings | undefined
  // An incomplete response must never become an editable blank replacement.
  if (!data?.templates || !registrationLanguages.every(lang => {
    const entry = data.templates[lang]
    return entry && typeof entry.subject === 'string' && typeof entry.body === 'string' && typeof entry.custom_html === 'string' && typeof entry.custom_html_enabled === 'boolean'
  })) throw new Error('Invalid registration template response')
  const result: RegistrationEmailSettings = { templates: structuredClone(data.templates) }
  if (props.requireScenes || data.scenes !== undefined) {
    if (!data.scenes || !verificationScenes.slice(1).every(scene => {
      const templates = verificationTemplates(data, scene)
      return templates && registrationLanguages.every(lang => {
        const entry = templates[lang]
        return entry && typeof entry.subject === 'string' && typeof entry.body === 'string' && typeof entry.custom_html === 'string' && typeof entry.custom_html_enabled === 'boolean'
      })
    })) throw new Error('Incomplete verification scenes response')
    result.scenes = structuredClone(data.scenes)
  }
  return result
}
async function load() {
  if (loading.value || loaded.value) return
  loading.value = true
  loadFailed.value = false
  try {
    const res = await adminAPI.getRegistrationEmailTemplateSettings()
    form.value = readSettings(res.data?.data)
    savedSnapshot.value = JSON.stringify(form.value)
    loaded.value = true
  } catch (error) {
    loadFailed.value = true
    reportError(error, text('loadFailed'))
  } finally { loading.value = false }
}
async function save() {
  if (!loaded.value || loadFailed.value || submitting.value || !form.value) return
  if (validationErrors.value.length) {
    notifyError(validationErrors.value.map(error => `${error.scene} / ${error.lang}: ${text(`validation.${error.reason}`)}`).join('\n'))
    return
  }
  submitting.value = true
  const snapshot = JSON.stringify(form.value)
  const payload = JSON.parse(snapshot) as RegistrationEmailSettings
  try {
    const response = await adminAPI.updateRegistrationEmailTemplateSettings(payload)
    const saved = readSettings(response.data?.data)
    // Incorporate server normalization only where the admin has not edited in flight.
    for (const scene of verificationScenes) {
      const current = verificationTemplates(form.value, scene)
      const sent = verificationTemplates(payload, scene)
      const canonical = verificationTemplates(saved, scene)
      if (!current || !sent || !canonical) continue
      for (const lang of registrationLanguages) {
        for (const field of ['subject', 'body', 'custom_html', 'custom_html_enabled'] as const) {
          if (current[lang][field] === sent[lang][field]) Object.assign(current[lang], { [field]: canonical[lang][field] })
        }
      }
    }
    savedSnapshot.value = JSON.stringify(saved)
    notifySuccess(t('admin.settings.alerts.saveSuccess'))
  } catch (error) { reportError(error, t('admin.settings.alerts.saveFailed')) }
  finally { submitting.value = false }
}
const hasUnsavedChanges = computed(() => loaded.value && JSON.stringify(form.value) !== savedSnapshot.value)
const currentIsDirty = computed(() => loaded.value && JSON.stringify(currentTemplate.value) !== JSON.stringify(verificationTemplates(JSON.parse(savedSnapshot.value) as RegistrationEmailSettings, currentScene.value)?.[props.currentLang]))
const previewMode = ref<'desktop' | 'mobile'>('desktop')
const preview = computed(() => currentTemplate.value ? renderRegistrationEmailPreview(currentTemplate.value, props.currentLang, props.brand || {}, props.expireMinutes, currentScene.value) : { subject: '', plain: '', html: '' })
const htmlBytes = computed(() => new TextEncoder().encode(currentTemplate.value?.custom_html || '').length)
type EditorField = 'subject' | 'body' | 'custom_html'
const focusedField = ref<EditorField>('body')
const subjectInput = ref<HTMLInputElement>()
const bodyInput = ref<HTMLTextAreaElement>()
const htmlInput = ref<HTMLTextAreaElement>()
const editorElement = (field: EditorField) => field === 'subject' ? subjectInput.value : field === 'body' ? bodyInput.value : htmlInput.value
async function insertVariable(key: string) {
  if (!currentTemplate.value) return
  const field = focusedField.value
  const element = editorElement(field)
  const value = currentTemplate.value[field]
  const start = element?.selectionStart ?? value.length
  const end = element?.selectionEnd ?? start
  const token = `{{${key}}}`
  currentTemplate.value[field] = value.slice(0, start) + token + value.slice(end)
  await nextTick()
  element?.focus()
  element?.setSelectionRange(start + token.length, start + token.length)
}
function revertCurrent() {
  if (!form.value || !loaded.value || submitting.value) return
  const templates = verificationTemplates(form.value, currentScene.value)
  const saved = verificationTemplates(JSON.parse(savedSnapshot.value) as RegistrationEmailSettings, currentScene.value)
  if (templates && saved) templates[props.currentLang] = saved[props.currentLang]
}
function warnBeforeUnload(event: BeforeUnloadEvent) {
  if (!hasUnsavedChanges.value && !submitting.value) return
  event.preventDefault()
  event.returnValue = ''
}
onBeforeRouteLeave(() => (!hasUnsavedChanges.value && !submitting.value) || window.confirm(text('leaveConfirm')))
onMounted(() => { void load(); window.addEventListener('beforeunload', warnBeforeUnload) })
onBeforeUnmount(() => window.removeEventListener('beforeunload', warnBeforeUnload))
defineExpose({ save, submitting, loaded, loadFailed, hasUnsavedChanges })
</script>

<template>
  <section class="rounded-xl border border-border bg-card p-4 sm:p-6">
    <h2 class="text-lg font-semibold">{{ t(`admin.settings.emailTemplates.scenes.${currentScene}`) }}</h2>
    <p class="mt-1 text-xs text-muted-foreground">{{ text('subtitle') }}</p>
    <p v-if="loading" role="status" class="mt-4">{{ text('loading') }}</p>
    <div v-else-if="loadFailed" role="alert" class="mt-4 space-y-3">
      <p>{{ text('loadFailed') }}</p>
      <Button type="button" data-testid="registration-email-retry" variant="outline" @click="load">{{ text('retry') }}</Button>
    </div>
    <div v-if="loaded && currentTemplate" class="mt-6 space-y-4">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div class="flex flex-wrap items-center gap-2 text-sm">
          <span>{{ t('admin.settings.registrationEmailTemplate.editingLanguage', { language: currentLang }) }}</span>
          <span v-if="hasUnsavedChanges" data-testid="registration-email-dirty" class="rounded-full bg-amber-100 px-2 py-1 text-xs text-amber-900 dark:bg-amber-950 dark:text-amber-200">{{ text('unsaved') }}</span>
        </div>
        <Button type="button" variant="outline" size="sm" data-testid="registration-email-revert" :disabled="!currentIsDirty || submitting" @click="revertCurrent">{{ text('revert') }}</Button>
      </div>
      <div v-if="validationErrors.length" role="alert" class="rounded-lg border border-destructive/40 bg-destructive/10 p-3 text-xs text-destructive">
        <ul class="list-inside list-disc"><li v-for="(error, index) in validationErrors" :key="index">{{ t(`admin.settings.emailTemplates.scenes.${error.scene}`) }} / {{ error.lang }}: {{ text(`validation.${error.reason}`) }}</li></ul>
      </div>
      <div class="grid min-w-0 grid-cols-1 gap-6 xl:grid-cols-2">
        <div class="min-w-0 space-y-5">
          <div class="space-y-2">
            <label for="registration-email-subject" class="text-sm font-medium">{{ text('subject') }}</label>
            <input id="registration-email-subject" ref="subjectInput" v-model="currentTemplate.subject" class="w-full rounded-md border border-input bg-background p-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" @focus="focusedField = 'subject'" />
          </div>
          <div class="space-y-2">
            <label for="registration-email-body" class="text-sm font-medium">{{ text('body') }}</label>
            <textarea id="registration-email-body" ref="bodyInput" v-model="currentTemplate.body" rows="10" class="w-full rounded-md border border-input bg-background p-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" @focus="focusedField = 'body'"></textarea>
            <p class="text-xs text-muted-foreground">{{ text('bodyHint') }}</p>
          </div>
          <section class="rounded-lg border border-border bg-muted/20 p-3">
            <h3 class="text-sm font-medium">{{ text('variables') }}</h3>
            <p class="mb-3 text-xs text-muted-foreground">{{ text('variableHint') }}</p>
            <div class="flex flex-wrap gap-2">
              <button v-for="key in registrationVariables" :key="key" type="button" :data-variable="key" :title="text(`variableList.${key}`)" class="rounded border border-border bg-background px-2 py-1 font-mono text-xs hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring" @pointerdown.prevent @mousedown.prevent @click="insertVariable(key)"><span v-text="'{{' + key + '}}'"></span></button>
            </div>
          </section>
          <details class="rounded-lg border border-amber-300 bg-amber-50/40 p-4 dark:border-amber-900 dark:bg-amber-950/20">
            <summary class="cursor-pointer text-sm font-semibold">{{ text('advanced') }}</summary>
            <div class="mt-4 space-y-3">
              <p class="text-xs leading-relaxed text-muted-foreground">{{ text('advancedHint') }}</p>
              <label class="flex items-center gap-2 text-sm" for="registration-email-html-enabled">
                <input id="registration-email-html-enabled" v-model="currentTemplate.custom_html_enabled" type="checkbox" class="h-4 w-4 accent-primary" />{{ text('enableHtml') }}
              </label>
              <label for="registration-email-html" class="block text-sm font-medium">{{ text('html') }}</label>
              <textarea id="registration-email-html" ref="htmlInput" v-model="currentTemplate.custom_html" rows="14" spellcheck="false" class="w-full rounded-md border border-input bg-background p-3 font-mono text-xs focus-visible:ring-2 focus-visible:ring-ring" @focus="focusedField = 'custom_html'"></textarea>
              <p class="text-xs text-muted-foreground">{{ htmlBytes }} / 204800 {{ text('bytes') }}</p>
            </div>
          </details>
        </div>
        <aside class="min-w-0 space-y-4 rounded-lg bg-muted/20 p-4">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <h3 class="font-semibold">{{ text('preview') }}</h3>
            <div class="flex gap-1">
              <Button v-for="mode in (['desktop', 'mobile'] as const)" :key="mode" type="button" size="sm" :variant="previewMode === mode ? 'default' : 'outline'" :aria-pressed="previewMode === mode" @click="previewMode = mode">{{ text(mode) }}</Button>
            </div>
          </div>
          <p class="text-xs leading-relaxed text-muted-foreground">{{ t('admin.settings.registrationEmailTemplate.synthetic', { minutes: expireMinutes }) }}</p>
          <div class="mx-auto space-y-4" :class="previewMode === 'mobile' ? 'max-w-[360px]' : 'max-w-[640px]'">
            <div class="rounded-lg border border-border bg-background p-4">
              <p class="text-xs text-muted-foreground">{{ text('subject') }}</p>
              <p class="mt-1 break-words font-semibold">{{ preview.subject }}</p>
            </div>
            <iframe sandbox="" referrerpolicy="no-referrer" :srcdoc="preview.html" :title="text('htmlPreview')" class="min-h-[420px] w-full rounded-lg border border-border bg-white"></iframe>
            <div class="rounded-lg border border-border bg-background p-4">
              <p class="mb-3 text-xs font-medium text-muted-foreground">{{ text('plainPreview') }}</p>
              <p class="whitespace-pre-wrap break-words text-sm leading-relaxed">{{ preview.plain }}</p>
            </div>
          </div>
        </aside>
      </div>
    </div>
  </section>
</template>
