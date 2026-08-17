import type { PropsWithChildren } from 'react'
import type { ArtifactRegistry, BuildEnvironmentConfig, DeploymentTarget, RepositoryBinding, RuntimeCluster } from '@/api'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { deploymentTargetDefaults, normalizeDeploymentTargetPayload } from './application-deployments-panel-utils'
import { deploymentTargetFormValues, useDeploymentTargetForm } from './use-deployment-target-form'
import '@/i18n'

function deploymentTarget(id: string, overrides: Partial<DeploymentTarget> = {}) {
  return {
    ...deploymentTargetDefaults,
    id,
    applicationId: 'application-1',
    projectId: 'project-1',
    kubernetesName: id,
    createdAt: '2026-08-10T00:00:00Z',
    createdBy: 'user-1',
    secretFilesSet: false,
    secretRefsSet: false,
    status: 'ready',
    desiredReplicas: 1,
    updatedReplicas: 1,
    readyReplicas: 1,
    availableReplicas: 1,
    deleteStatus: 'active',
    deleteMessage: '',
    ...overrides,
  } as DeploymentTarget
}

function formOptions(overrides: Partial<Parameters<typeof useDeploymentTargetForm>[0]> = {}) {
  return {
    applicationId: 'application-1',
    applicationIdentifier: 'app',
    projectId: 'project-1',
    projectIdentifier: 'project',
    registries: [] as ArtifactRegistry[],
    repositoryBindings: [] as RepositoryBinding[],
    ...overrides,
  }
}

function wrapper(queryClient: QueryClient) {
  return ({ children }: PropsWithChildren) => <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise
  })
  return { promise, resolve }
}

describe('deployment target form', () => {
  it('preserves the immutable stage of a system-managed target', () => {
    const target = deploymentTarget('target-system', { stage: 'sys-cluster1' })
    const formValues = deploymentTargetFormValues({
      applicationIdentifier: 'probe',
      projectIdentifier: 'platform-system',
      registries: [],
      repositoryBindings: [],
      target,
    })

    expect(formValues.stage).toBe('sys-cluster1')
    expect(normalizeDeploymentTargetPayload(formValues).stage).toBe('sys-cluster1')
  })

  it('builds create and edit defaults without exposing configured secrets', () => {
    const registry = { credentialSet: true, id: 'registry-1', isDefault: true } as ArtifactRegistry
    const binding = { id: 'binding-1' } as RepositoryBinding
    const cluster = { id: 'cluster-1', isDefault: true } as RuntimeCluster
    const createValues = deploymentTargetFormValues({
      applicationIdentifier: 'app',
      defaultRuntimeCluster: cluster,
      projectIdentifier: 'project',
      registries: [registry],
      repositoryBindings: [binding],
    })

    expect(createValues.clusterId).toBe('cluster-1')
    expect(createValues.repositoryBindingId).toBe('binding-1')
    expect(createValues.targetRegistryId).toBe('registry-1')
    expect(createValues.targetImageRef).toBe('project/project-app:latest')

    const editValues = deploymentTargetFormValues({
      applicationIdentifier: 'app',
      defaultRuntimeCluster: cluster,
      projectIdentifier: 'project',
      registries: [registry],
      repositoryBindings: [binding],
      target: deploymentTarget('target-1', {
        runtimeConfigRefs: [{ mode: 'live', setId: 'live-set' }, { mode: 'snapshot', setId: 'snapshot-set' }],
        runtimeConfigSetIds: ['legacy-set'],
        secretFilesSet: true,
        secretRefsSet: true,
      }),
    })

    expect(editValues.runtimeConfigSetIds).toEqual(['live-set'])
    expect(editValues.secretFiles).toBe('')
  })

  it('ignores an older build environment response after switching targets', async () => {
    const requests = new Map<string, ReturnType<typeof deferred<BuildEnvironmentConfig>>>()
    const loadBuildEnvironment = vi.fn((targetId: string) => {
      const request = deferred<BuildEnvironmentConfig>()
      requests.set(targetId, request)
      return request.promise
    })
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const { result } = renderHook(
      () => useDeploymentTargetForm(formOptions({ loadBuildEnvironment })),
      { wrapper: wrapper(queryClient) },
    )

    act(() => result.current.openDialog(deploymentTarget('target-a')))
    act(() => result.current.openDialog(deploymentTarget('target-b')))
    await act(async () => {
      requests.get('target-a')?.resolve({ scope: 'deployment', scopeRef: 'target-a', variables: { OLD: 'value' }, secrets: {} })
      await requests.get('target-a')?.promise
    })

    expect(result.current.editingTarget?.id).toBe('target-b')
    expect(result.current.buildEnvironmentStatus).toBe('loading')
    expect(result.current.buildVariableRows.some(row => row.key === 'OLD')).toBe(false)

    await act(async () => {
      requests.get('target-b')?.resolve({ scope: 'deployment', scopeRef: 'target-b', variables: { CURRENT: 'value' }, secrets: { TOKEN: true } })
      await requests.get('target-b')?.promise
    })

    expect(result.current.buildEnvironmentStatus).toBe('ready')
    expect(result.current.buildVariableRows.some(row => row.key === 'CURRENT')).toBe(true)
    expect(result.current.buildSecretRows).toEqual(expect.arrayContaining([expect.objectContaining({ existing: true, key: 'TOKEN', value: '' })]))
  })

  it('invalidates an in-flight environment request when the dialog closes', async () => {
    const request = deferred<BuildEnvironmentConfig>()
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const { result } = renderHook(
      () => useDeploymentTargetForm(formOptions({ loadBuildEnvironment: () => request.promise })),
      { wrapper: wrapper(queryClient) },
    )

    act(() => result.current.openDialog(deploymentTarget('target-a')))
    act(() => result.current.handleDialogOpenChange(false))
    await act(async () => {
      request.resolve({ scope: 'deployment', scopeRef: 'target-a', variables: { OLD: 'value' }, secrets: {} })
      await request.promise
    })

    expect(result.current.dialogOpen).toBe(false)
    expect(result.current.editingTarget).toBeNull()
    expect(result.current.buildEnvironmentStatus).toBe('ready')
    expect(result.current.buildVariableRows.some(row => row.key === 'OLD')).toBe(false)
  })

  it('adds build variables and secrets to the payload only after the draft changes', () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const { result } = renderHook(
      () => useDeploymentTargetForm(formOptions()),
      { wrapper: wrapper(queryClient) },
    )
    const values = { ...deploymentTargetDefaults, name: 'api' }

    expect(result.current.buildSubmissionPayload(values)).not.toHaveProperty('buildVariables')
    expect(result.current.buildSubmissionPayload(values)).not.toHaveProperty('buildSecrets')

    act(() => {
      result.current.updateBuildVariableRows([{ id: 'variable-1', key: 'NODE_ENV', value: 'production' }])
      result.current.updateBuildSecretRows([{ id: 'secret-1', key: 'TOKEN', value: 'secret' }])
    })

    expect(result.current.buildSubmissionPayload(values)).toMatchObject({
      buildSecrets: { TOKEN: 'secret' },
      buildVariables: { NODE_ENV: 'production' },
    })
  })
})
