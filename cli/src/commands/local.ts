import type { CompletionShell } from './completion.js'
import type { CommandRegistry } from './registry.js'
import type {
  CommandMetadata,
  CommandParameter,
  CommandResult,
  LunaApiMeta,
  RuntimePorts,
} from './types.js'
import process from 'node:process'
import {
  getAuthStatus,
  logoutLocal,
  storeValidatedAccessToken,
  storeValidatedOAuthCredential,
} from '../auth/index.js'
import { resolveRuntimeContext } from '../config/resolve.js'
import { DEFAULT_LUNA_SERVER } from '../config/schema.js'
import { CLI_VERSION } from '../version.js'
import {
  isVersionAtLeast,
  SUPPORTED_SERVER_API_VERSIONS,
} from './compatibility.js'
import { generateCompletion } from './completion.js'
import { CliCommandError, toCliCommandError } from './errors.js'
import { catalogResult, commandHelpResult } from './help.js'

const stringSchema = { type: 'string' } as const
const booleanSchema = { type: 'boolean' } as const
const integerSchema = { type: 'integer' } as const

function translate(
  ports: RuntimePorts,
  key: string,
  fallback: string,
  locale?: string,
): string {
  return ports.translate?.(key, fallback, locale) ?? fallback
}

export function registerLocalCommands(registry: CommandRegistry): void {
  registerVersion(registry)
  registerHelp(registry)
  registerCompletion(registry)
  registerAuth(registry)
  registerProjectSelection(registry)
  registerDoctor(registry)
  registerApiDiagnostic(registry)
}

