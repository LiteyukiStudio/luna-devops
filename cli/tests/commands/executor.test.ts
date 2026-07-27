import type { CommandExecutionGlobals, CommandResult, LunaConfigDocument, NormalizedCommandMetadata, RuntimePorts } from '../../src/commands/index.js'
import { describe, expect, it } from 'vitest'
import {
  CommandOutput,
  CommandRegistry,
  createCliProgram,
  DefaultInputPort,
  memoryOutputStreams,
  runCli,
} from '../../src/commands/index.js'

const emptyConfig: LunaConfigDocument = {
  version: 2,
  server: 'https://devops.liteyuki.org',
  credential: null,
  project: null,
  language: '',
  output: '',
}

describe('commander command execution', () => {
  it('forces machine-readable agent globals', async () => {
    const registry = new CommandRegistry()
    registry.register({
      category: 'version',
      tool: 'show',
      source: 'local',
    }, async invocation => ({ data: invocation.globals }))
    const captures = capturePorts()
    const program = createCliProgram({ registry, ports: captures.ports })

    const result = await runCli(program, ['node', 'luna', 'version', 'show', '--agent'])

    expect(result.exitCode).toBe(0)
    expect(captures.successes[0]?.globals).toMatchObject({
      agent: true,
      output: 'json',
      color: false,
      interactive: false,
      quiet: true,
    })
  })

  it('requires a command with one structured error in agent mode', async () => {
    const registry = new CommandRegistry()
    const streams = memoryOutputStreams()
    const output = new CommandOutput({ streams: streams.streams, version: 'test' })
    const captures = capturePorts()
    const program = createCliProgram({
      registry,
      ports: { ...captures.ports, output },
    })
    routeCommanderOutput(program, streams.streams)

    const result = await runCli(program, ['node', 'luna', '--agent'], output)

    expect(result.exitCode).toBe(2)
    expect(streams.stdout()).toBe('')
    expect(JSON.parse(streams.stderr())).toMatchObject({
      error: {
        code: 'command_required',
      },
    })
  })

  it.each([
    {
      name: 'unknown category',
      argv: ['node', 'luna', 'missing', 'show', 'agent=true', 'output=json', 'interactive=false'],
    },
    {
      name: 'unknown tool in a known category',
      argv: ['node', 'luna', 'version', 'missing', 'agent=true', 'output=json', 'interactive=false'],
    },
  ])('keeps $name failures as pure JSON in agent mode', async ({ argv }) => {
    const registry = new CommandRegistry()
    registry.register({
      category: 'version',
      tool: 'show',
      source: 'local',
    }, async () => ({ data: { version: 'test' } }))
    const streams = memoryOutputStreams()
    const output = new CommandOutput({ streams: streams.streams, version: 'test' })
    const captures = capturePorts()
    const program = createCliProgram({
      registry,
      ports: { ...captures.ports, output },
    })
    routeCommanderOutput(program, streams.streams)

    const result = await runCli(program, argv, output)

    expect(result.exitCode).toBeGreaterThan(0)
    expect(streams.stdout()).toBe('')
    const lines = streams.stderr().trim().split('\n')
    expect(lines).toHaveLength(1)
    expect(JSON.parse(lines[0]!)).toMatchObject({
      error: {
        code: 'unknown_command',
      },
    })
    expect(streams.stderr()).not.toContain('Usage:')
    expect(streams.stderr()).not.toContain('Commands:')
  })

  it('renders one actionable error for unknown tools in human mode', async () => {
    const registry = new CommandRegistry()
    registry.register({
      category: 'version',
      tool: 'show',
      source: 'local',
    }, async () => ({ data: { version: 'test' } }))
    const streams = memoryOutputStreams()
    const output = new CommandOutput({ streams: streams.streams, version: 'test' })
    const captures = capturePorts()
    const program = createCliProgram({
      registry,
      ports: { ...captures.ports, output },
    })
    routeCommanderOutput(program, streams.streams)

    const result = await runCli(
      program,
      ['node', 'luna', 'version', 'missing'],
      output,
    )

    expect(result.exitCode).toBeGreaterThan(0)
    expect(streams.stderr()).not.toContain('Usage:')
    expect(streams.stderr()).toContain('unknown_command:')
    expect(streams.stderr()).toContain('luna version --help')
    expect(streams.stderr()).not.toContain('undefined')
  })

  it('localizes unknown-command suggestions in human mode', async () => {
    const registry = new CommandRegistry()
    registry.register({
      category: 'project',
      tool: 'get-projects',
      source: 'local',
    }, async () => ({ data: {} }))
    const streams = memoryOutputStreams()
    const output = new CommandOutput({
      streams: streams.streams,
      version: 'test',
      translate(key, fallback) {
        const messages: Record<string, string> = {
          'help.errors.unknownCommand': '未知命令',
          'help.errors.didYouMean': '你是否想输入',
          'help.errors.nextStep': '下一步',
        }
        return messages[key] ?? fallback
      },
    })
    const captures = capturePorts()
    const ports = {
      ...captures.ports,
      output,
      translate(key: string, fallback: string) {
        const messages: Record<string, string> = {
          'help.errors.unknownCommand': '未知命令',
          'help.errors.didYouMean': '你是否想输入',
          'help.errors.nextStep': '下一步',
        }
        return messages[key] ?? fallback
      },
    }
    const program = createCliProgram({ registry, ports })
    routeCommanderOutput(program, streams.streams)

    const result = await runCli(
      program,
      ['node', 'luna', 'projec', 'get-projects'],
      output,
    )

    expect(result.exitCode).toBe(2)
    expect(streams.stderr()).toContain('unknown_command: 未知命令: projec')
    expect(streams.stderr()).toContain('你是否想输入: project')
    expect(streams.stderr()).toContain('下一步: luna --help')
    expect(streams.stderr()).not.toContain('Did you mean')
    expect(streams.stderr()).not.toContain('undefined')
  })

  it('rejects an explicit project for a project-independent command', async () => {
    const registry = new CommandRegistry()
    registry.register({
      category: 'version',
      tool: 'show',
      source: 'local',
      projectContext: 'none',
    }, async () => ({ data: {} }))
    const captures = capturePorts()
    const program = createCliProgram({ registry, ports: captures.ports })

    const result = await runCli(
      program,
      ['node', 'luna', 'version', 'show', 'project=project-1'],
      captures.ports.output,
    )

    expect(result.exitCode).toBe(2)
    expect(captures.errors).toHaveLength(1)
    expect((captures.errors[0] as { code?: string }).code).toBe('project_not_supported')
  })

  it('uses the configured default project for required project path parameters', async () => {
    const registry = new CommandRegistry()
    registry.register({
      category: 'application',
      tool: 'list',
      source: 'openapi',
      operationId: 'listApplications',
      projectContext: 'required',
      parameters: [
        { name: 'projectId', location: 'path', required: true },
      ],
    }, async invocation => invocation.params)
    const captures = capturePorts()
    captures.ports.config.read = async () => ({
      ...emptyConfig,
      project: { id: 'prj_configured' },
    })
    const program = createCliProgram({ registry, ports: captures.ports })

    const result = await runCli(
      program,
      ['node', 'luna', 'application', 'list'],
      captures.ports.output,
    )

    expect(result.exitCode).toBe(0)
    expect(captures.successes[0]?.result.data).toEqual({ projectId: 'prj_configured' })
  })

  it('accepts an explicit command project parameter for agent mutations', async () => {
    const registry = new CommandRegistry()
    registry.register({
      category: 'application',
      tool: 'create',
      source: 'openapi',
      operationId: 'createApplication',
      projectContext: 'required',
      risk: 'medium',
      parameters: [
        {
          name: 'projectId',
          location: 'path',
          required: true,
          schema: { type: 'string' },
        },
      ],
    }, async invocation => invocation.params)
    const captures = capturePorts()
    const program = createCliProgram({ registry, ports: captures.ports })

    const result = await runCli(
      program,
      [
        'node',
        'luna',
        'application',
        'create',
        'projectId=prj_explicit',
        'agent=true',
        'yes=true',
      ],
      captures.ports.output,
    )

    expect(result.exitCode).toBe(0)
    expect(captures.successes[0]?.result.data).toEqual({
      projectId: 'prj_explicit',
    })
  })

  it('rejects an implicit configured project for agent mutations', async () => {
    const registry = new CommandRegistry()
    registry.register({
      category: 'application',
      tool: 'create',
      source: 'openapi',
      operationId: 'createApplication',
      projectContext: 'required',
      risk: 'medium',
      parameters: [
        {
          name: 'projectId',
          location: 'path',
          required: true,
          schema: { type: 'string' },
        },
      ],
    }, async invocation => invocation.params)
    const captures = capturePorts()
    captures.ports.config.read = async () => ({
      ...emptyConfig,
      project: { id: 'prj_configured' },
    })
    const program = createCliProgram({ registry, ports: captures.ports })

    const result = await runCli(
      program,
      ['node', 'luna', 'application', 'create', 'agent=true', 'yes=true'],
      captures.ports.output,
    )

    expect(result.exitCode).toBe(2)
    expect((captures.errors[0] as { code?: string }).code)
      .toBe('explicit_project_required')
  })

  it('reports the effective command project instead of the configured default project', () => {
    const streams = memoryOutputStreams()
    const output = new CommandOutput({ streams: streams.streams, version: 'test' })
    const metadata = {
      category: 'application',
      tool: 'list',
      canonicalPath: 'application.list',
      source: 'openapi',
    } as NormalizedCommandMetadata

    output.writeSuccess(
      metadata,
      {
        data: { items: [] },
        meta: { projectId: 'prj_explicit' },
      },
      {
        project: 'prj_configured',
        output: 'json',
        color: false,
        interactive: false,
        yes: false,
        quiet: true,
        agent: true,
        timeoutMs: 30_000,
        debug: false,
        insecureSkipTlsVerify: false,
      },
    )

    expect(JSON.parse(streams.stdout())).toMatchObject({
      meta: { projectId: 'prj_explicit' },
    })
  })

  it('does not treat Commander boolean defaults as explicit conflicts', async () => {
    const registry = new CommandRegistry()
    registry.register({
      category: 'help',
      tool: 'catalog',
      source: 'local',
    }, async invocation => ({ data: invocation.globals }))
    const captures = capturePorts()
    const program = createCliProgram({ registry, ports: captures.ports })

    const result = await runCli(
      program,
      ['node', 'luna', 'help', 'catalog', 'interactive=false', 'color=false'],
      captures.ports.output,
    )

    expect(result.exitCode).toBe(0)
    expect(captures.successes[0]?.globals).toMatchObject({
      interactive: false,
      color: false,
    })
  })

  it('requires confirmation for medium-risk API mutations in non-interactive mode', async () => {
    const registry = new CommandRegistry()
    registry.register({
      category: 'application',
      tool: 'create',
      source: 'openapi',
      operationId: 'createApplication',
      risk: 'medium',
    }, async () => ({ data: {} }))
    const captures = capturePorts()
    const program = createCliProgram({ registry, ports: captures.ports })

    const result = await runCli(
      program,
      ['node', 'luna', 'application', 'create'],
      captures.ports.output,
    )

    expect(result.exitCode).toBe(6)
    expect((captures.errors[0] as { code?: string }).code).toBe('confirmation_required')
  })

  it('allows yes=true for medium-risk API mutations', async () => {
    const registry = new CommandRegistry()
    registry.register({
      category: 'application',
      tool: 'create',
      source: 'openapi',
      operationId: 'createApplication',
      risk: 'medium',
    }, async () => ({ data: { created: true } }))
    const captures = capturePorts()
    const program = createCliProgram({ registry, ports: captures.ports })

    const result = await runCli(
      program,
      ['node', 'luna', 'application', 'create', 'yes=true'],
      captures.ports.output,
    )

    expect(result.exitCode).toBe(0)
    expect(captures.successes).toHaveLength(1)
  })

  it.each(['high', 'critical'] as const)(
    'allows --yes for %s-risk API commands in non-interactive mode',
    async (risk) => {
      const registry = new CommandRegistry()
      registry.register({
        category: 'application',
        tool: 'delete',
        source: 'openapi',
        operationId: 'deleteApplication',
        risk,
      }, async () => ({ data: { deleted: true } }))
      const captures = capturePorts()
      const program = createCliProgram({ registry, ports: captures.ports })

      const result = await runCli(
        program,
        ['node', 'luna', 'application', 'delete', '--yes'],
        captures.ports.output,
      )

      expect(result.exitCode).toBe(0)
      expect(captures.successes[0]?.result.data).toEqual({
        data: { deleted: true },
      })
    },
  )

  it.each(['high', 'critical'] as const)(
    'requires explicit confirmation for %s-risk API commands in non-interactive mode',
    async (risk) => {
      const registry = new CommandRegistry()
      registry.register({
        category: 'application',
        tool: 'delete',
        source: 'openapi',
        operationId: 'deleteApplication',
        risk,
      }, async () => ({ data: {} }))
      const captures = capturePorts()
      const program = createCliProgram({ registry, ports: captures.ports })

      const result = await runCli(
        program,
        ['node', 'luna', 'application', 'delete'],
        captures.ports.output,
      )

      expect(result.exitCode).toBe(6)
      expect((captures.errors[0] as { code?: string }).code).toBe('confirmation_required')
    },
  )

  it('prompts before executing a high-risk API command interactively', async () => {
    const registry = new CommandRegistry()
    registry.register({
      category: 'application',
      tool: 'delete',
      source: 'openapi',
      operationId: 'deleteApplication',
      risk: 'high',
    }, async () => ({ data: { deleted: true } }))
    const captures = capturePorts()
    let prompted = false
    const program = createCliProgram({
      registry,
      ports: {
        ...captures.ports,
        isTTY: true,
        input: {
          parse: captures.ports.input.parse,
          async confirm() {
            prompted = true
            return true
          },
        },
      },
    })

    const result = await runCli(
      program,
      ['node', 'luna', 'application', 'delete'],
      captures.ports.output,
    )

    expect(result.exitCode).toBe(0)
    expect(prompted).toBe(true)
    expect(captures.successes[0]?.result.data).toEqual({
      data: { deleted: true },
    })
  })

  it('requires --yes for high-risk API commands in agent mode', async () => {
    const registry = new CommandRegistry()
    registry.register({
      category: 'application',
      tool: 'delete',
      source: 'openapi',
      operationId: 'deleteApplication',
      risk: 'high',
    }, async () => ({ data: { deleted: true } }))
    const captures = capturePorts()
    const program = createCliProgram({ registry, ports: captures.ports })

    const rejected = await runCli(
      program,
      ['node', 'luna', 'application', 'delete', '--agent'],
      captures.ports.output,
    )

    expect(rejected.exitCode).toBe(6)
    expect((captures.errors[0] as { code?: string }).code).toBe('confirmation_required')

    const allowed = await runCli(
      program,
      ['node', 'luna', 'application', 'delete', '--agent', '--yes'],
      captures.ports.output,
    )

    expect(allowed.exitCode).toBe(0)
    expect(captures.successes.at(-1)?.result.data).toEqual({
      data: { deleted: true },
    })
  })

  it('uses the shared prompt for high-risk local commands', async () => {
    const registry = new CommandRegistry()
    registry.register({
      category: 'retention',
      tool: 'purge-local-cache',
      source: 'local',
      risk: 'high',
    }, async () => ({ data: { deleted: true } }))
    const captures = capturePorts()
    let prompted = false
    const program = createCliProgram({
      registry,
      ports: {
        ...captures.ports,
        isTTY: true,
        input: {
          parse: captures.ports.input.parse,
          async confirm() {
            prompted = true
            return true
          },
        },
      },
    })

    const result = await runCli(
      program,
      ['node', 'luna', 'retention', 'purge-local-cache'],
      captures.ports.output,
    )

    expect(result.exitCode).toBe(0)
    expect(prompted).toBe(true)
  })
})

