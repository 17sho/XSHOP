import { isSafeRegistrationHTML, registrationHTMLTokensInBody } from './registrationEmailHTML'

export const registrationLanguages = ['zh-CN', 'zh-TW', 'en-US'] as const
export type RegistrationLanguage = (typeof registrationLanguages)[number]
export type RegistrationTemplate = { subject: string; body: string; custom_html: string; custom_html_enabled: boolean }
export const verificationScenes = ['registration', 'reset', 'telegram_bind', 'change_email_old', 'change_email_new'] as const
export type VerificationScene = (typeof verificationScenes)[number]
export type VerificationAdditionalScene = Exclude<VerificationScene, 'registration'>
export type RegistrationEmailSettings = { templates: Record<RegistrationLanguage, RegistrationTemplate>; scenes?: Record<VerificationAdditionalScene, Record<RegistrationLanguage, RegistrationTemplate>> }
export const verificationTemplates = (data: RegistrationEmailSettings, scene: VerificationScene) => scene === 'registration' ? data.templates : data.scenes?.[scene]
export const registrationVariables = ['code', 'expire_minutes', 'site_name', 'site_url'] as const
const variablePattern = /{{(code|expire_minutes|site_name|site_url)}}/g
export const escapeEmailHTML = (value: string) => value.replace(/[&<>"']/g, char => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[char]!)
export const renderRegistrationText = (value: string, variables: Record<string, string>) => value.replace(variablePattern, (token, key: string) => variables[key.trim()] ?? token)

export function validateRegistrationTemplates(data: RegistrationEmailSettings) {
  const errors: { lang: RegistrationLanguage; reason: string }[] = []
  for (const lang of registrationLanguages) {
    const template = data.templates[lang]
    const add = (reason: string) => errors.push({ lang, reason })
    if (!template.subject.trim() || /[\r\n]/.test(template.subject)) add('subjectRequired')
    const values = [template.subject, template.body, template.custom_html]
    if (values.some(value => /{{|}}/.test(value.replace(variablePattern, '')))) add('unknownVariables')
    for (const value of [template.body, ...(template.custom_html_enabled ? [template.custom_html] : [])]) {
      const keys = [...value.matchAll(variablePattern)].map(match => match[1]!.trim())
      if (!keys.includes('code') || !keys.includes('expire_minutes')) add('requiredTokens')
    }
    if (template.custom_html_enabled && !registrationHTMLTokensInBody(template.custom_html, true)) add('requiredTokens')
    if (new TextEncoder().encode(template.custom_html).length > 200 * 1024) add('htmlTooLarge')
    else if (!isSafeRegistrationHTML(template.custom_html)) add('unsafeHtml')
  }
  return errors
}

export { registrationPreviewDocument } from './registrationEmailPreview'

export { isSafeRegistrationHTML } from './registrationEmailHTML'
