import type {
  CommandExecutionGlobals,
  CommandInvocation,
  RuntimePorts,
} from '../../src/commands/types.js'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { CommandRegistry } from '../../src/commands/registry.js'
import { emptyConfigDocument } from '../../src/config/schema.js'

describe('server-sent events protocol adapter', () => {
  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('keeps reading after the 30 second connection timeout has elapsed', async () => {
    vi.useFakeTimers()
    const command = new CommandRegistry().require('build.job-logs-follow')
    let streamController: ReadableStreamDefaultController<Uint8Array> | undefined
    let requestSignal: AbortSignal | undefined
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        streamController = controller
      },
    })
    const fetch = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      requestSignal = init?.signal ?? undefined
      return new Response(body, {
        headers: { 'content-type': 'text/event-stream' },
      })
    })

    const resultPromise = command.handler(invocation(command.metadata, {
      projectId: 'project-a',
      jobId: 'job-a',
    }), createPorts(fetch as typeof globalThis.fetch))

    await vi.advanceTimersByTimeAsync(0)
    expect(fetch).toHaveBeenCalledOnce()
    await vi.advanceTimersByTimeAsync(31_000)
    expect(requestSignal?.aborted).toBe(false)

    streamController?.enqueue(new TextEncoder().encode(
      'event: done\ndata: {"status":"complete"}\n\n',
    ))
    streamController?.close()

    await expect(resultPromise).resolves.toMatchObject({
      data: {
        summary: {
          data: {
            completed: true,
            reason: 'done',
            eventsRead: 1,
          },
        },
      },
      meta: { transport: 'sse' },
    })
  })
})

const DEFAULT_GLOBALS: CommandExecutionGlobals = {
  output: 'json',
  color: false,
  interactive: false,
  yes: true,
  quiet: false,
  agent: true,
  timeoutMs: 30_000,
  debug: false,
  insecureSkipTlsVerify: false,
}

function invocation(
  metadata: CommandInvocation['metadata'],
  params: Readonly<Record<string, unknown>>,
): CommandInvocation {
  return {
    metadata,
    params,
    globals: DEFAULT_GLOBALS,
    explicitGlobalKeys: new Set(),
    canonicalGlobalValues: {},
  }
}

function createPorts(fetch: typeof globalThis.fetch): RuntimePorts {
  return {
    config: {
      read: async () => ({
        ...emptyConfigDocument(),
        server: 'https://luna.example.test',
        credential: { type: 'access_token', token: 'secret' },
      }),
      write: async () => undefined,
    },
    input: { parse: async () => ({}) },
    output: {
      writeSuccess: () => undefined,
      writeError: () => undefined,
    },
    api: {
      execute: async () => ({}),
      request: async () => ({}),
    },
    protocol: { fetch },
    env: {},
    isTTY: false,
  }
}
