export interface HomepageAdCacheStorage {
  getItem(key: string): string | null
  setItem(key: string, value: string): void
  removeItem(key: string): void
}

export interface CachedHomepageAd {
  title?: Record<string, string>
  content?: Record<string, string>
}

const cacheKey = (host: string) => `dujiao:homepage-ad:${String(host || '').trim().toLowerCase()}`

const isHomepageAd = (value: unknown): value is CachedHomepageAd => {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const ad = value as CachedHomepageAd
  return !!(ad.title && typeof ad.title === 'object') || !!(ad.content && typeof ad.content === 'object')
}

export const readCachedHomepageAd = (
  storage: HomepageAdCacheStorage | null | undefined,
  host: string,
): CachedHomepageAd | null => {
  if (!storage || String(host || '').trim() === '') return null
  try {
    const raw = storage.getItem(cacheKey(host))
    if (!raw) return null
    const parsed = JSON.parse(raw)
    if (!isHomepageAd(parsed)) {
      storage.removeItem(cacheKey(host))
      return null
    }
    return parsed
  } catch {
    try { storage.removeItem(cacheKey(host)) } catch { /* storage unavailable */ }
    return null
  }
}

export const writeCachedHomepageAd = (
  storage: HomepageAdCacheStorage | null | undefined,
  host: string,
  ad: CachedHomepageAd | null | undefined,
): void => {
  if (!storage || String(host || '').trim() === '') return
  try {
    if (!isHomepageAd(ad)) {
      storage.removeItem(cacheKey(host))
      return
    }
    storage.setItem(cacheKey(host), JSON.stringify(ad))
  } catch {
    // The page must keep working when Safari private mode rejects storage.
  }
}
