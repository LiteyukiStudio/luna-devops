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

  it('strips deleted or unbound channel and filter IDs when editing a rule with stale references', async () => {
    const user = userEvent.setup()
    mocks.updateProjectNotificationRule.mockResolvedValue({})
    mocks.listProjectNotificationChannels.mockResolvedValue(page([
      {
        id: 'nch_feishu',
        name: 'Feishu Bot',
        adapterKind: 'webhook',
        presetId: 'feishu-bot',
        scope: 'project',
        projectIds: ['prj_1'],
        canManage: true,
        configJson: '{}',
        enabled: true,
      },
    ]))
    mocks.listApplications.mockResolvedValue([
      { id: 'app_live', identifier: 'blog', name: 'Blog', projectId: 'prj_1' },
    ])
    mocks.listDeploymentTargets.mockResolvedValue([
      { id: 'dplt_live', applicationId: 'app_live', name: 'Production', projectId: 'prj_1', stage: 'prod' },
    ])
    mocks.listProjectNotificationRules.mockResolvedValue(page([
      {
        id: 'nrl_stale',
        projectId: 'prj_1',
        name: 'Blog Alert Rule',
        eventTypesJson: JSON.stringify(['release.failed']),
        filterJson: JSON.stringify({
          scope: 'projects',
          projectIds: ['prj_1'],
          applicationIds: ['app_deleted', 'app_live'],
          deploymentTargetIds: ['dplt_deleted', 'dplt_live'],
        }),
        channelIdsJson: JSON.stringify(['nch_deleted']),
        channelTargetsJson: JSON.stringify({ nch_deleted: { to: ['old@example.com'] } }),
        templateId: '',
        locale: '',
        timeZone: 'Asia/Shanghai',
        remindIntervalMinutes: 60,
        maxReminders: 3,
        notifyOnRecovery: true,
        enabled: true,
      },
    ]))

    renderPanel({
      currentUserActions: ['notification:read', 'notification:manage'],
    })

    const actionsButton = await screen.findByRole('button', { name: i18next.t('notificationsPage.rowActions', { name: 'Blog Alert Rule' }) })
    await user.click(actionsButton)
    await user.click(await screen.findByRole('menuitem', { name: i18next.t('common.edit') }))

    const dialog = screen.getByRole('dialog', { name: 'Notification rule' })
    const saveButton = within(dialog).getByRole('button', { name: i18next.t('common.save') })
    expect(saveButton).toBeDisabled()

    await user.click(within(dialog).getByRole('checkbox', { name: 'Feishu Bot' }))
    expect(saveButton).toBeEnabled()

    await user.click(saveButton)
    await waitFor(() => expect(mocks.updateProjectNotificationRule).toHaveBeenCalledWith(
      'prj_1',
      'nrl_stale',
      expect.objectContaining({
        name: 'Blog Alert Rule',
        channelIds: ['nch_feishu'],
        filter: {
          scope: 'projects',
          projectIds: ['prj_1'],
          applicationIds: ['app_live'],
          deploymentTargetIds: ['dplt_live'],
        },
      }),
    ))
    const submittedPayload = mocks.updateProjectNotificationRule.mock.calls[0]?.[2]
    expect(submittedPayload.channelTargets).toBeUndefined()
  })

  it('blocks silent scope widening when a rule only targeted deleted applications until cleared', async () => {
    const user = userEvent.setup()
    mocks.updateProjectNotificationRule.mockResolvedValue({})
    mocks.listProjectNotificationChannels.mockResolvedValue(page([
      {
        id: 'nch_feishu',
        name: 'Feishu Bot',
        adapterKind: 'webhook',
        presetId: 'feishu-bot',
        scope: 'project',
        projectIds: ['prj_1'],
        canManage: true,
        configJson: '{}',
        enabled: true,
      },
    ]))
    mocks.listApplications.mockResolvedValue([
      { id: 'app_live', identifier: 'blog', name: 'Blog', projectId: 'prj_1' },
    ])
    mocks.listDeploymentTargets.mockResolvedValue([
      { id: 'dplt_live', applicationId: 'app_live', name: 'Production', projectId: 'prj_1', stage: 'prod' },
    ])
    mocks.listProjectNotificationRules.mockResolvedValue(page([
      {
        id: 'nrl_sole_deleted_app',
        projectId: 'prj_1',
        name: 'Narrowed App Alert',
        eventTypesJson: JSON.stringify(['release.failed']),
        filterJson: JSON.stringify({
          scope: 'projects',
          projectIds: ['prj_1'],
          applicationIds: ['app_deleted_only'],
        }),
        channelIdsJson: JSON.stringify(['nch_feishu']),
        channelTargetsJson: '{}',
        templateId: '',
        locale: '',
        timeZone: 'Asia/Shanghai',
        remindIntervalMinutes: 60,
        maxReminders: 3,
        notifyOnRecovery: true,
        enabled: true,
      },
    ]))

    renderPanel({
      currentUserActions: ['notification:read', 'notification:manage'],
    })

    const actionsButton = await screen.findByRole('button', { name: i18next.t('notificationsPage.rowActions', { name: 'Narrowed App Alert' }) })
    await user.click(actionsButton)
    await user.click(await screen.findByRole('menuitem', { name: i18next.t('common.edit') }))

    const dialog = screen.getByRole('dialog', { name: 'Notification rule' })
    expect(await within(dialog).findByText(i18next.t('errors.notification.rule_application_not_found'))).toBeInTheDocument()
    const saveButton = within(dialog).getByRole('button', { name: i18next.t('common.save') })
    expect(saveButton).toBeDisabled()

    const filterAppsTrigger = within(dialog).queryByRole('button', { name: i18next.t('notificationsPage.filterApplications') })
      ?? within(dialog).getByRole('button', { name: i18next.t('notificationsPage.filterApplicationsPlaceholder') })
    await user.click(filterAppsTrigger)
    await user.click(await screen.findByRole('button', { name: i18next.t('common.clearSelection') }))

    await waitFor(() => expect(saveButton).toBeEnabled())
    await user.click(saveButton)

    await waitFor(() => expect(mocks.updateProjectNotificationRule).toHaveBeenCalledWith(
      'prj_1',
      'nrl_sole_deleted_app',
      expect.objectContaining({
        name: 'Narrowed App Alert',
        channelIds: ['nch_feishu'],
        filter: {
          scope: 'projects',
          projectIds: ['prj_1'],
        },
      }),
    ))
  })

  it('renders loading skeletons and retryable ErrorState in the Bind Channel dialog instead of flashing noBindableChannels', async () => {
    const user = userEvent.setup()
    mocks.listScopedNotificationChannels.mockReturnValueOnce(new Promise(() => undefined))

    const { unmount } = renderPanel({
      currentUserActions: ['notification:read', 'notification:manage'],
      initialEntry: '/projects/prj_1?tab=notifications&subtab=channels',
    })

    await user.click(await screen.findByRole('button', { name: i18next.t('notificationsPage.bindExistingChannel') }))
    const pendingDialog = await screen.findByRole('dialog', { name: i18next.t('notificationsPage.bindChannelTitle') })
    expect(pendingDialog.querySelectorAll('[data-slot="skeleton"]').length).toBeGreaterThan(0)
    expect(within(pendingDialog).queryByText(i18next.t('notificationsPage.noBindableChannels'))).not.toBeInTheDocument()
    unmount()

    mocks.listScopedNotificationChannels
      .mockRejectedValueOnce(new Error('scoped channels failed'))
      .mockResolvedValueOnce(page([]))

    renderPanel({
      currentUserActions: ['notification:read', 'notification:manage'],
      initialEntry: '/projects/prj_1?tab=notifications&subtab=channels',
    })

    await user.click(await screen.findByRole('button', { name: i18next.t('notificationsPage.bindExistingChannel') }))
    const errorDialog = await screen.findByRole('dialog', { name: i18next.t('notificationsPage.bindChannelTitle') })
    expect(await within(errorDialog).findByText(i18next.t('notificationsPage.loadFailedTitle'))).toBeInTheDocument()
    expect(within(errorDialog).queryByText(i18next.t('notificationsPage.noBindableChannels'))).not.toBeInTheDocument()

    await user.click(within(errorDialog).getByRole('button', { name: i18next.t('common.retry') }))
    expect(await within(errorDialog).findByText(i18next.t('notificationsPage.noBindableChannels'))).toBeInTheDocument()
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
