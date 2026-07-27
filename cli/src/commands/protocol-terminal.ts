import type {
  CommandInvocation,
  CommandResult,
  ProtocolInputStream,
  ProtocolOutputStream,
  ProtocolWebSocket,
  ProtocolWebSocketEvent,
  RuntimePorts,
} from './types.js'
import { Buffer } from 'node:buffer'
import process from 'node:process'
import { CliCommandError } from './errors.js'
import { authorizeProtocolTicket } from './protocol-ticket.js'

const WEB_SOCKET_OPEN = 1
const TERMINAL_EOF_CLOSE_CODES = new Set([1000, 1001, 1005, 1006])

export async function executeWebSocketTerminal(
  invocation: CommandInvocation,
  ports: RuntimePorts,
): Promise<CommandResult> {
  assertInteractiveTty(invocation, ports)
  const operationId = authorizationOperation(invocation)
  const authorization = await authorizeProtocolTicket(invocation, ports, operationId)
  const stdin = ports.protocol?.stdin ?? asInputStream(process.stdin)
  const stdout = ports.protocol?.stdout ?? asOutputStream(process.stdout)
  const webSocket = createWebSocket(terminalUrl(
    authorization.server,
    invocation,
    authorization.ticket,
  ), ports)

  return runTerminalSession(invocation, webSocket, stdin, stdout, ports)
}

async function runTerminalSession(
  invocation: CommandInvocation,
  socket: ProtocolWebSocket,
  stdin: ProtocolInputStream,
  stdout: ProtocolOutputStream,
  ports: RuntimePorts,
): Promise<CommandResult> {
  socket.binaryType = 'arraybuffer'
  const previousRawMode = Boolean(stdin.isRaw)
  let bytesSent = 0
  let bytesReceived = 0
  let remoteExitCode: number | undefined
  let opened = false
  let settled = false
  let outputQueue = Promise.resolve()

  return new Promise<CommandResult>((resolve, reject) => {
    const handshakeTimeout = setTimeout(() => {
      if (opened || settled)
        return
      socket.close(1000, 'handshake timeout')
      settleError(new CliCommandError(
        'terminal_connection_timeout',
        'The terminal WebSocket connection timed out.',
        {
          status: 504,
          retryable: true,
          details: { timeoutMs: invocation.globals.timeoutMs },
        },
      ))
    }, invocation.globals.timeoutMs)

    const onOpen = () => {
      opened = true
      clearTimeout(handshakeTimeout)
      try {
        stdin.setRawMode?.(true)
        stdin.resume?.()
        sendResize(socket, stdout)
      }
      catch (error) {
        socket.close(1011, 'terminal initialization failed')
        settleError(new CliCommandError(
          'terminal_initialization_failed',
          'The local terminal could not enter interactive mode.',
          { status: 500, cause: error },
        ))
      }
    }
    const onMessage = (event: ProtocolWebSocketEvent) => {
      outputQueue = outputQueue.then(async () => {
        const result = await writeTerminalMessage(event.data, stdout)
        bytesReceived += result.bytes
        if (result.exitCode !== undefined)
          remoteExitCode = result.exitCode
      }).catch((error) => {
        socket.close(1011, 'terminal output failed')
        settleError(new CliCommandError(
          'terminal_output_failed',
          'The terminal output could not be written.',
          { status: 500, cause: error },
        ))
      })
    }
    const onError = () => {
      socket.close(1011, 'terminal connection failed')
      settleError(new CliCommandError(
        opened ? 'terminal_connection_error' : 'terminal_connection_failed',
        opened
          ? 'The terminal WebSocket connection failed.'
          : 'The terminal WebSocket could not be established.',
        { status: 502, retryable: true },
      ))
    }
    const onClose = (event: ProtocolWebSocketEvent) => {
      if (settled)
        return
      void outputQueue.then(() => {
        if (settled)
          return
        const closeCode = event.code ?? 1006
        const exitCode = remoteExitCode ?? exitCodeFromReason(event.reason)
        // The backend may finish an established exec stream without sending a
        // WebSocket close frame, which runtimes expose as 1005/1006. A preceding
        // error event still settles the session as a failure.
        const backendReachedEof = opened && TERMINAL_EOF_CLOSE_CODES.has(closeCode)
        if (!backendReachedEof && exitCode === undefined) {
          settleError(new CliCommandError(
            'terminal_connection_closed',
            'The terminal WebSocket closed unexpectedly.',
            {
              status: 502,
              retryable: true,
              details: { closeCode, reason: event.reason ?? '' },
            },
          ))
          return
        }
        settleSuccess(closeCode, event.reason ?? '', exitCode ?? 0)
      })
    }
    const onInput = (chunk: unknown) => {
      if (socket.readyState !== WEB_SOCKET_OPEN)
        return
      const payload = terminalInput(chunk)
      if (!payload)
        return
      bytesSent += byteLength(payload)
      socket.send(payload)
    }
    const onResize = () => {
      if (socket.readyState === WEB_SOCKET_OPEN)
        sendResize(socket, stdout)
    }
    const onInterrupt = () => {
      if (settled)
        return
      socket.close(1000, 'interrupted')
      settleError(new CliCommandError(
        'terminal_interrupted',
        'The terminal session was interrupted.',
        { status: 499, exitCode: 130 },
      ))
    }
    const unsubscribeInterrupt = subscribeInterrupt(ports, onInterrupt)

    function cleanup(): void {
      clearTimeout(handshakeTimeout)
      socket.removeEventListener?.('open', onOpen)
      socket.removeEventListener?.('message', onMessage)
      socket.removeEventListener?.('error', onError)
      socket.removeEventListener?.('close', onClose)
      stdin.off?.('data', onInput)
      stdout.off?.('resize', onResize)
      unsubscribeInterrupt()
      try {
        stdin.setRawMode?.(previousRawMode)
        if (!previousRawMode)
          stdin.pause?.()
      }
      catch {
        // The process may already be shutting down.
      }
    }
    function settleError(error: CliCommandError): void {
      if (settled)
        return
      settled = true
      cleanup()
      reject(error)
    }
    function settleSuccess(closeCode: number, reason: string, exitCode: number): void {
      if (settled)
        return
      if (exitCode !== 0) {
        settleError(new CliCommandError(
          'terminal_remote_exit',
          `The remote terminal exited with code ${exitCode}.`,
          {
            status: 500,
            exitCode: processExitCode(exitCode),
            details: {
              remoteExitCode: exitCode,
              closeCode,
              reason,
            },
          },
        ))
        return
      }
      settled = true
      cleanup()
      resolve({
        schemaVersion: 'cli.luna.devops/terminal/v1',
        data: {
          exitCode,
          closeCode,
          reason,
          bytesSent,
          bytesReceived,
        },
        meta: { transport: 'websocket' },
      })
    }

    socket.addEventListener('open', onOpen)
    socket.addEventListener('message', onMessage)
    socket.addEventListener('error', onError)
    socket.addEventListener('close', onClose)
    stdin.on('data', onInput)
    stdout.on?.('resize', onResize)
  })
}

