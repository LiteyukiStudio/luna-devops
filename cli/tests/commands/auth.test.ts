import type { CommandExecutionGlobals, CommandResult, LunaConfigDocument, NormalizedCommandMetadata, RuntimePorts } from '../../src/commands/index.js'
import { describe, expect, it } from 'vitest'
import {

  CommandRegistry,
  createCliProgram,
  DefaultInputPort,
  registerLocalCommands,
  runCli,
} from '../../src/commands/index.js'

describe('auth commands', () => {
  it('uses OAuth Device Code for the root login shortcut by default', async () => {
    const harness = createHarness()
    const result = await runCli(harness.program, [
      'node',
      'luna',
      'login',
      'server=https://luna.example.com',
    ], harness.ports.output)

    expect(result.exitCode).toBe(0)
    expect(harness.oauthLogins).toEqual([{
      server: 'https://luna.example.com',
      scopes: [],
      mode: 'device_code',
    }])
    expect(harness.config.server).toBe('https://luna.example.com')
    expect(harness.config.credential).toMatchObject({
      type: 'oauth',
      accessToken: 'oauth-access-secret',
      refreshToken: 'oauth-refresh-secret',
    })
    expect(harness.infos).toEqual([
      'Enter this device code to authorize Luna CLI: LUNA-CODE',
      'A browser was opened. Complete authorization there; manual URL: https://luna.example.com/device',
    ])
    expect(harness.successes[0]?.result.data).toMatchObject({
      authenticated: true,
      server: 'https://luna.example.com',
      credentialType: 'oauth',
    })
    expect(JSON.stringify(harness.successes[0]?.result)).not.toContain('oauth-access-secret')
  })

  it('uses the official server and replaces the previous login when no server is provided', async () => {
    const harness = createHarness({
      config: {
        version: 2,
        server: 'https://custom.example.com',
        credential: {
          type: 'access_token',
          token: 'old-token',
          scopes: [],
        },
        project: { id: 'prj_custom' },
        language: '',
        output: '',
      },
    })

    const result = await runCli(harness.program, [
      'node',
      'luna',
      'login',
    ], harness.ports.output)

    expect(result.exitCode).toBe(0)
    expect(harness.oauthLogins).toEqual([{
      server: 'https://devops.liteyuki.org',
      scopes: [],
      mode: 'device_code',
    }])
    expect(harness.config.server).toBe('https://devops.liteyuki.org')
    expect(harness.config.project).toBeNull()
    expect(harness.config.credential?.type).toBe('oauth')
  })

  it('supports whoami and logout shortcuts through the canonical handlers', async () => {
    const harness = createHarness()
    await runCli(harness.program, [
      'node',
      'luna',
      'login',
      'server=https://luna.example.com',
    ], harness.ports.output)

    const status = await runCli(harness.program, [
      'node',
      'luna',
      'whoami',
    ], harness.ports.output)
    const logout = await runCli(harness.program, [
      'node',
      'luna',
      'logout',
      'yes=true',
    ], harness.ports.output)

    expect(status.exitCode).toBe(0)
    expect(logout.exitCode).toBe(0)
    expect(harness.successes.at(-2)?.result.data).toMatchObject({
      authenticated: true,
      server: 'https://luna.example.com',
    })
    expect(harness.successes.at(-1)?.result.data).toMatchObject({
      server: 'https://luna.example.com',
      loggedOut: true,
      remoteRevocation: 'succeeded',
    })
    expect(harness.revocations).toEqual([
      { token: 'oauth-refresh-secret', tokenTypeHint: 'refresh_token' },
      { token: 'oauth-access-secret', tokenTypeHint: 'access_token' },
    ])
    expect(harness.config.credential).toBeNull()
  })

  it('keeps the API adapter receiver when logout revokes OAuth credentials', async () => {
    const harness = createHarness()
    const api = harness.ports.api
    const receivers: unknown[] = []
    const revoked: string[] = []
    api.revokeOAuthCredential = async function (this: unknown, request: { token: string }) {
      receivers.push(this)
      revoked.push(request.token)
    }

    await runCli(harness.program, [
      'node',
      'luna',
      'login',
      'server=https://luna.example.com',
    ], harness.ports.output)
    const logout = await runCli(harness.program, [
      'node',
      'luna',
      'logout',
      'yes=true',
    ], harness.ports.output)

    expect(logout.exitCode).toBe(0)
    expect(receivers).toEqual([api, api])
    expect(revoked).toEqual([
      'oauth-refresh-secret',
      'oauth-access-secret',
    ])
  })

  it('fails device-code login before starting OAuth when the server disables it', async () => {
    const harness = createHarness({ deviceCode: false })
    const result = await runCli(harness.program, [
      'node',
      'luna',
      'login',
      'server=https://luna.example.com',
    ], harness.ports.output)

    expect(result.exitCode).not.toBe(0)
    expect(harness.errors[0]).toMatchObject({
      code: 'oauth_server_capability_unavailable',
      status: 501,
    })
  })

  it('does not persist a token when server validation fails', async () => {
    const harness = createHarness({ validationError: new Error('unauthorized') })
    const result = await runCli(harness.program, [
      'node',
      'luna',
      'auth',
      'login',
      'server=https://luna.example.com',
      'mode=access-token',
    ], harness.ports.output)

    expect(result.exitCode).toBe(8)
    expect(harness.config.credential).toBeNull()
  })

  it('trims surrounding whitespace before validating and storing a token', async () => {
    const harness = createHarness({ token: 'test-token\n' })
    const result = await runCli(harness.program, [
      'node',
      'luna',
      'auth',
      'login',
      'server=https://luna.example.com',
      'mode=access-token',
    ], harness.ports.output)

    expect(result.exitCode).toBe(0)
    expect(harness.validations).toEqual([{
      server: 'https://luna.example.com',
      token: 'test-token',
    }])
    expect(harness.config.credential && 'token' in harness.config.credential
      ? harness.config.credential.token
      : null).toBe('test-token')
  })

  it('uses an access token only when the fallback mode is explicit', async () => {
    const harness = createHarness()
    const result = await runCli(harness.program, [
      'node',
      'luna',
      'login',
      'mode=access-token',
      'server=https://luna.example.com',
    ], harness.ports.output)

    expect(result.exitCode).toBe(0)
    expect(harness.oauthLogins).toEqual([])
    expect(harness.validations).toEqual([{
      server: 'https://luna.example.com',
      token: 'test-token',
    }])
    expect(harness.config.credential).toMatchObject({
      type: 'access_token',
      token: 'test-token',
    })
  })
})

