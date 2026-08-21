import type { AddressInfo } from 'node:net'
import { createServer } from 'node:http'
import { resourceFromAttributes } from '@opentelemetry/resources'
import { BatchSpanProcessor, WebTracerProvider } from '@opentelemetry/sdk-trace-web'
import { describe, expect, it } from 'vitest'
import { createBrowserTraceExporter } from './browser-telemetry-runtime'
import { normalizeTelemetryRoute } from './telemetry'

describe('browser trace exporter', () => {
  it('sends OTLP protobuf to the same-origin relay contract', async () => {
    let contentType = ''
    let bodyByteLength = 0
    const server = createServer((request, response) => {
      contentType = request.headers['content-type'] ?? ''
      request.on('data', (chunk: Uint8Array) => {
        bodyByteLength += chunk.byteLength
      })
      request.on('end', () => {
        response.writeHead(200).end()
      })
    })
    await new Promise<void>((resolve, reject) => {
      server.once('error', reject)
      server.listen(0, '127.0.0.1', resolve)
    })
    const address = server.address() as AddressInfo

    const exporter = createBrowserTraceExporter(`http://127.0.0.1:${address.port}/api/v1/telemetry/v1/traces`)
    const provider = new WebTracerProvider({
      resource: resourceFromAttributes({ 'service.name': 'luna-web-test' }),
      spanProcessors: [new BatchSpanProcessor(exporter)],
    })
    provider.getTracer('browser-telemetry-test').startSpan('navigation.change').end()
    await provider.forceFlush()
    await provider.shutdown()
    await new Promise<void>((resolve, reject) => server.close(error => error ? reject(error) : resolve()))

    expect(contentType).toBe('application/x-protobuf')
    expect(bodyByteLength).toBeGreaterThan(0)
  })
})

describe('normalizeTelemetryRoute', () => {
  it('removes query data and resource identifiers from span names', () => {
    expect(normalizeTelemetryRoute('/api/v1/projects/prj_secret/applications/app_secret?token=hidden'))
      .toBe('/api/v1/projects/:id/applications/:id')
  })

  it('keeps stable action segments', () => {
    expect(normalizeTelemetryRoute('/api/v1/auth/oidc/provider-id/start'))
      .toBe('/api/v1/auth/oidc/:id/start')
  })

  it('removes project volume and transfer IDs from telemetry routes', () => {
    expect(normalizeTelemetryRoute('/api/v1/projects/prj_1/volumes/pvol_secret/exports'))
      .toBe('/api/v1/projects/:id/volumes/:id/exports')
    expect(normalizeTelemetryRoute('/api/v1/projects/prj_1/volume-transfers/vtx_secret/content?ticket=hidden'))
      .toBe('/api/v1/projects/:id/volume-transfers/:id/content')
  })
})
