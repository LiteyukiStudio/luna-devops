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
  it('validates an environment token before storing the context', async () => {
    const harness = createHarness()
    const result = await runCli(harness.program, [
      'node',
      'luna',
      'auth',
      'login',
      'server=https://luna.example.com',
      'context=work',
    ], harness.ports.output)

    expect(result.exitCode).toBe(0)
    expect(harness.validations).toEqual([{
      server: 'https://luna.example.com',
      token: 'test-token',
    }])
    expect(harness.config.currentContext).toBe('work')
    expect(harness.config.contexts.work?.credential).toBe('work-access-token')
    expect(harness.config.credentials['work-access-token']?.token).toBe('test-token')
    expect(harness.successes[0]?.result.data).toMatchObject({
      authenticated: true,
      context: 'work',
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
    expect(harness.config.credentials).toEqual({})
    expect(harness.config.contexts).toEqual({})
  })

  it('trims surrounding whitespace before validating and storing a token', async () => {
    const harness = createHarness({ token: 'test-token\n' })
    const result = await runCli(harness.program, [
      'node',
      'luna',
      'auth',
      'login',
      'server=https://luna.example.com',
      'context=work',
    ], harness.ports.output)

    expect(result.exitCode).toBe(0)
    expect(harness.validations).toEqual([{
      server: 'https://luna.example.com',
      token: 'test-token',
    }])
    expect(harness.config.credentials['work-access-token']?.token).toBe('test-token')
  })
})

function createHarness(options: { validationError?: Error, token?: string } = {}) {
  let config: LunaConfigDocument = {
    version: 1,
    currentContext: null,
    instances: {},
    credentials: {},
    contexts: {},
  }
  const successes: Array<{
    metadata: NormalizedCommandMetadata
    result: CommandResult
    globals: CommandExecutionGlobals
  }> = []
  const validations: Array<{ server: string, token: string }> = []
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
      writeError() {},
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
    validations,
    get config() {
      return config
    },
  }
}
