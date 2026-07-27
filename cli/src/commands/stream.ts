import type {
  CommandInvocation,
  CommandResult,
  RuntimePorts,
} from './types.js'
import { Buffer } from 'node:buffer'
import { randomUUID } from 'node:crypto'
import {
  CLI_STREAM_VERSION,
  createStreamEvent,
  createStreamSummary,
} from '../output/envelope.js'
import { CliCommandError } from './errors.js'
import { openProtocolRequest } from './protocol-request.js'

const DEFAULT_MAX_EVENTS = 100
const DEFAULT_MAX_BYTES = 4 * 1024 * 1024

interface ParsedSseEvent {
  readonly event: string
  readonly id?: string
  readonly data: string
}

export async function executeSseStream(
  invocation: CommandInvocation,
  ports: RuntimePorts,
): Promise<CommandResult> {
  const maxEvents = integerParam(invocation.params.maxEvents, DEFAULT_MAX_EVENTS)
  const maxBytes = integerParam(invocation.params.maxBytes, DEFAULT_MAX_BYTES)
  // Connection setup uses --timeout. Once connected, SSE lifetime is bounded by
  // maxEvents, maxBytes, a terminal event, or the server closing the stream.
  const { response, requestId } = await openProtocolRequest(
    invocation,
    ports,
    'text/event-stream',
  )
  if (!(response.headers.get('content-type') ?? '').includes('text/event-stream')) {
    await response.body?.cancel()
    throw new CliCommandError(
      'sse_content_type_invalid',
      'The endpoint did not return a Server-Sent Events stream.',
      {
        status: 502,
        details: { contentType: response.headers.get('content-type') ?? '' },
      },
    )
  }
  if (!response.body) {
    throw new CliCommandError('sse_body_missing', 'The SSE response has no body.', {
      status: 502,
    })
  }

  const correlationId = requestId ?? invocation.globals.requestId ?? randomUUID()
  const resourceRef = streamResourceRef(invocation)
  const events: unknown[] = []
  let bytesRead = 0
  let completed = false
  let reason = 'eof'

  try {
    for await (const event of parseSse(response.body)) {
      bytesRead += Buffer.byteLength(event.data, 'utf8')
      if (bytesRead > maxBytes) {
        reason = 'max_bytes'
        break
      }
      if (event.event === 'error') {
        const payload = parseEventData(event.data)
        throw new CliCommandError(
          stringField(payload, 'code') ?? 'sse_remote_error',
          stringField(payload, 'message') ?? 'The server ended the stream with an error.',
          {
            status: 502,
            retryable: true,
            details: { eventId: event.id ?? '', payload },
          },
        )
      }

      events.push(createStreamEvent({
        type: event.event || 'message',
        sequence: events.length + 1,
        correlationId,
        operationId: invocation.metadata.operationId
          ?? invocation.metadata.consumedOperations?.[0]
          ?? invocation.metadata.canonicalPath,
        resourceRef,
        data: parseEventData(event.data),
        ...(event.id ? { resumeCursor: event.id } : {}),
      }))
      if (event.event === 'done') {
        completed = true
        reason = 'done'
        break
      }
      if (events.length >= maxEvents) {
        reason = 'max_events'
        break
      }
    }
  }
  catch (error) {
    if (error instanceof CliCommandError)
      throw error
    throw new CliCommandError('sse_stream_failed', 'The SSE stream ended unexpectedly.', {
      status: 502,
      retryable: true,
      details: { eventsRead: events.length, bytesRead },
      cause: error,
    })
  }
  finally {
    await response.body.cancel().catch(() => undefined)
  }

  const summary = createStreamSummary({
    sequence: events.length + 1,
    correlationId,
    operationId: invocation.metadata.operationId
      ?? invocation.metadata.consumedOperations?.[0]
      ?? invocation.metadata.canonicalPath,
    resourceRef,
    data: {
      completed,
      reason,
      eventsRead: events.length,
      bytesRead,
    },
  })
  return {
    schemaVersion: CLI_STREAM_VERSION,
    data: {
      streamVersion: CLI_STREAM_VERSION,
      events,
      summary,
    },
    meta: {
      ...(requestId ? { requestId } : {}),
      transport: 'sse',
    },
  }
}

export async function* parseSse(
  body: ReadableStream<Uint8Array>,
): AsyncGenerator<ParsedSseEvent> {
  const reader = body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  let eventName = ''
  let eventId: string | undefined
  let dataLines: string[] = []

  const dispatch = (): ParsedSseEvent | undefined => {
    if (dataLines.length === 0 && !eventName && eventId === undefined)
      return undefined
    const event = {
      event: eventName || 'message',
      ...(eventId !== undefined ? { id: eventId } : {}),
      data: dataLines.join('\n'),
    }
    eventName = ''
    eventId = undefined
    dataLines = []
    return event
  }

  try {
    while (true) {
      const chunk = await reader.read()
      if (chunk.done)
        break
      buffer += decoder.decode(chunk.value, { stream: true })
      const lines = buffer.split(/\r\n|\r|\n/)
      buffer = lines.pop() ?? ''
      for (const line of lines) {
        if (line === '') {
          const event = dispatch()
          if (event)
            yield event
          continue
        }
        if (line.startsWith(':'))
          continue
        const separator = line.indexOf(':')
        const field = separator < 0 ? line : line.slice(0, separator)
        const rawValue = separator < 0 ? '' : line.slice(separator + 1)
        const value = rawValue.startsWith(' ') ? rawValue.slice(1) : rawValue
        if (field === 'event')
          eventName = value
        else if (field === 'id' && !value.includes('\0'))
          eventId = value
        else if (field === 'data')
          dataLines.push(value)
      }
    }
    buffer += decoder.decode()
    if (buffer) {
      const separator = buffer.indexOf(':')
      const field = separator < 0 ? buffer : buffer.slice(0, separator)
      const rawValue = separator < 0 ? '' : buffer.slice(separator + 1)
      const value = rawValue.startsWith(' ') ? rawValue.slice(1) : rawValue
      if (field === 'event')
        eventName = value
      else if (field === 'id' && !value.includes('\0'))
        eventId = value
      else if (field === 'data')
        dataLines.push(value)
    }
    const finalEvent = dispatch()
    if (finalEvent)
      yield finalEvent
  }
  finally {
    reader.releaseLock()
  }
}

function streamResourceRef(invocation: CommandInvocation): { kind: string, id: string } {
  const id = stringParam(invocation.params.jobId)
    ?? stringParam(invocation.params.targetId)
    ?? stringParam(invocation.params.releaseId)
    ?? invocation.metadata.canonicalPath
  return {
    kind: invocation.params.jobId
      ? 'BuildJob'
      : invocation.params.targetId
        ? 'DeploymentTarget'
        : 'Resource',
    id,
  }
}

function parseEventData(value: string): unknown {
  if (!value)
    return null
  try {
    return JSON.parse(value)
  }
  catch {
    return value
  }
}

function integerParam(value: unknown, fallback: number): number {
  return typeof value === 'number' && Number.isSafeInteger(value) && value > 0
    ? value
    : fallback
}

function stringParam(value: unknown): string | undefined {
  return typeof value === 'string' && value ? value : undefined
}

function stringField(value: unknown, key: string): string | undefined {
  if (typeof value !== 'object' || value === null || Array.isArray(value))
    return undefined
  const field = (value as Readonly<Record<string, unknown>>)[key]
  return typeof field === 'string' && field ? field : undefined
}
