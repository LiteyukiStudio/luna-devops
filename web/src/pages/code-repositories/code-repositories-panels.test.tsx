import type { GitProvider } from '@/api'
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { CredentialsPanel } from './code-repositories-panels'

describe('credentialsPanel OAuth readiness for redacted providers', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('zh-CN')
  })

  it('exposes the OAuth connect action when clientId is redacted for non-admin users but oauthReady is true', () => {
    const redactedGlobalProvider: GitProvider = {
      id: 'gp_global',
      type: 'github',
      name: 'GitHub Enterprise',
      baseUrl: '',
      scope: 'global',
      ownerRef: '',
      projectIds: [],
      authType: 'oauth',
      clientId: '',
      clientSecretSet: true,
      oauthReady: true,
      enabled: true,
      createdAt: '2026-07-01T00:00:00Z',
    }

    render(
      <CredentialsPanel
        credentials={[]}
        loading={false}
        page={1}
        pageSize={10}
        projectMap={{}}
        providers={[redactedGlobalProvider]}
        refreshPending={false}
        total={0}
        totalPages={0}
        onDelete={vi.fn()}
        onEdit={vi.fn()}
        onPageChange={vi.fn()}
        onPageSizeChange={vi.fn()}
        onRefresh={vi.fn()}
      />,
    )

    expect(screen.getByRole('button', { name: i18next.t('codeRepositoriesView.oauthConnect', { provider: 'GitHub Enterprise' }) })).toBeInTheDocument()
    expect(screen.queryByText(i18next.t('codeRepositoriesView.oauthUnavailableTitle'))).not.toBeInTheDocument()
  })

  it('renders skeletons for provider and project scope cells while lookup queries are loading', () => {
    const { container } = render(
      <CredentialsPanel
        credentials={[{
          id: 'ga_1',
          userId: 'usr_1',
          providerId: 'gp_pending',
          scope: 'project',
          ownerRef: '',
          projectIds: ['prj_pending'],
          username: 'octo',
          externalUserId: '',
          avatarUrl: '',
          scopes: 'repo',
          accessTokenSet: true,
          refreshTokenSet: false,
          status: 'ready',
          createdAt: '2026-07-01T00:00:00Z',
        }]}
        loading={false}
        page={1}
        pageSize={10}
        projectMap={{}}
        projectsLoading
        providers={[]}
        providersLoading
        refreshPending={false}
        total={1}
        totalPages={1}
        onDelete={vi.fn()}
        onEdit={vi.fn()}
        onPageChange={vi.fn()}
        onPageSizeChange={vi.fn()}
        onRefresh={vi.fn()}
      />,
    )

    expect(screen.getByText('octo')).toBeInTheDocument()
    expect(screen.queryByText('gp_pending')).not.toBeInTheDocument()
    expect(screen.queryByText('prj_pending')).not.toBeInTheDocument()
    expect(container.querySelectorAll('[data-slot="skeleton"]').length).toBeGreaterThanOrEqual(2)
  })
})
