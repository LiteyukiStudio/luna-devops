import { describe, expect, it } from 'vitest'
import {
  assertServerCompatibility,
  CliCommandError,
  isVersionAtLeast,
} from '../../src/commands/index.js'

const meta = {
  apiVersion: 'v1',
  serverVersion: '0.1.0',
  openapiDigest: 'sha256:contract',
  minimumCliVersion: '0.0.7',
  features: {},
}

describe('server compatibility', () => {
  it('accepts a supported API version, CLI version, and contract digest', () => {
    expect(() => assertServerCompatibility(meta, {
      cliVersion: '0.0.7',
      openapiDigest: 'sha256:contract',
    })).not.toThrow()
  })

  it('accepts the source development version during local development', () => {
    expect(() => assertServerCompatibility(meta, {
      cliVersion: '0.0.0-development',
      openapiDigest: 'sha256:contract',
    })).not.toThrow()
  })

  it.each([
    {
      expectedCode: 'server_api_version_unsupported',
      value: { ...meta, apiVersion: 'v2' },
      requirements: {
        cliVersion: '0.0.7',
        openapiDigest: 'sha256:contract',
      },
    },
    {
      expectedCode: 'cli_version_too_old',
      value: meta,
      requirements: {
        cliVersion: '0.0.6',
        openapiDigest: 'sha256:contract',
      },
    },
    {
      expectedCode: 'openapi_digest_mismatch',
      value: meta,
      requirements: {
        cliVersion: '0.0.7',
        openapiDigest: 'sha256:different',
      },
    },
  ])('rejects $expectedCode', ({ expectedCode, requirements, value }) => {
    try {
      assertServerCompatibility(value, requirements)
      throw new Error('expected compatibility validation to fail')
    }
    catch (error) {
      expect(error).toBeInstanceOf(CliCommandError)
      expect((error as CliCommandError).code).toBe(expectedCode)
    }
  })

  it('compares release and prerelease versions by their SemVer core', () => {
    expect(isVersionAtLeast('0.0.7', '0.0.7')).toBe(true)
    expect(isVersionAtLeast('0.1.0-beta.1', '0.0.7')).toBe(true)
    expect(isVersionAtLeast('0.0.6', '0.0.7')).toBe(false)
    expect(isVersionAtLeast('development', '0.0.7')).toBeUndefined()
  })
})
