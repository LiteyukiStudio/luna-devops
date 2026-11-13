import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { ProjectNotificationsPanel } from './ProjectNotificationsPanel'

const mocks = vi.hoisted(() => ({
  bindProjectNotificationChannel: vi.fn(),
  createProjectNotificationRule: vi.fn(),
  createScopedNotificationChannel: vi.fn(),
  deleteProjectNotificationRule: vi.fn(),
  deleteScopedNotificationChannel: vi.fn(),
  listApplications: vi.fn(),
  listDeploymentTargets: vi.fn(),
  listProjectNotificationChannels: vi.fn(),
  listProjectNotificationDeliveries: vi.fn(),
  listProjectNotificationRules: vi.fn(),
  listProjects: vi.fn(),
  listScopedNotificationChannels: vi.fn(),
  listScopedNotificationPresets: vi.fn(),
  retryProjectNotificationDelivery: vi.fn(),
  testScopedNotificationChannel: vi.fn(),
  unbindProjectNotificationChannel: vi.fn(),
  updateProjectNotificationRule: vi.fn(),
  updateScopedNotificationChannel: vi.fn(),
}))

vi.mock('@/app/session-context', () => ({
  useSession: () => ({ user: { id: 'usr_owner', permissions: [], role: 'user' } }),
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

describe('project notifications panel', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18next.changeLanguage('en-US')
    mocks.listScopedNotificationPresets.mockResolvedValue([
      {
        id: 'feishu',
        name: 'Feishu Bot',
        adapterKind: 'webhook',
        description: 'Feishu interactive card bot',
        secretFields: ['webhookUrl'],
        optionalSecretFields: ['signSecret'],
      },
    ])
    mocks.listProjects.mockResolvedValue([
      { id: 'prj_1', identifier: 'alpha', name: 'Alpha', description: '' },
    ])
    mocks.listApplications.mockResolvedValue([])
    mocks.listDeploymentTargets.mockResolvedValue([])
    mocks.listProjectNotificationRules.mockResolvedValue(page([]))
    mocks.listProjectNotificationChannels.mockResolvedValue(page([
      {
        id: 'nch_smtp',
        name: 'Ops SMTP Sender',
        adapterKind: 'smtp',
        scope: 'project',
        projectIds: ['prj_1'],
        canManage: true,
        configJson: JSON.stringify({
          host: 'smtp.example.com',
          port: 465,
          security: 'tls',
          from: 'alerts@example.com',
        }),
        enabled: true,
      },
    ]))
    mocks.listScopedNotificationChannels.mockResolvedValue(page([]))
    mocks.listProjectNotificationDeliveries.mockResolvedValue(page([]))
    mocks.createProjectNotificationRule.mockResolvedValue({})
    mocks.unbindProjectNotificationChannel.mockResolvedValue(undefined)
    mocks.retryProjectNotificationDelivery.mockResolvedValue({})
  })

  it('allows a project owner to create a rule with per-rule SMTP recipients and throttling settings', async () => {
    const user = userEvent.setup()
    renderPanel({
      currentUserActions: ['notification:read', 'notification:manage'],
    })

    await user.click(await screen.findByRole('button', { name: 'Create rule' }))
    const dialog = screen.getByRole('dialog', { name: 'Notification rule' })

    const nameInput = within(dialog).getByRole('textbox', { name: 'Name' })
    await user.type(nameInput, 'Runtime OOM & SMTP alerts')
    await user.click(within(dialog).getByRole('checkbox', { name: 'Ops SMTP Sender' }))

    // Reusable SMTP sender without default recipients requires rule-level recipients before saving.
    const submitButton = within(dialog).getByRole('button', { name: i18next.t('notificationsPage.createRule') })
    expect(submitButton).toBeDisabled()
    expect(within(dialog).getByText(i18next.t('notificationsPage.smtpRecipientRequiredError'))).toBeInTheDocument()

    const recipientInput = within(dialog).getByPlaceholderText('oncall@example.com, dev@example.com')
    await user.type(recipientInput, 'oncall@example.com')

    expect(submitButton).toBeEnabled()
    await user.click(submitButton)

    await waitFor(() => expect(mocks.createProjectNotificationRule).toHaveBeenCalledWith(
      'prj_1',
      expect.objectContaining({
        name: 'Runtime OOM & SMTP alerts',
        channelIds: ['nch_smtp'],
        channelTargets: {
          nch_smtp: {
            to: ['oncall@example.com'],
          },
        },
        remindIntervalMinutes: 60,
        maxReminders: 3,
        notifyOnRecovery: true,
        timeZone: 'Asia/Shanghai',
      }),
    ))
  })

  it('shows read-only mode for non-owners without management or redelivery controls', async () => {
    mocks.listProjectNotificationDeliveries.mockResolvedValue(page([
      {
        id: 'ndv_1',
        eventId: 'evt_1',
        eventType: 'runtime.pod_oom_killed',
        channelId: 'nch_smtp',
        channelName: 'Ops SMTP Sender',
        recipientSummary: 'oncall@example.com',
        status: 'failed',
        attemptCount: 3,
        durationMillis: 245,
        errorMessage: 'dial tcp timeout',
        createdAt: '2026-11-12T10:00:00Z',
        queuedAt: '2026-11-12T10:00:00Z',
        finishedAt: '2026-11-12T10:00:01Z',
      },
    ]))

    renderPanel({
      currentUserActions: ['notification:read'],
      initialEntry: '/projects/prj_1?tab=notifications&subtab=deliveries',
    })

    expect(await screen.findByText(i18next.t('notificationsPage.projectReadOnlyHint'))).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: i18next.t('notificationsPage.createRule') })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: i18next.t('notificationsPage.retryDelivery') })).not.toBeInTheDocument()
    expect(await screen.findByText('245 ms')).toBeInTheDocument()
  })

  it('marks multi-space shared channels as read-only when canManage is false and allows unbinding without editing or deleting', async () => {
    const user = userEvent.setup()
    mocks.listProjectNotificationChannels.mockResolvedValue(page([
      {
        id: 'nch_shared',
        name: 'Shared Feishu Bot',
        adapterKind: 'webhook',
        presetId: 'feishu',
        scope: 'project',
        projectIds: ['prj_1', 'prj_2'],
        canManage: false,
        enabled: true,
      },
    ]))

    renderPanel({
      currentUserActions: ['notification:read', 'notification:manage'],
      initialEntry: '/projects/prj_1?tab=notifications&subtab=channels',
    })

    expect(await screen.findByText('Shared Feishu Bot')).toBeInTheDocument()
    expect(screen.getByText(i18next.t('notificationsPage.sharedReadOnlyBadge'))).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: i18next.t('notificationsPage.rowActions', { name: 'Shared Feishu Bot' }) }))
    expect(screen.queryByRole('menuitem', { name: i18next.t('common.edit') })).not.toBeInTheDocument()
    expect(screen.queryByRole('menuitem', { name: i18next.t('common.delete') })).not.toBeInTheDocument()
    await user.click(screen.getByRole('menuitem', { name: i18next.t('notificationsPage.unbindChannel') }))

    const confirmDialog = await screen.findByRole('alertdialog')
    await user.click(within(confirmDialog).getByRole('button', { name: i18next.t('notificationsPage.unbindChannel') }))
    await waitFor(() => expect(mocks.unbindProjectNotificationChannel).toHaveBeenCalledWith('prj_1', 'nch_shared'))
  })

  it('opens the delivery diagnostics drawer with request/response snapshots and triggers manual redelivery', async () => {
    const user = userEvent.setup()
    mocks.listProjectNotificationDeliveries.mockResolvedValue(page([
      {
        id: 'ndv_oom',
        projectId: 'prj_1',
        ruleId: 'nrl_1',
        ruleName: 'Runtime alerts',
        eventId: 'evt_oom_1',
        eventType: 'runtime.pod_oom_killed',
        channelId: 'nch_feishu',
        channelName: 'Team Feishu',
        recipientSummary: 'https://open.feishu.cn/open-apis/bot/v2/hook/***',
        status: 'failed',
        attemptCount: 6,
        durationMillis: 412,
        timeZone: 'Asia/Shanghai',
        errorMessage: 'webhook provider returned error code 19021',
        requestSnapshot: JSON.stringify({ msg_type: 'interactive', card: { header: { template: 'red' } } }),
        responseSnippet: '{"code":19021,"msg":"sign match fail"}',
        createdAt: '2026-11-12T12:00:00Z',
        queuedAt: '2026-11-12T12:00:00Z',
        startedAt: '2026-11-12T12:00:01Z',
        finishedAt: '2026-11-12T12:00:01Z',
      },
    ]))

    renderPanel({
      currentUserActions: ['notification:read', 'notification:manage'],
      initialEntry: '/projects/prj_1?tab=notifications&subtab=deliveries',
    })

    expect(await screen.findByText('412 ms')).toBeInTheDocument()
    await user.click(screen.getByText('Team Feishu'))

    const drawer = await screen.findByRole('dialog', { name: i18next.t('notificationsPage.deliveryDetailTitle') })
    expect(within(drawer).getByText(/"msg_type": "interactive"/)).toBeInTheDocument()
    expect(within(drawer).getByText(/"sign match fail"/)).toBeInTheDocument()
    expect(within(drawer).getByRole('link', { name: i18next.t('notificationsPage.openSourceEvent') })).toHaveAttribute(
      'href',
      '/events?eventId=evt_oom_1&projectId=prj_1',
    )

    await user.click(within(drawer).getByRole('button', { name: i18next.t('notificationsPage.retryDelivery') }))
    await waitFor(() => expect(mocks.retryProjectNotificationDelivery).toHaveBeenCalledWith('prj_1', 'ndv_oom'))
  })

  it('guides owners to configure channels first when no channels are bound and supports switching channel mode cards', async () => {
    const user = userEvent.setup()
    mocks.listProjectNotificationChannels.mockResolvedValue(page([]))

    renderPanel({
      currentUserActions: ['notification:read', 'notification:manage'],
    })

    const setupButton = await screen.findByRole('button', { name: i18next.t('notificationsPage.setupChannelFirst') })
    await user.click(setupButton)

    const createChannelButton = await screen.findByRole('button', { name: i18next.t('notificationsPage.createChannel') })
    await user.click(createChannelButton)

    const dialog = await screen.findByRole('dialog', { name: i18next.t('notificationsPage.channelDialog') })
    const smtpCard = within(dialog).getByRole('radio', { name: new RegExp(i18next.t('notificationsPage.modeSmtpShort')) })
    await user.click(smtpCard)
    expect(smtpCard).toHaveAttribute('aria-checked', 'true')
    expect(within(dialog).getByPlaceholderText('smtp.example.com')).toBeInTheDocument()
  })
})

function renderPanel({
  currentUserActions,
  initialEntry = '/projects/prj_1?tab=notifications',
}: {
  currentUserActions: string[]
  initialEntry?: string
}) {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <MemoryRouter initialEntries={[initialEntry]}>
          <Routes>
            <Route
              element={<ProjectNotificationsPanel currentUserActions={currentUserActions} projectId="prj_1" />}
              path="/projects/:projectId"
            />
          </Routes>
        </MemoryRouter>
      </TooltipProvider>
    </QueryClientProvider>,
  )
}

function page<T>(items: T[]) {
  return {
    items,
    page: 1,
    pageSize: 20,
    sortBy: 'createdAt',
    sortOrder: 'desc' as const,
    total: items.length,
    totalPages: items.length > 0 ? 1 : 0,
  }
}
