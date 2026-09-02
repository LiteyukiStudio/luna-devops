import type { CurrentUser, RuntimeCluster } from '@/api'
import { render, screen } from '@testing-library/react'
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
    await user.click(screen.getByRole('menuitem', { name: i18next.t('common.edit') }))
    expect(callbacks.onEdit).toHaveBeenCalledWith(cluster)

    await openActions(user, actionLabel)
    const deleteItem = screen.getByRole('menuitem', { name: i18next.t('common.delete') })
    expect(deleteItem).toHaveAttribute('data-variant', 'destructive')
    await user.click(deleteItem)
    expect(callbacks.onDelete).toHaveBeenCalledWith(cluster)
  })

  it('keeps a failed deletion behind one destructive retry action', async () => {
    const user = userEvent.setup()
    const cluster = runtimeCluster({ deleteStatus: 'delete_failed' })
    const callbacks = actionCallbacks()
    renderTable({ callbacks, clusters: [cluster] })

    await openActions(user, i18next.t('clustersPage.clusterActions', { name: cluster.name }))
    const menuItems = screen.getAllByRole('menuitem')
    expect(menuItems).toHaveLength(1)
    expect(menuItems[0]).toHaveTextContent(i18next.t('clustersPage.retryDelete'))
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
}: {
  callbacks?: ReturnType<typeof actionCallbacks>
  clusters?: RuntimeCluster[]
} = {}) {
  return render(
    <RuntimeClusterTable
      clusters={clusters}
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
    onDelete: vi.fn(),
    onEdit: vi.fn(),
    onTest: vi.fn(),
  }
}

async function openActions(user: ReturnType<typeof userEvent.setup>, label: string) {
  await user.click(screen.getByRole('button', { name: label }))
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
