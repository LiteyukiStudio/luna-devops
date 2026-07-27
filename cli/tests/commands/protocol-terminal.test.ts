import type {
  CommandExecutionGlobals,
  CommandInvocation,
  ProtocolInputStream,
  ProtocolOutputStream,
  ProtocolWebSocket,
  ProtocolWebSocketEvent,
  RuntimePorts,
} from '../../src/commands/types.js'
import { Buffer } from 'node:buffer'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { CommandRegistry } from '../../src/commands/registry.js'
import { emptyConfigDocument } from '../../src/config/schema.js'

describe('webSocket terminal protocol adapter', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('authorizes and runs a pod terminal with TTY input, output, and resize', async () => {
    const command = new CommandRegistry().require('cluster.pod-terminal')
    const stdin = new FakeInput()
    const stdout = new FakeOutput()
    const socket = new FakeWebSocket()
    const requests: Array<{ url: string, method: string }> = []
    let socketUrl = ''
    const ports = createPorts({
      stdin,
      stdout,
      fetch: async (input, init) => {
        requests.push({ url: String(input), method: String(init?.method) })
        return Response.json({
          ticket: 'pod-ticket',
          expiresAt: '2026-07-27T10:00:00Z',
        })
      },
      createWebSocket(url) {
        socketUrl = url
        return socket
      },
    })
    const resultPromise = command.handler(invocation(command.metadata, {
      clusterId: 'cluster-a',
      namespace: 'default',
      name: 'pod-a',
      container: 'app',
    }), ports)

    await vi.waitFor(() => expect(socketUrl).not.toBe(''))
    socket.open()
    stdin.emit('data', Buffer.from('echo ok\n'))
    socket.message('terminal output\n')
    stdout.columns = 132
    stdout.rows = 48
    stdout.emit('resize')
    socket.closeFromServer(1000, 'exitCode=0')

    await expect(resultPromise).resolves.toMatchObject({
      schemaVersion: 'cli.luna.devops/terminal/v1',
      data: {
        exitCode: 0,
        closeCode: 1000,
        bytesSent: 8,
        bytesReceived: 16,
      },
    })
    expect(requests).toEqual([{
      url: 'https://luna.example.test/api/v1/runtime/clusters/cluster-a/pods/terminal/authorize?namespace=default&name=pod-a&container=app',
      method: 'POST',
    }])
    expect(socketUrl).toBe(
      'wss://luna.example.test/api/v1/runtime/clusters/cluster-a/pods/terminal?namespace=default&name=pod-a&container=app&ticket=pod-ticket',
    )
    expect(socket.sent).toEqual([
      JSON.stringify({ type: 'resize', cols: 120, rows: 40 }),
      Buffer.from('echo ok\n'),
      JSON.stringify({ type: 'resize', cols: 132, rows: 48 }),
    ])
    expect(stdout.text).toBe('terminal output\n')
    expect(stdin.rawModes).toEqual([true, false])
    expect(stdin.resumed).toBe(true)
    expect(stdin.paused).toBe(true)
  })

  it('authorizes and connects a release terminal', async () => {
    const command = new CommandRegistry().require('release.terminal')
    const socket = new FakeWebSocket()
    let authorizeUrl = ''
    let socketUrl = ''
    const ports = createPorts({
      fetch: async (input) => {
        authorizeUrl = String(input)
        return Response.json({ data: { ticket: 'release-ticket' } })
      },
      createWebSocket(url) {
        socketUrl = url
        return socket
      },
    })
    const resultPromise = command.handler(invocation(command.metadata, {
      projectId: 'project-a',
      releaseId: 'release-a',
      container: 'api',
    }), ports)

    await vi.waitFor(() => expect(socketUrl).not.toBe(''))
    socket.open()
    socket.message(JSON.stringify({ type: 'exit', code: 7 }))
    socket.closeFromServer(1000, 'complete')

    await expect(resultPromise).rejects.toMatchObject({
      code: 'terminal_remote_exit',
      exitCode: 7,
      details: {
        remoteExitCode: 7,
        closeCode: 1000,
      },
    })
    expect(authorizeUrl).toBe(
      'https://luna.example.test/api/v1/projects/project-a/releases/release-a/terminal/authorize?container=api',
    )
    expect(socketUrl).toBe(
      'wss://luna.example.test/api/v1/projects/project-a/releases/release-a/terminal?container=api&ticket=release-ticket',
    )
  })

  it('accepts backend EOF without a WebSocket close frame after opening', async () => {
    const command = new CommandRegistry().require('release.terminal')
    const socket = new FakeWebSocket()
    const ports = createPorts({
      fetch: async () => Response.json({ ticket: 'ticket' }),
      createWebSocket: () => socket,
    })
    const resultPromise = command.handler(invocation(command.metadata, {
      projectId: 'project-a',
      releaseId: 'release-a',
    }), ports)

    await vi.waitFor(() => expect(socket.listenerCount('open')).toBe(1))
    socket.open()
    socket.message('complete\n')
    socket.closeFromServer(1006, '')

    await expect(resultPromise).resolves.toMatchObject({
      data: {
        exitCode: 0,
        closeCode: 1006,
        bytesReceived: 9,
      },
    })
  })

  it('honors an explicit exit frame before backend EOF', async () => {
    const command = new CommandRegistry().require('release.terminal')
    const socket = new FakeWebSocket()
    const ports = createPorts({
      fetch: async () => Response.json({ ticket: 'ticket' }),
      createWebSocket: () => socket,
    })
    const resultPromise = command.handler(invocation(command.metadata, {
      projectId: 'project-a',
      releaseId: 'release-a',
    }), ports)

    await vi.waitFor(() => expect(socket.listenerCount('open')).toBe(1))
    socket.open()
    socket.message(JSON.stringify({ type: 'exit', code: 23 }))
    socket.closeFromServer(1006, '')

    await expect(resultPromise).rejects.toMatchObject({
      code: 'terminal_remote_exit',
      exitCode: 23,
      details: {
        remoteExitCode: 23,
        closeCode: 1006,
      },
    })
  })

  it('rejects agent and non-TTY terminal sessions before authorization', async () => {
    const command = new CommandRegistry().require('cluster.pod-terminal')
    const fetch = vi.fn()
    const baseParams = {
      clusterId: 'cluster-a',
      namespace: 'default',
      name: 'pod-a',
    }

    await expect(command.handler(
      invocation(command.metadata, baseParams, { agent: true }),
      createPorts({ fetch }),
    )).rejects.toMatchObject({
      code: 'terminal_agent_unsupported',
      status: 422,
    })
    await expect(command.handler(
      invocation(command.metadata, baseParams, { interactive: false }),
      createPorts({ fetch }),
    )).rejects.toMatchObject({
      code: 'terminal_tty_required',
      status: 422,
    })
    expect(fetch).not.toHaveBeenCalled()
  })

  it('propagates MFA authorization errors without opening a socket', async () => {
    const command = new CommandRegistry().require('cluster.pod-terminal')
    const createWebSocket = vi.fn()
    const ports = createPorts({
      fetch: async () => Response.json({
        error: {
          code: 'mfa_required',
          message: 'Complete step-up authentication.',
          purpose: 'runtime_terminal',
        },
      }, { status: 403 }),
      createWebSocket,
    })

    await expect(command.handler(invocation(command.metadata, {
      clusterId: 'cluster-a',
      namespace: 'default',
      name: 'pod-a',
    }), ports)).rejects.toMatchObject({
      code: 'mfa_required',
      status: 403,
    })
    expect(createWebSocket).not.toHaveBeenCalled()
  })

  it('restores the terminal and returns exit code 130 when interrupted', async () => {
    const command = new CommandRegistry().require('release.terminal')
    const stdin = new FakeInput()
    const socket = new FakeWebSocket()
    let interrupt: (() => void) | undefined
    const ports = createPorts({
      stdin,
      fetch: async () => Response.json({ ticket: 'ticket' }),
      createWebSocket: () => socket,
      onInterrupt(listener) {
        interrupt = listener
        return () => {
          interrupt = undefined
        }
      },
    })
    const resultPromise = command.handler(invocation(command.metadata, {
      projectId: 'project-a',
      releaseId: 'release-a',
    }), ports)

    await vi.waitFor(() => expect(interrupt).toBeTypeOf('function'))
    socket.open()
    interrupt?.()

    await expect(resultPromise).rejects.toMatchObject({
      code: 'terminal_interrupted',
      exitCode: 130,
    })
    expect(socket.closed).toEqual({ code: 1000, reason: 'interrupted' })
    expect(stdin.rawModes).toEqual([true, false])
    expect(interrupt).toBeUndefined()
  })

  it('times out an unopened WebSocket and cleans up local listeners', async () => {
    const command = new CommandRegistry().require('release.terminal')
    const stdin = new FakeInput()
    const socket = new FakeWebSocket()
    const ports = createPorts({
      stdin,
      fetch: async () => Response.json({ ticket: 'ticket' }),
      createWebSocket: () => socket,
    })

    await expect(command.handler(invocation(command.metadata, {
      projectId: 'project-a',
      releaseId: 'release-a',
    }, { timeoutMs: 5 }), ports)).rejects.toMatchObject({
      code: 'terminal_connection_timeout',
      status: 504,
      retryable: true,
    })
    expect(socket.closed).toEqual({ code: 1000, reason: 'handshake timeout' })
    expect(stdin.rawModes).toEqual([false])
  })
})

