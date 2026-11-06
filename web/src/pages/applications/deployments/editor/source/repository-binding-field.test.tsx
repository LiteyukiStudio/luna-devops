import type { DeploymentTargetFormValues } from '../deployment-target-form'
import type { RepositoryBinding } from '@/api'
import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { FormProvider, useForm } from 'react-hook-form'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { emptyDeploymentTargetFormValues } from '../deployment-target-form'
import { RepositoryBindingField } from './repository-binding-field'

const mocks = vi.hoisted(() => ({
  createRepositoryBinding: vi.fn(),
  listGitAccounts: vi.fn(),
  listGitBranches: vi.fn(),
  listGitProviders: vi.fn(),
  listGitRepositories: vi.fn(),
  listRepositoryBindings: vi.fn(),
}))

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return { ...actual, api: { ...actual.api, ...mocks } }
})

vi.mock('sonner', () => ({
  toast: { error: vi.fn(), success: vi.fn() },
}))

function bindingFixture(id: string, repo: string): RepositoryBinding {
  return {
    id,
    projectId: 'prj_1',
    applicationId: 'app_1',
    gitProviderId: 'gp_1',
    gitAccountId: 'ga_1',
    owner: 'liteyuki',
    repo,
    cloneUrl: '',
    defaultBranch: 'main',
    webhookEnabled: true,
    webhookStatus: 'ready',
    webhookId: '',
    webhookCallbackUrl: '',
    lastEvent: '',
    lastCommitSha: '',
    createdAt: '2026-11-01T00:00:00Z',
  }
}

const existingBinding = bindingFixture('rb_existing', 'api')
const newBinding = bindingFixture('rb_new', 'web')

describe('repository binding field', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18next.changeLanguage('en-US')
    mocks.listGitProviders.mockResolvedValue([{ id: 'gp_1', name: 'GitHub', type: 'github', enabled: true }])
    mocks.listGitAccounts.mockResolvedValue([{ id: 'ga_1', providerId: 'gp_1', username: 'octo', scope: 'user' }])
    mocks.listGitRepositories.mockResolvedValue({ items: [], page: 1, pageSize: 50, total: 0, totalPages: 0 })
    mocks.listGitBranches.mockResolvedValue({ items: [{ name: 'main' }], limited: false })
    mocks.createRepositoryBinding.mockResolvedValue(newBinding)
    mocks.listRepositoryBindings.mockResolvedValueOnce([existingBinding]).mockResolvedValue([existingBinding, newBinding])
  })

  it('binds a new code repository in place and selects it for the deploy config', async () => {
    const user = userEvent.setup()
    const onRepositoryChange = vi.fn()
    renderField(onRepositoryChange)

    const select = await screen.findByRole('combobox', { name: /Code repository/ })
    expect(within(select).getByRole('option', { name: 'liteyuki/api' })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Bind code repository' }))

    const dialog = await screen.findByRole('dialog', { name: 'Bind code repository' })
    expect(within(dialog).getByRole('link', { name: 'Manage Git providers and credentials' })).toHaveAttribute('href', '/code-repositories')
    await user.selectOptions(within(dialog).getByRole('combobox', { name: 'Git credential' }), 'ga_1')
    await user.type(within(dialog).getByRole('textbox', { name: /^Owner/ }), 'liteyuki')
    await user.type(within(dialog).getByRole('textbox', { name: /^Repo/ }), 'web')
    await user.click(within(dialog).getByRole('button', { name: 'Bind code repository' }))

    expect(mocks.createRepositoryBinding).toHaveBeenCalledWith('prj_1', expect.objectContaining({ applicationId: 'app_1', gitAccountId: 'ga_1', owner: 'liteyuki', repo: 'web' }))
    expect(onRepositoryChange).toHaveBeenLastCalledWith(expect.objectContaining({ id: 'rb_new' }))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(await within(select).findByRole('option', { name: 'liteyuki/web' })).toBeInTheDocument()
    expect(select).toHaveValue('rb_new')
  })
})

/** 与部署配置编辑页一致：仓库列表来自同一查询键，字段绑定成功后写回该缓存。 */
function FieldHarness({ onRepositoryChange }: { onRepositoryChange: (binding: RepositoryBinding | undefined) => void }) {
  const form = useForm<DeploymentTargetFormValues>({ defaultValues: emptyDeploymentTargetFormValues() })
  const bindings = useQuery({ queryKey: ['repository-bindings', 'prj_1', 'app_1'], queryFn: () => mocks.listRepositoryBindings('prj_1', 'app_1') })
  if (!bindings.data)
    return null
  return (
    <FormProvider {...form}>
      <RepositoryBindingField applicationId="app_1" projectId="prj_1" repositoryBindings={bindings.data} onRepositoryChange={onRepositoryChange} />
    </FormProvider>
  )
}

function renderField(onRepositoryChange: (binding: RepositoryBinding | undefined) => void) {
  const queryClient = new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <MemoryRouter>
          <FieldHarness onRepositoryChange={onRepositoryChange} />
        </MemoryRouter>
      </TooltipProvider>
    </QueryClientProvider>,
  )
}
