import type {
  CommandInvocation,
  CommandParameter,
  RuntimePorts,
} from './types.js'
import { CliCommandError } from './errors.js'
import { openProtocolRequest } from './protocol-request.js'

const LOCAL_PARAMETERS = new Set([
  'destination',
  'maxBytes',
  'maxEvents',
  'overwrite',
  'ticket',
])

export interface ProtocolTicket {
  readonly ticket: string
  readonly expiresAt?: string
  readonly server: string
}

export async function authorizeProtocolTicket(
  invocation: CommandInvocation,
  ports: RuntimePorts,
  operationId: string,
): Promise<ProtocolTicket> {
  const authorizationInvocation: CommandInvocation = {
    ...invocation,
    metadata: {
      ...invocation.metadata,
      operationId,
      method: 'POST',
      path: `${invocation.metadata.path}/authorize`,
      transport: 'http',
      streaming: false,
      parameters: authorizationParameters(invocation.metadata.parameters),
    },
    params: Object.fromEntries(
      Object.entries(invocation.params).filter(([name]) => !LOCAL_PARAMETERS.has(name)),
    ),
  }
  const { response, server } = await openProtocolRequest(
    authorizationInvocation,
    ports,
    'application/json',
  )
  const headerTicket = nonEmpty(response.headers.get('x-luna-terminal-ticket'))
    ?? nonEmpty(response.headers.get('x-terminal-ticket'))
    ?? nonEmpty(response.headers.get('x-luna-ticket'))
  const payload = await readAuthorizationPayload(response)
  const root = asRecord(payload)
  const data = asRecord(root.data)
  const ticket = headerTicket
    ?? nonEmpty(root.ticket)
    ?? nonEmpty(data.ticket)
  if (!ticket) {
    throw new CliCommandError(
      'protocol_ticket_missing',
      'The authorization endpoint did not issue a protocol ticket.',
      {
        status: 502,
        details: {
          command: invocation.metadata.canonicalPath,
          operationId,
          remediation: 'Upgrade the Luna DevOps server to a version that supports CLI protocol tickets.',
        },
      },
    )
  }
  const expiresAt = nonEmpty(root.expiresAt) ?? nonEmpty(data.expiresAt)
  return {
    ticket,
    server,
    ...(expiresAt ? { expiresAt } : {}),
  }
}

async function readAuthorizationPayload(response: Response): Promise<unknown> {
  if (response.status === 204)
    return {}
  const contentType = response.headers.get('content-type') ?? ''
  try {
    return contentType.includes('json') ? await response.json() : {}
  }
  catch {
    return {}
  }
}

function authorizationParameters(
  parameters: readonly CommandParameter[],
): readonly CommandParameter[] {
  return parameters.filter(parameter => !LOCAL_PARAMETERS.has(parameter.name))
}

function asRecord(value: unknown): Readonly<Record<string, unknown>> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
    ? value as Readonly<Record<string, unknown>>
    : {}
}

function nonEmpty(value: unknown): string | undefined {
  return typeof value === 'string' && value.trim() ? value.trim() : undefined
}
