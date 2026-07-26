import type {
  OutputFormat,
  ProjectContextSnapshot,
} from '../commands/types.js'
import type { LunaCredential } from './schema.js'
import process from 'node:process'
import { CliCommandError } from '../commands/errors.js'
import {
  DEFAULT_LUNA_SERVER,
  OUTPUT_FORMATS,
  parseConfigDocument,
} from './schema.js'
import { normalizeServerOrigin } from './server.js'

export type ResolutionSource
  = | 'argument'
    | 'environment'
    | 'config'
    | 'default'
    | 'none'

export interface ResolveRuntimeOptions {
  readonly server?: string
  readonly project?: string
  readonly output?: OutputFormat | ''
  readonly language?: string
  readonly env?: Readonly<Record<string, string | undefined>>
}

export interface ResolvedRuntimeContext {
  readonly server: string
  readonly project?: ProjectContextSnapshot
  readonly credential?: LunaCredential
  readonly output?: OutputFormat | ''
  readonly language?: string
  readonly sources: {
    readonly server: ResolutionSource
    readonly project: ResolutionSource
    readonly credential: ResolutionSource
    readonly output: ResolutionSource
    readonly language: ResolutionSource
  }
}

export function resolveRuntimeContext(
  rawConfig: unknown,
  options: ResolveRuntimeOptions = {},
): ResolvedRuntimeContext {
  const config = parseConfigDocument(rawConfig)
  const env = options.env ?? process.env
  const configuredServer = normalizeServerOrigin(config.server || DEFAULT_LUNA_SERVER)
  const explicitServer = nonEmpty(options.server)
  const environmentServer = nonEmpty(env.LUNA_SERVER)
  const serverOverride = explicitServer ?? environmentServer
  const server = serverOverride
    ? normalizeServerOrigin(serverOverride)
    : configuredServer
  const sameOrigin = server === configuredServer

  const environmentToken = nonEmpty(env.LUNA_TOKEN)
  const credential = environmentToken
    ? {
        type: 'access_token' as const,
        token: environmentToken,
        scopes: [],
      }
    : sameOrigin
      ? config.credential ?? undefined
      : undefined

  const explicitProject = nonEmpty(options.project)
  const environmentProject = nonEmpty(env.LUNA_PROJECT)
  const projectOverride = explicitProject ?? environmentProject
  const project = projectOverride
    ? { id: projectOverride }
    : sameOrigin
      ? config.project ?? undefined
      : undefined

  const explicitOutput = options.output === '' ? undefined : options.output
  const environmentOutput = outputValue(env.LUNA_OUTPUT)
  const configuredOutput = config.output || undefined
  const output = explicitOutput ?? environmentOutput ?? configuredOutput
  const explicitLanguage = nonEmpty(options.language)
  const environmentLanguage = nonEmpty(env.LUNA_LANG)
  const configuredLanguage = nonEmpty(config.language)
  const language = explicitLanguage ?? environmentLanguage ?? configuredLanguage

  return {
    server,
    project,
    credential,
    output,
    language,
    sources: {
      server: explicitServer
        ? 'argument'
        : environmentServer
          ? 'environment'
          : config.server
            ? 'config'
            : 'default',
      project: explicitProject
        ? 'argument'
        : environmentProject
          ? 'environment'
          : project
            ? 'config'
            : 'none',
      credential: environmentToken
        ? 'environment'
        : credential
          ? 'config'
          : 'none',
      output: explicitOutput
        ? 'argument'
        : environmentOutput
          ? 'environment'
          : configuredOutput
            ? 'config'
            : 'default',
      language: explicitLanguage
        ? 'argument'
        : environmentLanguage
          ? 'environment'
          : configuredLanguage
            ? 'config'
            : 'default',
    },
  }
}

function nonEmpty(value: string | undefined | null): string | undefined {
  const normalized = value?.trim()
  return normalized || undefined
}

function outputValue(value: string | undefined): OutputFormat | undefined {
  const normalized = nonEmpty(value)
  if (!normalized)
    return undefined
  if (!(OUTPUT_FORMATS as readonly string[]).includes(normalized)) {
    throw new CliCommandError(
      'output_format_invalid',
      `Unsupported output format "${normalized}".`,
      { status: 422 },
    )
  }
  return normalized as OutputFormat
}
