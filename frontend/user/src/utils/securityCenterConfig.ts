export const securityCenterKeys = ['telegram_binding', 'google_binding', 'email_change', 'password_change', 'two_factor', 'login_history'] as const
export type SecurityCenterKey = (typeof securityCenterKeys)[number]
export type SecurityCenterConfig = Record<SecurityCenterKey, boolean>

/** Unknown public config is closed; a resolved legacy config defaults missing keys on. */
export function normalizeSecurityCenterConfig(config: unknown): SecurityCenterConfig {
  const resolved = !!config && typeof config === 'object' && !Array.isArray(config)
  const raw = resolved ? (config as Record<string, unknown>).security_center_config : null
  const legacy = resolved && raw === undefined
  const valid = !!raw && typeof raw === 'object' && !Array.isArray(raw)
  const data = valid ? raw as Record<string, unknown> : {}
  return Object.fromEntries(securityCenterKeys.map(key => [key,
    legacy || (valid && data[key] === undefined) ? true : data[key] === true,
  ])) as SecurityCenterConfig
}
