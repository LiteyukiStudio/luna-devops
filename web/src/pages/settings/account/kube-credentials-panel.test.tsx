import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { KubeCredentialsPanel } from './kube-credentials-panel'

const mocks = vi.hoisted(() => ({
  listKubeCredentials: vi.fn(),
}))

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      listKubeCredentials: mocks.listKubeCredentials,
    },
  }
})

const emptyPage = {
  items: [],
  page: 1,
  pageSize: 10,
  total: 0,
  totalPages: 0,
}

const credentialPage = {
  items: [{
    id: 'credential-1',
    name: 'production-debug',
    scopes: ['kube:read'],
    status: 'active',
    expiresAt: '2026-09-08T00:00:00Z',
    createdAt: '2026-09-01T00:00:00Z',
    bindingCount: 1,
  }],
  page: 1,
  pageSize: 10,
  total: 1,
  totalPages: 1,
}

describe('kubectl credentials panel', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18next.changeLanguage('en-US')
  })

  it('shows loading before a focused first-use empty state', async () => {
    let resolveCredentials!: (value: typeof emptyPage) => void
    mocks.listKubeCredentials.mockReturnValue(new Promise((resolve) => {
      resolveCredentials = resolve
    }))

    renderPanel()

    expect(screen.getByRole('status')).toHaveAttribute('aria-busy', 'true')
    expect(screen.queryByText(i18next.t('kubectlAccess.credentials.emptyTitle'))).not.toBeInTheDocument()

    await act(async () => resolveCredentials(emptyPage))

    expect(await screen.findByText(i18next.t('kubectlAccess.credentials.emptyTitle'))).toBeVisible()
    expect(screen.getByRole('link', { name: i18next.t('kubectlAccess.credentials.emptyAction') })).toHaveAttribute('href', '/projects')
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
    expect(screen.queryByRole('combobox')).not.toBeInTheDocument()
    expect(screen.queryByRole('navigation')).not.toBeInTheDocument()
  })

  it('keeps compact accessible filters and clears a filtered empty result', async () => {
    mocks.listKubeCredentials.mockImplementation(({ search, status }: { search?: string, status?: string }) => (
      search || status ? Promise.resolve(emptyPage) : Promise.resolve(credentialPage)
    ))

    renderPanel()

    expect(await screen.findByText('production-debug')).toBeVisible()
    const searchInput = screen.getByRole('textbox', { name: i18next.t('kubectlAccess.credentials.searchPlaceholder') })
    const statusSelect = screen.getByRole('combobox', { name: i18next.t('kubectlAccess.credentials.statusFilter') })
    expect(statusSelect.parentElement).toHaveClass('w-full', 'sm:w-40', 'sm:flex-none')
    expect(screen.getByRole('columnheader', { name: i18next.t('kubectlAccess.credentials.scopes') })).toHaveClass('hidden', 'md:table-cell')

    fireEvent.change(searchInput, { target: { value: 'missing' } })
    fireEvent.change(statusSelect, { target: { value: 'expired' } })

    await waitFor(() => {
      expect(mocks.listKubeCredentials).toHaveBeenLastCalledWith(expect.objectContaining({
        page: 1,
        search: 'missing',
        status: 'expired',
      }))
    })
    expect(await screen.findByText(i18next.t('kubectlAccess.credentials.filteredEmptyTitle'))).toBeVisible()

    fireEvent.click(screen.getByRole('button', { name: i18next.t('kubectlAccess.credentials.clearFilters') }))

    await waitFor(() => {
      expect(searchInput).toHaveValue('')
      expect(statusSelect).toHaveValue('')
      expect(mocks.listKubeCredentials).toHaveBeenLastCalledWith(expect.objectContaining({
        page: 1,
        search: '',
        status: undefined,
      }))
    })
    expect(await screen.findByText('production-debug')).toBeVisible()
  })

  it('offers a localized retry when the credential list fails', async () => {
    mocks.listKubeCredentials.mockRejectedValueOnce(new Error('private upstream detail'))
    renderPanel()

    expect(await screen.findByText(i18next.t('kubectlAccess.credentials.loadFailedTitle'))).toBeVisible()
    expect(screen.queryByText('private upstream detail')).not.toBeInTheDocument()

    mocks.listKubeCredentials.mockResolvedValue(emptyPage)
    fireEvent.click(screen.getByRole('button', { name: i18next.t('common.retry') }))

    expect(await screen.findByText(i18next.t('kubectlAccess.credentials.emptyTitle'))).toBeVisible()
  })
})

function renderPanel() {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  })
  return render(
    <MemoryRouter>
      <QueryClientProvider client={queryClient}>
        <KubeCredentialsPanel featureEnabled />
      </QueryClientProvider>
    </MemoryRouter>,
  )
}
