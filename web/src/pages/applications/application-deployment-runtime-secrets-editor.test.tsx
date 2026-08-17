import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '@/api'
import i18next from '@/i18n'
import { ApplicationDeploymentRuntimeSecretsEditor } from './application-deployment-runtime-secrets-editor'

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      getDeploymentTargetRuntimeSecretsSummary: vi.fn(),
      updateDeploymentTargetRuntimeSecrets: vi.fn(),
    },
  }
})

const summary = vi.mocked(api.getDeploymentTargetRuntimeSecretsSummary)
function renderEditor() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <ApplicationDeploymentRuntimeSecretsEditor
        applicationId="app-1"
        canManage={false}
        open
        projectId="project-1"
        targetId="target-1"
      />
    </QueryClientProvider>,
  )
}

describe('application deployment runtime secrets editor', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('en-US')
    summary.mockResolvedValue({ secretKeys: ['API_TOKEN'], secretRefsSet: true })
  })

  afterEach(() => {
    vi.clearAllMocks()
  })

  it('keeps values masked and hides the reveal action for non-owner roles', async () => {
    renderEditor()
    expect(await screen.findByText('API_TOKEN')).toBeInTheDocument()
    expect(screen.getByTestId('runtime-secret-value-API_TOKEN')).toHaveTextContent('••••••••')
    expect(screen.queryByRole('button', { name: /reveal/i })).not.toBeInTheDocument()
  })
})
