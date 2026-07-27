import type {
  CommandHandler,
  CommandInvocation,
  CommandMetadata,
  CommandParameter,
  CommandResult,
  RuntimePorts,
} from './types.js'
import { executeDownload } from './download.js'
import { CliCommandError } from './errors.js'
import { executeWebSocketTerminal } from './protocol-terminal.js'
import { executeSseStream } from './stream.js'

const DATA_EXPORT_PATH
  = '/api/v1/projects/{projectId}/applications/{applicationId}/deployment-targets/{targetId}/data-export'
const BUILD_LOG_STREAM_PATH = '/api/v1/projects/{projectId}/build-jobs/{jobId}/logs/stream'
const DEPLOYMENT_METRICS_STREAM_PATH
  = '/api/v1/projects/{projectId}/applications/{applicationId}/deployment-targets/{targetId}/metrics/stream'
const RUNTIME_TERMINAL_PATH = '/api/v1/runtime/clusters/{clusterId}/pods/terminal'
const RELEASE_TERMINAL_PATH = '/api/v1/projects/{projectId}/releases/{releaseId}/terminal'

export interface ProtocolCommandDefinition {
  readonly metadata: CommandMetadata
  readonly handler: CommandHandler
}

export function protocolCommandDefinitions(): readonly ProtocolCommandDefinition[] {
  return [
    protocolDefinition({
      category: 'build',
      tool: 'job-logs-follow',
      source: 'protocol',
      consumedOperations: ['StreamBuildJobLogs'],
      method: 'GET',
      path: BUILD_LOG_STREAM_PATH,
      summary: 'Follow build job logs',
      transport: 'sse',
      streaming: true,
      projectContext: 'required',
      parameters: [
        pathParameter('projectId'),
        pathParameter('jobId'),
        queryParameter('after', { type: 'integer', minimum: 0 }),
        localParameter('maxEvents', { type: 'integer', minimum: 1, maximum: 10_000 }),
        localParameter('maxBytes', { type: 'integer', minimum: 1, maximum: 16 * 1024 * 1024 }),
      ],
      scopes: ['build:read'],
      examples: [
        'luna build job-logs-follow jobId=bldj_example maxEvents=200',
        'luna build job-logs-follow jobId=bldj_example --agent',
      ],
    }),
    protocolDefinition({
      category: 'deployment',
      tool: 'metrics-follow',
      source: 'protocol',
      consumedOperations: ['StreamDeploymentTargetMetrics'],
      method: 'GET',
      path: DEPLOYMENT_METRICS_STREAM_PATH,
      summary: 'Follow deployment runtime metrics',
      transport: 'sse',
      streaming: true,
      projectContext: 'required',
      parameters: [
        pathParameter('projectId'),
        pathParameter('applicationId'),
        pathParameter('targetId'),
        localParameter('maxEvents', { type: 'integer', minimum: 1, maximum: 10_000 }),
        localParameter('maxBytes', { type: 'integer', minimum: 1, maximum: 16 * 1024 * 1024 }),
      ],
      scopes: ['deployment:read'],
      examples: [
        'luna deployment metrics-follow applicationId=app_example targetId=dplt_example maxEvents=10',
      ],
    }),
    protocolDefinition({
      category: 'deployment',
      tool: 'data-export',
      source: 'protocol',
      consumedOperations: [
        'fallback_deployments_post_api_v1_projects_by_project_id_applications_by_application_id_deployment_targets_by_target_id_data_export_authorize',
        'fallback_deployments_get_api_v1_projects_by_project_id_applications_by_application_id_deployment_targets_by_target_id_data_export',
      ],
      method: 'GET',
      path: DATA_EXPORT_PATH,
      summary: 'Download a deployment data export',
      transport: 'download',
      projectContext: 'required',
      risk: 'high',
      mfaPurpose: 'data_export',
      parameters: [
        pathParameter('projectId'),
        pathParameter('applicationId'),
        pathParameter('targetId'),
        queryParameter('ticket', { type: 'string', minLength: 1 }),
        localParameter('destination', { type: 'string', minLength: 1 }),
        localParameter('overwrite', { type: 'boolean' }),
        localParameter('maxBytes', { type: 'integer', minimum: 1 }),
      ],
      scopes: ['runtime:data:export'],
      examples: [
        'luna deployment data-export applicationId=app_example targetId=dplt_example destination=backup.tar.gz',
      ],
    }),
    webSocketDefinition({
      category: 'cluster',
      tool: 'pod-terminal',
      path: RUNTIME_TERMINAL_PATH,
      consumedOperations: [
        'AuthorizeRuntimeClusterPodTerminal',
        'StreamRuntimeClusterPodTerminal',
      ],
      parameters: [
        pathParameter('clusterId'),
        queryParameter('namespace', { type: 'string', minLength: 1 }, true),
        queryParameter('name', { type: 'string', minLength: 1 }, true),
        queryParameter('container', { type: 'string' }),
      ],
      mfaPurpose: 'runtime_terminal',
    }),
    webSocketDefinition({
      category: 'release',
      tool: 'terminal',
      path: RELEASE_TERMINAL_PATH,
      consumedOperations: [
        'AuthorizeReleaseRuntimeTerminal',
        'StreamReleaseRuntimeTerminal',
      ],
      projectContext: 'required',
      parameters: [
        pathParameter('projectId'),
        pathParameter('releaseId'),
        queryParameter('container', { type: 'string' }),
      ],
      mfaPurpose: 'runtime_terminal',
    }),
  ]
}

