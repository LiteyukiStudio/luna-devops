import type {
  CommandExecutionGlobals,
  CommandInvocation,
  RuntimePorts,
} from '../../src/commands/types.js'
import { Buffer } from 'node:buffer'
import { mkdtemp, readFile, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { CommandRegistry } from '../../src/commands/registry.js'
import { emptyConfigDocument } from '../../src/config/schema.js'

describe('download protocol adapter', () => {
  const temporaryDirectories: string[] = []

  afterEach(async () => {
    vi.useRealTimers()
    vi.restoreAllMocks()
    await Promise.all(temporaryDirectories.splice(0).map(path =>
      rm(path, { recursive: true, force: true })))
  })

  it('authorizes a deployment export and downloads it with the issued ticket', async () => {
    const command = new CommandRegistry().require('deployment.data-export')
    const directory = await mkdtemp(join(tmpdir(), 'luna-download-'))
    temporaryDirectories.push(directory)
    const destination = join(directory, 'backup.tar.gz')
    const requests: Array<{ url: string, method: string }> = []
    const ports = createPorts(async (input, init) => {
      const url = String(input)
      const method = String(init?.method)
      requests.push({ url, method })
      if (method === 'POST')
        return Response.json({ ticket: 'download-ticket' })
      return new Response(Buffer.from('archive bytes'), {
        headers: {
          'content-disposition': 'attachment; filename="export.tar.gz"',
          'content-type': 'application/gzip',
          'x-request-id': 'request-a',
        },
      })
    })

    const result = await command.handler(invocation(command.metadata, {
      projectId: 'project-a',
      applicationId: 'application-a',
      targetId: 'target-a',
      destination,
    }), ports)

    expect(requests).toEqual([
      {
        url: 'https://luna.example.test/api/v1/projects/project-a/applications/application-a/deployment-targets/target-a/data-export/authorize',
        method: 'POST',
      },
      {
        url: 'https://luna.example.test/api/v1/projects/project-a/applications/application-a/deployment-targets/target-a/data-export?ticket=download-ticket',
        method: 'GET',
      },
    ])
    await expect(readFile(destination, 'utf8')).resolves.toBe('archive bytes')
    expect(result).toMatchObject({
      schemaVersion: 'cli.luna.devops/download/v1',
      data: {
        destination,
        filename: 'backup.tar.gz',
        bytes: 13,
        contentType: 'application/gzip',
      },
      meta: {
        requestId: 'request-a',
        transport: 'download',
      },
    })
  })

  it('keeps an explicit ticket as a debugging override', async () => {
    const command = new CommandRegistry().require('deployment.data-export')
    const directory = await mkdtemp(join(tmpdir(), 'luna-download-'))
    temporaryDirectories.push(directory)
    const destination = join(directory, 'backup.tar.gz')
    const fetch = vi.fn(async (input: RequestInfo | URL) => {
      expect(String(input)).toContain('ticket=debug-ticket')
      return new Response(Buffer.from('ok'))
    })

    await command.handler(invocation(command.metadata, {
      projectId: 'project-a',
      applicationId: 'application-a',
      targetId: 'target-a',
      ticket: 'debug-ticket',
      destination,
    }), createPorts(fetch as typeof globalThis.fetch))

    expect(fetch).toHaveBeenCalledTimes(1)
  })

  it('continues a large transfer after the 30 second connection timeout', async () => {
    vi.useFakeTimers()
    const command = new CommandRegistry().require('deployment.data-export')
    const directory = await mkdtemp(join(tmpdir(), 'luna-download-'))
    temporaryDirectories.push(directory)
    const destination = join(directory, 'slow-export.tar.gz')
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
        headers: { 'content-type': 'application/gzip' },
      })
    })

    const resultPromise = command.handler(invocation(command.metadata, {
      projectId: 'project-a',
      applicationId: 'application-a',
      targetId: 'target-a',
      ticket: 'download-ticket',
      destination,
    }, { timeoutMs: 30_000 }), createPorts(fetch as typeof globalThis.fetch))

    await vi.advanceTimersByTimeAsync(0)
    expect(fetch).toHaveBeenCalledOnce()
    await vi.advanceTimersByTimeAsync(31_000)
    expect(requestSignal?.aborted).toBe(false)

    streamController?.enqueue(Buffer.from('slow archive bytes'))
    streamController?.close()

    await expect(resultPromise).resolves.toMatchObject({
      data: {
        destination,
        bytes: 18,
      },
      meta: { transport: 'download' },
    })
    await expect(readFile(destination, 'utf8')).resolves.toBe('slow archive bytes')
  })

  it('propagates MFA errors from export authorization', async () => {
    const command = new CommandRegistry().require('deployment.data-export')
    const ports = createPorts(async () => Response.json({
      error: {
        code: 'mfa_required',
        message: 'Complete step-up authentication.',
        purpose: 'data_export',
      },
    }, { status: 403 }))

    await expect(command.handler(invocation(command.metadata, {
      projectId: 'project-a',
      applicationId: 'application-a',
      targetId: 'target-a',
      destination: 'unused.tar.gz',
    }), ports)).rejects.toMatchObject({
      code: 'mfa_required',
      status: 403,
    })
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
    protocol: { fetch },
    env: {},
    isTTY: false,
  }
}
