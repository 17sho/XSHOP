export interface CatalogNotice {
  id: number
  slug: string
  title: Record<string, string>
  summary: Record<string, string>
}
type Storage = Pick<globalThis.Storage, 'getItem' | 'setItem' | 'removeItem'>
const cacheKey = (host: string) => `dujiao:catalog-notices:v1:${host.trim().toLowerCase()}`
const localized = (value: unknown): value is Record<string, string> => !!value && typeof value === 'object' && !Array.isArray(value) && Object.values(value).every(text => typeof text === 'string' && text.length <= 10000)
export function validateCatalogNotices(value: unknown): CatalogNotice[] {
  if (!Array.isArray(value)) return []
  return value.slice(0, 2).filter(notice => notice && Number.isSafeInteger(notice.id) && notice.id > 0 && typeof notice.slug === 'string' && /^[\p{L}\p{N}_-]{1,200}$/u.test(notice.slug) && localized(notice.title) && (notice.summary == null || localized(notice.summary)))
    .map(({ id, slug, title, summary }) => ({ id, slug, title, summary: summary || {} }))
}
export function readCachedCatalogNotices(storage: Storage | null | undefined, host: string, now = Date.now()): CatalogNotice[] {
  if (!storage || !host.trim()) return []
  try {
    const record = JSON.parse(storage.getItem(cacheKey(host)) || 'null')
    if (!record || !Number.isFinite(record.cachedAt) || record.cachedAt > now || now - record.cachedAt > 300000) return []
    return validateCatalogNotices(record.notices)
  } catch { return [] }
}
export function clearCachedCatalogNotices(storage: Storage | null | undefined, host: string): void {
  if (!storage || !host.trim()) return
  try { storage.removeItem(cacheKey(host)) } catch { /* Optional public layout cache. */ }
}
export function writeCachedCatalogNotices(storage: Storage | null | undefined, host: string, notices: unknown, now = Date.now()): void {
  if (!storage || !host.trim()) return
  try { storage.setItem(cacheKey(host), JSON.stringify({ cachedAt: now, notices: validateCatalogNotices(notices) })) } catch { /* Optional public layout cache. */ }
}
