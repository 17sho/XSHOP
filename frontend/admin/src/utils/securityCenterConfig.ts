export const securityCenterKeys = ['telegram_binding', 'google_binding', 'email_change', 'password_change', 'two_factor', 'login_history'] as const
export type SecurityCenterConfig = Record<(typeof securityCenterKeys)[number], boolean>
export type SecurityCenterSettingsPayload = { key: 'security_center_config'; value: SecurityCenterConfig & Record<string, unknown> }

export function normalizeSecurityCenterSettings(raw: Record<string, unknown>): SecurityCenterConfig {
  return Object.fromEntries(securityCenterKeys.map(key => [key, raw[key] === undefined ? true : raw[key] === true])) as SecurityCenterConfig
}