function registerAuth(registry: CommandRegistry): void {
  registry.register(localMetadata('auth', 'login', {
    summary: 'Authenticate to one Luna DevOps server with OAuth Device Code.',
    schemaVersion: 'auth.login/v1',
    risk: 'medium',
    parameters: [
      parameter('mode'),
      parameter('token', {
        sensitive: true,
        valueSources: ['file', 'stdin'],
      }),
      parameter('scope', { repeated: true }),
    ],
    examples: [
      'luna login',
      'luna login server=https://luna.example.com scope=project:read',
      'printf \'%s\' "$LUNA_TOKEN" | luna auth login mode=access-token token=@-',
    ],
  }), async (invocation, ports) => {
    const mode = optionalString(invocation.params.mode) ?? 'device-code'
    const server = invocation.globals.server ?? DEFAULT_LUNA_SERVER
    if (mode === 'device-code') {
      if (ports.api.getMeta) {
        const meta = await ports.api.getMeta(server, invocation.globals)
        if (meta.features.deviceCode === false) {
          throw new CliCommandError(
            'oauth_server_capability_unavailable',
            'The selected Luna server does not support device-code login.',
            {
              status: 501,
              details: {
                server,
                feature: 'deviceCode',
                fallback: 'access-token',
              },
            },
          )
        }
      }
      if (!ports.api.beginOAuthLogin) {
        throw new CliCommandError(
          'unsupported_feature',
          'The API client cannot start OAuth Device Code login.',
          { status: 501 },
        )
      }
      const result = await ports.api.beginOAuthLogin({
        server,
        scopes: stringList(invocation.params.scope),
        mode: 'device_code',
        onVerification: async (verification) => {
          const codePrompt = translate(
            ports,
            'auth.deviceCode.code',
            'Enter this device code to authorize Luna CLI:',
            invocation.globals.lang,
          )
          await ports.output.writeInfo?.(
            `${codePrompt} ${verification.userCode}`,
            invocation.globals,
          )
          const browserPrompt = translate(
            ports,
            verification.browserOpened
              ? 'auth.deviceCode.browserOpened'
              : 'auth.deviceCode.browserFallback',
            verification.browserOpened
              ? 'A browser was opened. Complete authorization there; manual URL:'
              : 'Open this URL in a browser to complete authorization:',
            invocation.globals.lang,
          )
          await ports.output.writeInfo?.(
            `${browserPrompt} ${verification.verificationUri}`,
            invocation.globals,
          )
        },
      })
      const userId = optionalString(result.user?.id)
      await storeValidatedOAuthCredential(ports.config, {
        server: result.server,
        accessToken: result.accessToken,
        refreshToken: result.refreshToken,
        tokenType: result.tokenType,
        scopes: result.scopes,
        expiresAt: result.expiresAt,
        user: userId
          ? {
              id: userId,
              ...result.user,
            }
          : undefined,
      })
      return {
        schemaVersion: 'auth.login/v1',
        data: {
          server: result.server,
          authenticated: true,
          credentialType: 'oauth',
          user: result.user,
        },
      }
    }
    if (mode !== 'access-token') {
      throw invalidArguments(
        'mode must be access-token or device-code.',
        'mode',
      )
    }

    const token = optionalString(invocation.params.token)?.trim()
      ?? optionalString(ports.env?.LUNA_TOKEN)
        ?.trim()
    if (!token) {
      throw invalidArguments(
        'Access-token login requires token=@- or the LUNA_TOKEN environment variable.',
        'token',
      )
    }
    if (!ports.api.validateAccessToken) {
      throw new CliCommandError(
        'unsupported_feature',
        'The API client cannot validate access tokens.',
        { status: 501 },
      )
    }

    const user = await ports.api.validateAccessToken(server, token, invocation.globals)
    const userId = optionalString(user.id)
    await storeValidatedAccessToken(ports.config, {
      server,
      token,
      scopes: stringList(invocation.params.scope),
      user: userId
        ? {
            id: userId,
            ...user,
          }
        : undefined,
    })
    return {
      schemaVersion: 'auth.login/v1',
      data: {
        server,
        authenticated: true,
        user,
      },
    }
  })

  registry.register(localMetadata('auth', 'status', {
    summary: 'Show authentication status without exposing credentials.',
    schemaVersion: 'auth.status/v1',
    examples: ['luna auth status'],
  }), async (invocation, ports) => ({
    schemaVersion: 'auth.status/v1',
    data: await getAuthStatus(ports.config, {
      env: ports.env,
    }),
  }))

  registry.register(localMetadata('auth', 'logout', {
    summary: 'Revoke OAuth tokens when possible and remove the local credential.',
    schemaVersion: 'auth.logout/v1',
    risk: 'medium',
    examples: ['luna auth logout'],
  }), async (invocation, ports) => ({
    schemaVersion: 'auth.logout/v1',
    data: await logoutLocal(ports.config, {
      revoke: request => ports.api.revokeOAuthCredential!(request),
    }),
  }))
}

