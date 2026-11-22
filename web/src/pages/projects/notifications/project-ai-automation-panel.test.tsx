import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { ProjectAIAutomationPanel } from './project-ai-automation-panel'

const mocks = vi.hoisted(() => ({
  getProjectAIAutomationConfig: vi.fn(),
  listProjectAIDiagnoses: vi.fn(),
  updateProjectAIAutomationConfig: vi.fn(),
}))

vi.mock('@/app/session-context', () => ({
  useSession: () => ({
    user: { id: 'usr_self', name: 'Alice Ops', email: 'alice@example.com', role: 'user' },
  }),
}))

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      ...mocks,
    },
  }
})

vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))

function renderPanel(canManage = true) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <MemoryRouter>
      <QueryClientProvider client={queryClient}>
        <TooltipProvider>
          <ProjectAIAutomationPanel canManage={canManage} projectId="prj_1" />
        </TooltipProvider>
      </QueryClientProvider>
    </MemoryRouter>,
  )
}

describe('project AI automation panel', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18next.changeLanguage('en-US')
    mocks.getProjectAIAutomationConfig.mockResolvedValue({
      projectId: 'prj_1',
      deliveryDiagnosisEnabled: false,
      deliveryBillingUserId: '',
      deliveryBillingUserName: '',
      deliveryCooldownSeconds: 300,
      deliveryDailyLimit: 20,
      runtimeDiagnosisEnabled: false,
      runtimeBillingUserId: '',
      runtimeBillingUserName: '',
      runtimeCooldownSeconds: 300,
      runtimeDailyLimit: 20,
      selfHealMode: 'suggest_only',
      stageSelfHealModes: {},
      allowedAutoHealTools: [],
      maxConsecutiveAutoHeals: 2,
      circuitBreakerWindowSeconds: 1800,
      updatedBy: '',
      updatedAt: '2026-04-17T00:00:00Z',
    })
    mocks.listProjectAIDiagnoses.mockResolvedValue({
      items: [],
      page: 1,
      pageSize: 10,
      sortBy: 'createdAt',
      sortOrder: 'desc',
      total: 0,
      totalPages: 0,
    })
    mocks.updateProjectAIAutomationConfig.mockImplementation(async (_projectId, payload) => ({
      projectId: 'prj_1',
      deliveryDiagnosisEnabled: Boolean(payload.deliveryDiagnosisEnabled),
      deliveryBillingUserId: payload.deliveryBillingUserId ?? 'usr_self',
      deliveryBillingUserName: 'Alice Ops',
      deliveryCooldownSeconds: payload.deliveryCooldownSeconds ?? 300,
      deliveryDailyLimit: payload.deliveryDailyLimit ?? 20,
      runtimeDiagnosisEnabled: Boolean(payload.runtimeDiagnosisEnabled),
      runtimeBillingUserId: payload.runtimeBillingUserId ?? '',
      runtimeBillingUserName: payload.runtimeBillingUserId ? 'Alice Ops' : '',
      runtimeCooldownSeconds: payload.runtimeCooldownSeconds ?? 300,
      runtimeDailyLimit: payload.runtimeDailyLimit ?? 20,
      selfHealMode: payload.selfHealMode ?? 'suggest_only',
      stageSelfHealModes: payload.stageSelfHealModes ?? {},
      allowedAutoHealTools: payload.allowedAutoHealTools ?? [],
      maxConsecutiveAutoHeals: payload.maxConsecutiveAutoHeals ?? 2,
      circuitBreakerWindowSeconds: payload.circuitBreakerWindowSeconds ?? 1800,
      updatedBy: 'usr_self',
      updatedAt: '2026-04-17T00:05:00Z',
    }))
  })

  it('auto-binds current user personal wallet when enabling delivery diagnosis and saves configuration', async () => {
    const user = userEvent.setup()
    renderPanel(true)

    const toggle = await screen.findByRole('switch', { name: 'Enable automatic failure diagnosis' })
    expect(toggle).toHaveAttribute('data-state', 'unchecked')

    await user.click(toggle)
    expect(screen.getByText('Alice Ops')).toBeInTheDocument()

    const cooldownInputs = screen.getAllByRole('spinbutton', { name: /Cooldown window \(seconds\)/i })
    await user.clear(cooldownInputs[0]!)
    expect(await screen.findByText(i18next.t('errors.ai_automation.cooldown_invalid'))).toBeInTheDocument()
    await user.type(cooldownInputs[0]!, '600')

    const saveButton = screen.getByRole('button', { name: i18next.t('common.save') })
    expect(saveButton).toBeEnabled()
    await user.click(saveButton)

    await waitFor(() => expect(mocks.updateProjectAIAutomationConfig).toHaveBeenCalledWith('prj_1', expect.objectContaining({
      deliveryDiagnosisEnabled: true,
      deliveryCooldownSeconds: 600,
      deliveryDailyLimit: 20,
      deliveryBillingUserId: 'usr_self',
      selfHealMode: 'suggest_only',
    })))
  })

  it('requires high-risk secondary confirmation before saving auto_l2 runtime self-healing', async () => {
    mocks.getProjectAIAutomationConfig.mockResolvedValue({
      projectId: 'prj_1',
      deliveryDiagnosisEnabled: false,
      deliveryBillingUserId: '',
      deliveryBillingUserName: '',
      deliveryCooldownSeconds: 300,
      deliveryDailyLimit: 20,
      runtimeDiagnosisEnabled: true,
      runtimeBillingUserId: 'usr_self',
      runtimeBillingUserName: 'Alice Ops',
      runtimeCooldownSeconds: 300,
      runtimeDailyLimit: 20,
      selfHealMode: 'auto_l2',
      stageSelfHealModes: { prod: 'auto_l2' },
      allowedAutoHealTools: [],
      maxConsecutiveAutoHeals: 2,
      circuitBreakerWindowSeconds: 1800,
      updatedBy: 'usr_self',
      updatedAt: '2026-04-17T00:00:00Z',
    })

    const user = userEvent.setup()
    renderPanel(true)

    const restartCheckbox = await screen.findByRole('checkbox', { name: /delivery_restart_target/i })
    await user.click(restartCheckbox)

    const saveButton = screen.getByRole('button', { name: i18next.t('common.save') })
    await user.click(saveButton)

    // Must NOT save before explicit secondary confirmation!
    expect(mocks.updateProjectAIAutomationConfig).not.toHaveBeenCalled()
    expect(await screen.findByText(i18next.t('aiAssistant.projectAutomation.autoL2ConfirmTitle'))).toBeInTheDocument()

    const confirmButton = screen.getByRole('button', { name: i18next.t('aiAssistant.projectAutomation.autoL2ConfirmAction') })
    await user.click(confirmButton)

    await waitFor(() => expect(mocks.updateProjectAIAutomationConfig).toHaveBeenCalledWith('prj_1', expect.objectContaining({
      runtimeDiagnosisEnabled: true,
      runtimeBillingUserId: 'usr_self',
      selfHealMode: 'auto_l2',
      stageSelfHealModes: { prod: 'auto_l2' },
      allowedAutoHealTools: ['delivery_restart_target'],
    })))
  })

  it('disables controls and hides save button when user is read-only', async () => {
    renderPanel(false)

    const toggle = await screen.findByRole('switch', { name: 'Enable automatic failure diagnosis' })
    expect(toggle).toBeDisabled()
    expect(screen.queryByRole('button', { name: i18next.t('common.save') })).not.toBeInTheDocument()
  })

  it('renders diagnosis & self-healing history and opens detail drawer on click', async () => {
    mocks.listProjectAIDiagnoses.mockResolvedValue({
      items: [
        {
          id: 'aidg_1',
          projectId: 'prj_1',
          applicationId: 'app_1',
          applicationName: 'Wordle Helper',
          deploymentTargetId: 'dplt_1',
          deploymentTargetName: 'prod',
          category: 'delivery',
          subjectType: 'build_run',
          subjectId: 'bldr_1',
          status: 'succeeded',
          triggerCode: 'job_failed',
          rootCauseSummary: 'Dockerfile typo FROM nginxx:1.27-alpine',
          confidence: 'high',
          suggestions: ['Change FROM nginxx to FROM nginx'],
          deepLink: '/projects/prj_1/apps/app_1?tab=builds#buildRunId=bldr_1',
          createdAt: '2026-11-22T06:53:45Z',
          finishedAt: '2026-11-22T06:53:55Z',
        },
      ],
      page: 1,
      pageSize: 10,
      sortBy: 'createdAt',
      sortOrder: 'desc',
      total: 1,
      totalPages: 1,
    })

    const user = userEvent.setup()
    renderPanel(true)

    expect(await screen.findByText('Wordle Helper / prod')).toBeInTheDocument()
    expect(screen.getByText('Dockerfile typo FROM nginxx:1.27-alpine')).toBeInTheDocument()

    await user.click(screen.getByText('Wordle Helper / prod'))
    expect(await screen.findByText('Change FROM nginxx to FROM nginx')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Open related resource/i })).toHaveAttribute(
      'href',
      '/projects/prj_1/apps/app_1?tab=builds#buildRunId=bldr_1',
    )
  })
})