function capturePorts(): {
  ports: RuntimePorts
  successes: Array<{
    metadata: NormalizedCommandMetadata
    result: CommandResult
    globals: CommandExecutionGlobals
  }>
  errors: unknown[]
} {
  const successes: Array<{
    metadata: NormalizedCommandMetadata
    result: CommandResult
    globals: CommandExecutionGlobals
  }> = []
  const errors: unknown[] = []
  return {
    successes,
    errors,
    ports: {
      config: {
        read: async () => emptyConfig,
        write: async () => undefined,
      },
      input: new DefaultInputPort(),
      output: {
        writeSuccess(metadata, result, commandGlobals) {
          successes.push({ metadata, result, globals: commandGlobals })
        },
        writeError(error) {
          errors.push(error)
        },
      },
      api: {
        execute: async () => ({ data: {} }),
        request: async () => ({ data: {} }),
      },
      env: {},
      isTTY: false,
      version: 'test',
      distribution: 'source',
    },
  }
}

function routeCommanderOutput(
  command: import('commander').Command,
  streams: ReturnType<typeof memoryOutputStreams>['streams'],
): void {
  command.configureOutput({
    writeOut: value => streams.stdout.write(value),
    writeErr: value => streams.stderr.write(value),
    outputError: (value, write) => write(value),
  })
  for (const child of command.commands)
    routeCommanderOutput(child, streams)
}