function registerDoctor(registry: CommandRegistry): void {
  registry.register({
    category: 'health',
    tool: 'doctor',
    source: 'protocol',
    consumedOperations: ['getApiMeta'],
    summary: 'Check local CLI, authentication, and server compatibility.',
    schemaVersion: 'health.doctor/v1',
    risk: 'low',
    transport: 'http',
    projectContext: 'none',
    examples: [
      'luna doctor',
      'luna health doctor output=json',
      'luna health doctor server=https://luna.example.com',
    ],
  }, async (invocation, ports) => {
    const checks: DoctorCheck[] = []
    const config = await ports.config.read()
    const runtime = resolveRuntimeContext(config, {
      server: invocation.globals.server,
      project: invocation.globals.project,
      output: invocation.globals.output,
      language: invocation.globals.lang,
      env: ports.env,
    })
    const server = runtime.server
    const auth = await getAuthStatus(ports.config, {
      env: ports.env,
    })
    const authEntry = auth

    checks.push(doctorCheck('server-config', 'ok', 'server_configured', { server }))
    checks.push(authEntry?.authenticated
      ? doctorCheck('authentication', 'ok', 'authenticated', {
          credentialType: authEntry.credential?.type,
        })
      : doctorCheck('authentication', 'warning', 'not_authenticated'))

    let meta: LunaApiMeta | null = null
    if (server && ports.api.getMeta) {
      try {
        meta = await ports.api.getMeta(server, invocation.globals)
        checks.push(doctorCheck('server', 'ok', 'server_reachable', {
          serverVersion: meta.serverVersion,
          apiVersion: meta.apiVersion,
        }))
      }
      catch (error) {
        const normalized = toCliCommandError(error)
        checks.push(doctorCheck('server', 'error', normalized.code, {
          status: normalized.status,
          message: normalized.message,
        }))
      }
    }
    else if (server) {
      checks.push(doctorCheck('server', 'error', 'server_meta_unsupported'))
    }

    const localVersion = ports.version ?? CLI_VERSION
    if (meta) {
      checks.push(SUPPORTED_SERVER_API_VERSIONS.includes(meta.apiVersion)
        ? doctorCheck('api-version', 'ok', 'server_api_version_supported', {
            server: meta.apiVersion,
          })
        : doctorCheck('api-version', 'error', 'server_api_version_unsupported', {
            server: meta.apiVersion,
            supported: SUPPORTED_SERVER_API_VERSIONS,
          }))

      const compatible = isVersionAtLeast(localVersion, meta.minimumCliVersion)
      checks.push(compatible === true
        ? doctorCheck('cli-version', 'ok', 'cli_version_supported', {
            current: localVersion,
            minimum: meta.minimumCliVersion,
          })
        : compatible === false
          ? doctorCheck('cli-version', 'error', 'cli_version_too_old', {
              current: localVersion,
              minimum: meta.minimumCliVersion,
            })
          : doctorCheck('cli-version', 'warning', 'cli_version_unparseable', {
              current: localVersion,
              minimum: meta.minimumCliVersion,
            }))

      const localDigest = registry.catalogMetadata.openapiDigest
      if (localDigest === 'unavailable') {
        checks.push(doctorCheck('openapi-contract', 'warning', 'local_contract_unavailable'))
      }
      else if (localDigest === meta.openapiDigest) {
        checks.push(doctorCheck('openapi-contract', 'ok', 'openapi_digest_matches'))
      }
      else {
        checks.push(doctorCheck('openapi-contract', 'warning', 'openapi_digest_mismatch', {
          local: localDigest,
          server: meta.openapiDigest,
        }))
      }
    }

    const unsupported = meta
      ? Object.entries(meta.features)
          .filter(([, supported]) => !supported)
          .map(([feature]) => feature)
          .sort()
      : []
    if (unsupported.length > 0) {
      checks.push(doctorCheck('capabilities', 'warning', 'server_features_unavailable', {
        unsupported,
      }))
    }
    else if (meta) {
      checks.push(doctorCheck('capabilities', 'ok', 'server_features_available'))
    }

    return {
      schemaVersion: 'health.doctor/v1',
      data: {
        status: doctorStatus(checks),
        local: {
          version: localVersion,
          distribution:
            ports.distribution
            ?? (typeof process.versions.bun === 'string' ? 'binary' : 'source'),
          runtime: typeof process.versions.bun === 'string'
            ? `bun-${process.versions.bun}`
            : `node-${process.versions.node}`,
          catalogVersion: registry.catalogMetadata.catalogVersion,
          openapiDigest: registry.catalogMetadata.openapiDigest,
          schemaDigest: registry.catalogMetadata.schemaDigest,
        },
        login: {
          server: server ?? null,
          authenticated: authEntry?.authenticated ?? false,
          authType: authEntry?.credential?.type ?? null,
          project: runtime.project ?? null,
        },
        server: meta,
        unsupported,
        checks,
      },
    }
  })
}

type DoctorCheckStatus = 'ok' | 'warning' | 'error'

interface DoctorCheck {
  readonly name: string
  readonly status: DoctorCheckStatus
  readonly code: string
  readonly details?: Readonly<Record<string, unknown>>
}

