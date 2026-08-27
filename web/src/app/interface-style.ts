import { safeStorageGet, safeStorageRemove, safeStorageSet } from '@/lib/safe-storage'

export type InterfaceStyle = 'minimal' | 'themed'
export type UserInterfaceStylePreference = '' | InterfaceStyle

const siteMinimalModeStorageKey = 'luna-devops-site-minimal-mode'
const activeUserStorageKey = 'luna-devops-interface-style-active-user'
const userStyleStoragePrefix = 'luna-devops-user-interface-style:'

export function applySiteMinimalModeDefault(value: unknown) {
  const minimal = String(value).trim().toLowerCase() === 'true'
  safeStorageSet(siteMinimalModeStorageKey, String(minimal))
  return applyInterfaceStyle(readActiveUserPreference() || (minimal ? 'minimal' : 'themed'))
}

export function applyUserInterfaceStylePreference(userId: string, value: unknown) {
  const normalizedUserId = userId.trim()
  const preference = normalizeUserInterfaceStylePreference(value)
  if (!normalizedUserId)
    return applyInterfaceStyle(readSiteDefault())

  safeStorageSet(activeUserStorageKey, normalizedUserId)
  if (preference)
    safeStorageSet(userStorageKey(normalizedUserId), preference)
  else
    safeStorageRemove(userStorageKey(normalizedUserId))

  return applyInterfaceStyle(preference || readSiteDefault())
}

export function clearActiveUserInterfaceStylePreference() {
  safeStorageRemove(activeUserStorageKey)
  return applyInterfaceStyle(readSiteDefault())
}

export function normalizeUserInterfaceStylePreference(value: unknown): UserInterfaceStylePreference {
  return value === 'minimal' || value === 'themed' ? value : ''
}

function applyInterfaceStyle(style: InterfaceStyle) {
  document.documentElement.dataset.interfaceStyle = style
  return style
}

function readActiveUserPreference() {
  const userId = safeStorageGet(activeUserStorageKey)
  return userId ? normalizeUserInterfaceStylePreference(safeStorageGet(userStorageKey(userId))) : ''
}

function readSiteDefault(): InterfaceStyle {
  return safeStorageGet(siteMinimalModeStorageKey) === 'true' ? 'minimal' : 'themed'
}

function userStorageKey(userId: string) {
  return `${userStyleStoragePrefix}${userId}`
}