function createHarness(options: {
  validationError?: Error
  token?: string
  deviceCode?: boolean
  config?: LunaConfigDocument
} = {}) {
  let config: LunaConfigDocument = structuredClone(options.config ?? {
    version: 2,
    server: 'https://devops.liteyuki.org',
    credential: null,
    project: null,
    language: '',
    output: '',
  })
  const successes: Array<{
    metadata: NormalizedCommandMetadata
    result: CommandResult
    globals: CommandExecutionGlobals
  }> = []
  const validations: Array<{ server: string, token: string }> = []
  const oauthLogins: Array<{
    server: string
    scopes: readonly string[]
    mode: string
  }> = []
  const revocations: Array<{ token: string, tokenTypeHint?: string }> = []
  const infos: string[] = []
  const errors: unknown[] = []
  const ports: RuntimePorts = {
    config: {
      read: async () => config,
      write: async (next) => {
        config = structuredClone(next)
      },
    },
    input: new DefaultInputPort(),
    output: {
      writeSuccess(metadata, result, globals) {
        successes.push({ metadata, result, globals })
      },
      writeError(error) {
        errors.push(error)
      },
      writeInfo(message) {
        infos.push(message)
      },
    },
    api: {
      execute: async () => ({ data: {} }),
      request: async () => ({ data: {} }),
      async validateAccessToken(server, token) {
        validations.push({ server, token })
        if (options.validationError)
          throw options.validationError
        return { id: 'user-1', name: 'Test User' }
      },
      async getMeta() {
        return {
          apiVersion: 'v1',
          serverVersion: 'test',
          openapiDigest: 'test',
          minimumCliVersion: '0.1.0',
          features: {
            deviceCode: options.deviceCode ?? true,
          },
        }
      },
      async beginOAuthLogin(request) {
        oauthLogins.push({
          server: request.server,
          scopes: request.scopes,
          mode: request.mode,
        })
        await request.onVerification?.({
          userCode: 'LUNA-CODE',
          verificationUri: `${request.server}/device`,
          expiresIn: 600,
          interval: 5,
          browserOpened: true,
        })
        return {
          server: request.server,
          accessToken: 'oauth-access-secret',
          refreshToken: 'oauth-refresh-secret',
          tokenType: 'Bearer',
          scopes: request.scopes,
          expiresAt: '2030-01-01T00:00:00.000Z',
          user: { id: 'user-1', name: 'Test User' },
          verification: {
            userCode: 'LUNA-CODE',
            verificationUri: `${request.server}/device`,
            expiresIn: 600,
            interval: 5,
            browserOpened: true,
          },
        }
      },
      async revokeOAuthCredential(request) {
        revocations.push({
          token: request.token,
          tokenTypeHint: request.tokenTypeHint,
        })
      },
    },
    env: { LUNA_TOKEN: options.token ?? 'test-token' },
    isTTY: false,
    version: 'test',
    distribution: 'source',
  }
  const registry = new CommandRegistry()
  registerLocalCommands(registry)
  return {
    ports,
    program: createCliProgram({ registry, ports }),
    successes,
    errors,
    validations,
    oauthLogins,
    revocations,
    infos,
    get config() {
      return config
    },
  }
}
