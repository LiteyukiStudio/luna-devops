import { beforeEach, describe, expect, it } from 'vitest'
import { applySiteBrandColorPreset, applyUserBrandColorPreference, brandColorPresets, brandColorUsesDarkForeground, brandThemeSwatchBackground, brandThemeSwatchColors, clearActiveUserBrandColorPreference, defaultBrandColorPreset, normalizeBrandColorPreset, normalizeUserBrandColorPreference } from './brand-theme'

describe('brand theme presets', () => {
  beforeEach(() => {
    localStorage.clear()
    delete document.documentElement.dataset.brandTheme
  })

  it('uses one accepted catalog for the curated picker', () => {
    expect(brandColorPresets).toEqual([
      'aurora',
      'harbor',
      'sunset',
      'botanical',
      'meadow',
      'citrus',
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
    ])
  })

  it('falls back to the default for unknown values', () => {
    expect(normalizeBrandColorPreset(' Teal ')).toBe('teal')
    expect(normalizeBrandColorPreset('custom-css')).toBe(defaultBrandColorPreset)
  })

  it('uses a dark foreground for the retained bright solid scale', () => {
    expect(brandColorPresets.filter(brandColorUsesDarkForeground)).toEqual(['lime'])
  })

  it('describes composite and single-color swatches consistently', () => {
    expect(brandThemeSwatchColors('aurora')).toEqual(['#3b6fe8', '#7867d9', '#2e9eaa', '#e7a63a'])
    expect(brandThemeSwatchColors('botanical')?.[0]).toBe('#0d4336')
    expect(brandThemeSwatchColors('blue')).toBeNull()
    expect(brandThemeSwatchBackground('aurora')).toBe('#3b6fe8')
    expect(brandThemeSwatchBackground('blue')).toBe('var(--blue-9)')
  })

  it('keeps an empty user preference as platform inheritance', () => {
    expect(normalizeUserBrandColorPreference('')).toBe('')
    expect(normalizeUserBrandColorPreference(' Teal ')).toBe('teal')
    expect(normalizeUserBrandColorPreference(' Ruby ')).toBe('')
    expect(normalizeUserBrandColorPreference('custom-css')).toBe('')
  })

  it('applies user preference before site preference and restores the site preference', () => {
    applySiteBrandColorPreset('blue')
    applyUserBrandColorPreference('usr_theme', 'teal')
    expect(document.documentElement.dataset.brandTheme).toBe('teal')

    applySiteBrandColorPreset('red')
    expect(document.documentElement.dataset.brandTheme).toBe('teal')

    applyUserBrandColorPreference('usr_theme', '')
    expect(document.documentElement.dataset.brandTheme).toBe('red')

    clearActiveUserBrandColorPreference()
    expect(document.documentElement.dataset.brandTheme).toBe('red')
  })
})
