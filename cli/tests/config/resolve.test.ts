import { describe, expect, it } from 'vitest'

import {
  emptyConfigDocument,
  redactConfig,
  redactText,
  redactUrl,
  resolveRuntimeContext,
} from '../../src/config/index.js'

function configFixture() {
  const config = emptyConfigDocument()
  config.server = 'https://work.example.com'
  config.credential = {
    type: 'oauth',
    accessToken: 'access-secret',
    refreshToken: 'refresh-secret',
    scopes: ['project:read'],
  }
  config.project = { id: 'prj_config', name: 'Configured Project' }
  config.output = 'table'
  config.language = 'zh-CN'
  return config
}

describe('resolveRuntimeContext', () => {
  it('uses deterministic argument, environment, and config precedence', () => {
    const resolved = resolveRuntimeContext(configFixture(), {
      project: 'prj_argument',
      env: {
        LUNA_PROJECT: 'prj_environment',
        LUNA_OUTPUT: 'json',
        LUNA_LANG: 'en-US',
      },
    })

    expect(resolved.project).toEqual({ id: 'prj_argument' })
    expect(resolved.output).toBe('json')
    expect(resolved.language).toBe('en-US')
    expect(resolved.sources).toMatchObject({
      project: 'argument',
      output: 'environment',
      language: 'environment',
      credential: 'config',
    })
  })

  it('does not reuse persisted credentials or projects across origins', () => {
    const resolved = resolveRuntimeContext(configFixture(), {
      server: 'https://other.example.com',
      env: {},
    })

    expect(resolved.server).toBe('https://other.example.com')
    expect(resolved.credential).toBeUndefined()
    expect(resolved.project).toBeUndefined()
  })

  it('allows an explicit process token for a temporary server', () => {
    const resolved = resolveRuntimeContext(configFixture(), {
      server: 'https://other.example.com',
      env: { LUNA_TOKEN: 'temporary-secret', LUNA_PROJECT: 'prj_explicit' },
    })

    expect(resolved.credential).toMatchObject({
      type: 'access_token',
      token: 'temporary-secret',
    })
    expect(resolved.sources.credential).toBe('environment')
  })

  it('uses the official server for a new configuration', () => {
    const resolved = resolveRuntimeContext(emptyConfigDocument(), { env: {} })

    expect(resolved.server).toBe('https://devops.liteyuki.org')
    expect(resolved.credential).toBeUndefined()
  })
})

describe('redaction', () => {
  it('redacts credentials, sensitive URLs, and known secret strings', () => {
    const redacted = redactConfig(configFixture())
    expect(JSON.stringify(redacted)).not.toContain('access-secret')
    expect(JSON.stringify(redacted)).not.toContain('refresh-secret')
    expect(redactUrl('https://example.com/callback?code=abc&state=visible')).toContain(
      'code=%5BREDACTED%5D',
    )
    expect(redactUrl('https://example.com/callback?code=abc&state=visible')).toContain(
      'state=%5BREDACTED%5D',
    )
    expect(redactText('request failed with abc', ['abc'])).toBe(
      'request failed with [REDACTED]',
    )
  })
})
