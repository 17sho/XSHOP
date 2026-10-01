import { describe, expect, it, vi } from 'vitest'
import { readFileSync } from 'node:fs'
import { adminAPI } from './admin'
const client = vi.hoisted(() => ({ get: vi.fn(), put: vi.fn() }))
vi.mock('./client', () => ({ api: client }))

describe('registration email settings integration', () => {
  it('uses only the independent registration endpoint for GET and PUT', async () => {
    const payload = { templates: {} }
    await adminAPI.getRegistrationEmailTemplateSettings()
    await adminAPI.updateRegistrationEmailTemplateSettings(payload)
    expect(client.get).toHaveBeenCalledWith('/admin/settings/registration-email-template')
    expect(client.put).toHaveBeenCalledWith('/admin/settings/registration-email-template', payload)
  })
  it('keeps the parent language, live brand/SMTP expiry and sole save action with fail-closed gating', () => {
    const parent = readFileSync('src/views/admin/Settings.vue', 'utf8')
    const child = readFileSync('src/views/admin/components/SettingsRegistrationEmailTemplateTab.vue', 'utf8')
    expect(parent).toContain("value: 'email_templates'")
    expect(parent).toContain("currentTab.value === 'email_templates'")
    expect(parent).toContain('registrationEmailTemplateTabRef.value?.save()')
    expect(parent).toContain('!!registrationEmailTemplateTabRef.value?.loaded && !registrationEmailTemplateTabRef.value?.loadFailed')
    expect(parent).toContain('registrationEmailTemplateTabRef.value?.submitting')
    expect(parent).toMatch(/<SettingsRegistrationEmailTemplateTab[^>]*:current-lang="currentLang"[^>]*:brand="form.brand"[^>]*:expire-minutes="smtpData.verify_code.expire_minutes"/)
    expect(parent).toMatch(/<TabsContent value="email_templates" :forceMount="true"/)
    expect(parent).not.toContain('getRegistrationEmailTemplateSettings()')
    expect(child).not.toMatch(/@click="save"|v-html|send.*[Cc]ode|testSMTPSettings/)
  })
})
