export const defaultBrandColorPreset = 'blue'
export const siteBrandColorPresetStorageKey = 'luna-devops-site-brand-color-preset'
export const activeThemeUserStorageKey = 'luna-devops-theme-active-user'
export const userBrandColorPresetStoragePrefix = 'luna-devops-user-brand-color-preset:'

export const compositeBrandColorPresets = ['aurora', 'harbor', 'sunset', 'botanical', 'meadow', 'citrus'] as const

// This catalog is both the accepted contract and the settings picker source.
export const brandColorPresets = [
  ...compositeBrandColorPresets,
  'gold',
  'orange',
  'red',
  'pink',
  'violet',
  'blue',
  'cyan',
  'teal',
  'green',
  'lime',
] as const

export type BrandColorPreset = typeof brandColorPresets[number]
export type UserBrandColorPreference = BrandColorPreset | ''

const brandColorPresetSet = new Set<string>(brandColorPresets)
const compositeBrandColorPresetSet = new Set<string>(compositeBrandColorPresets)

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
  writeStorage(siteBrandColorPresetStorageKey, sitePreset)
  return applyBrandColorPreset(readActiveUserBrandColorPreference() || sitePreset)
}

export function applyUserBrandColorPreference(userId: string, value: unknown) {
  const normalizedUserId = userId.trim()
  const preference = normalizeUserBrandColorPreference(value)
  if (!normalizedUserId)
    return applyBrandColorPreset(readSiteBrandColorPreset())

  writeStorage(activeThemeUserStorageKey, normalizedUserId)
  if (preference)
    writeStorage(userBrandColorPresetStorageKey(normalizedUserId), preference)
  else
    removeStorage(userBrandColorPresetStorageKey(normalizedUserId))

  return applyBrandColorPreset(preference || readSiteBrandColorPreset())
}

export function clearActiveUserBrandColorPreference() {
  removeStorage(activeThemeUserStorageKey)
  return applyBrandColorPreset(readSiteBrandColorPreset())
}

export function userBrandColorPresetStorageKey(userId: string) {
  return `${userBrandColorPresetStoragePrefix}${userId}`
}

export function brandColorUsesDarkForeground(preset: BrandColorPreset) {
  return preset === 'lime'
}

export function brandThemeIsComposite(preset: BrandColorPreset) {
  return compositeBrandColorPresetSet.has(preset)
}

export function brandThemeSwatchColors(preset: BrandColorPreset): readonly [string, string, string, string] | null {
  switch (preset) {
    case 'aurora':
      return ['#3b6fe8', '#7867d9', '#2e9eaa', '#e7a63a']
    case 'harbor':
      return ['#147d73', '#47709b', '#ce8a62', '#e7a63a']
    case 'sunset':
      return ['#eb7a53', '#f7d44c', '#7c5cc4', '#d84a8c']
    case 'botanical':
      return ['#0d4336', '#ce8a62', '#147d73', '#eb7a53']
    case 'meadow':
      return ['#5cab8c', '#bdee63', '#86ead4', '#f7d44c']
    case 'citrus':
      return ['#eb7a53', '#f7d44c', '#f6ecc9', '#e5484d']
    default:
      return null
  }
}

export function brandThemeSwatchBackground(preset: BrandColorPreset) {
  return brandThemeSwatchColors(preset)?.[0] ?? `var(--${preset}-9)`
}

function readActiveUserBrandColorPreference() {
  const userId = readStorage(activeThemeUserStorageKey)
  return userId ? normalizeUserBrandColorPreference(readStorage(userBrandColorPresetStorageKey(userId))) : ''
}

function readSiteBrandColorPreset() {
  return normalizeBrandColorPreset(readStorage(siteBrandColorPresetStorageKey))
}

function readStorage(key: string) {
  try {
    return localStorage.getItem(key)
  }
  catch {
    return null
  }
}

function writeStorage(key: string, value: string) {
  try {
    localStorage.setItem(key, value)
  }
  catch {
    // The current DOM theme remains usable when browser storage is unavailable.
  }
}

function removeStorage(key: string) {
  try {
    localStorage.removeItem(key)
  }
  catch {
    // The current DOM theme remains usable when browser storage is unavailable.
  }
}