function doctorCheck(
  name: string,
  status: DoctorCheckStatus,
  code: string,
  details?: Readonly<Record<string, unknown>>,
): DoctorCheck {
  return {
    name,
    status,
    code,
    ...(details ? { details } : {}),
  }
}

function doctorStatus(checks: readonly DoctorCheck[]): DoctorCheckStatus {
  if (checks.some(check => check.status === 'error'))
    return 'error'
  if (checks.some(check => check.status === 'warning'))
    return 'warning'
  return 'ok'
}

function registerVersion(registry: CommandRegistry): void {
  registry.register(localMetadata('version', 'show', {
    summary: 'Show Luna CLI version and runtime information.',
    schemaVersion: 'version.show/v1',
  }), async (_invocation, ports) => ({
    schemaVersion: 'version.show/v1',
    data: {
      version: ports.version ?? CLI_VERSION,
      distribution:
        ports.distribution
        ?? (typeof process.versions.bun === 'string' ? 'binary' : 'source'),
      runtime: typeof process.versions.bun === 'string'
        ? `bun-${process.versions.bun}`
        : `node-${process.versions.node}`,
      platform: process.platform,
      arch: process.arch,
    },
  }))
}

function registerHelp(registry: CommandRegistry): void {
  registry.register(localMetadata('help', 'catalog', {
    summary: 'List commands from the machine-readable command catalog.',
    schemaVersion: 'help.catalog/v1',
    parameters: [
      parameter('query'),
      parameter('category'),
      parameter('risk'),
      parameter('scope'),
      parameter('transport'),
      parameter('limit', { schema: integerSchema }),
      parameter('cursor'),
      parameter('all', { schema: booleanSchema }),
    ],
    examples: [
      'luna help catalog query=project limit=10',
      'luna help catalog category=deployment output=json interactive=false',
    ],
  }), async invocation => catalogResult(registry, invocation.params))

  registry.register(localMetadata('help', 'command', {
    summary: 'Show the complete machine-readable contract for one command.',
    schemaVersion: 'help.command/v1',
    parameters: [parameter('path', { required: true })],
    examples: ['luna help command path=project.get-projects output=json'],
  }), async invocation => commandHelpResult(registry, invocation.params))
}

function registerCompletion(registry: CommandRegistry): void {
  for (const shell of ['bash', 'zsh', 'fish', 'powershell'] as const) {
    registry.register(localMetadata('completion', shell, {
      summary: `Generate ${shell} completion for Luna CLI.`,
      schemaVersion: `completion.${shell}/v1`,
    }), async () => ({
      schemaVersion: `completion.${shell}/v1`,
      data: {
        shell,
        script: generateCompletion(shell as CompletionShell, registry),
      },
    }))
  }
}

function registerProjectSelection(registry: CommandRegistry): void {
  registry.register(localMetadata('project', 'current', {
    summary: 'Show the active default project.',
    schemaVersion: 'project.current/v1',
    projectContext: 'optional',
    examples: ['luna project current'],
  }), async (invocation, ports) => {
    const config = await ports.config.read()
    const runtime = resolveRuntimeContext(config, {
      server: invocation.globals.server,
      project: invocation.globals.project,
      env: ports.env,
    })
    return {
      schemaVersion: 'project.current/v1',
      data: {
        server: runtime.server,
        project: runtime.project ?? null,
        source: runtime.sources.project,
      },
    }
  })

  registry.register(localMetadata('project', 'use', {
    summary: 'Set the active default project after server validation.',
    schemaVersion: 'project.use/v1',
    projectContext: 'optional',
    examples: ['luna project use project=prj_example'],
  }), async (invocation, ports) => {
    if (!invocation.explicitGlobalKeys.has('project')) {
      throw invalidArguments('project.use requires an explicit project=<id-or-identifier>.', 'project')
    }
    const value = requiredString(invocation.globals.project, 'project')
    if (!ports.api.resolveProject) {
      throw new CliCommandError(
        'unsupported_feature',
        'The API client does not provide project resolution.',
        { status: 501 },
      )
    }
    const project = await ports.api.resolveProject(value, invocation.globals)
    const config = await ports.config.read()
    await ports.config.write({ ...config, project: { ...project } })
    return { schemaVersion: 'project.use/v1', data: project }
  })

  registry.register(localMetadata('project', 'unset', {
    summary: 'Clear the active default project.',
    schemaVersion: 'project.unset/v1',
    examples: ['luna project unset'],
  }), async (_invocation, ports) => {
    const config = await ports.config.read()
    await ports.config.write({ ...config, project: null })
    return { schemaVersion: 'project.unset/v1', data: { project: null } }
  })
}

