import type { DeploymentTargetFormValues } from './deployment-target-form'
import type { DeploymentTarget, Release } from '@/api'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryRouter, Link, RouterProvider } from 'react-router-dom'
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

function imageTarget() {
  // 与线上返回结构一致的完整镜像来源部署配置，保证编辑器载入后表单校验通过。
  return {
    id: 'target_1',
    projectId: 'prj_1',
    applicationId: 'app_1',
    name: 'web',
    stage: 'dev',
    kubernetesName: 'web-dev',
    clusterId: '',
    workloadType: 'Deployment',
    replicas: 1,
    cpuRequest: '500m',
    memoryRequest: '512Mi',
    imagePullPolicy: 'IfNotPresent',
    containerCommand: '',
    containerArgs: '',
    lifecycle: '',
    initContainers: '',
    sidecarContainers: '',
    readinessProbe: '',
    livenessProbe: '',
    startupProbe: '',
    runAsUser: '',
    runAsGroup: '',
    fsGroup: '',
    fsGroupChangePolicy: '',
    readOnlyRootFilesystem: false,
    capabilityDrop: '',
    nodeSelector: '',
    tolerations: '',
    affinity: '',
    topologySpreadConstraints: '',
    priorityClassName: '',
    serviceAnnotations: '',
    serviceSessionAffinity: '',
    autoScalingEnabled: false,
    autoScalingMinReplicas: 1,
    autoScalingMaxReplicas: 1,
    autoScalingCpuPercent: 0,
    autoScalingMemoryPercent: 0,
    autoScalingBehavior: '',
    servicePorts: [{ name: 'http', port: 8080, appProtocol: '' }],
    sourceType: 'image',
    repositoryBindingId: '',
    buildDefinitionMode: 'repository_dockerfile',
    buildTemplateId: '',
    buildTemplateVersion: '',
    buildTemplateValues: '{}',
    dockerfilePath: '',
    buildContext: '',
    buildDirectory: '',
    buildArgs: '',
    buildEnvironmentId: '',
    buildCpuRequest: '',
    buildMemoryRequest: '',
    buildTimeoutSeconds: 1800,
    targetRegistryId: '',
    targetRepository: '',
    targetTag: '',
    imageRef: 'registry.example/team/web:v1',
    buildVariableSetIds: [],
    buildHooksEnabled: false,
    buildHookBindings: [],
    autoDeploy: false,
    branchPattern: '',
    tagPattern: '',
    runtimeConfigRefs: [],
    environmentVariables: [],
    configFiles: '',
    secretFilesSet: false,
    dataVolumes: [],
    requireApproval: false,
    webConsoleEnabled: true,
    enabled: true,
    status: 'ready',
    desiredReplicas: 1,
    updatedReplicas: 1,
    readyReplicas: 1,
    availableReplicas: 1,
    deleteStatus: 'active',
    deleteMessage: '',
    createdBy: 'usr_1',
    createdAt: '2026-10-01T00:00:00Z',
  } as unknown as DeploymentTarget
}

