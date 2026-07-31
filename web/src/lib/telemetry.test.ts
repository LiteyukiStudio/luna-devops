import { describe, expect, it } from 'vitest'
import { normalizeTelemetryRoute } from './telemetry'

describe('normalizeTelemetryRoute', () => {
  it('removes query data and resource identifiers from span names', () => {
    expect(normalizeTelemetryRoute('/api/v1/projects/prj_secret/applications/app_secret?token=hidden'))
      .toBe('/api/v1/projects/:id/applications/:id')
  })

  it('keeps stable action segments', () => {
    expect(normalizeTelemetryRoute('/api/v1/auth/oidc/provider-id/start'))
      .toBe('/api/v1/auth/oidc/:id/start')
  })
})
