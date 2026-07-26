import type {
  CommandExecutionGlobals,
  CommandResult,
  LunaConfigDocument,
  NormalizedCommandMetadata,
  RuntimePorts,
} from '../../src/commands/index.js'
import { describe, expect, it } from 'vitest'
import {
  CommandRegistry,
  createCliProgram,
  DefaultInputPort,
  registerLocalCommands,
  runCli,
} from '../../src/commands/index.js'

describe('health doctor', () => {
  it('checks the active login and server through the root shortcut', async () => {
    const harness = createHarness()

    const result = await runCli(harness.program, [
      'node',
      'luna',
      'doctor',
    ], harness.ports.output)

    expect(result.exitCode).toBe(0)
    expect(harness.successes[0]?.result.data).toMatchObject({
      status: 'ok',
      login: {
        server: 'https://luna.example.com',
        authenticated: true,
      },
      unsupported: [],
      server: {
        apiVersion: 'v1',
        minimumCliVersion: '0.1.0',
      },
    })
  })

  it('keeps root shortcuts out of strict agent mode', async () => {
    const harness = createHarness()

    const result = await runCli(harness.program, [
      'node',
      'luna',
      'doctor',
      'agent=true',
      'output=json',
    ], harness.ports.output)

    expect(result.exitCode).not.toBe(0)
    expect(harness.errors[0]).toMatchObject({
      code: 'agent_alias_forbidden',
    })
  })
})

function createHarness() {
  const config: LunaConfigDocument = {
    version: 2,
    server: 'https://luna.example.com',
    credential: {
      type: 'access_token',
      token: 'test-token',
      scopes: [],
    },
    project: null,
    language: '',
    output: '',
  }
  const successes: Array<{
    metadata: NormalizedCommandMetadata
    result: CommandResult
    globals: CommandExecutionGlobals
  }> = []
  const errors: unknown[] = []
  const ports: RuntimePorts = {
    config: {
      read: async () => config,
      write: async () => {},
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
      async getMeta() {
        return {
          apiVersion: 'v1',
          serverVersion: 'test',
          openapiDigest: 'test-digest',
          minimumCliVersion: '0.1.0',
          features: {
            accessToken: true,
            deviceCode: true,
            mfaBearer: true,
            oauthAuthorization: true,
            openapiOperations: true,
          },
        }
      },
    },
    env: {},
    isTTY: false,
    version: '0.1.0',
    distribution: 'source',
  }
  const registry = new CommandRegistry({
    openapiDigest: 'test-digest',
  })
  registerLocalCommands(registry)
  return {
    ports,
    program: createCliProgram({ registry, ports }),
    successes,
    errors,
  }
}
