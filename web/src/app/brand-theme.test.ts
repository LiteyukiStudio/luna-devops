import { beforeEach, describe, expect, it } from 'vitest'
import { applySiteBrandColorPreset, applyUserBrandColorPreference, clearActiveUserBrandColorPreference, defaultBrandColorPreset, normalizeBrandColorPreset, normalizeUserBrandColorPreference } from './brand-theme'

describe('brand theme presets', () => {
  beforeEach(() => {
    localStorage.clear()
    delete document.documentElement.dataset.brandTheme
  })

  it('falls back to the default for unknown values', () => {
    expect(normalizeBrandColorPreset(' Teal ')).toBe('teal')
    expect(normalizeBrandColorPreset('custom-css')).toBe(defaultBrandColorPreset)
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
