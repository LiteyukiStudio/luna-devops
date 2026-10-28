import type { DeploymentTarget } from '@/api'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { api, ApiError } from '@/api'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { DeploymentTargetEditor } from './deployment-target-editor'
import { deploymentTargetToFormValues, emptyDeploymentTargetFormValues } from './deployment-target-form'

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      createDeploymentTarget: vi.fn(),
      createRelease: vi.fn(),
      getGitRepositoryBuildOptions: vi.fn(),
      getRegistryImageTemplateDefault: vi.fn(),
      listBuildTemplates: vi.fn(),
      listProjectHooks: vi.fn(),
      listProjectRuntimeConfigSets: vi.fn(),
      observeRuntimeClusterPressure: vi.fn(),
      updateDeploymentTarget: vi.fn(),
    },
  }
})

const repositoryBinding = { applicationId: 'app_1', defaultBranch: 'main', gitAccountId: 'acc_1', id: 'rb_1', owner: 'team', repo: 'demo' }

function renderEditor({ target = null as DeploymentTarget | null } = {}) {
  const queryClient = new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })
  const initialValues = target
    ? deploymentTargetToFormValues(target)
    : { ...emptyDeploymentTargetFormValues(), repositoryBindingId: 'rb_1', targetImageRef: 'team/demo:latest', targetRegistryId: 'reg_1' }
  return render(
    <MemoryRouter initialEntries={['/projects/prj_1/apps/app_1/deployments/new']}>
      <QueryClientProvider client={queryClient}>
        <TooltipProvider>
          <Routes>
            <Route
              path="/projects/:projectId/apps/:applicationId/deployments/new"
              element={(
                <DeploymentTargetEditor
                  applicationId="app_1"
                  buildEnvironmentAvailable
                  canManageRuntimeSecrets
                  deploymentTargets={[]}
                  initialValues={initialValues}
                  projectId="prj_1"
                  registries={[{ credentialSet: true, endpoint: 'https://registry.example.com', id: 'reg_1', isDefault: true, name: 'Registry', namespace: 'team', provider: 'harbor' } as never]}
                  releases={[]}
                  repositoryBindings={[repositoryBinding as never]}
                  runtimeClusters={[]}
                  target={target}
                />
              )}
            />
            <Route path="/projects/:projectId/apps/:applicationId" element={<p>deployments list</p>} />
          </Routes>
        </TooltipProvider>
      </QueryClientProvider>
    </MemoryRouter>,
  )
}

