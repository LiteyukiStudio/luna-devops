import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { ClusterNetworkPolicyStatus } from './cluster-network-policy-status'

const mocks = vi.hoisted(() => ({ observeRuntimeClusterNetworkPolicies: vi.fn() }))

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return { ...actual, api: { ...actual.api, observeRuntimeClusterNetworkPolicies: mocks.observeRuntimeClusterNetworkPolicies } }
})

function renderStatus() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}><ClusterNetworkPolicyStatus clusterId="clu_test" /></QueryClientProvider>)
}

describe('cluster network policy status', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18next.changeLanguage('en-US')
  })

  it('shows drift counts from the live observation', async () => {
    mocks.observeRuntimeClusterNetworkPolicies.mockResolvedValue({
      clusterId: 'clu_test',
      status: 'degraded',
      observationCode: 'runtime_cluster.network_policy_drifted',
      observedAt: '2026-11-11T00:00:00Z',
      namespaceCount: 3,
      syncedCount: 2,
      driftedCount: 1,
      enforcement: 'verified',
    })
    renderStatus()
    expect(await screen.findByText('Drifted')).toBeInTheDocument()
    expect(screen.getByText('3 project spaces: 2 synced, 1 drifted')).toBeInTheDocument()
    expect(mocks.observeRuntimeClusterNetworkPolicies).toHaveBeenCalledWith('clu_test', expect.any(AbortSignal))
  })

  it('shows unavailable with the localized observation code instead of a stale value', async () => {
    mocks.observeRuntimeClusterNetworkPolicies.mockResolvedValue({
      clusterId: 'clu_test',
      status: 'unavailable',
      observationCode: 'runtime_cluster.network_policy_forbidden',
      observedAt: '2026-11-11T00:00:00Z',
      namespaceCount: 0,
      syncedCount: 0,
      driftedCount: 0,
      enforcement: 'unknown',
    })
    renderStatus()
    expect(await screen.findByText('Unavailable')).toBeInTheDocument()
    expect(screen.getByText('The cluster credential cannot read network policies, nodes, or namespaces.')).toBeInTheDocument()
  })

  it('warns when the cluster network plugin does not enforce policies', async () => {
    mocks.observeRuntimeClusterNetworkPolicies.mockResolvedValue({
      clusterId: 'clu_test',
      status: 'ready',
      observedAt: '2026-11-11T00:00:00Z',
      namespaceCount: 1,
      syncedCount: 1,
      driftedCount: 0,
      enforcement: 'unsupported',
      probedAt: '2026-11-11T00:00:00Z',
    })
    renderStatus()
    expect(await screen.findByText('Not enforced')).toBeInTheDocument()
    expect(screen.getByText(/does not enforce NetworkPolicy/)).toBeInTheDocument()
  })

  it('treats a failed request as unavailable', async () => {
    mocks.observeRuntimeClusterNetworkPolicies.mockRejectedValue(new Error('boom'))
    renderStatus()
    expect(await screen.findByText('Unavailable')).toBeInTheDocument()
    expect(screen.getByText('Cluster network policies cannot be read right now.')).toBeInTheDocument()
  })
})
