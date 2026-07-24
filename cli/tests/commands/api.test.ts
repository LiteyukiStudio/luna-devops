import type { ApiExecutionRequest, CommandExecutionGlobals } from '../../src/commands/index.js'
import { describe, expect, it } from 'vitest'
import {

  CliCommandError,
  LunaApiAdapter,

  normalizeMetadata,
  planOpenApiRequest,
} from '../../src/commands/index.js'

const globals: CommandExecutionGlobals = {
  project: 'project alpha',
  output: 'json',
  color: false,
  interactive: false,
  yes: false,
  quiet: true,
  agent: true,
  timeoutMs: 30_000,
  debug: false,
  insecureSkipTlsVerify: false,
}

describe('openAPI request planning', () => {
  it('reports the effective project from explicit command parameters', async () => {
    const adapter = new LunaApiAdapter({
      config: {
        read: async () => ({
          version: 1,
          currentContext: null,
          instances: {},
          credentials: {},
          contexts: {},
        }),
        write: async () => {},
      },
      clientFactory: () => ({
        request: async () => ({
          ok: true,
          status: 200,
          data: { items: [] },
          requestId: 'request-1',
        }),
      }) as never,
    })
    const metadata = normalizeMetadata({
      category: 'application',
      tool: 'list',
      source: 'openapi',
      operationId: 'listApplications',
      method: 'get',
      path: '/api/v1/projects/{projectId}/applications',
      parameters: [{ name: 'projectId', location: 'path', required: true }],
    })

    const result = await adapter.execute({
      operationId: 'listApplications',
      globals: { ...globals, server: 'https://luna.example.test' },
      params: { projectId: 'project explicit' },
      metadata,
    })

    expect(result.meta).toMatchObject({
      projectId: 'project explicit',
      requestId: 'request-1',
      status: 200,
    })
  })

  it('maps path, query, headers, body and server dry-run', () => {
    const request: ApiExecutionRequest = {
      operationId: 'updateApplication',
      globals: { ...globals, dryRun: 'server' },
      params: {
        applicationId: 'app/one',
        trace: 'trace-1',
        body: { name: 'demo' },
      },
      metadata: normalizeMetadata({
        category: 'application',
        tool: 'update',
        source: 'openapi',
        operationId: 'updateApplication',
        method: 'patch',
        path: '/api/v1/projects/{projectId}/applications/{applicationId}',
        parameters: [
          { name: 'projectId', location: 'path', required: true },
          { name: 'applicationId', location: 'path', required: true },
          { name: 'trace', location: 'header' },
          { name: 'body', location: 'body' },
        ],
      }),
    }

    expect(planOpenApiRequest(request)).toEqual({
      method: 'PATCH',
      path: '/api/v1/projects/project%20alpha/applications/app%2Fone',
      query: { dryRun: true },
      headers: { trace: 'trace-1' },
      body: { name: 'demo' },
    })
  })

  it('rejects header injection', () => {
    const request: ApiExecutionRequest = {
      operationId: 'inspect',
      globals,
      params: { trace: 'ok\r\nx-unsafe: yes' },
      metadata: normalizeMetadata({
        category: 'api',
        tool: 'inspect',
        source: 'openapi',
        operationId: 'inspect',
        method: 'get',
        path: '/api/v1/inspect',
        parameters: [{ name: 'trace', location: 'header' }],
      }),
    }

    expect(() => planOpenApiRequest(request)).toThrowError(CliCommandError)
  })

  it('does not inject the project context into optional parameters', () => {
    const request: ApiExecutionRequest = {
      operationId: 'listGlobalBuildTemplates',
      globals,
      params: {
        scope: 'global',
      },
      metadata: normalizeMetadata({
        category: 'build-template',
        tool: 'list',
        source: 'openapi',
        operationId: 'listGlobalBuildTemplates',
        method: 'get',
        path: '/api/v1/build-templates',
        parameters: [
          { name: 'projectId', location: 'query' },
          { name: 'scope', location: 'query' },
        ],
      }),
    }

    expect(planOpenApiRequest(request)).toEqual({
      method: 'GET',
      path: '/api/v1/build-templates',
      query: { scope: 'global' },
    })
  })
})
