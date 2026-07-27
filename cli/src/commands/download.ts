import type { Buffer } from 'node:buffer'
import type {
  CommandInvocation,
  CommandResult,
  RuntimePorts,
} from './types.js'
import { createHash, randomUUID } from 'node:crypto'
import { createWriteStream } from 'node:fs'
import { access, mkdir, rename, rm } from 'node:fs/promises'
import { basename, dirname, resolve } from 'node:path'
import { Readable, Transform } from 'node:stream'
import { pipeline } from 'node:stream/promises'
import { CliCommandError } from './errors.js'
import { openProtocolRequest } from './protocol-request.js'
import { authorizeProtocolTicket } from './protocol-ticket.js'

const DEFAULT_MAX_BYTES = 10 * 1024 * 1024 * 1024

export async function executeDownload(
  invocation: CommandInvocation,
  ports: RuntimePorts,
): Promise<CommandResult> {
  const authorizedInvocation = await withDownloadTicket(invocation, ports)
  // Connection setup uses --timeout. The transfer itself may exceed that value
  // and is bounded by maxBytes plus transport and filesystem failures.
  const { response, requestId } = await openProtocolRequest(
    authorizedInvocation,
    ports,
    'application/octet-stream, application/gzip',
  )
  if (!response.body) {
    throw new CliCommandError('download_body_missing', 'The download response has no body.', {
      status: 502,
    })
  }

  const requestedDestination = stringParam(invocation.params.destination)
  if (requestedDestination === '-' || requestedDestination === '@-') {
    await response.body.cancel()
    throw new CliCommandError(
      'download_stdout_unsupported',
      'Binary stdout is not available through this CLI output pipeline.',
      {
        status: 501,
        details: {
          remediation: 'Set destination to a file path.',
        },
      },
    )
  }
  const suggestedName = contentDispositionFilename(
    response.headers.get('content-disposition'),
  ) ?? 'luna-download.bin'
  const destination = resolve(requestedDestination ?? suggestedName)
  const overwrite = invocation.params.overwrite === true
  const maxBytes = integerParam(invocation.params.maxBytes, DEFAULT_MAX_BYTES)
  if (!overwrite && await exists(destination)) {
    await response.body.cancel()
    throw new CliCommandError(
      'download_destination_exists',
      `Download destination "${destination}" already exists.`,
      { status: 409, details: { destination } },
    )
  }

  await mkdir(dirname(destination), { recursive: true })
  const temporary = `${destination}.part-${randomUUID()}`
  const hash = createHash('sha256')
  let bytes = 0
  const meter = new Transform({
    transform(chunk: Buffer, _encoding, callback) {
      bytes += chunk.length
      if (bytes > maxBytes) {
        callback(new CliCommandError(
          'download_too_large',
          'The download exceeded the configured byte limit.',
          { status: 413, details: { maxBytes, bytesRead: bytes } },
        ))
        return
      }
      hash.update(chunk)
      callback(null, chunk)
    },
  })

  try {
    await pipeline(
      Readable.fromWeb(response.body as import('node:stream/web').ReadableStream),
      meter,
      createWriteStream(temporary, { flags: 'wx' }),
    )
    if (overwrite)
      await rm(destination, { force: true })
    await rename(temporary, destination)
  }
  catch (error) {
    await response.body.cancel().catch(() => undefined)
    await rm(temporary, { force: true }).catch(() => undefined)
    if (error instanceof CliCommandError)
      throw error
    throw new CliCommandError('download_failed', 'The download did not complete.', {
      status: 502,
      retryable: true,
      details: { destination, bytesRead: bytes },
      cause: error,
    })
  }

  return {
    schemaVersion: 'cli.luna.devops/download/v1',
    data: {
      destination,
      filename: basename(destination),
      bytes,
      sha256: hash.digest('hex'),
      contentType: response.headers.get('content-type') ?? 'application/octet-stream',
    },
    meta: {
      ...(requestId ? { requestId } : {}),
      transport: 'download',
    },
  }
}

async function withDownloadTicket(
  invocation: CommandInvocation,
  ports: RuntimePorts,
): Promise<CommandInvocation> {
  if (stringParam(invocation.params.ticket))
    return invocation
  const operationId = invocation.metadata.consumedOperations?.find(candidate =>
    candidate.toLocaleLowerCase().includes('authorize'))
  if (!operationId) {
    throw new CliCommandError(
      'download_authorization_operation_missing',
      'The download command has no authorization operation.',
      { status: 500, details: { command: invocation.metadata.canonicalPath } },
    )
  }
  const authorization = await authorizeProtocolTicket(invocation, ports, operationId)
  return {
    ...invocation,
    params: {
      ...invocation.params,
      ticket: authorization.ticket,
    },
  }
}

export function contentDispositionFilename(value: string | null): string | undefined {
  if (!value)
    return undefined
  const encoded = /filename\*\s*=\s*UTF-8''([^;]+)/i.exec(value)?.[1]
  const plain = /filename\s*=\s*(?:"([^"]+)"|([^;]+))/i.exec(value)
  let candidate: string | undefined
  try {
    candidate = encoded ? decodeURIComponent(encoded.trim()) : plain?.[1] ?? plain?.[2]?.trim()
  }
  catch {
    candidate = undefined
  }
  if (!candidate)
    return undefined
  const safe = basename(candidate)
    .split('')
    .filter((character) => {
      const code = character.charCodeAt(0)
      return code > 31 && code !== 127
    })
    .join('')
    .replace(/[\\/]/g, '-')
    .trim()
  return safe && safe !== '.' && safe !== '..' ? safe : undefined
}

async function exists(path: string): Promise<boolean> {
  try {
    await access(path)
    return true
  }
  catch {
    return false
  }
}

function stringParam(value: unknown): string | undefined {
  return typeof value === 'string' && value.trim() ? value.trim() : undefined
}

function integerParam(value: unknown, fallback: number): number {
  return typeof value === 'number' && Number.isSafeInteger(value) && value > 0
    ? value
    : fallback
}
