import type { DeploymentTargetFormValues } from './deployment-target-form'
import type { BuildTemplate, DeploymentTarget, Release } from '@/api'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryRouter, Link, RouterProvider } from 'react-router-dom'
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { api, ApiError } from '@/api'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { DeploymentTargetEditor } from './deployment-target-editor'
import { deploymentTargetSectionAnchor, scrollChildIntoHorizontalContainer } from './deployment-target-editor-utils'
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
      listProjectVolumes: vi.fn(),
      observeRuntimeClusterPressure: vi.fn(),
      updateDeploymentTarget: vi.fn(),
    },
  }
})

const repositoryBinding = { applicationId: 'app_1', defaultBranch: 'main', gitAccountId: 'acc_1', id: 'rb_1', owner: 'team', repo: 'demo' }

function imageTarget(overrides: Partial<DeploymentTarget> = {}) {
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
    buildConcurrencyPolicy: 'queue',
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
    ...overrides,
  } as unknown as DeploymentTarget
}

function renderEditor({ releases = [] as Release[], target = null as DeploymentTarget | null, values = {} as Partial<DeploymentTargetFormValues> } = {}) {
  const queryClient = new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })
  const initialValues = target
    ? { ...deploymentTargetToFormValues(target), ...values }
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

describe('deployment target editor', { timeout: 15_000 }, () => {
  beforeAll(() => {
    Element.prototype.scrollIntoView = vi.fn()
  })

  beforeEach(async () => {
    await i18next.changeLanguage('en-US')
    vi.mocked(api.listBuildTemplates).mockResolvedValue([])
    vi.mocked(api.listProjectHooks).mockResolvedValue([])
    vi.mocked(api.listProjectRuntimeConfigSets).mockResolvedValue([])
    vi.mocked(api.listProjectVolumes).mockResolvedValue({ items: [], page: 1, pageSize: 20, sortBy: 'displayName', sortOrder: 'asc', total: 0, totalPages: 0 })
    vi.mocked(api.observeRuntimeClusterPressure).mockResolvedValue([])
    vi.mocked(api.getGitRepositoryBuildOptions).mockResolvedValue({ directories: ['.'], dockerfiles: ['Dockerfile'], exposedPorts: {}, recommendedTemplateIds: [] } as never)
    vi.mocked(api.getRegistryImageTemplateDefault).mockResolvedValue({ targetImageRef: 'team/demo:dev' } as never)
    vi.mocked(api.createDeploymentTarget).mockResolvedValue({ id: 'target_1' } as DeploymentTarget)
  })

  afterEach(() => {
    vi.clearAllMocks()
  })

  it('groups the form into 4 balanced sections with workload type in basics and 6+1 collapsed advanced items', async () => {
    renderEditor()

    const navigation = screen.getByRole('navigation', { name: 'Deploy config sections' })
    for (const section of ['Basics', 'Source and build', 'Runtime and scaling', 'Advanced options'])
      expect(within(navigation).getByRole('link', { name: new RegExp(section) })).toBeInTheDocument()
    expect(screen.getByRole('textbox', { name: /Name/ })).toBeInTheDocument()
    expect(screen.getByRole('combobox', { name: /Workload type/ })).toBeInTheDocument()
    expect(screen.getByRole('radio', { name: /Code repository|Repository/ })).toBeChecked()
    expect(screen.getByRole('combobox', { name: /Image pull policy/ })).toBeInTheDocument()
    expect(screen.getByRole('spinbutton', { name: /Replicas/ })).toBeInTheDocument()
    expect(screen.getByRole('switch', { name: 'Enable this deploy config' })).toBeChecked()
    expect(screen.getByRole('button', { name: /Advanced options/ })).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByRole('switch', { name: 'Enable deployment hooks' })).not.toBeInTheDocument()
  })

  it('expands the advanced section from the navigation and summarizes each item', async () => {
    const user = userEvent.setup()
    renderEditor()

    await user.click(within(screen.getByRole('navigation', { name: 'Deploy config sections' })).getByRole('link', { name: /Advanced options/ }))

    expect(screen.getByRole('button', { name: /Advanced options/ })).toHaveAttribute('aria-expanded', 'true')
    const probes = screen.getByRole('button', { name: /Health check probes/ })
    expect(probes).toHaveAttribute('aria-expanded', 'false')
    expect(probes).toHaveTextContent('Using defaults')
    await user.click(probes)
    expect(screen.getByRole('combobox', { name: 'Readiness probe' })).toBeInTheDocument()
  })

  it('opens configured advanced items and forces them open and focused when they fail validation', async () => {
    const user = userEvent.setup()
    renderEditor({ values: { name: 'dev', runAsUser: 'abc' } })

    const advanced = screen.getByRole('button', { name: /Advanced options/ })
    expect(advanced).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByRole('button', { name: /Security context and node scheduling/ })).toHaveTextContent('Set: 1')
    await user.click(advanced)
    expect(advanced).toHaveAttribute('aria-expanded', 'false')

    await user.click(screen.getByRole('button', { name: 'Create deploy config' }))

    expect(api.createDeploymentTarget).not.toHaveBeenCalled()
    expect(advanced).toHaveAttribute('aria-expanded', 'true')
    const uidInput = screen.getByRole('textbox', { name: /Run as UID/ })
    expect(uidInput).toHaveAttribute('aria-invalid', 'true')
    await waitFor(() => expect(uidInput).toHaveFocus())
    expect(within(screen.getByRole('navigation', { name: 'Deploy config sections' })).getByRole('link', { name: /Advanced options/ })).toHaveTextContent('Errors: 1')
  })

  it('writes size presets into the CPU and memory requests and supports decimal custom quotas', async () => {
    const user = userEvent.setup()
    renderEditor({ values: { name: 'dev' } })

    expect(screen.getByRole('radio', { name: /^L/ })).toBeChecked()
    expect(screen.queryByRole('textbox', { name: /CPU quota/ })).not.toBeInTheDocument()
    await user.click(screen.getByRole('radio', { name: /Custom/ }))
    const cpuInput = screen.getByRole('textbox', { name: /CPU quota/ })
    await user.clear(cpuInput)
    await user.type(cpuInput, '1.5')
    await user.click(screen.getByRole('combobox', { name: 'CPU unit' }))
    await user.click(await screen.findByRole('option', { name: 'core' }))
    await user.click(screen.getByRole('button', { name: 'Create deploy config' }))

    expect(await screen.findByText('deployments list')).toBeInTheDocument()
    expect(api.createDeploymentTarget).toHaveBeenCalledWith('prj_1', 'app_1', expect.objectContaining({ cpuRequest: '1.5', memoryRequest: '1Gi' }))
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

  it('syncs derived targetImageRef from registry template on submit even when form field starts empty', async () => {
    const user = userEvent.setup()
    renderEditor({ values: { name: 'dev', targetImageRef: '' } })

    expect(await screen.findByDisplayValue('team/demo:dev')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Create deploy config' }))

    expect(await screen.findByText('deployments list')).toBeInTheDocument()
    expect(api.createDeploymentTarget).toHaveBeenCalledWith('prj_1', 'app_1', expect.objectContaining({
      name: 'dev',
      targetImageRef: 'team/demo:dev',
    }))
  })

  it('validates required build template parameters and focuses the invalid parameter input', async () => {
    const templates: BuildTemplate[] = [{
      id: 'go-service',
      version: '1.0.0',
      runtime: 'go',
      category: 'backend',
      defaultServicePort: 8080,
      parameters: [
        { key: 'port', type: 'port', required: true, defaultValue: '9000' },
      ],
    }]
    vi.mocked(api.listBuildTemplates).mockResolvedValue(templates)
    const user = userEvent.setup()
    renderEditor({ values: { name: 'dev' } })

    // Wait for listBuildTemplates query to settle before switching mode
    await waitFor(() => expect(api.listBuildTemplates).toHaveBeenCalled())
    await user.click(screen.getByRole('combobox', { name: /Build definition/ }))
    await user.click(await screen.findByRole('option', { name: 'Platform build template' }))
    const portInput = await screen.findByDisplayValue('9000')
    await user.clear(portInput)
    await user.click(screen.getByRole('button', { name: 'Create deploy config' }))

    expect(api.createDeploymentTarget).not.toHaveBeenCalled()
    expect(portInput).toHaveAttribute('aria-invalid', 'true')
    await waitFor(() => expect(portInput).toHaveFocus())
  })

  it('expands config files and focuses the path input when a relative file path fails validation', async () => {
    const user = userEvent.setup()
    renderEditor({
      values: {
        name: 'dev',
        configFiles: JSON.stringify([{ path: 'relative.yaml', content: 'port: 8080' }]),
      },
    })

    // Collapse advanced first to verify submit re-expands and focuses the invalid file path input.
    const advanced = screen.getByRole('button', { name: /Advanced options/ })
    await user.click(advanced)
    expect(advanced).toHaveAttribute('aria-expanded', 'false')

    await user.click(screen.getByRole('button', { name: 'Create deploy config' }))

    expect(api.createDeploymentTarget).not.toHaveBeenCalled()
    expect(advanced).toHaveAttribute('aria-expanded', 'true')
    const pathInput = screen.getByRole('textbox', { name: 'File path' })
    expect(pathInput).toHaveAttribute('aria-invalid', 'true')
    await waitFor(() => expect(pathInput).toHaveFocus())
  })

  it('focuses the actual invalid data volume input rather than an unfocusable group container', async () => {
    const user = userEvent.setup()
    renderEditor({ values: { name: 'dev' } })

    await user.click(screen.getByRole('button', { name: 'Add data volume' }))
    await user.click(screen.getByRole('combobox', { name: 'Source' }))
    await user.click(await screen.findByRole('option', { name: 'Temporary emptyDir' }))
    const mountPathInput = screen.getByRole('textbox', { name: 'Data path' })
    await user.type(mountPathInput, 'relative/cache')
    await user.click(screen.getByRole('button', { name: 'Create deploy config' }))

    expect(api.createDeploymentTarget).not.toHaveBeenCalled()
    expect(mountPathInput).toHaveAttribute('aria-invalid', 'true')
    await waitFor(() => expect(mountPathInput).toHaveFocus())
  })

  it('eliminates ghost build errors when switching sourceType to image', async () => {
    const user = userEvent.setup()
    renderEditor({ values: { name: 'dev', buildCpuRequest: '' } })

    await user.click(screen.getByRole('button', { name: 'Create deploy config' }))
    expect(within(screen.getByRole('navigation', { name: 'Deploy config sections' })).getByRole('link', { name: /Advanced options/ })).toHaveTextContent('Errors: 1')

    await user.click(screen.getByRole('radio', { name: /Existing image/ }))
    expect(within(screen.getByRole('navigation', { name: 'Deploy config sections' })).getByRole('link', { name: /Advanced options/ })).not.toHaveTextContent('Errors:')
  })

  it('allows explicitly clearing existing secret files and shows mutually exclusive sticky status', async () => {
    const user = userEvent.setup()
    const target = imageTarget({ secretFilesSet: true })
    vi.mocked(api.updateDeploymentTarget).mockResolvedValue(target)
    renderEditor({ target })

    await user.click(within(screen.getByRole('navigation', { name: 'Deploy config sections' })).getByRole('link', { name: /Advanced options/ }))
    await user.click(screen.getByRole('button', { name: 'Clear existing secret files' }))

    expect(screen.getByText('Existing secret files will be cleared on save')).toBeInTheDocument()
    expect(screen.getByText(/On save:.*clear 1 encrypted items\./)).toBeInTheDocument()

    // Triggering a validation error hides pendingSecretSummary in favor of fixErrors jump button.
    await user.clear(screen.getByRole('textbox', { name: /Name/ }))
    await user.click(screen.getByRole('button', { name: 'Save only' }))
    expect(screen.getByRole('button', { name: /Fields to fix: 1/ })).toBeInTheDocument()
    expect(screen.queryByText(/On save:/)).not.toBeInTheDocument()
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

  it('serializes structured HTTP readiness probes into Kubernetes Probe JSON', async () => {
    const user = userEvent.setup()
    renderEditor({ values: { name: 'dev' } })

    await user.click(within(screen.getByRole('navigation', { name: 'Deploy config sections' })).getByRole('link', { name: /Advanced options/ }))
    await user.click(screen.getByRole('button', { name: /Health check probes/ }))
    await user.click(screen.getByRole('combobox', { name: 'Readiness probe' }))
    await user.click(await screen.findByRole('option', { name: 'HTTP GET' }))
    const pathInput = screen.getByRole('textbox', { name: 'Path' })
    await user.clear(pathInput)
    await user.type(pathInput, '/readyz')
    await user.click(screen.getByRole('button', { name: 'Create deploy config' }))

    expect(await screen.findByText('deployments list')).toBeInTheDocument()
    expect(api.createDeploymentTarget).toHaveBeenCalledWith('prj_1', 'app_1', expect.objectContaining({
      readinessProbe: JSON.stringify({
        httpGet: { path: '/readyz', port: 8080 },
        initialDelaySeconds: 5,
        periodSeconds: 10,
        timeoutSeconds: 3,
        failureThreshold: 3,
      }),
    }))
  })

  it('syncs horizontal nav scrollLeft on activeSection changes without calling scrollIntoView or snapping mobile main scroll back', () => {
    const originalIntersectionObserver = globalThis.IntersectionObserver
    let intersectionCallback: IntersectionObserverCallback | null = null

    class MockIntersectionObserver implements IntersectionObserver {
      readonly root = null
      readonly rootMargin = '0px 0px -65% 0px'
      readonly scrollMargin = '0px'
      readonly thresholds = [0]

      constructor(callback: IntersectionObserverCallback) {
        intersectionCallback = callback
      }

      disconnect = vi.fn()
      observe = vi.fn()
      takeRecords = () => []
      unobserve = vi.fn()
    }

    globalThis.IntersectionObserver = MockIntersectionObserver as unknown as typeof IntersectionObserver

    try {
      const scrollIntoViewMock = vi.mocked(Element.prototype.scrollIntoView)
      scrollIntoViewMock.mockClear()

      renderEditor({ values: { name: 'dev' } })

      // Simulate narrow mobile viewport where the action bar is non-sticky by default (< @lg/workspace).
      const actionBar = document.querySelector('[data-slot="editor-action-bar"]')
      expect(actionBar).not.toBeNull()
      expect(actionBar).not.toHaveClass('sticky', 'bottom-0')
      expect(actionBar).toHaveClass('@lg/workspace:sticky', '@lg/workspace:bottom-4')

      const navigation = screen.getByRole('navigation', { name: 'Deploy config sections' })
      const navList = navigation.querySelector('ol') as HTMLOListElement
      const runtimeLink = within(navigation).getByRole('link', { name: /Runtime and scaling/ })

      // Mock horizontal overflow on the narrow mobile nav strip.
      Object.defineProperty(navList, 'clientWidth', { configurable: true, value: 320 })
      Object.defineProperty(navList, 'scrollWidth', { configurable: true, value: 680 })
      navList.scrollLeft = 0
      const scrollToSpy = vi.fn(({ left }: ScrollToOptions) => {
        if (typeof left === 'number')
          navList.scrollLeft = left
      })
      navList.scrollTo = scrollToSpy as unknown as typeof navList.scrollTo
      vi.spyOn(navList, 'getBoundingClientRect').mockReturnValue({
        bottom: 48,
        height: 48,
        left: 0,
        right: 320,
        toJSON: () => ({}),
        top: 0,
        width: 320,
        x: 0,
        y: 0,
      })
      vi.spyOn(runtimeLink, 'getBoundingClientRect').mockReturnValue({
        bottom: 44,
        height: 40,
        left: 360,
        right: 504,
        toJSON: () => ({}),
        top: 4,
        width: 144,
        x: 360,
        y: 4,
      })

      const runtimeSection = document.getElementById(deploymentTargetSectionAnchor('runtime')) as HTMLElement
      expect(runtimeSection).not.toBeNull()
      expect(intersectionCallback).not.toBeNull()

      act(() => {
        intersectionCallback?.([
          {
            boundingClientRect: runtimeSection.getBoundingClientRect(),
            intersectionRatio: 1,
            intersectionRect: runtimeSection.getBoundingClientRect(),
            isIntersecting: true,
            rootBounds: null,
            target: runtimeSection,
            time: 100,
          },
        ], {} as IntersectionObserver)
      })

      expect(runtimeLink).toHaveAttribute('aria-current', 'location')
      // Only the horizontal <ol> container scrolls; Element.scrollIntoView is NEVER invoked on passive section changes.
      expect(scrollIntoViewMock).not.toHaveBeenCalled()
      expect(scrollToSpy).toHaveBeenCalledWith({
        behavior: expect.stringMatching(/^(?:auto|smooth)$/),
        left: 272,
      })
      expect(navList.scrollLeft).toBe(272)
    }
    finally {
      globalThis.IntersectionObserver = originalIntersectionObserver
    }
  })

  it('clamps horizontal scroll offsets safely and skips non-overflowing containers', () => {
    const container = document.createElement('ol')
    const child = document.createElement('a')
    container.appendChild(child)
    const scrollToSpy = vi.fn()
    container.scrollTo = scrollToSpy

    // Desktop grid: scrollWidth === clientWidth -> no-op.
    Object.defineProperty(container, 'clientWidth', { configurable: true, value: 240 })
    Object.defineProperty(container, 'scrollWidth', { configurable: true, value: 240 })
    scrollChildIntoHorizontalContainer(container, child, 'auto')
    expect(scrollToSpy).not.toHaveBeenCalled()

    // Mobile strip: clamps to maxScrollLeft when targeting the trailing item.
    Object.defineProperty(container, 'clientWidth', { configurable: true, value: 300 })
    Object.defineProperty(container, 'scrollWidth', { configurable: true, value: 600 })
    container.scrollLeft = 100
    vi.spyOn(container, 'getBoundingClientRect').mockReturnValue({
      bottom: 40,
      height: 40,
      left: 16,
      right: 316,
      toJSON: () => ({}),
      top: 0,
      width: 300,
      x: 16,
      y: 0,
    })
    vi.spyOn(child, 'getBoundingClientRect').mockReturnValue({
      bottom: 36,
      height: 36,
      left: 450,
      right: 590,
      toJSON: () => ({}),
      top: 2,
      width: 140,
      x: 450,
      y: 2,
    })
    scrollChildIntoHorizontalContainer(container, child, 'smooth')
    expect(scrollToSpy).toHaveBeenCalledWith({ behavior: 'smooth', left: 300 })
  })

  it('shows the floating unsaved-changes capsule only after editing and discards changes in place', async () => {
    const user = userEvent.setup()
    const target = imageTarget({ name: 'prod-web' })
    renderEditor({ target })

    expect(document.querySelector('[data-slot="editor-action-bar"]')).toBeNull()

    const nameInput = screen.getByRole('textbox', { name: /Name/ })
    await user.clear(nameInput)
    await user.type(nameInput, 'prod-web-v2')

    const actionBar = document.querySelector('[data-slot="editor-action-bar"]') as HTMLElement
    expect(actionBar).not.toBeNull()
    expect(within(actionBar).getByText('Unsaved changes')).toBeInTheDocument()
    expect(within(actionBar).getByRole('button', { name: 'Save' })).toBeInTheDocument()

    await user.click(within(actionBar).getByRole('button', { name: 'Discard changes' }))
    expect(nameInput).toHaveValue('prod-web')
    expect(document.querySelector('[data-slot="editor-action-bar"]')).toBeNull()
  })
})
