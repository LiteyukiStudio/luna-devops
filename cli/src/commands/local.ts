import type { CompletionShell } from './completion.js'
import type { CommandRegistry } from './registry.js'
import type {
  CommandMetadata,
  CommandParameter,
  CommandResult,
  LunaConfigDocument,
  LunaContext,
  LunaInstance,
  OutputFormat,
} from './types.js'
import process from 'node:process'
import {
  beginOAuthLogin,
  getAuthStatus,
  logoutLocal,
  storeValidatedAccessToken,
} from '../auth/index.js'
import { CLI_VERSION } from '../version.js'
import { generateCompletion } from './completion.js'
import { CliCommandError } from './errors.js'
import { catalogResult, commandHelpResult } from './help.js'

const stringSchema = { type: 'string' } as const
const booleanSchema = { type: 'boolean' } as const
const integerSchema = { type: 'integer' } as const

export function registerLocalCommands(registry: CommandRegistry): void {
  registerVersion(registry)
  registerHelp(registry)
  registerCompletion(registry)
  registerAuth(registry)
  registerContext(registry)
  registerProjectContext(registry)
  registerApiDiagnostic(registry)
}

function registerAuth(registry: CommandRegistry): void {
  registry.register(localMetadata('auth', 'login', {
    summary: 'Authenticate a Luna context with an access token.',
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
  }), async (invocation, ports) => {
    const mode = optionalString(invocation.params.mode) ?? 'access-token'
    if (mode === 'device-code') {
      return beginOAuthLogin({
        server: invocation.globals.server ?? '',
        context: invocation.globals.context ?? 'default',
        scopes: stringList(invocation.params.scope),
        mode: 'device_code',
      })
    }
    if (mode !== 'access-token') {
      throw invalidArguments(
        'mode must be access-token or device-code.',
        'mode',
      )
    }

    const config = await ports.config.read()
    const selected = selectedContext(config, invocation.globals.context)
    const context = invocation.globals.context ?? selected.name ?? 'default'
    const server = invocation.globals.server ?? selected.instance?.server
    if (!server) {
      throw invalidArguments(
        'auth.login requires server=<https://luna.example.com> when no context is configured.',
        'server',
      )
    }
    const token = optionalString(invocation.params.token)?.trim()
      ?? optionalString(ports.env?.LUNA_TOKEN)
        ?.trim()
    if (!token) {
      throw invalidArguments(
        'auth.login requires token=@- or the LUNA_TOKEN environment variable.',
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
      context,
      server,
      token,
      scopes: stringList(invocation.params.scope),
      user: userId
        ? {
            id: userId,
            ...user,
          }
        : undefined,
      makeCurrent: true,
    })
    return {
      schemaVersion: 'auth.login/v1',
      data: {
        context,
        server,
        authenticated: true,
        user,
      },
    }
  })

  registry.register(localMetadata('auth', 'status', {
    summary: 'Show authentication status without exposing credentials.',
    schemaVersion: 'auth.status/v1',
    parameters: [parameter('all', { schema: booleanSchema })],
  }), async (invocation, ports) => ({
    schemaVersion: 'auth.status/v1',
    data: await getAuthStatus(ports.config, {
      context: invocation.globals.context,
      all: invocation.params.all === true,
      env: ports.env,
    }),
  }))

  registry.register(localMetadata('auth', 'logout', {
    summary: 'Remove credentials from one or all local Luna contexts.',
    schemaVersion: 'auth.logout/v1',
    risk: 'medium',
    parameters: [parameter('all', { schema: booleanSchema })],
  }), async (invocation, ports) => ({
    schemaVersion: 'auth.logout/v1',
    data: await logoutLocal(ports.config, {
      context: invocation.globals.context,
      all: invocation.params.all === true,
    }),
  }))
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
  }), async invocation => catalogResult(registry, invocation.params))

  registry.register(localMetadata('help', 'command', {
    summary: 'Show the complete machine-readable contract for one command.',
    schemaVersion: 'help.command/v1',
    parameters: [parameter('path', { required: true })],
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

function registerContext(registry: CommandRegistry): void {
  registry.register(localMetadata('context', 'list', {
    summary: 'List configured Luna contexts.',
    schemaVersion: 'context.list/v1',
  }), async (_invocation, ports) => {
    const config = await ports.config.read()
    return {
      schemaVersion: 'context.list/v1',
      data: Object.entries(config.contexts)
        .sort(([left], [right]) => left.localeCompare(right))
        .map(([name, context]) => contextView(name, context, config, config.currentContext === name)),
    }
  })

  registry.register(localMetadata('context', 'current', {
    summary: 'Show the current Luna context.',
    schemaVersion: 'context.current/v1',
  }), async (_invocation, ports) => {
    const config = await ports.config.read()
    if (!config.currentContext) {
      return { schemaVersion: 'context.current/v1', data: null }
    }
    const context = config.contexts[config.currentContext]
    if (!context) {
      throw new CliCommandError(
        'context_invalid',
        `Current context "${config.currentContext}" does not exist.`,
        { status: 409, details: { context: config.currentContext } },
      )
    }
    return {
      schemaVersion: 'context.current/v1',
      data: contextView(config.currentContext, context, config, true),
    }
  })

  registry.register(localMetadata('context', 'use', {
    summary: 'Switch the current Luna context.',
    schemaVersion: 'context.use/v1',
    parameters: [parameter('name', { required: true })],
  }), async (invocation, ports) => {
    const name = requiredString(invocation.params.name, 'name')
    const config = await ports.config.read()
    if (!config.contexts[name]) {
      throw new CliCommandError('context_not_found', `Context "${name}" was not found.`, {
        status: 404,
        details: { name },
      })
    }
    await ports.config.write({ ...config, currentContext: name })
    return {
      schemaVersion: 'context.use/v1',
      data: contextView(name, config.contexts[name]!, config, true),
    }
  })

  registry.register(localMetadata('context', 'set', {
    summary: 'Create or update a Luna context.',
    schemaVersion: 'context.set/v1',
    projectContext: 'optional',
    parameters: [
      parameter('name', { required: true }),
      parameter('credential'),
      parameter('projectName'),
      parameter('projectIdentifier'),
      parameter('language'),
    ],
  }), async (invocation, ports) => {
    const name = requiredString(invocation.params.name, 'name')
    const config = await ports.config.read()
    const existing = config.contexts[name]
    const requestedServer = invocation.explicitGlobalKeys.has('server')
      ? optionalString(invocation.globals.server)
      : undefined
    const existingInstance = existing ? config.instances[existing.instance] : undefined
    const server = requestedServer
      ? normalizeServer(requestedServer)
      : existingInstance?.server
    if (!server) {
      throw invalidArguments('server is required when creating a context.', 'server')
    }
    const instance = findInstanceName(config.instances, server) ?? uniqueInstanceName(name, config.instances)
    const serverChanged = Boolean(existingInstance && existingInstance.server !== server)
    const projectId = invocation.explicitGlobalKeys.has('project')
      ? optionalString(invocation.globals.project)
      : undefined
    const project = projectId
      ? {
          id: projectId,
          name: optionalString(invocation.params.projectName),
          identifier: optionalString(invocation.params.projectIdentifier),
        }
      : serverChanged
        ? null
        : existing?.project
    const output = invocation.explicitGlobalKeys.has('output')
      ? outputFormat(invocation.canonicalGlobalValues.output)
      : undefined
    const context: LunaContext = {
      ...existing,
      instance,
      credential: serverChanged
        ? undefined
        : optionalString(invocation.params.credential) ?? existing?.credential,
      project,
      output: output ?? existing?.output,
      language: optionalString(invocation.params.language) ?? existing?.language,
    }
    const next = {
      ...config,
      currentContext: config.currentContext ?? name,
      instances: {
        ...config.instances,
        [instance]: { ...(config.instances[instance] ?? {}), server },
      },
      contexts: { ...config.contexts, [name]: context },
    }
    await ports.config.write(next)
    return {
      schemaVersion: 'context.set/v1',
      data: contextView(name, context, next, next.currentContext === name),
    }
  })

  registry.register(localMetadata('context', 'rename', {
    summary: 'Rename a Luna context.',
    schemaVersion: 'context.rename/v1',
    parameters: [
      parameter('name', { required: true }),
      parameter('newName', { required: true }),
    ],
  }), async (invocation, ports) => {
    const name = requiredString(invocation.params.name, 'name')
    const newName = requiredString(invocation.params.newName, 'newName')
    const config = await ports.config.read()
    const context = config.contexts[name]
    if (!context)
      throw notFound(name)
    if (config.contexts[newName]) {
      throw new CliCommandError('context_exists', `Context "${newName}" already exists.`, {
        status: 409,
        details: { newName },
      })
    }
    const contexts = { ...config.contexts }
    delete contexts[name]
    contexts[newName] = context
    const next = {
      ...config,
      currentContext: config.currentContext === name ? newName : config.currentContext,
      contexts,
    }
    await ports.config.write(next)
    return {
      schemaVersion: 'context.rename/v1',
      data: contextView(newName, context, next, next.currentContext === newName),
    }
  })

  registry.register(localMetadata('context', 'delete', {
    summary: 'Delete a Luna context.',
    schemaVersion: 'context.delete/v1',
    risk: 'high',
    parameters: [parameter('name', { required: true })],
  }), async (invocation, ports) => {
    const name = requiredString(invocation.params.name, 'name')
    const config = await ports.config.read()
    if (!config.contexts[name])
      throw notFound(name)
    if (config.currentContext === name && !invocation.globals.yes) {
      throw new CliCommandError(
        'confirmation_required',
        'Deleting the current context requires yes=true or --yes.',
        { status: 409, details: { name } },
      )
    }
    const contexts = { ...config.contexts }
    delete contexts[name]
    await ports.config.write({
      ...config,
      currentContext: config.currentContext === name ? null : config.currentContext,
      contexts,
    })
    return { schemaVersion: 'context.delete/v1', data: { name, deleted: true } }
  })

  registry.register(localMetadata('context', 'view', {
    summary: 'Show the redacted Luna configuration.',
    schemaVersion: 'context.view/v1',
  }), async (_invocation, ports) => ({
    schemaVersion: 'context.view/v1',
    data: redactConfig(await ports.config.read()),
  }))
}

function registerProjectContext(registry: CommandRegistry): void {
  registry.register(localMetadata('project', 'current', {
    summary: 'Show the project selected by the current context.',
    schemaVersion: 'project.current/v1',
    projectContext: 'optional',
  }), async (invocation, ports) => {
    const config = await ports.config.read()
    const selected = selectedContext(config, invocation.globals.context)
    return {
      schemaVersion: 'project.current/v1',
      data: {
        context: selected.name,
        server: selected.instance?.server ?? invocation.globals.server ?? null,
        project: invocation.explicitGlobalKeys.has('project')
          ? invocation.globals.project
            ? { id: invocation.globals.project }
            : null
          : selected.context?.project ?? null,
        source: invocation.explicitGlobalKeys.has('project')
          ? 'argument'
          : ports.env?.LUNA_PROJECT
            ? 'environment'
            : selected.context?.project
              ? 'context'
              : 'none',
      },
    }
  })

  registry.register(localMetadata('project', 'use', {
    summary: 'Set the current context project after server validation.',
    schemaVersion: 'project.use/v1',
    projectContext: 'optional',
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
    const config = await ports.config.read()
    const selected = selectedContext(config, invocation.globals.context)
    if (!selected.name || !selected.context) {
      throw new CliCommandError('context_required', 'A current context is required.', {
        status: 400,
        exitCode: 2,
      })
    }
    const project = await ports.api.resolveProject(value, invocation.globals)
    const context = { ...selected.context, project }
    const next = {
      ...config,
      contexts: { ...config.contexts, [selected.name]: context },
    }
    await ports.config.write(next)
    return { schemaVersion: 'project.use/v1', data: project }
  })

  registry.register(localMetadata('project', 'unset', {
    summary: 'Clear the project selected by the current context.',
    schemaVersion: 'project.unset/v1',
  }), async (invocation, ports) => {
    const config = await ports.config.read()
    const selected = selectedContext(config, invocation.globals.context)
    if (!selected.name || !selected.context) {
      throw new CliCommandError('context_required', 'A current context is required.', {
        status: 400,
        exitCode: 2,
      })
    }
    const context = { ...selected.context, project: null }
    await ports.config.write({
      ...config,
      contexts: { ...config.contexts, [selected.name]: context },
    })
    return { schemaVersion: 'project.unset/v1', data: { context: selected.name, project: null } }
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

function contextView(
  name: string,
  context: LunaContext,
  config: LunaConfigDocument,
  current: boolean,
): Readonly<Record<string, unknown>> {
  const instance = config.instances[context.instance]
  const credential = context.credential ? config.credentials[context.credential] : undefined
  return {
    name,
    current,
    instance: context.instance,
    server: instance?.server ?? null,
    credential: context.credential ?? null,
    authType: optionalString(credential?.type) ?? null,
    userId: optionalString(credential?.userId) ?? null,
    expiresAt: optionalString(credential?.expiresAt) ?? null,
    scopes: Array.isArray(credential?.scopes) ? credential.scopes : [],
    project: context.project ?? null,
    language: context.language ?? null,
    output: context.output || null,
  }
}

function selectedContext(config: LunaConfigDocument, override?: string): {
  name?: string
  context?: LunaContext
  instance?: LunaInstance
} {
  const name = override ?? config.currentContext ?? undefined
  const context = name ? config.contexts[name] : undefined
  return {
    name,
    context,
    instance: context ? config.instances[context.instance] : undefined,
  }
}

function findInstanceName(
  instances: Readonly<Record<string, LunaInstance>>,
  server: string,
): string | undefined {
  return Object.entries(instances).find(([, instance]) => instance.server === server)?.[0]
}

function uniqueInstanceName(
  preferred: string,
  instances: Readonly<Record<string, LunaInstance>>,
): string {
  if (!instances[preferred])
    return preferred
  let suffix = 2
  while (instances[`${preferred}-${suffix}`]) suffix += 1
  return `${preferred}-${suffix}`
}

function normalizeServer(value: string): string {
  let url: URL
  try {
    url = new URL(value)
  }
  catch (cause) {
    throw new CliCommandError('invalid_arguments', 'server must be an absolute HTTP(S) URL.', {
      status: 400,
      exitCode: 2,
      details: { key: 'server' },
      cause,
    })
  }
  if (!['http:', 'https:'].includes(url.protocol)
    || url.username
    || url.password
    || url.hash
    || (url.pathname !== '/' && url.pathname !== '')) {
    throw invalidArguments(
      'server must be an HTTP(S) origin without credentials, path, query, or fragment.',
      'server',
    )
  }
  if (url.search)
    throw invalidArguments('server must not contain a query.', 'server')
  return url.origin
}

function redactConfig(config: LunaConfigDocument): LunaConfigDocument {
  return {
    ...config,
    credentials: Object.fromEntries(
      Object.entries(config.credentials).map(([name, credential]) => [
        name,
        redactRecord(credential),
      ]),
    ),
  }
}

function redactRecord(value: Readonly<Record<string, unknown>>): Readonly<Record<string, unknown>> {
  return Object.fromEntries(
    Object.entries(value).map(([key, entry]) => [
      key,
      /token|secret|password|cookie|authorization|recovery/i.test(key)
        ? entry === undefined || entry === null || entry === ''
          ? entry
          : '******'
        : typeof entry === 'object' && entry !== null && !Array.isArray(entry)
          ? redactRecord(entry as Readonly<Record<string, unknown>>)
          : entry,
    ]),
  )
}

function outputFormat(value: unknown): OutputFormat | '' | undefined {
  if (value === '')
    return ''
  if (
    value === 'table'
    || value === 'json'
    || value === 'raw-json'
    || value === 'yaml'
    || value === 'jsonl'
    || value === 'name'
  ) {
    return value
  }
  if (value === undefined)
    return undefined
  throw invalidArguments(`Unsupported output format "${String(value)}".`, 'output')
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

function notFound(name: string): CliCommandError {
  return new CliCommandError('context_not_found', `Context "${name}" was not found.`, {
    status: 404,
    details: { name },
  })
}

function invalidArguments(message: string, key?: string): CliCommandError {
  return new CliCommandError('invalid_arguments', message, {
    status: 400,
    exitCode: 2,
    details: key ? { key } : {},
  })
}