function assertInteractiveTty(invocation: CommandInvocation, ports: RuntimePorts): void {
  if (invocation.globals.agent) {
    throw new CliCommandError(
      'terminal_agent_unsupported',
      'Interactive terminal commands are not available in agent mode.',
      {
        status: 422,
        details: {
          command: invocation.metadata.canonicalPath,
          remediation: 'Use a non-interactive exec command in agent mode.',
        },
      },
    )
  }
  const stdin = ports.protocol?.stdin ?? asInputStream(process.stdin)
  const stdout = ports.protocol?.stdout ?? asOutputStream(process.stdout)
  if (!invocation.globals.interactive || !ports.isTTY || !stdin.isTTY || !stdout.isTTY) {
    throw new CliCommandError(
      'terminal_tty_required',
      'This command requires an interactive TTY.',
      {
        status: 422,
        details: {
          command: invocation.metadata.canonicalPath,
          remediation: 'Run the command in an interactive terminal without --no-interactive.',
        },
      },
    )
  }
}

function authorizationOperation(invocation: CommandInvocation): string {
  const operation = invocation.metadata.consumedOperations?.find(candidate =>
    candidate.toLocaleLowerCase().includes('authorize'))
  if (!operation) {
    throw new CliCommandError(
      'terminal_authorization_operation_missing',
      'The terminal command has no authorization operation.',
      { status: 500, details: { command: invocation.metadata.canonicalPath } },
    )
  }
  return operation
}

function createWebSocket(url: string, ports: RuntimePorts): ProtocolWebSocket {
  if (ports.protocol?.createWebSocket)
    return ports.protocol.createWebSocket(url)
  if (typeof globalThis.WebSocket !== 'function') {
    throw new CliCommandError(
      'websocket_runtime_unavailable',
      'This CLI runtime does not provide WebSocket support.',
      { status: 501 },
    )
  }
  return new globalThis.WebSocket(url) as unknown as ProtocolWebSocket
}

