import type { AIDeliveryDiagnosisView } from '@/api'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { AIDeliveryDiagnosisCard } from './ai-delivery-diagnosis-card'

const mocks = vi.hoisted(() => ({
  executeProjectAIActionProposal: vi.fn(),
  previewProjectAIActionProposal: vi.fn(),
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

function renderCard(diagnosis?: AIDeliveryDiagnosisView, onActionExecuted = vi.fn()) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <AIDeliveryDiagnosisCard
          applicationId="app_1"
          deploymentTargetId="dt_1"
          diagnosis={diagnosis}
          projectId="prj_1"
          onActionExecuted={onActionExecuted}
        />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('ai delivery diagnosis card', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18next.changeLanguage('en-US')
  })

  it('renders running state and localized error state', () => {
    const { rerender } = renderCard({
      id: 'diag_1',
      subjectType: 'build_run',
      subjectId: 'run_1',
      status: 'running',
      failureCode: 'job_failed',
      rootCauseSummary: '',
      confidence: 'medium',
      suggestions: [],
      createdAt: '2026-04-17T00:00:00Z',
    })
    expect(screen.getByRole('status')).toHaveTextContent('Analyzing tail logs and deployment configuration')

    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    rerender(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <AIDeliveryDiagnosisCard
            applicationId="app_1"
            deploymentTargetId="dt_1"
            diagnosis={{
              id: 'diag_1',
              subjectType: 'build_run',
              subjectId: 'run_1',
              status: 'failed',
              failureCode: 'job_failed',
              rootCauseSummary: '',
              confidence: 'medium',
              suggestions: [],
              errorCode: 'ai.wallet_balance_insufficient',
              createdAt: '2026-04-17T00:00:00Z',
            }}
            projectId="prj_1"
          />
        </MemoryRouter>
      </QueryClientProvider>,
    )
    expect(screen.getByText(i18next.t('aiAssistant.deliveryDiagnosis.failedWalletInsufficient'))).toBeInTheDocument()
  })

  it('renders diagnosis summary, edit link, and runs live preview before executing a proposed action', async () => {
    const user = userEvent.setup()
    const onActionExecuted = vi.fn()
    mocks.previewProjectAIActionProposal.mockResolvedValue({
      toolName: 'build_retry_run',
      approvalTier: 'L2',
      approvalPreview: {
        summary: 'Retry build run run_1 with current target configuration.',
        changes: [{ field: 'status', before: 'failed', after: 'queued' }],
      },
      previewLock: 'lock_live_user_1',
    })
    mocks.executeProjectAIActionProposal.mockResolvedValue({
      toolName: 'build_retry_run',
      result: { id: 'run_2' },
    })

    renderCard({
      id: 'diag_2',
      subjectType: 'build_run',
      subjectId: 'run_1',
      status: 'succeeded',
      failureCode: 'job_failed',
      rootCauseSummary: 'pnpm install failed due to transient upstream registry timeout.',
      confidence: 'high',
      suggestions: ['Retry the build once upstream npm registry recovers.'],
      proposedAction: {
        toolName: 'build_retry_run',
        arguments: { project: 'prj_1', buildRunId: 'run_1' },
        summary: 'Retry failed build run run_1',
      },
      createdAt: '2026-04-17T00:00:00Z',
    }, onActionExecuted)

    expect(screen.getByText('pnpm install failed due to transient upstream registry timeout.')).toBeInTheDocument()
    expect(screen.getByText('Retry the build once upstream npm registry recovers.')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Edit deploy config' })).toHaveAttribute(
      'href',
      '/projects/prj_1/apps/app_1/deployments/dt_1/edit',
    )

    await user.click(screen.getByRole('button', { name: 'Preview & confirm' }))
    const dialog = await screen.findByRole('dialog', { name: 'Confirm AI remediation proposal' })

    await waitFor(() => expect(mocks.previewProjectAIActionProposal).toHaveBeenCalledWith(
      'prj_1',
      {
        toolName: 'build_retry_run',
        arguments: { project: 'prj_1', buildRunId: 'run_1' },
      },
      expect.any(AbortSignal),
    ))
    expect(await within(dialog).findByText('Retry build run run_1 with current target configuration.')).toBeInTheDocument()
    expect(within(dialog).getByText('buildRunId')).toBeInTheDocument()
    expect(within(dialog).getByText('run_1')).toBeInTheDocument()

    await user.click(within(dialog).getByRole('button', { name: 'Confirm & execute' }))
    await waitFor(() => expect(mocks.executeProjectAIActionProposal).toHaveBeenCalledWith('prj_1', {
      toolName: 'build_retry_run',
      arguments: { project: 'prj_1', buildRunId: 'run_1' },
      previewLock: 'lock_live_user_1',
    }))
    expect(onActionExecuted).toHaveBeenCalledTimes(1)
  })
})
