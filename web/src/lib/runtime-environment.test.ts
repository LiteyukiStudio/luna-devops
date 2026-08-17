import { describe, expect, it } from 'vitest'
import { publicRuntimeEnvironmentInputs, publicRuntimeEnvironmentRecord, runtimeSecretKeys } from './runtime-environment'

describe('runtime environment contract', () => {
  it('serializes public values with an explicit value mode', () => {
    expect(publicRuntimeEnvironmentInputs({ LOG_LEVEL: 'debug', APP_ENV: 'prod' })).toEqual([
      { key: 'APP_ENV', value: 'prod', valueMode: 'public' },
      { key: 'LOG_LEVEL', value: 'debug', valueMode: 'public' },
    ])
  })

  it('never copies secret entries into a public payload', () => {
    const variables = [
      { configured: true, key: 'LOG_LEVEL', value: 'debug', valueMode: 'public' as const },
      { configured: true, key: 'TOKEN', valueMode: 'secret' as const },
    ]
    expect(publicRuntimeEnvironmentRecord(variables)).toEqual({ LOG_LEVEL: 'debug' })
    expect(runtimeSecretKeys(variables)).toEqual(['TOKEN'])
  })
})