function registerApiDiagnostic(registry: CommandRegistry): void {
  registry.register({
    ...localMetadata('api', 'request', {
      summary: 'Send a diagnostic request to a Luna API path.',
      schemaVersion: 'api.request/v1',
      risk: 'medium',
      agentAllowed: false,
      transport: 'http',
      parameters: [
        parameter('method', { required: true }),
        parameter('path', { required: true }),
        parameter('body', {
          valueSources: ['file', 'stdin'],
          schema: { type: ['object', 'array', 'string', 'null'] },
        }),
        parameter('allowDiagnostic', { schema: booleanSchema }),
      ],
      inputSchema: { type: 'object', additionalProperties: true },
      examples: [
        'luna api request method=GET path=/api/v1/health allowDiagnostic=true',
        'luna api request method=POST path=/api/v1/example body=@request.json allowDiagnostic=true',
      ],
    }),
    source: 'local',
  }, async (invocation, ports) => {
    const method = requiredString(invocation.params.method, 'method').toUpperCase()
    if (!['GET', 'HEAD', 'POST', 'PUT', 'PATCH', 'DELETE'].includes(method)) {
      throw invalidArguments(`Unsupported HTTP method "${method}".`, 'method')
    }
    const path = requiredString(invocation.params.path, 'path')
    if (!path.startsWith('/api/') || path.startsWith('//') || /^[a-z][a-z0-9+.-]*:/i.test(path)) {
      throw invalidArguments('path must be a relative Luna API path beginning with /api/.', 'path')
    }
    const { method: _method, path: _path, allowDiagnostic: _allow, ...params } = invocation.params
    const result = await ports.api.request({ method, path, params, globals: invocation.globals })
    return asResult(result, 'api.request/v1')
  })
}

function localMetadata(
  category: string,
  tool: string,
  details: Omit<CommandMetadata, 'category' | 'tool' | 'source'>,
): CommandMetadata {
  return {
    category,
    tool,
    source: 'local',
    risk: 'low',
    transport: 'local',
    projectContext: 'none',
    ...details,
  }
}

function parameter(
  name: string,
  options: Omit<CommandParameter, 'name'> = {},
): CommandParameter {
  return { name, schema: stringSchema, valueSources: ['inline'], ...options }
}

function asResult(value: unknown, schemaVersion: string): CommandResult {
  if (typeof value === 'object' && value !== null && 'data' in value) {
    return value as CommandResult
  }
  return { data: value, schemaVersion }
}

function requiredString(value: unknown, key: string): string {
  const result = optionalString(value)
  if (!result)
    throw invalidArguments(`Missing required argument "${key}".`, key)
  return result
}

function optionalString(value: unknown): string | undefined {
  return typeof value === 'string' && value.length > 0 ? value : undefined
}

function stringList(value: unknown): string[] {
  if (Array.isArray(value)) {
    return value.filter((entry): entry is string => typeof entry === 'string')
  }
  return typeof value === 'string' ? [value] : []
}

function invalidArguments(message: string, key?: string): CliCommandError {
  return new CliCommandError('invalid_arguments', message, {
    status: 400,
    exitCode: 2,
    details: key ? { key } : {},
  })
}
