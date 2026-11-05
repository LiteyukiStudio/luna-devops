import type { DeploymentTarget, Release } from '@/api'
import { describe, expect, it } from 'vitest'
import { aggregateApplicationRuntime, latestReleaseForTarget } from './application-facts'

const target = (overrides: Partial<DeploymentTarget>) => ({ desiredReplicas: 1, enabled: true, id: 'target', readyReplicas: 1, status: 'ready', ...overrides }) as DeploymentTarget
const release = (overrides: Partial<Release>) => ({ createdAt: '2026-10-01T00:00:00Z', deploymentTargetId: 'target', id: 'release', status: 'succeeded', ...overrides }) as Release

describe('application facts', () => {
  it('reports not deployed until an enabled target has a release', () => {
    expect(aggregateApplicationRuntime([target({})], [])).toEqual({ deployed: false, desiredReplicas: 0, readyReplicas: 0, status: 'not-deployed' })
    expect(aggregateApplicationRuntime([target({ enabled: false })], [release({})]).deployed).toBe(false)
  })

  it('sums replicas and surfaces the status that needs the most attention', () => {
    const result = aggregateApplicationRuntime(
      [target({ id: 'prod', desiredReplicas: 2, readyReplicas: 1, status: 'degraded' }), target({ id: 'dev' })],
      [release({ deploymentTargetId: 'prod' }), release({ deploymentTargetId: 'dev', id: 'release-dev' })],
    )
    expect(result).toEqual({ deployed: true, desiredReplicas: 3, readyReplicas: 2, status: 'degraded' })
  })

  it('picks the newest release for a target', () => {
    const older = release({ id: 'older' })
    const newer = release({ createdAt: '2026-10-02T00:00:00Z', id: 'newer' })
    expect(latestReleaseForTarget([older, newer, release({ deploymentTargetId: 'other', id: 'other', createdAt: '2026-10-03T00:00:00Z' })], target({}))?.id).toBe('newer')
  })
})
