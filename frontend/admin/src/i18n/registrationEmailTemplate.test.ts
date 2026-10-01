import { describe, expect, it } from 'vitest'
import i18n from './index'

describe('registration email localization', () => {
  it.each(['zh-CN', 'zh-TW', 'en-US'] as const)('registers complete editor copy for %s without fallback', locale => {
    const keys = ['title', 'subtitle', 'loading', 'loadFailed', 'retry', 'editingLanguage', 'unsaved', 'revert', 'subject', 'body', 'bodyHint', 'variables', 'variableHint', 'advanced', 'advancedHint', 'enableHtml', 'html', 'bytes', 'preview', 'desktop', 'mobile', 'synthetic', 'htmlPreview', 'plainPreview', 'leaveConfirm', 'variableList.code', 'variableList.expire_minutes', 'variableList.site_name', 'variableList.site_url', 'validation.subjectRequired', 'validation.unknownVariables', 'validation.requiredTokens', 'validation.htmlTooLarge', 'validation.unsafeHtml']
    expect(i18n.global.te('admin.settings.tabs.registrationEmailTemplate', locale)).toBe(true)
    for (const key of ['verification', 'orders', 'scenes.registration', 'scenes.reset', 'scenes.telegram_bind', 'scenes.change_email_old', 'scenes.change_email_new', 'scenes.order']) {
      expect(i18n.global.te(`admin.settings.emailTemplates.${key}`, locale)).toBe(true)
    }
    expect(i18n.global.te('admin.settings.tabs.emailTemplates', locale)).toBe(true)
    for (const key of keys) {
      const path = `admin.settings.registrationEmailTemplate.${key}`
      expect(i18n.global.te(path, locale), path).toBe(true)
      expect(i18n.global.t(path, { language: 'en-US', minutes: 17 }, { locale })).not.toBe(path)
    }
  })
})
