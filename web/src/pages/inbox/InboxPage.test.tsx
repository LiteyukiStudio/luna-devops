import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/api/core'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { InboxPage } from './InboxPage'

const mocks = vi.hoisted(() => ({
  listInboxMessages: vi.fn(),
  getInboxMessage: vi.fn(),
  markInboxMessageRead: vi.fn(),
  markAllInboxMessagesRead: vi.fn(),
  archiveInboxMessage: vi.fn(),
  decideInboxActionRequest: vi.fn(),
  previewInboxSelfHealAction: vi.fn(),
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

function renderInbox(initialEntry = '/inbox?message=msg_sh_1') {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <QueryClientProvider client={queryClient}>
        <TooltipProvider>
          <InboxPage />
        </TooltipProvider>
      </QueryClientProvider>
    </MemoryRouter>,
  )
}

describe('inbox runtime self-heal proposal workflow', () => {
  const selfHealMessage = {
    id: 'msg_sh_1',
    type: 'runtime.pod_oom_killed',
    category: 'action' as const,
    priority: 'critical' as const,
    actorId: '',
    projectId: 'prj_1',
    resourceType: 'deployment_target',
    resourceId: 'dplt_prod',
    titleKey: 'inbox.runtime.oomKilled.title',
    contentKey: 'inbox.runtime.anomaly.body',
    params: {
      projectName: 'Core Platform',
      applicationName: 'Order API',
      targetName: 'prod',
      reason: 'OOMKilled',
      readyReplicas: 0,
      desiredReplicas: 2,
      aiRootCauseSummary: 'Worker container exceeded 256Mi memory limit during batch export.',
      aiSuggestions: ['Increase memoryRequest to 512Mi'],
      aiCircuitBreakerTripped: true,
    },
    actionRequestId: 'iar_sh_1',
    actionRequest: {
      id: 'iar_sh_1',
      type: 'runtime.self_heal',
      status: 'pending' as const,
      rowVersion: 1,
      expiresAt: '2026-12-01T00:00:00Z',
      allowedDecisions: ['accept' as const, 'reject' as const],
      proposal: {
        incidentId: 'rai_1',
        applicationId: 'app_1',
        deploymentTargetId: 'dplt_prod',
        toolName: 'delivery_restart_target',
        arguments: { project: 'prj_1', application: 'app_1', target: 'dplt_prod' },
        expectedSummary: 'Rolling restart Order API (prod)',
      },
    },
    deepLink: '/projects/prj_1/apps/app_1?tab=deployments#targetId=dplt_prod',
    groupKey: '',
    readAt: '2026-11-21T00:00:00Z',
    expiresAt: null,
    createdAt: '2026-11-21T00:00:00Z',
    updatedAt: '2026-11-21T00:00:00Z',
  }

  beforeEach(async () => {
    vi.clearAllMocks()
    await i18next.changeLanguage('en-US')
    mocks.listInboxMessages.mockResolvedValue({
      items: [selfHealMessage],
      page: 1,
      pageSize: 20,
      sortBy: 'createdAt',
      sortOrder: 'desc',
      total: 1,
      totalPages: 1,
    })
    mocks.getInboxMessage.mockResolvedValue(selfHealMessage)
  })

  it('displays AI diagnosis, circuit-breaker warning, and handles state-drift re-preview before confirming', async () => {
    // First accept attempt fails with 409 inbox.self_heal_state_drifted; second succeeds with fresh previewLock!
    mocks.decideInboxActionRequest
      .mockRejectedValueOnce(new ApiError('Target state drifted', {
        code: 'inbox.self_heal_state_drifted',
        path: '/inbox/action-requests/iar_sh_1/decision',
        status: 409,
      }))
      .mockResolvedValueOnce({
        ...selfHealMessage.actionRequest,
        status: 'completed',
        rowVersion: 2,
        allowedDecisions: [],
      })

    mocks.previewInboxSelfHealAction.mockResolvedValue({
      toolName: 'delivery_restart_target',
      approvalTier: 'L2',
      approvalPreview: {
        summary: 'Restart Order API (prod) on release rel_v2',
        changes: [{ field: 'restartedAt', before: 'none', after: 'now' }],
      },
      previewLock: 'lock_fresh_user_bound_v2',
      stateDrifted: true,
      driftReason: 'newer_release_created',
    })

    const user = userEvent.setup()
    renderInbox()

    expect(await screen.findByText(i18next.t('inbox.selfHeal.circuitBreakerBanner'))).toBeInTheDocument()
    expect(screen.getAllByText('Worker container exceeded 256Mi memory limit during batch export.')).toHaveLength(2)

    // Click "Confirm self-heal"
    await user.click(screen.getByRole('button', { name: i18next.t('inbox.selfHeal.confirmAction') }))
    const dialogConfirmButtons = await screen.findAllByRole('button', { name: i18next.t('inbox.selfHeal.confirmAction') })
    await user.click(dialogConfirmButtons[dialogConfirmButtons.length - 1]!)

    // First attempt had no previewLock and detected state drift -> shows state-drifted banner & re-preview button
    expect(await screen.findByText(i18next.t('inbox.selfHeal.stateDriftedTitle'))).toBeInTheDocument()
    expect(mocks.decideInboxActionRequest).toHaveBeenCalledWith('iar_sh_1', 'accept', 1, undefined)

    // Click "Re-preview live state"
    await user.click(screen.getByRole('button', { name: i18next.t('inbox.selfHeal.repreviewAction') }))
    expect(await screen.findByText('Restart Order API (prod) on release rel_v2')).toBeInTheDocument()

    // Confirm with fresh user-bound previewLock!
    await user.click(screen.getByRole('button', { name: i18next.t('inbox.selfHeal.confirmWithLockAction') }))
    const lockDialogButtons = await screen.findAllByRole('button', { name: i18next.t('inbox.selfHeal.confirmWithLockAction') })
    await user.click(lockDialogButtons[lockDialogButtons.length - 1]!)

    await waitFor(() => expect(mocks.decideInboxActionRequest).toHaveBeenLastCalledWith(
      'iar_sh_1',
      'accept',
      1,
      'lock_fresh_user_bound_v2',
    ))
  })
})
