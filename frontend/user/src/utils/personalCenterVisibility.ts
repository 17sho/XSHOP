export type PersonalCenterVisibilityKey = 'overview' | 'orders' | 'wallet' | 'affiliate' | 'reseller' | 'gift_cards' | 'security' | 'api' | 'profile'
export type PersonalCenterVisibility = Record<PersonalCenterVisibilityKey, boolean>

export const PERSONAL_CENTER_SECTION_KEYS: PersonalCenterVisibilityKey[] = [
  'overview', 'orders', 'wallet', 'affiliate', 'reseller', 'gift_cards', 'security', 'api', 'profile',
]

export const DEFAULT_PERSONAL_CENTER_VISIBILITY: PersonalCenterVisibility = {
  overview: true,
  orders: true,
  wallet: true,
  affiliate: true,
  reseller: true,
  gift_cards: true,
  security: true,
  api: true,
  profile: true,
}

export const PERSONAL_CENTER_SECTION_TO_COMPONENT = {
  overview: 'overview',
  orders: 'orders',
  wallet: 'wallet',
  affiliate: 'affiliate',
  reseller: 'reseller',
  gift_cards: 'giftCard',
  security: 'security',
  api: 'api',
  profile: 'profile',
} as const

export const PERSONAL_CENTER_SECTION_PATHS: Record<PersonalCenterVisibilityKey, string> = {
  overview: '/me',
  orders: '/me/orders',
  wallet: '/me/wallet',
  affiliate: '/me/affiliate',
  reseller: '/reseller',
  gift_cards: '/me/gift-cards',
  security: '/me/security',
  api: '/me/api',
  profile: '/me/profile',
}

export const normalizePersonalCenterVisibility = (raw: unknown): PersonalCenterVisibility => {
  const source = raw && typeof raw === 'object' ? raw as Record<string, unknown> : {}
  return PERSONAL_CENTER_SECTION_KEYS.reduce((result, key) => {
    result[key] = typeof source[key] === 'boolean' ? source[key] as boolean : true
    return result
  }, { ...DEFAULT_PERSONAL_CENTER_VISIBILITY })
}

export const firstVisiblePersonalCenterPath = (visibility: PersonalCenterVisibility): string => {
  const first = PERSONAL_CENTER_SECTION_KEYS.find((key) => visibility[key])
  return first ? PERSONAL_CENTER_SECTION_PATHS[first] : '/'
}

export const resolvePersonalCenterFallback = (
  requested: PersonalCenterVisibilityKey,
  visibility: PersonalCenterVisibility,
): string | null => visibility[requested] ? null : firstVisiblePersonalCenterPath(visibility)