function renderEditor({ releases = [] as Release[], target = null as DeploymentTarget | null, values = {} as Partial<DeploymentTargetFormValues> } = {}) {
  const queryClient = new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })
  const initialValues = target
    ? deploymentTargetToFormValues(target)
    : { ...emptyDeploymentTargetFormValues(), repositoryBindingId: 'rb_1', targetImageRef: 'team/demo:latest', targetRegistryId: 'reg_1', ...values }
  const editorPath = target ? `/projects/prj_1/apps/app_1/deployments/${target.id}/edit` : '/projects/prj_1/apps/app_1/deployments/new'
  // 编辑器用 useBlocker 拦截未保存离开，需要数据路由。
  const router = createMemoryRouter([
    {
      path: target ? '/projects/:projectId/apps/:applicationId/deployments/:targetId/edit' : '/projects/:projectId/apps/:applicationId/deployments/new',
      element: (
        <>
          <Link to="/projects/prj_1">project workspace</Link>
          <DeploymentTargetEditor
            applicationId="app_1"
            buildEnvironmentAvailable
            canManageRuntimeSecrets
            deploymentTargets={[]}
            initialValues={initialValues}
            projectId="prj_1"
            registries={[{ credentialSet: true, endpoint: 'https://registry.example.com', id: 'reg_1', isDefault: true, name: 'Registry', namespace: 'team', provider: 'harbor' } as never]}
            releases={releases}
            repositoryBindings={[repositoryBinding as never]}
            runtimeClusters={[]}
            target={target}
          />
        </>
      ),
    },
    { path: '/projects/:projectId/apps/:applicationId', element: <p>deployments list</p> },
    { path: '/projects/:projectId', element: <p>project workspace page</p> },
  ], { initialEntries: [editorPath] })
  render(
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <RouterProvider router={router} />
      </TooltipProvider>
    </QueryClientProvider>,
  )
  return router
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

  it('groups the form into common sections and a collapsed advanced section', async () => {
    renderEditor()

    const navigation = screen.getByRole('navigation', { name: 'Deploy config sections' })
    for (const section of ['Basics and source', 'Runtime', 'Release triggers', 'Advanced \\(Kubernetes\\)'])
      expect(within(navigation).getByRole('link', { name: new RegExp(section) })).toBeInTheDocument()
    expect(screen.getByRole('textbox', { name: /Name/ })).toBeInTheDocument()
    expect(screen.getByRole('radio', { name: /Code repository|Repository/ })).toBeChecked()
    expect(screen.getByRole('spinbutton', { name: /Replicas/ })).toBeInTheDocument()
    expect(screen.getByRole('switch', { name: 'Enable this deploy config' })).toBeChecked()
    expect(screen.getByRole('button', { name: /Advanced \(Kubernetes\)/ })).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByRole('combobox', { name: /Workload type/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('switch', { name: 'Enable deployment hooks' })).not.toBeInTheDocument()
  })

  it('expands the advanced section from the navigation and summarizes each item', async () => {
    const user = userEvent.setup()
    renderEditor()

    await user.click(within(screen.getByRole('navigation', { name: 'Deploy config sections' })).getByRole('link', { name: /Advanced/ }))

    expect(screen.getByRole('button', { name: /Advanced \(Kubernetes\)/ })).toHaveAttribute('aria-expanded', 'true')
    const container = screen.getByRole('button', { name: /Container startup and health checks/ })
    expect(container).toHaveAttribute('aria-expanded', 'false')
    expect(container).toHaveTextContent('Using defaults')
    await user.click(container)
    expect(screen.getByRole('combobox', { name: /Image pull policy/ })).toBeInTheDocument()
  })

  it('opens configured advanced items and forces them open when they fail validation', async () => {
    const user = userEvent.setup()
    renderEditor({ values: { name: 'dev', runAsUser: 'abc' } })

    const advanced = screen.getByRole('button', { name: /Advanced \(Kubernetes\)/ })
    expect(advanced).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByRole('button', { name: /Security context and scheduling/ })).toHaveTextContent('Set: 1')
    await user.click(advanced)
    expect(advanced).toHaveAttribute('aria-expanded', 'false')

    await user.click(screen.getByRole('button', { name: 'Create deploy config' }))

    expect(api.createDeploymentTarget).not.toHaveBeenCalled()
    expect(advanced).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByRole('textbox', { name: /Run as UID/ })).toHaveAttribute('aria-invalid', 'true')
    expect(within(screen.getByRole('navigation', { name: 'Deploy config sections' })).getByRole('link', { name: /Advanced/ })).toHaveTextContent('Errors: 1')
  })

  it('writes size presets into the CPU and memory requests', async () => {
    const user = userEvent.setup()
    renderEditor({ values: { name: 'dev' } })

    expect(screen.getByRole('radio', { name: /^L/ })).toBeChecked()
    expect(screen.queryByRole('textbox', { name: /CPU quota/ })).not.toBeInTheDocument()
    await user.click(screen.getByRole('radio', { name: /^M/ }))
    await user.click(screen.getByRole('button', { name: 'Create deploy config' }))

    expect(await screen.findByText('deployments list')).toBeInTheDocument()
    expect(api.createDeploymentTarget).toHaveBeenCalledWith('prj_1', 'app_1', expect.objectContaining({ cpuRequest: '500m', memoryRequest: '512Mi' }))
  })

  it('shows the raw CPU and memory inputs for custom sizes', async () => {
    const user = userEvent.setup()
    renderEditor({ values: { cpuRequest: '750m', memoryRequest: '768Mi' } })

    expect(screen.getByRole('radio', { name: /Custom/ })).toBeChecked()
    expect(screen.getByRole('textbox', { name: /CPU quota/ })).toHaveValue('750')
    await user.click(screen.getByRole('radio', { name: /^S/ }))
    expect(screen.queryByRole('textbox', { name: /CPU quota/ })).not.toBeInTheDocument()
    await user.click(screen.getByRole('radio', { name: /Custom/ }))
    expect(screen.getByRole('textbox', { name: /CPU quota/ })).toHaveValue('250')
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

  it('asks before leaving with unsaved changes and keeps the draft when the user stays', async () => {
    const user = userEvent.setup()
    renderEditor()

    await user.type(screen.getByRole('textbox', { name: /Name/ }), 'dev')
    await user.click(screen.getByRole('link', { name: 'project workspace' }))

    const dialog = await screen.findByRole('alertdialog', { name: 'Discard unsaved changes?' })
    await user.click(within(dialog).getByRole('button', { name: 'Keep editing' }))
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
    expect(screen.getByRole('textbox', { name: /Name/ })).toHaveValue('dev')

    await user.click(screen.getByRole('link', { name: 'project workspace' }))
    await user.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Discard and leave' }))
    expect(await screen.findByText('project workspace page')).toBeInTheDocument()
  })

  it('does not block leaving when nothing changed or the user cancels', async () => {
    const user = userEvent.setup()
    renderEditor()

    await user.click(screen.getByRole('link', { name: 'project workspace' }))
    expect(await screen.findByText('project workspace page')).toBeInTheDocument()
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  })

  it('leaves through cancel without prompting even with a draft', async () => {
    const user = userEvent.setup()
    renderEditor()

    await user.type(screen.getByRole('textbox', { name: /Name/ }), 'dev')
    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(await screen.findByText('deployments list')).toBeInTheDocument()
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  })

  it('reports a saved config separately when the redeploy fails and retries only the deploy', async () => {
    const user = userEvent.setup()
    const target = imageTarget()
    vi.mocked(api.updateDeploymentTarget).mockImplementation(async (_projectId, _applicationId, _targetId, payload) => ({ ...target, imageRef: payload.imageRef ?? target.imageRef }) as DeploymentTarget)
    vi.mocked(api.createRelease).mockRejectedValueOnce(new Error('queue unavailable'))
    vi.mocked(api.createRelease).mockResolvedValueOnce({ id: 'rel_2' } as Release)
    renderEditor({ target })

    const image = screen.getByRole('textbox', { name: /Image reference/ })
    await user.clear(image)
    await user.click(image)
    await user.paste('registry.example/team/web:v2')
    await user.click(screen.getByRole('button', { name: 'Save and redeploy' }))

    expect(await screen.findByText('Config saved, but redeploy failed: queue unavailable')).toBeInTheDocument()
    expect(api.updateDeploymentTarget).toHaveBeenCalledTimes(1)
    expect(screen.queryByRole('button', { name: 'Save and redeploy' })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Retry deploy' }))
    expect(await screen.findByText('deployments list')).toBeInTheDocument()
    expect(api.updateDeploymentTarget).toHaveBeenCalledTimes(1)
    expect(api.createRelease).toHaveBeenLastCalledWith('prj_1', expect.objectContaining({ deploymentTargetId: 'target_1', imageRef: 'registry.example/team/web:v2' }))
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
