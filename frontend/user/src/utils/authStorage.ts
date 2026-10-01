import { getBrowserStorage, readStorageItem, writeStorageItem, removeStorageItem } from './browserStorage'

// Shared by auth and the API client. Failed writes/removals must shadow any
// older persisted credential; storage-denied sessions intentionally stay in RAM.
const fallback = new Map<string, string | null>()
export const readAuthStorage = (key: string): string | null => {
  if (fallback.has(key)) return fallback.get(key) ?? null
  return readStorageItem(getBrowserStorage('localStorage'), key)
}
export const writeAuthStorage = (key: string, value: string): void => {
  if (writeStorageItem(getBrowserStorage('localStorage'), key, value)) fallback.delete(key)
  else fallback.set(key, value)
}
export const removeAuthStorage = (key: string): void => {
  if (removeStorageItem(getBrowserStorage('localStorage'), key)) fallback.delete(key)
  else fallback.set(key, null)
}
