import { describe, expect, it } from 'vitest'
import { matchRuntimeSizePreset } from './resource-units'

describe('runtime size presets', () => {
  it('matches presets by quantity regardless of unit spelling', () => {
    expect(matchRuntimeSizePreset('250m', '256Mi')).toBe('s')
    expect(matchRuntimeSizePreset('0.5', '512Mi')).toBe('m')
    expect(matchRuntimeSizePreset('1000m', '1024Mi')).toBe('l')
  })

  it('treats any other combination as custom', () => {
    expect(matchRuntimeSizePreset('500m', '1Gi')).toBeUndefined()
    expect(matchRuntimeSizePreset('', '')).toBeUndefined()
  })
})
