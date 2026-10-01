import assert from 'node:assert/strict'
import fs from 'node:fs'
import test from 'node:test'

const component = fs.readFileSync(new URL('../src/views/admin/components/SettingsOrderEmailTemplateTab.vue', import.meta.url), 'utf8')
const settings = fs.readFileSync(new URL('../src/views/admin/Settings.vue', import.meta.url), 'utf8')

test('order email editor exposes scene navigator, clickable variables and responsive preview', () => {
  assert.match(component, /data-testid="order-email-scene-list"/)
  assert.match(component, /insertVariable/)
  assert.match(component, /data-testid="order-email-preview"/)
  assert.match(component, /previewMode/)
  assert.match(component, /renderPreview/)
})

test('order email editor validates unknown variables and tracks unsaved changes', () => {
  assert.match(component, /unknownVariables/)
  assert.match(component, /hasUnsavedChanges/)
  assert.match(component, /beforeunload/)
  assert.match(component, /resetCurrentTemplate/)
})

test('order email editor uses one save surface and separates guest copy', () => {
  assert.match(component, /data-testid="order-email-guest-copy"/)
  assert.doesNotMatch(component, /@click="save"/)
})

test('order email editor blocks invalid saves and renders escaped plain text', () => {
  assert.match(component, /if \(validationErrors\.value\.length\) return/)
  assert.match(component, /subject\.trim\(\)/)
  assert.doesNotMatch(component, /v-html/)
  assert.match(component, /whitespace-pre-wrap/)
})

test('order email editor only resets the selected template from its props', () => {
  assert.match(component, /resetCurrentTemplate/)
  assert.doesNotMatch(component, /resetOrderEmailTemplateSettings/)
})

test('preview mirrors the real branded order email and receives live site branding', () => {
  assert.match(settings, /:brand="form\.brand"/)
  assert.match(component, /data-testid="order-email-brand-header"/)
  assert.match(component, /data-testid="order-email-order-details"/)
  assert.match(component, /data-testid="order-email-items"/)
  assert.match(component, /data-testid="order-email-delivery"/)
  assert.match(component, /data-testid="order-email-message"/)
  assert.match(component, /data-testid="order-email-notice"/)
  assert.match(component, /data-testid="order-email-footer"/)
  assert.match(component, /previewStatusTitle/)
  assert.match(component, /safeBrandLogo/)
})

test('module visibility defaults on, has switches, and is persisted in the update payload', () => {
  assert.match(component, /modules:\s*createDefaultModules\(\)/)
  assert.match(component, /const moduleKeys = \['header', 'order_details', 'items', 'delivery', 'instructions', 'message', 'notice', 'footer'\] as const/)
  assert.match(component, /data-testid="order-email-module-selector"/)
  assert.match(component, /v-model="form\.modules\[key\]"/)
  assert.match(component, /modules: \{ \.\.\.form\.modules \}/)
  assert.match(component, /props\.data\.modules\?\.\[key\] !== false/)
})

test('each email module controls its corresponding preview section', () => {
  assert.match(component, /v-if="form\.modules\.header" data-testid="order-email-brand-header"/)
  assert.match(component, /v-if="form\.modules\.order_details" data-testid="order-email-order-details"/)
  assert.match(component, /v-if="form\.modules\.items" data-testid="order-email-items"/)
  assert.match(component, /v-if="form\.modules\.delivery && hasDeliveryPreview" data-testid="order-email-delivery"/)
  assert.match(component, /v-if="form\.modules\.instructions && hasDeliveryPreview"/)
  assert.match(component, /v-if="form\.modules\.message" data-testid="order-email-message"/)
  assert.match(component, /v-if="form\.modules\.notice" data-testid="order-email-notice"/)
  assert.match(component, /v-if="form\.modules\.footer" data-testid="order-email-footer"/)
})

test('advanced HTML is localized, validated, saved, and previewed only in a sandboxed iframe', () => {
  assert.match(component, /custom_html: string/)
  assert.match(component, /custom_html_enabled: boolean/)
  assert.match(component, /data-testid="order-email-advanced"/)
  assert.match(component, /v-model="currentTemplate\.custom_html_enabled"/)
  assert.match(component, /v-model="currentTemplate\.custom_html"/)
  assert.match(component, /resetCurrentAdvancedHTML/)
  assert.match(component, /insertAdvancedVariable/)
  assert.match(component, /200 \* 1024/)
  assert.match(component, /script\|style\|link\|iframe\|object\|embed\|form/)
  assert.match(component, /javascript:.*data:/)
  assert.match(component, /image-set/)
  assert.match(component, /<iframe[^>]+sandbox=""[^>]+:srcdoc="advancedPreviewHTML"/)
  assert.doesNotMatch(component, /v-html/)
})

test('email layout modules can be selected and are persisted with backwards-compatible defaults', () => {
  assert.match(component, /data-testid="order-email-module-selector"/)
  for (const module of ['header', 'order_details', 'items', 'delivery', 'instructions', 'message', 'notice', 'footer']) {
    assert.match(component, new RegExp(`${module}: true`))
    assert.match(component, new RegExp(`form\\.modules\\.${module}`))
  }
  assert.match(component, /modules: \{ \.\.\.form\.modules \}/)
})
