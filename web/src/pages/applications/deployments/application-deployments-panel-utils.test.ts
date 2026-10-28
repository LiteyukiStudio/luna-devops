import type { DeploymentTarget, Release } from '@/api'
import { describe, expect, it } from 'vitest'
import { formatTargetRuntimeSize, latestReleaseByTarget } from './application-deployments-panel-utils'

describe('deployment panel utils', () => {
  it('formats runtime specs without repeating replicas', () => {
    const format = (_key: string, options?: Record<string, unknown>) => `${options?.cpu} · ${options?.memory}`
    const target = (cpuRequest: string, memoryRequest: string) => ({ cpuRequest, memoryRequest }) as DeploymentTarget

    expect(formatTargetRuntimeSize(target('500m', '1Gi'), format)).toBe('0.5 · 1G')
    expect(formatTargetRuntimeSize(target('125m', '512Mi'), format)).toBe('0.125 · 0.5G')
  })

  it('keeps the newest release for each deployment target', () => {
    const release = (id: string, deploymentTargetId: string, createdAt: string) => ({ createdAt, deploymentTargetId, id }) as Release
    const latest = latestReleaseByTarget([
      release('rel_old', 'target_1', '2026-08-17T00:00:00Z'),
      release('rel_new', 'target_1', '2026-08-18T00:00:00Z'),
      release('rel_other', 'target_2', '2026-08-16T00:00:00Z'),
    ])

    expect(latest.target_1?.id).toBe('rel_new')
    expect(latest.target_2?.id).toBe('rel_other')
  })
})
