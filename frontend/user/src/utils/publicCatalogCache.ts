export interface PublicCatalogCacheStorage {
  getItem(key: string): string | null
  setItem(key: string, value: string): void
  removeItem(key: string): void
}

export interface PublicCatalogCacheValue {
  products: any[]
  categories: any[]
  totalPages: number
}

interface PublicCatalogCacheRecord extends PublicCatalogCacheValue {
  cachedAt: number
}

const MAX_AGE_MS = 5 * 60 * 1000
const cacheKey = (host: string) => `dujiao:public-catalog:${String(host || '').trim().toLowerCase()}`

const isRecord = (value: unknown): value is PublicCatalogCacheRecord => {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const record = value as PublicCatalogCacheRecord
  return Array.isArray(record.products)
    && Array.isArray(record.categories)
    && Number.isFinite(record.totalPages)
    && Number.isFinite(record.cachedAt)
}

export const readPublicCatalogCache = (
  storage: PublicCatalogCacheStorage | null | undefined,
  host: string,
  now = Date.now(),
): PublicCatalogCacheValue | null => {
  if (!storage || !String(host || '').trim()) return null
  try {
    const raw = storage.getItem(cacheKey(host))
    if (!raw) return null
    const parsed = JSON.parse(raw)
    if (!isRecord(parsed) || now - parsed.cachedAt > MAX_AGE_MS) {
      storage.removeItem(cacheKey(host))
      return null
    }
    return { products: parsed.products, categories: parsed.categories, totalPages: parsed.totalPages }
  } catch {
    try { storage.removeItem(cacheKey(host)) } catch { /* storage unavailable */ }
    return null
  }
}

export const writePublicCatalogCache = (
  storage: PublicCatalogCacheStorage | null | undefined,
  host: string,
  value: PublicCatalogCacheValue,
  now = Date.now(),
): void => {
  if (!storage || !String(host || '').trim()) return
  try {
    storage.setItem(cacheKey(host), JSON.stringify({ ...value, cachedAt: now }))
  } catch {
    // Cache is best effort; Safari private mode may reject writes.
  }
}
