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
  it('supports the root login shortcut and stores a validated token', async () => {
    const harness = createHarness()
    const result = await runCli(harness.program, [
      'node',
      'luna',
      'login',
      'server=https://luna.example.com',
    ], harness.ports.output)

    expect(result.exitCode).toBe(0)
    expect(harness.validations).toEqual([{
      server: 'https://luna.example.com',
      token: 'test-token',
    }])
    expect(harness.config.server).toBe('https://luna.example.com')
    expect(harness.config.credential?.type).toBe('access_token')
    expect(harness.config.credential && 'token' in harness.config.credential
      ? harness.config.credential.token
      : null).toBe('test-token')
    expect(harness.successes[0]?.result.data).toMatchObject({
      authenticated: true,
      server: 'https://luna.example.com',
    })
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
    expect(harness.validations).toEqual([{
      server: 'https://devops.liteyuki.org',
      token: 'test-token',
    }])
    expect(harness.config.server).toBe('https://devops.liteyuki.org')
    expect(harness.config.project).toBeNull()
    expect(harness.config.credential && 'token' in harness.config.credential
      ? harness.config.credential.token
      : null).toBe('test-token')
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
    })
    expect(harness.config.credential).toBeNull()
  })

  it('fails device-code login before starting OAuth when the server disables it', async () => {
    const harness = createHarness({ deviceCode: false })
    const result = await runCli(harness.program, [
      'node',
      'luna',
      'login',
      'server=https://luna.example.com',
      'mode=device-code',
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
            deviceCode: options.deviceCode ?? false,
          },
        }
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
    get config() {
      return config
    },
  }
}
