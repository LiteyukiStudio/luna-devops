import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { ProjectAIAutomationPanel } from './project-ai-automation-panel'

const mocks = vi.hoisted(() => ({
  getProjectAIAutomationConfig: vi.fn(),
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
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <ProjectAIAutomationPanel canManage={canManage} projectId="prj_1" />
      </TooltipProvider>
    </QueryClientProvider>,
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
      updatedBy: '',
      updatedAt: '2026-04-17T00:00:00Z',
    })
    mocks.updateProjectAIAutomationConfig.mockImplementation(async (_projectId, payload) => ({
      projectId: 'prj_1',
      deliveryDiagnosisEnabled: Boolean(payload.deliveryDiagnosisEnabled),
      deliveryBillingUserId: payload.deliveryBillingUserId ?? 'usr_self',
      deliveryBillingUserName: 'Alice Ops',
      deliveryCooldownSeconds: payload.deliveryCooldownSeconds ?? 300,
      deliveryDailyLimit: payload.deliveryDailyLimit ?? 20,
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

    const cooldownInput = screen.getByRole('spinbutton', { name: /Cooldown window \(seconds\)/i })
    await user.clear(cooldownInput)
    expect(await screen.findByText(i18next.t('errors.ai_automation.cooldown_invalid'))).toBeInTheDocument()
    await user.type(cooldownInput, '600')

    const saveButton = screen.getByRole('button', { name: i18next.t('common.save') })
    expect(saveButton).toBeEnabled()
    await user.click(saveButton)

    await waitFor(() => expect(mocks.updateProjectAIAutomationConfig).toHaveBeenCalledWith('prj_1', {
      deliveryDiagnosisEnabled: true,
      deliveryCooldownSeconds: 600,
      deliveryDailyLimit: 20,
      deliveryBillingUserId: 'usr_self',
    }))
  })

  it('disables controls and hides save button when user is read-only', async () => {
    renderPanel(false)

    const toggle = await screen.findByRole('switch', { name: 'Enable automatic failure diagnosis' })
    expect(toggle).toBeDisabled()
    expect(screen.queryByRole('button', { name: i18next.t('common.save') })).not.toBeInTheDocument()
  })
})
