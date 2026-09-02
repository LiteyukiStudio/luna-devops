import type { CurrentUser, RuntimeCluster } from '@/api'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { RuntimeClusterTable } from './runtime-cluster-table'

const originalMatchMedia = window.matchMedia

const platformAdmin = {
  id: 'user-1',
  role: 'platform_admin',
} as CurrentUser

beforeEach(async () => {
  await i18next.changeLanguage('en-US')
})

afterEach(() => {
  Object.defineProperty(window, 'matchMedia', { configurable: true, value: originalMatchMedia })
})

describe('runtime cluster table', () => {
  it('shows observed, checking, and unsupported kubectl gateway states', () => {
    const readyCluster = runtimeCluster({ id: 'cluster-ready', name: 'ready-cluster' })
    const checkingCluster = runtimeCluster({ id: 'cluster-checking', name: 'checking-cluster', type: 'k3s' })
    const unsupportedCluster = runtimeCluster({ id: 'cluster-compose', name: 'compose-cluster', type: 'docker-compose' })

    renderTable({
      clusters: [readyCluster, checkingCluster, unsupportedCluster],
      kubeGatewayStatusByClusterId: {
        [readyCluster.id]: 'ready',
        [unsupportedCluster.id]: 'ready',
      },
    })

    expect(screen.getByRole('columnheader', { name: i18next.t('kubectlAccess.gatewayStatusLabel') })).toBeVisible()
    expect(within(rowFor('ready-cluster')).getByText(i18next.t('kubectlAccess.gatewayStatuses.ready'))).toBeVisible()
    expect(within(rowFor('checking-cluster')).getByText(i18next.t('kubectlAccess.gatewayStatuses.checking'))).toBeVisible()
    expect(within(rowFor('compose-cluster')).getByText('—')).toBeVisible()
  })

  it('uses one accessible actions menu and invokes every regular action', async () => {
    const user = userEvent.setup()
    const cluster = runtimeCluster()
    const callbacks = actionCallbacks()
    renderTable({ callbacks, clusters: [cluster] })

    const actionLabel = i18next.t('clustersPage.clusterActions', { name: cluster.name })
    expect(screen.getAllByRole('button', { name: actionLabel })).toHaveLength(1)

    await openActions(user, actionLabel)
    await user.click(screen.getByRole('menuitem', { name: i18next.t('common.test') }))
    expect(callbacks.onTest).toHaveBeenCalledWith(cluster.id)

    await openActions(user, actionLabel)
    await user.click(screen.getByRole('menuitem', { name: i18next.t('kubectlAccess.gatewayAction') }))
    expect(callbacks.onConfigureKubeGateway).toHaveBeenCalledWith(cluster)

    await openActions(user, actionLabel)
    await user.click(screen.getByRole('menuitem', { name: i18next.t('common.edit') }))
    expect(callbacks.onEdit).toHaveBeenCalledWith(cluster)

    await openActions(user, actionLabel)
    const deleteItem = screen.getByRole('menuitem', { name: i18next.t('common.delete') })
    expect(deleteItem).toHaveAttribute('data-variant', 'destructive')
    await user.click(deleteItem)
    expect(callbacks.onDelete).toHaveBeenCalledWith(cluster)
  })

  it('hides the kubectl gateway column and action when the feature is unavailable', async () => {
    const user = userEvent.setup()
    const cluster = runtimeCluster()
    renderTable({ clusters: [cluster], kubectlGatewayAvailable: false })

    expect(screen.queryByRole('columnheader', { name: i18next.t('kubectlAccess.gatewayStatusLabel') })).not.toBeInTheDocument()
    await openActions(user, i18next.t('clustersPage.clusterActions', { name: cluster.name }))
    expect(screen.queryByRole('menuitem', { name: i18next.t('kubectlAccess.gatewayAction') })).not.toBeInTheDocument()
  })

  it('keeps a failed deletion behind one destructive retry action', async () => {
    const user = userEvent.setup()
    const cluster = runtimeCluster({ deleteStatus: 'delete_failed' })
    const callbacks = actionCallbacks()
    renderTable({ callbacks, clusters: [cluster] })

    await openActions(user, i18next.t('clustersPage.clusterActions', { name: cluster.name }))
    expect(within(rowFor(cluster.name)).getByText('—')).toBeVisible()
    const menuItems = screen.getAllByRole('menuitem')
    expect(menuItems).toHaveLength(1)
    expect(menuItems[0]).toHaveTextContent(i18next.t('kubectlAccess.retryDelete'))
    expect(menuItems[0]).toHaveAttribute('data-variant', 'destructive')

    await user.click(menuItems[0])
    expect(callbacks.onDelete).toHaveBeenCalledWith(cluster)
  })

  it('does not wrap the explicit actions menu in the generic mobile menu', () => {
    setMobileViewport()
    const cluster = runtimeCluster()
    renderTable({ clusters: [cluster] })

    expect(screen.getAllByRole('button', {
      name: i18next.t('clustersPage.clusterActions', { name: cluster.name }),
    })).toHaveLength(1)
    expect(screen.queryByRole('button', { name: i18next.t('common.actions') })).not.toBeInTheDocument()
  })
})

