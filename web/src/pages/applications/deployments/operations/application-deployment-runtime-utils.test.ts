import type { ClusterResource, DeploymentTarget, RuntimeCluster } from '@/api'
import { describe, expect, it } from 'vitest'
import { buildDeploymentRuntimeStatus, effectiveReplicaState, replicaStateTone } from './application-deployment-runtime-utils'

describe('deployment runtime status', () => {
  const cluster = { id: 'cluster-1', name: 'Runtime' } as RuntimeCluster

  it('keeps status and replica counts on the deployment target observation boundary', () => {
    const target = { clusterId: '', effectiveClusterId: cluster.id, id: 'target-1', status: 'progressing' } as DeploymentTarget
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
    const target = { clusterId: cluster.id, effectiveClusterId: cluster.id, id: 'target-1', status: 'scaled-to-zero' } as DeploymentTarget
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

  it('uses only the backend effective cluster and never guesses a default', () => {
    const target = { clusterId: '', effectiveClusterId: '', id: 'target-1', status: '' } as DeploymentTarget
    const pod = { deploymentTargetId: target.id, kind: 'Pod', status: 'Running', summary: 'Ready 1/1' } as ClusterResource

    const status = buildDeploymentRuntimeStatus(target, undefined, { [cluster.id]: [pod] }, {}, {})

    expect(status.value).toBe('not-configured')
    expect(status.podCount).toBe(0)
  })
})

describe('effective replica state', () => {
  it('treats a missing deployment as undeployed without replica counts', () => {
    expect(effectiveReplicaState({ deployed: false, desiredReplicas: 0, readyReplicas: 0, status: 'ready' })).toEqual({ showReplicas: false, value: 'not-deployed' })
    expect(replicaStateTone('not-deployed')).toBe('neutral')
  })

  it('downgrades an inconsistent ready state and recognises scaled-down workloads', () => {
    expect(effectiveReplicaState({ availableReplicas: 0, desiredReplicas: 1, readyReplicas: 0, status: 'ready' })).toEqual({ showReplicas: true, value: 'progressing' })
    expect(effectiveReplicaState({ desiredReplicas: 0, readyReplicas: 0, status: 'ready' })).toEqual({ showReplicas: true, value: 'scaled-to-zero' })
    expect(replicaStateTone('scaled-to-zero')).toBe('neutral')
  })

  it('shows observed replicas of a disabled target that is still running', () => {
    expect(effectiveReplicaState({ desiredReplicas: 2, readyReplicas: 2, status: 'disabled' })).toEqual({ showReplicas: true, value: 'disabled' })
    expect(effectiveReplicaState({ desiredReplicas: 0, readyReplicas: 0, status: 'disabled' })).toEqual({ showReplicas: false, value: 'disabled' })
  })

  it('hides replica counts when the runtime could not be observed', () => {
    expect(effectiveReplicaState({ desiredReplicas: 2, readyReplicas: 0, status: 'unavailable' })).toEqual({ showReplicas: false, value: 'unavailable' })
    expect(replicaStateTone('unavailable')).toBe('danger')
  })
})
