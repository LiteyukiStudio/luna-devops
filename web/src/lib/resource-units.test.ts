import { describe, expect, it } from 'vitest'
import { isValidResourceQuantity, matchRuntimeSizePreset } from './resource-units'

describe('resource units', () => {
  it('matches runtime size presets by quantity regardless of unit spelling', () => {
    expect(matchRuntimeSizePreset('250m', '256Mi')).toBe('s')
    expect(matchRuntimeSizePreset('0.5', '512Mi')).toBe('m')
    expect(matchRuntimeSizePreset('1000m', '1024Mi')).toBe('l')
  })

  it('treats any other combination as custom', () => {
    expect(matchRuntimeSizePreset('500m', '1Gi')).toBeUndefined()
    expect(matchRuntimeSizePreset('', '')).toBeUndefined()
  })

  it('validates positive integer and decimal Kubernetes resource quantities', () => {
    expect(isValidResourceQuantity('0.5')).toBe(true)
    expect(isValidResourceQuantity('250m')).toBe(true)
    expect(isValidResourceQuantity('1.5Gi')).toBe(true)
    expect(isValidResourceQuantity('512Mi')).toBe(true)
    expect(isValidResourceQuantity('2Ti')).toBe(true)

    expect(isValidResourceQuantity('')).toBe(false)
    expect(isValidResourceQuantity('0Gi')).toBe(false)
    expect(isValidResourceQuantity('1.')).toBe(false)
    expect(isValidResourceQuantity('1..5Gi')).toBe(false)
  })
})
