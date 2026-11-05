import { safeStorageGet, safeStorageRemove, safeStorageSet } from '@/lib/safe-storage'

export const defaultBrandColorPreset = 'blue'
export const siteBrandColorPresetStorageKey = 'luna-devops-site-brand-color-preset'
export const activeThemeUserStorageKey = 'luna-devops-theme-active-user'
export const userBrandColorPresetStoragePrefix = 'luna-devops-user-brand-color-preset:'

// This catalog is both the accepted contract and the settings picker source.
export const brandColorPresets = ['blue', 'violet', 'teal', 'green', 'orange', 'pink'] as const

export type BrandColorPreset = typeof brandColorPresets[number]
export type UserBrandColorPreference = BrandColorPreset | ''

const brandColorPresetSet = new Set<string>(brandColorPresets)

export function normalizeBrandColorPreset(value: unknown): BrandColorPreset {
  const normalized = String(value ?? '').trim().toLowerCase()
  return brandColorPresetSet.has(normalized) ? normalized as BrandColorPreset : defaultBrandColorPreset
}

export function normalizeUserBrandColorPreference(value: unknown): UserBrandColorPreference {
  const normalized = String(value ?? '').trim().toLowerCase()
  return brandColorPresetSet.has(normalized) ? normalized as BrandColorPreset : ''
}

export function applyBrandColorPreset(value: unknown) {
  const preset = normalizeBrandColorPreset(value)
  document.documentElement.dataset.brandTheme = preset
  return preset
}

export function applySiteBrandColorPreset(value: unknown) {
  const sitePreset = normalizeBrandColorPreset(value)
  safeStorageSet(siteBrandColorPresetStorageKey, sitePreset)
  return applyBrandColorPreset(readActiveUserBrandColorPreference() || sitePreset)
}

export function applyUserBrandColorPreference(userId: string, value: unknown) {
  const normalizedUserId = userId.trim()
  const preference = normalizeUserBrandColorPreference(value)
  if (!normalizedUserId)
    return applyBrandColorPreset(readSiteBrandColorPreset())

  safeStorageSet(activeThemeUserStorageKey, normalizedUserId)
  if (preference)
    safeStorageSet(userBrandColorPresetStorageKey(normalizedUserId), preference)
  else
    safeStorageRemove(userBrandColorPresetStorageKey(normalizedUserId))

  return applyBrandColorPreset(preference || readSiteBrandColorPreset())
}

export function clearActiveUserBrandColorPreference() {
  safeStorageRemove(activeThemeUserStorageKey)
  return applyBrandColorPreset(readSiteBrandColorPreset())
}

export function userBrandColorPresetStorageKey(userId: string) {
  return `${userBrandColorPresetStoragePrefix}${userId}`
}

function readActiveUserBrandColorPreference() {
  const userId = safeStorageGet(activeThemeUserStorageKey)
  return userId ? normalizeUserBrandColorPreference(safeStorageGet(userBrandColorPresetStorageKey(userId))) : ''
}

function readSiteBrandColorPreset() {
  return normalizeBrandColorPreset(safeStorageGet(siteBrandColorPresetStorageKey))
}
