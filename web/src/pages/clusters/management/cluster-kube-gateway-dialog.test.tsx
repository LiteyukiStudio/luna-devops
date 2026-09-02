import type { RuntimeCluster, RuntimeClusterKubeGateway } from '@/api'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { ClusterKubeGatewayDialog } from './cluster-kube-gateway-dialog'

const mocks = vi.hoisted(() => ({
  getRuntimeClusterKubeGateway: vi.fn(),
  updateRuntimeClusterKubeGateway: vi.fn(),
}))

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      getRuntimeClusterKubeGateway: mocks.getRuntimeClusterKubeGateway,
      updateRuntimeClusterKubeGateway: mocks.updateRuntimeClusterKubeGateway,
    },
  }
})

const cluster = {
  id: 'clu_gateway',
  name: 'Gateway cluster',
  type: 'kubernetes',
  kubeGatewayEnabled: false,
} as RuntimeCluster

describe('cluster kubectl gateway dialog', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18next.changeLanguage('en-US')
  })

  it('keeps the dialog usable when a legacy response contains null rules', async () => {
    renderDialog({
      enabled: true,
      extraResourceRules: null,
      status: 'ready',
      observationCode: '',
    } as unknown as RuntimeClusterKubeGateway)

    expect(await screen.findByText(i18next.t('kubectlAccess.gatewayRules.empty'))).toBeVisible()
    expect(screen.getByRole('checkbox', { name: i18next.t('kubectlAccess.gatewayEnabled') })).toBeChecked()
    expect(screen.getByRole('dialog')).toBeVisible()
  })

  it('treats omitted subresources as an empty list', async () => {
    renderDialog({
      enabled: true,
      extraResourceRules: [{
        action: 'project:read',
        apiGroup: 'example.io',
        apiVersion: 'v1',
        resource: 'widgets',
        verbs: ['get'],
      }],
      status: 'ready',
      observationCode: '',
    } as unknown as RuntimeClusterKubeGateway)

    expect(await screen.findByDisplayValue('widgets')).toBeVisible()
    expect(screen.getByPlaceholderText(i18next.t('kubectlAccess.gatewayRules.subresourcesPlaceholder'))).toHaveValue('')
  })
})

function renderDialog(gateway: RuntimeClusterKubeGateway) {
  mocks.getRuntimeClusterKubeGateway.mockResolvedValue(gateway)
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <ClusterKubeGatewayDialog cluster={cluster} open onOpenChange={vi.fn()} />
      </TooltipProvider>
    </QueryClientProvider>,
  )
}
