export interface BrowserStorage {
  getItem(key: string): string | null
  setItem(key: string, value: string): void
  removeItem(key: string): void
}

export type BrowserStorageKind = 'sessionStorage' | 'localStorage'

export const getBrowserStorage = (kind: BrowserStorageKind): BrowserStorage | null => {
  try {
    return typeof window === 'undefined' ? null : window[kind]
  } catch {
    return null
  }
}

export const readStorageItem = (storage: BrowserStorage | null | undefined, key: string): string | null => {
  if (!storage) return null
  try {
    return storage.getItem(key)
  } catch {
    return null
  }
}

export const writeStorageItem = (storage: BrowserStorage | null | undefined, key: string, value: string): boolean => {
  if (!storage) return false
  try {
    storage.setItem(key, value)
    return true
  } catch {
    return false
  }
}

export const removeStorageItem = (storage: BrowserStorage | null | undefined, key: string): boolean => {
  if (!storage) return false
  try {
    storage.removeItem(key)
    return true
  } catch {
    return false
  }
}
