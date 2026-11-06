import type { RuntimeCluster } from '@/api'
import { describe, expect, it } from 'vitest'
import { platformDefaultRuntimeCluster } from './runtime-cluster-default'

const cluster = (overrides: Partial<RuntimeCluster>) => ({ createdAt: '2026-01-01T00:00:00Z', deleteStatus: 'active', id: 'cluster', isDefault: false, scope: 'global', ...overrides }) as RuntimeCluster

describe('platform default runtime cluster', () => {
  it('prefers the marked global default', () => {
    const clusters = [cluster({ id: 'old' }), cluster({ createdAt: '2026-02-01T00:00:00Z', id: 'marked', isDefault: true })]
    expect(platformDefaultRuntimeCluster(clusters)?.id).toBe('marked')
  })

  it('falls back to the earliest active global cluster and never picks project or user clusters', () => {
    const clusters = [
      cluster({ createdAt: '2025-01-01T00:00:00Z', id: 'project', scope: 'project' }),
      cluster({ createdAt: '2025-02-01T00:00:00Z', id: 'user', scope: 'user' }),
      cluster({ createdAt: '2025-03-01T00:00:00Z', deleteStatus: 'deleting', id: 'deleting' }),
      cluster({ createdAt: '2026-03-01T00:00:00Z', id: 'newer' }),
      cluster({ createdAt: '2026-01-01T00:00:00Z', id: 'earliest' }),
    ]
    expect(platformDefaultRuntimeCluster(clusters)?.id).toBe('earliest')
    expect(platformDefaultRuntimeCluster([cluster({ id: 'project', scope: 'project' })])).toBeUndefined()
  })
})