const DEFAULT_GLOBALS: CommandExecutionGlobals = {
  output: 'json',
  color: false,
  interactive: true,
  yes: true,
  quiet: false,
  agent: false,
  timeoutMs: 1_000,
  debug: false,
  insecureSkipTlsVerify: false,
}

function invocation(
  metadata: CommandInvocation['metadata'],
  params: Readonly<Record<string, unknown>>,
  globals: Partial<CommandExecutionGlobals> = {},
): CommandInvocation {
  return {
    metadata,
    params,
    globals: { ...DEFAULT_GLOBALS, ...globals },
    explicitGlobalKeys: new Set(),
    canonicalGlobalValues: {},
  }
}

function createPorts(overrides: {
  fetch?: typeof globalThis.fetch
  createWebSocket?: (url: string) => ProtocolWebSocket
  stdin?: ProtocolInputStream
  stdout?: ProtocolOutputStream
  onInterrupt?: (listener: () => void) => () => void
} = {}): RuntimePorts {
  const stdin = overrides.stdin ?? new FakeInput()
  const stdout = overrides.stdout ?? new FakeOutput()
  return {
    config: {
      read: async () => ({
        ...emptyConfigDocument(),
        server: 'https://luna.example.test',
        credential: { type: 'access_token', token: 'secret' },
      }),
      write: async () => undefined,
    },
    input: {
      parse: async () => ({}),
    },
    output: {
      writeSuccess: () => undefined,
      writeError: () => undefined,
    },
    api: {
      execute: async () => ({}),
      request: async () => ({}),
    },
    protocol: {
      fetch: overrides.fetch,
      createWebSocket: overrides.createWebSocket,
      stdin,
      stdout,
      onInterrupt: overrides.onInterrupt ?? (() => () => undefined),
    },
    env: {},
    isTTY: true,
  }
}

