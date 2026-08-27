type StorageProvider = () => Storage

const localStorageProvider = () => window.localStorage

export function safeStorageGet(key: string, provider: StorageProvider = localStorageProvider) {
  try {
    return provider().getItem(key)
  }
  catch {
    return null
  }
}

export function safeStorageSet(key: string, value: string, provider: StorageProvider = localStorageProvider) {
  try {
    provider().setItem(key, value)
  }
  catch {}
}

export function safeStorageRemove(key: string, provider: StorageProvider = localStorageProvider) {
  try {
    provider().removeItem(key)
  }
  catch {}
}
