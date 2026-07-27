import type { Writable } from 'node:stream'
import type { OutputStreams } from '../output/index.js'
import type {
  CommandExecutionGlobals,
  CommandResult,
  NormalizedCommandMetadata,
  OutputPort,
} from './types.js'
import { Buffer } from 'node:buffer'
import process from 'node:process'
import { LunaError, normalizeLunaError } from '../errors/index.js'
import {
  createSuccessEnvelope,
  OutputChannels,
} from '../output/index.js'
import { CLI_VERSION } from '../version.js'

export interface CommandOutputOptions {
  readonly streams?: OutputStreams
  readonly version?: string
  readonly translate?: (key: string, fallback: string, locale?: string) => string
}

export class CommandOutput implements OutputPort {
  readonly #streams: OutputStreams
  readonly #version: string
  readonly #translate?: CommandOutputOptions['translate']

  constructor(options: CommandOutputOptions = {}) {
    this.#streams = options.streams ?? {
      stdout: process.stdout,
      stderr: process.stderr,
    }
    this.#version = options.version ?? CLI_VERSION
    this.#translate = options.translate
  }

  writeSuccess(
    metadata: NormalizedCommandMetadata,
    result: CommandResult,
    globals: CommandExecutionGlobals,
  ): void {
    const channels = new OutputChannels(this.#streams, { quiet: globals.quiet })
    const envelope = createSuccessEnvelope(
      result.schemaVersion ?? metadata.schemaVersion ?? 'unversioned',
      metadata.operationId ?? metadata.canonicalPath,
      metadata.canonicalPath,
      result.data,
      {
        requestId: stringMeta(result.meta, 'requestId') ?? globals.requestId,
        server: globals.server,
        projectId: stringMeta(result.meta, 'projectId') ?? globals.project,
        cliVersion: this.#version,
        openapiDigest: metadata.schemaDigest,
      },
    )
    channels.writeResult(
      globals.output === 'table' || globals.output === 'name'
        ? result.data
        : envelope,
      {
        format: globals.output,
        rawData: result.data,
      },
    )
  }

  writeError(error: unknown, globals?: Partial<CommandExecutionGlobals>): void {
    const channels = new OutputChannels(this.#streams, { quiet: globals?.quiet })
    const machine = Boolean(globals?.agent || globals?.output !== 'table')
    if (machine || !this.#translate) {
      channels.writeError(error, machine)
      return
    }
    const normalized = normalizeLunaError(error)
    channels.writeError(new LunaError(
      normalized.code,
      this.#translate(`errors.${normalized.code}`, normalized.message, globals?.lang),
      {
        status: normalized.status,
        exitCode: normalized.exitCode,
        retryable: normalized.retryable,
        requestId: normalized.requestId,
        retryAfter: normalized.retryAfter,
        purpose: normalized.purpose,
        fields: normalized.fields,
        details: normalized.details,
      },
    ))
  }

  writeInfo(message: string, globals?: Partial<CommandExecutionGlobals>): void {
    const channels = new OutputChannels(this.#streams, { quiet: globals?.quiet })
    channels.writeInfo(message)
  }
}

export function memoryOutputStreams(): {
  streams: OutputStreams
  stdout: () => string
  stderr: () => string
} {
  let standardOutput = ''
  let standardError = ''
  const stream = (append: (value: string) => void): Pick<Writable, 'write'> => ({
    write(chunk: string | Uint8Array): boolean {
      append(typeof chunk === 'string' ? chunk : Buffer.from(chunk).toString('utf8'))
      return true
    },
  })
  return {
    streams: {
      stdout: stream(value => standardOutput += value),
      stderr: stream(value => standardError += value),
    },
    stdout: () => standardOutput,
    stderr: () => standardError,
  }
}

function stringMeta(
  meta: Readonly<Record<string, unknown>> | undefined,
  key: string,
): string | undefined {
  const value = meta?.[key]
  return typeof value === 'string' ? value : undefined
}