class FakeInput implements ProtocolInputStream {
  readonly isTTY = true
  isRaw = false
  readonly rawModes: boolean[] = []
  resumed = false
  paused = false
  readonly #listeners = new Map<string, Set<(...args: unknown[]) => void>>()

  setRawMode(enabled: boolean): void {
    this.isRaw = enabled
    this.rawModes.push(enabled)
  }

  resume(): void {
    this.resumed = true
  }

  pause(): void {
    this.paused = true
  }

  on(event: string, listener: (...args: unknown[]) => void): void {
    const listeners = this.#listeners.get(event) ?? new Set()
    listeners.add(listener)
    this.#listeners.set(event, listeners)
  }

  off(event: string, listener: (...args: unknown[]) => void): void {
    this.#listeners.get(event)?.delete(listener)
  }

  emit(event: string, ...args: unknown[]): void {
    for (const listener of this.#listeners.get(event) ?? [])
      listener(...args)
  }
}

class FakeOutput implements ProtocolOutputStream {
  readonly isTTY = true
  columns = 120
  rows = 40
  text = ''
  readonly #listeners = new Map<string, Set<(...args: unknown[]) => void>>()

  write(chunk: string | Uint8Array): boolean {
    this.text += typeof chunk === 'string' ? chunk : Buffer.from(chunk).toString()
    return true
  }

  on(event: string, listener: (...args: unknown[]) => void): void {
    const listeners = this.#listeners.get(event) ?? new Set()
    listeners.add(listener)
    this.#listeners.set(event, listeners)
  }

  off(event: string, listener: (...args: unknown[]) => void): void {
    this.#listeners.get(event)?.delete(listener)
  }

  emit(event: string, ...args: unknown[]): void {
    for (const listener of this.#listeners.get(event) ?? [])
      listener(...args)
  }
}

class FakeWebSocket implements ProtocolWebSocket {
  readyState = 0
  binaryType = ''
  readonly sent: Array<string | ArrayBuffer | ArrayBufferView> = []
  closed?: { code?: number, reason?: string }
  readonly #listeners = new Map<string, Set<(event: ProtocolWebSocketEvent) => void>>()

  send(data: string | ArrayBuffer | ArrayBufferView): void {
    this.sent.push(data)
  }

  close(code?: number, reason?: string): void {
    this.closed = { code, reason }
    this.readyState = 3
  }

  addEventListener(
    event: string,
    listener: (event: ProtocolWebSocketEvent) => void,
  ): void {
    const listeners = this.#listeners.get(event) ?? new Set()
    listeners.add(listener)
    this.#listeners.set(event, listeners)
  }

  removeEventListener(
    event: string,
    listener: (event: ProtocolWebSocketEvent) => void,
  ): void {
    this.#listeners.get(event)?.delete(listener)
  }

  open(): void {
    this.readyState = 1
    this.emit('open', {})
  }

  message(data: unknown): void {
    this.emit('message', { data })
  }

  closeFromServer(code: number, reason: string): void {
    this.readyState = 3
    this.emit('close', { code, reason })
  }

  listenerCount(event: string): number {
    return this.#listeners.get(event)?.size ?? 0
  }

  private emit(event: string, payload: ProtocolWebSocketEvent): void {
    for (const listener of this.#listeners.get(event) ?? [])
      listener(payload)
  }
}