function renderTable({
  callbacks = actionCallbacks(),
  clusters = [runtimeCluster()],
  kubeGatewayStatusByClusterId = { 'cluster-1': 'ready' },
  kubectlGatewayAvailable = true,
}: {
  callbacks?: ReturnType<typeof actionCallbacks>
  clusters?: RuntimeCluster[]
  kubeGatewayStatusByClusterId?: Record<string, string>
  kubectlGatewayAvailable?: boolean
} = {}) {
  return render(
    <RuntimeClusterTable
      clusters={clusters}
      kubeGatewayStatusByClusterId={kubeGatewayStatusByClusterId}
      kubectlGatewayAvailable={kubectlGatewayAvailable}
      loading={false}
      pagination={{
        page: 1,
        pageSize: 10,
        total: clusters.length,
        totalPages: clusters.length > 0 ? 1 : 0,
        onPageChange: vi.fn(),
        onPageSizeChange: vi.fn(),
      }}
      pressureByClusterId={{}}
      pressureLoading={false}
      projects={[]}
      user={platformAdmin}
      {...callbacks}
    />,
  )
}

function actionCallbacks() {
  return {
    onConfigureKubeGateway: vi.fn(),
    onDelete: vi.fn(),
    onEdit: vi.fn(),
    onTest: vi.fn(),
  }
}

async function openActions(user: ReturnType<typeof userEvent.setup>, label: string) {
  await user.click(screen.getByRole('button', { name: label }))
}

function rowFor(name: string) {
  const row = screen.getByText(name).closest('tr')
  if (!row)
    throw new Error(`Missing row for ${name}`)
  return row
}

function setMobileViewport() {
  Object.defineProperty(window, 'matchMedia', {
    configurable: true,
    value: (query: string) => ({
      matches: query === '(max-width: 47.999rem)',
      media: query,
      onchange: null,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
    }),
  })
}

function runtimeCluster(overrides: Partial<RuntimeCluster> = {}): RuntimeCluster {
  return {
    id: 'cluster-1',
    name: 'runtime-cluster',
    type: 'kubernetes',
    scope: 'global',
    ownerRef: '',
    projectIds: [],
    kubeconfigSet: true,
    isDefault: false,
    maxConcurrentBuilds: 4,
    gatewayRootDomain: 'apps.example.com',
    gatewayDomainSuffixes: ['apps.example.com'],
    gatewayPublicScheme: 'https',
    gatewayPublicPort: 443,
    status: 'active',
    deleteStatus: 'active',
    ...overrides,
  } as RuntimeCluster
}