describe('deployment target editor', () => {
  beforeAll(() => {
    Element.prototype.scrollIntoView = vi.fn()
  })

  beforeEach(async () => {
    await i18next.changeLanguage('en-US')
    vi.mocked(api.listBuildTemplates).mockResolvedValue([])
    vi.mocked(api.listProjectHooks).mockResolvedValue([])
    vi.mocked(api.listProjectRuntimeConfigSets).mockResolvedValue([])
    vi.mocked(api.observeRuntimeClusterPressure).mockResolvedValue([])
    vi.mocked(api.getGitRepositoryBuildOptions).mockResolvedValue({ directories: ['.'], dockerfiles: ['Dockerfile'], exposedPorts: {}, recommendedTemplateIds: [] } as never)
    vi.mocked(api.getRegistryImageTemplateDefault).mockResolvedValue({ targetImageRef: 'team/demo:dev' } as never)
    vi.mocked(api.createDeploymentTarget).mockResolvedValue({ id: 'target_1' } as DeploymentTarget)
  })

  afterEach(() => {
    vi.clearAllMocks()
  })

  it('groups the form into navigable sections with labelled controls', async () => {
    renderEditor()

    const navigation = screen.getByRole('navigation', { name: 'Deploy config sections' })
    for (const section of ['Basics', 'Source and build', 'Runtime', 'Release triggers', 'Hooks', 'Security and scheduling'])
      expect(within(navigation).getByRole('link', { name: new RegExp(section) })).toBeInTheDocument()
    expect(screen.getByRole('textbox', { name: /Name/ })).toBeInTheDocument()
    expect(screen.getByRole('spinbutton', { name: /Replicas/ })).toBeInTheDocument()
    expect(screen.getByRole('switch', { name: 'Enable this deploy config' })).toBeChecked()
    expect(screen.queryByRole('textbox', { name: /Readiness probe/ })).not.toBeInTheDocument()
  })

  it('shows inline errors and section counts instead of submitting invalid input', async () => {
    const user = userEvent.setup()
    renderEditor()

    await user.click(screen.getByRole('button', { name: 'Create deploy config' }))

    expect(api.createDeploymentTarget).not.toHaveBeenCalled()
    const name = screen.getByRole('textbox', { name: /Name/ })
    expect(name).toHaveAttribute('aria-invalid', 'true')
    expect(name).toHaveAccessibleDescription(expect.stringContaining('Required'))
    expect(within(screen.getByRole('navigation', { name: 'Deploy config sections' })).getByText('Errors: 1')).toBeInTheDocument()
  })

  it('swaps fixed replicas for autoscaling bounds and validates them together', async () => {
    const user = userEvent.setup()
    renderEditor()

    await user.type(screen.getByRole('textbox', { name: /Name/ }), 'dev')
    await user.click(screen.getByRole('switch', { name: 'Enable HPA' }))
    expect(screen.queryByRole('spinbutton', { name: /Replicas/ })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Create deploy config' }))

    expect(api.createDeploymentTarget).not.toHaveBeenCalled()
    expect(await screen.findByText('Set a CPU or memory target when autoscaling is enabled')).toBeInTheDocument()
  })

  it('creates the deploy config with the registry template image and returns to the list', async () => {
    const user = userEvent.setup()
    renderEditor()

    await user.type(screen.getByRole('textbox', { name: /Name/ }), 'dev')
    await user.click(screen.getByRole('button', { name: 'Create deploy config' }))

    expect(await screen.findByText('deployments list')).toBeInTheDocument()
    expect(api.createDeploymentTarget).toHaveBeenCalledWith('prj_1', 'app_1', expect.objectContaining({
      name: 'dev',
      replicas: 1,
      repositoryBindingId: 'rb_1',
      servicePorts: [{ appProtocol: '', name: 'http', port: 8080 }],
      targetImageRef: 'team/demo:dev',
    }))
    expect(api.createDeploymentTarget).toHaveBeenCalledWith('prj_1', 'app_1', expect.not.objectContaining({ buildVariables: expect.anything() }))
  })

  it('saves plain and encrypted variables from one table in a single request', async () => {
    const user = userEvent.setup()
    renderEditor()

    await user.type(screen.getByRole('textbox', { name: /Name/ }), 'dev')
    await user.click(screen.getByRole('button', { name: 'Add variable' }))
    await user.type(screen.getByRole('textbox', { name: 'Name of variable 1' }), 'LOG_LEVEL')
    await user.type(screen.getByRole('textbox', { name: 'Value of LOG_LEVEL' }), 'info')
    await user.click(screen.getByRole('button', { name: 'Add variable' }))
    await user.type(screen.getByRole('textbox', { name: 'Name of variable 2' }), 'API_TOKEN')
    await user.click(screen.getByRole('switch', { name: 'Encrypt API_TOKEN' }))
    await user.type(screen.getByLabelText('Value of API_TOKEN'), 's3cret')
    await user.click(screen.getByRole('button', { name: 'Create deploy config' }))

    expect(await screen.findByText('deployments list')).toBeInTheDocument()
    expect(api.createDeploymentTarget).toHaveBeenCalledWith('prj_1', 'app_1', expect.objectContaining({
      environmentVariables: [
        { key: 'LOG_LEVEL', valueMode: 'public', value: 'info' },
        { key: 'API_TOKEN', valueMode: 'secret', secretOperation: 'set', value: 's3cret' },
      ],
    }))
  })

  it('rejects names shared by a plain and an encrypted variable', async () => {
    const user = userEvent.setup()
    renderEditor()

    await user.type(screen.getByRole('textbox', { name: /Name/ }), 'dev')
    for (const index of [1, 2]) {
      await user.click(screen.getByRole('button', { name: 'Add variable' }))
      await user.type(screen.getByRole('textbox', { name: `Name of variable ${index}` }), 'TOKEN')
    }
    await user.click(screen.getByRole('button', { name: 'Create deploy config' }))

    expect(api.createDeploymentTarget).not.toHaveBeenCalled()
    expect(await screen.findByText('Name is already used by another variable')).toBeInTheDocument()
  })

  it('maps backend field errors back to the form field', async () => {
    const user = userEvent.setup()
    vi.mocked(api.createDeploymentTarget).mockRejectedValue(new ApiError('Repository binding not found', { code: 'deployment_target.repository_binding_not_found', path: '/x', status: 400 }))
    renderEditor()

    await user.type(screen.getByRole('textbox', { name: /Name/ }), 'dev')
    await user.click(screen.getByRole('button', { name: 'Create deploy config' }))

    expect(await screen.findByText('Repository binding not found')).toBeInTheDocument()
    expect(screen.getByRole('combobox', { name: /Code repository/ })).toHaveAttribute('aria-invalid', 'true')
  })
})
