import { describe, expect, it } from 'vitest'
import { publicRuntimeEnvironmentInputs, publicRuntimeEnvironmentRecord, runtimeSecretKeys } from './runtime-environment'

describe('runtime environment contract', () => {
  it('serializes public values with an explicit value mode', () => {
    expect(publicRuntimeEnvironmentInputs({ LOG_LEVEL: 'debug', APP_ENV: 'prod' })).toEqual([
      { key: 'APP_ENV', value: 'prod', valueMode: 'public' },
      { key: 'LOG_LEVEL', value: 'debug', valueMode: 'public' },
    ])
  })

  it('preserves both modes for an overlapping key without copying secret values', () => {
    const variables = [
      { configured: true, key: 'LOG_LEVEL', value: 'debug', valueMode: 'public' as const },
      { configured: true, key: 'TOKEN', value: 'public-fallback', valueMode: 'public' as const },
      { configured: true, key: 'TOKEN', valueMode: 'secret' as const },
    ]
    expect(publicRuntimeEnvironmentRecord(variables)).toEqual({ LOG_LEVEL: 'debug', TOKEN: 'public-fallback' })
    expect(runtimeSecretKeys(variables)).toEqual(['TOKEN'])
  })
})