export function prepareProtocolRegistration(
  metadata: CommandMetadata,
  handler: CommandHandler,
): { metadata: CommandMetadata, handler: CommandHandler } {
  const inferred = inferKnownTransport(metadata)
  if (!isProtocolTransport(inferred.transport))
    return { metadata: inferred, handler }
  return {
    metadata: inferred,
    handler: protocolHandler,
  }
}

async function protocolHandler(
  invocation: CommandInvocation,
  ports: RuntimePorts,
): Promise<CommandResult> {
  if (invocation.metadata.transport === 'sse')
    return executeSseStream(invocation, ports)
  if (invocation.metadata.transport === 'download')
    return executeDownload(invocation, ports)
  if (invocation.metadata.transport === 'websocket')
    return executeWebSocketTerminal(invocation, ports)
  throw new CliCommandError(
    'protocol_transport_unsupported',
    `Protocol transport "${invocation.metadata.transport}" is not supported.`,
    { status: 501, details: { transport: invocation.metadata.transport } },
  )
}

function protocolDefinition(metadata: CommandMetadata): ProtocolCommandDefinition {
  return { metadata, handler: protocolHandler }
}

function webSocketDefinition(
  options: Omit<CommandMetadata, 'source' | 'method' | 'transport' | 'streaming' | 'agentAllowed'>,
): ProtocolCommandDefinition {
  return protocolDefinition({
    ...options,
    source: 'protocol',
    method: 'GET',
    transport: 'websocket',
    streaming: true,
    agentAllowed: true,
    risk: options.risk ?? 'high',
    summary: options.summary ?? 'Open an interactive terminal',
  })
}

function inferKnownTransport(metadata: CommandMetadata): CommandMetadata {
  if (metadata.path === DATA_EXPORT_PATH && metadata.method?.toUpperCase() === 'GET') {
    return {
      ...metadata,
      transport: 'download',
      parameters: withMissingParameters(metadata.parameters, [
        localParameter('destination', { type: 'string', minLength: 1 }),
        localParameter('overwrite', { type: 'boolean' }),
        localParameter('maxBytes', { type: 'integer', minimum: 1 }),
      ]),
    }
  }
  return metadata
}

function isProtocolTransport(
  value: CommandMetadata['transport'],
): value is 'sse' | 'websocket' | 'download' | 'upload' {
  return value === 'sse' || value === 'websocket' || value === 'download' || value === 'upload'
}

function withMissingParameters(
  parameters: readonly CommandParameter[] | undefined,
  additions: readonly CommandParameter[],
): readonly CommandParameter[] {
  const result = [...(parameters ?? [])]
  const names = new Set(result.map(parameter => parameter.name))
  for (const parameter of additions) {
    if (!names.has(parameter.name))
      result.push(parameter)
  }
  return result
}

function pathParameter(name: string): CommandParameter {
  return {
    name,
    location: 'path',
    required: true,
    schema: { type: 'string', minLength: 1 },
  }
}

function queryParameter(
  name: string,
  schema: Readonly<Record<string, unknown>>,
  required = false,
): CommandParameter {
  return { name, location: 'query', required, schema }
}

function localParameter(
  name: string,
  schema: Readonly<Record<string, unknown>>,
): CommandParameter {
  return { name, schema }
}