function terminalUrl(
  server: string,
  invocation: CommandInvocation,
  ticket: string,
): string {
  const url = new URL(interpolatePath(invocation.metadata.path ?? '', invocation.params), server)
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  for (const parameter of invocation.metadata.parameters) {
    if (parameter.location !== 'query' || parameter.name === 'ticket')
      continue
    appendQueryValue(url, parameter.name, invocation.params[parameter.name])
  }
  url.searchParams.set('ticket', ticket)
  return url.toString()
}

function interpolatePath(
  template: string,
  params: Readonly<Record<string, unknown>>,
): string {
  return template.replace(/\{([^}]+)\}/g, (_match, name: string) => {
    const value = params[name]
    if (value === undefined || value === null || value === '') {
      throw new CliCommandError('invalid_arguments', `Missing path parameter "${name}".`, {
        status: 400,
        exitCode: 2,
      })
    }
    return encodeURIComponent(String(value))
  })
}

function appendQueryValue(url: URL, name: string, value: unknown): void {
  if (value === undefined || value === null || value === '')
    return
  if (Array.isArray(value)) {
    for (const item of value)
      appendQueryValue(url, name, item)
    return
  }
  url.searchParams.append(name, String(value))
}

function sendResize(socket: ProtocolWebSocket, stdout: ProtocolOutputStream): void {
  const cols = positiveInteger(stdout.columns)
  const rows = positiveInteger(stdout.rows)
  if (!cols || !rows)
    return
  socket.send(JSON.stringify({ type: 'resize', cols, rows }))
}

async function writeTerminalMessage(
  data: unknown,
  stdout: ProtocolOutputStream,
): Promise<{ bytes: number, exitCode?: number }> {
  const exitCode = remoteExitMessage(data)
  if (exitCode !== undefined)
    return { bytes: 0, exitCode }
  if (typeof data === 'string') {
    stdout.write(data)
    return { bytes: Buffer.byteLength(data) }
  }
  if (data instanceof ArrayBuffer) {
    const bytes = new Uint8Array(data)
    stdout.write(bytes)
    return { bytes: bytes.byteLength }
  }
  if (ArrayBuffer.isView(data)) {
    const bytes = new Uint8Array(data.buffer, data.byteOffset, data.byteLength)
    stdout.write(bytes)
    return { bytes: bytes.byteLength }
  }
  if (data instanceof Blob) {
    const bytes = new Uint8Array(await data.arrayBuffer())
    stdout.write(bytes)
    return { bytes: bytes.byteLength }
  }
  return { bytes: 0 }
}

function remoteExitMessage(data: unknown): number | undefined {
  if (typeof data !== 'string' || data.length > 256)
    return undefined
  try {
    const value = JSON.parse(data) as unknown
    if (
      typeof value === 'object'
      && value !== null
      && (value as { type?: unknown }).type === 'exit'
    ) {
      const code = (value as { code?: unknown }).code
      return typeof code === 'number' && Number.isSafeInteger(code) ? code : undefined
    }
  }
  catch {
    // Ordinary terminal text is not structured protocol data.
  }
  return undefined
}

function terminalInput(value: unknown): string | ArrayBuffer | ArrayBufferView | undefined {
  if (typeof value === 'string' || value instanceof ArrayBuffer || ArrayBuffer.isView(value))
    return value
  return undefined
}

function byteLength(value: string | ArrayBuffer | ArrayBufferView): number {
  if (typeof value === 'string')
    return Buffer.byteLength(value)
  return value.byteLength
}

function exitCodeFromReason(reason: string | undefined): number | undefined {
  const match = /(?:^|\b)exit(?:Code)?[=: ]+(-?\d+)(?:\b|$)/i.exec(reason ?? '')
  if (!match)
    return undefined
  const value = Number(match[1])
  return Number.isSafeInteger(value) ? value : undefined
}

function positiveInteger(value: number | undefined): number | undefined {
  return typeof value === 'number' && Number.isSafeInteger(value) && value > 0
    ? value
    : undefined
}

function processExitCode(value: number): number {
  return Number.isSafeInteger(value) && value > 0 && value <= 255 ? value : 1
}

function subscribeInterrupt(ports: RuntimePorts, listener: () => void): () => void {
  if (ports.protocol?.onInterrupt)
    return ports.protocol.onInterrupt(listener)
  process.once('SIGINT', listener)
  process.once('SIGTERM', listener)
  return () => {
    process.off('SIGINT', listener)
    process.off('SIGTERM', listener)
  }
}

function asInputStream(value: unknown): ProtocolInputStream {
  return value as ProtocolInputStream
}

function asOutputStream(value: unknown): ProtocolOutputStream {
  return value as ProtocolOutputStream
}
