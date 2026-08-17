import type { ClusterResource, DeploymentTarget, RuntimeCluster } from '@/api'
import { describe, expect, it } from 'vitest'
import { buildDeploymentRuntimeStatus } from './application-deployment-runtime-utils'

describe('deployment runtime status', () => {
  const cluster = { id: 'cluster-1', name: 'Runtime' } as RuntimeCluster

  it('keeps status and replica counts on the deployment target observation boundary', () => {
    const target = { clusterId: cluster.id, id: 'target-1', status: 'progressing' } as DeploymentTarget
    const pod = {
      deploymentTargetId: target.id,
      kind: 'Pod',
      status: 'Running',
      summary: 'Ready 1/1',
    } as ClusterResource

    const status = buildDeploymentRuntimeStatus(target, cluster, { [cluster.id]: [pod] }, {}, {})

    expect(status.value).toBe('progressing')
    expect(status.summary).toBe('Ready 1/1')
  })

  it('preserves the authoritative scaled-to-zero state when no pod exists', () => {
    const target = { clusterId: cluster.id, id: 'target-1', status: 'scaled-to-zero' } as DeploymentTarget
    const workload = {
      deploymentTargetId: target.id,
      kind: 'StatefulSet',
      status: 'scaled-to-zero',
      summary: 'ready 0/0',
    } as ClusterResource

    const status = buildDeploymentRuntimeStatus(target, cluster, { [cluster.id]: [workload] }, {}, {})

    expect(status.value).toBe('scaled-to-zero')
    expect(status.summary).toBe('ready 0/0')
  })
})
