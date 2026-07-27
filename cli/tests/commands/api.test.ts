import type { ApiExecutionRequest, CommandExecutionGlobals } from '../../src/commands/index.js'
import type { StoredLunaConfig } from '../../src/config/schema.js'
import { describe, expect, it } from 'vitest'
import {
  CliCommandError,
  LunaApiAdapter,
  normalizeMetadata,
  planOpenApiRequest,
} from '../../src/commands/index.js'
import { MemoryConfigStore } from '../config/memory-store.js'

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
          version: 2,
          server: 'https://devops.liteyuki.org',
          credential: null,
          project: null,
          language: '',
          output: '',
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

describe('automatic server compatibility negotiation', () => {
  it('validates metadata once before canonical remote commands', async () => {
    const paths: string[] = []
    const adapter = compatibleAdapter(paths)
    const request = listApplicationsRequest()

    await adapter.execute(request)
    await adapter.execute(request)

    expect(paths).toEqual([
      '/api/v1/meta',
      '/api/v1/projects/project%20explicit/applications',
      '/api/v1/projects/project%20explicit/applications',
    ])
  })

  it('fails closed before sending a command with a mismatched contract', async () => {
    const paths: string[] = []
    const adapter = compatibleAdapter(paths, 'sha256:different')

    await expect(adapter.execute(listApplicationsRequest()))
      .rejects
      .toMatchObject({ code: 'openapi_digest_mismatch' })
    expect(paths).toEqual(['/api/v1/meta'])
  })

  it('keeps the generic diagnostic request available as an escape hatch', async () => {
    const paths: string[] = []
    const adapter = compatibleAdapter(paths, 'sha256:different')

    await adapter.request({
      method: 'GET',
      path: '/api/v1/health',
      params: {},
      globals: { ...globals, server: 'https://luna.example.test' },
    })

    expect(paths).toEqual(['/api/v1/health'])
  })
})

describe('oAuth credential refresh', () => {
  it('refreshes an expiring credential before sending the API request', async () => {
    const store = new MemoryConfigStore(oauthConfig())
    const refreshedTokens: string[] = []
    const requestTokens: Array<string | undefined> = []
    const adapter = new LunaApiAdapter({
      config: store,
      now: () => Date.parse('2026-07-27T10:00:00.000Z'),
      oauthClient: {
        beginOAuthLogin: async () => {
          throw new Error('not used')
        },
        refreshOAuthCredential: async ({ refreshToken }) => {
          refreshedTokens.push(refreshToken)
          return {
            accessToken: 'access-refreshed',
            refreshToken: 'refresh-rotated',
            tokenType: 'Bearer',
            scopes: ['project:read'],
            expiresAt: '2026-07-27T11:00:00.000Z',
          }
        },
        revokeOAuthCredential: async () => {},
      },
      clientFactory: options => ({
        request: async () => {
          requestTokens.push(await options.tokenProvider?.getAccessToken())
          return {
            ok: true,
            status: 200,
            data: { ok: true },
            requestId: 'request-refreshed',
          }
        },
      }) as never,
    })

    await adapter.request({
      method: 'GET',
      path: '/api/v1/health',
      params: {},
      globals,
    })

    expect(refreshedTokens).toEqual(['refresh-original'])
    expect(requestTokens).toEqual(['access-refreshed'])
    expect(store.value.credential).toMatchObject({
      type: 'oauth',
      accessToken: 'access-refreshed',
      refreshToken: 'refresh-rotated',
      expiresAt: '2026-07-27T11:00:00.000Z',
    })
  })
})

function compatibleAdapter(paths: string[], serverDigest = 'sha256:contract') {
  return new LunaApiAdapter({
    config: {
      read: async () => ({
        version: 2,
        server: 'https://devops.liteyuki.org',
        credential: null,
        project: null,
        language: '',
        output: '',
      }),
      write: async () => {},
    },
    compatibility: {
      cliVersion: '0.0.7',
      openapiDigest: 'sha256:contract',
    },
    clientFactory: () => ({
      request: async ({ path }: { path: string }) => {
        paths.push(path)
        return {
          ok: true,
          status: 200,
          data: path === '/api/v1/meta'
            ? {
                apiVersion: 'v1',
                serverVersion: '0.1.0',
                openapiDigest: serverDigest,
                minimumCliVersion: '0.0.7',
                features: {},
              }
            : { items: [] },
          requestId: `request-${paths.length}`,
        }
      },
    }) as never,
  })
}

function listApplicationsRequest(): ApiExecutionRequest {
  return {
    operationId: 'listApplications',
    globals: { ...globals, server: 'https://luna.example.test' },
    params: { projectId: 'project explicit' },
    metadata: normalizeMetadata({
      category: 'application',
      tool: 'list',
      source: 'openapi',
      operationId: 'listApplications',
      method: 'get',
      path: '/api/v1/projects/{projectId}/applications',
      parameters: [{ name: 'projectId', location: 'path', required: true }],
    }),
  }
}

function oauthConfig(): StoredLunaConfig {
  return {
    version: 2,
    server: 'https://luna.example.test',
    credential: {
      type: 'oauth',
      accessToken: 'access-expiring',
      refreshToken: 'refresh-original',
      tokenType: 'Bearer',
      scopes: ['project:read'],
      expiresAt: '2026-07-27T10:00:10.000Z',
      createdAt: '2026-07-27T09:00:00.000Z',
    },
    project: null,
    language: '',
    output: '',
  }
}
