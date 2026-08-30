import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
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
const update = vi.mocked(api.updateDeploymentTargetRuntimeSecrets)
function renderEditor(canManage = false, onOuterSubmit?: () => void) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <form
        onSubmit={(event) => {
          event.preventDefault()
          onOuterSubmit?.()
        }}
      >
        <ApplicationDeploymentRuntimeSecretsEditor
          applicationId="app-1"
          canManage={canManage}
          open
          projectId="project-1"
          targetId="target-1"
        />
      </form>
    </QueryClientProvider>,
  )
}

describe('application deployment runtime secrets editor', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('en-US')
    summary.mockResolvedValue({ environmentVariables: [{ configured: true, key: 'API_TOKEN', valueMode: 'secret' }] })
    update.mockResolvedValue({ clearedKeys: [], configuredKeys: [], environmentVariables: [], generatedKeys: [] })
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

  it('uses an explicit typed clear operation', async () => {
    const user = userEvent.setup()
    renderEditor(true)
    await screen.findByText('API_TOKEN')
    await user.click(screen.getByRole('button', { name: 'Clear' }))
    expect(update).toHaveBeenCalledWith('project-1', 'app-1', 'target-1', {
      items: [{ key: 'API_TOKEN', operation: 'clear', valueMode: 'secret' }],
    })
  })

  it('does not submit an empty replacement value', async () => {
    const user = userEvent.setup()
    renderEditor(true)
    await screen.findByText('API_TOKEN')
    await user.click(screen.getByRole('button', { name: 'Replace' }))
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
    expect(update).not.toHaveBeenCalled()
  })

  it('adds a secret without submitting the surrounding deployment form', async () => {
    const user = userEvent.setup()
    const onOuterSubmit = vi.fn()
    renderEditor(true, onOuterSubmit)
    await screen.findByText('API_TOKEN')

    await user.type(screen.getByLabelText('New secret key'), 'DATABASE_PASSWORD')
    await user.type(screen.getByLabelText('New secret value'), 'example-secret-value')
    await user.click(screen.getByRole('button', { name: 'Add secret' }))

    expect(onOuterSubmit).not.toHaveBeenCalled()
    expect(update).toHaveBeenCalledWith('project-1', 'app-1', 'target-1', {
      items: [{ key: 'DATABASE_PASSWORD', operation: 'set', value: 'example-secret-value', valueMode: 'secret' }],
    })
  })

  it('replaces a secret without submitting the surrounding deployment form', async () => {
    const user = userEvent.setup()
    const onOuterSubmit = vi.fn()
    renderEditor(true, onOuterSubmit)
    await screen.findByText('API_TOKEN')

    await user.click(screen.getByRole('button', { name: 'Replace' }))
    await user.type(screen.getByLabelText('New value for API_TOKEN'), 'replacement-value')
    await user.click(screen.getByRole('button', { name: 'Save' }))

    expect(onOuterSubmit).not.toHaveBeenCalled()
    expect(update).toHaveBeenCalledWith('project-1', 'app-1', 'target-1', {
      items: [{ key: 'API_TOKEN', operation: 'set', value: 'replacement-value', valueMode: 'secret' }],
    })
  })
})
