import type { NotificationChannel, NotificationPreset, NotificationRule, Project } from '@/api'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { NotificationChannelDialog } from './notification-channel-dialog'
import { NotificationRuleDialog } from './notification-rule-dialog'

const feishuPreset: NotificationPreset = {
  id: 'feishu-bot',
  name: 'Feishu Bot',
  description: 'Feishu group webhook',
  adapterKind: 'webhook',
  configTemplate: '{}',
  jsonBodyTemplate: '{}',
  secretFields: ['webhookUrl'],
}

const dummyChannel: NotificationChannel = {
  id: 'nch_1',
  name: 'Ops Webhook',
  scope: 'global',
  projectId: '',
  projectIds: [],
  ownerUserId: '',
  adapterKind: 'webhook',
  configJson: '{}',
  secretSet: {},
  enabled: true,
  canManage: true,
  lastDeliveryStatus: '',
  lastDeliveryError: '',
  createdBy: 'usr_1',
  createdAt: '2026-11-01T00:00:00Z',
  updatedAt: '2026-11-01T00:00:00Z',
}

describe('notification dialogs loading & error safety', () => {
  it('preserves user-typed inputs in NotificationChannelDialog when presets finish loading', async () => {
    const user = userEvent.setup()
    const queryClient = new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })
    const { rerender } = render(
      <QueryClientProvider client={queryClient}>
        <TooltipProvider>
          <NotificationChannelDialog
            allowGlobalScope
            defaultMode="webhook"
            defaultScope="global"
            manageableProjects={[]}
            open
            presets={[]}
            presetsLoading
            saving={false}
            onClose={vi.fn()}
            onSave={vi.fn()}
          />
        </TooltipProvider>
      </QueryClientProvider>,
    )

    const nameInput = screen.getByLabelText(new RegExp(i18next.t('notificationsPage.name')))
    await user.type(nameInput, 'My Alert Channel')
    expect(nameInput).toHaveValue('My Alert Channel')

    rerender(
      <QueryClientProvider client={queryClient}>
        <TooltipProvider>
          <NotificationChannelDialog
            allowGlobalScope
            defaultMode="webhook"
            defaultScope="global"
            manageableProjects={[]}
            open
            presets={[feishuPreset]}
            presetsLoading={false}
            saving={false}
            onClose={vi.fn()}
            onSave={vi.fn()}
          />
        </TooltipProvider>
      </QueryClientProvider>,
    )

    expect(screen.getByLabelText(new RegExp(i18next.t('notificationsPage.name')))).toHaveValue('My Alert Channel')
  })

  it('allows clearing auto-populated defaultProjectIds after projects finish loading', async () => {
    const user = userEvent.setup()
    const queryClient = new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })
    const projectAlpha: Project = {
      id: 'prj_alpha',
      identifier: 'alpha',
      kubernetesNamespace: 'alpha',
      name: 'Alpha Space',
      description: '',
      maxConcurrentBuilds: 2,
      webConsoleEnabled: true,
      billingOwnerUserId: 'usr_1',
      systemKey: '',
      deleteStatus: 'active',
      deleteMessage: '',
      dashboardOrder: 0,
      useCount: 0,
      createdAt: '2026-11-01T00:00:00Z',
      updatedAt: '2026-11-01T00:00:00Z',
    }

    const { rerender } = render(
      <QueryClientProvider client={queryClient}>
        <TooltipProvider>
          <NotificationChannelDialog
            defaultMode="webhook"
            defaultProjectIds={[]}
            defaultScope="project"
            manageableProjects={[]}
            open
            presets={[feishuPreset]}
            projectsLoading
            saving={false}
            onClose={vi.fn()}
            onSave={vi.fn()}
          />
        </TooltipProvider>
      </QueryClientProvider>,
    )

    rerender(
      <QueryClientProvider client={queryClient}>
        <TooltipProvider>
          <NotificationChannelDialog
            defaultMode="webhook"
            defaultProjectIds={[projectAlpha.id]}
            defaultScope="project"
            manageableProjects={[projectAlpha]}
            open
            presets={[feishuPreset]}
            projectsLoading={false}
            saving={false}
            onClose={vi.fn()}
            onSave={vi.fn()}
          />
        </TooltipProvider>
      </QueryClientProvider>,
    )

    const projectsButton = screen.getByLabelText(new RegExp(i18next.t('notificationsPage.boundProjects')), {
      selector: '[data-slot="popover-trigger"]',
    })
    expect(projectsButton).toHaveTextContent('Alpha Space')

    await user.click(projectsButton)
    await user.click(screen.getByRole('button', { name: i18next.t('common.clearSelection') }))
    expect(projectsButton).toHaveTextContent(i18next.t('projectSpaces.selectProjects'))
  })

  it('shows unavailable + retry and blocks saving in NotificationRuleDialog when project or template queries fail', async () => {
    const user = userEvent.setup()
    const onRetryProjects = vi.fn()
    const onRetryTemplates = vi.fn()
    const existingRule: NotificationRule = {
      id: 'nrl_1',
      projectId: '',
      name: 'Critical Alerts',
      eventTypesJson: JSON.stringify(['build.failed']),
      filterJson: JSON.stringify({ scope: 'projects', projectIds: ['prj_1'] }),
      channelIdsJson: JSON.stringify(['nch_1']),
      templateId: 'ntpl_custom',
      locale: 'zh-CN',
      remindIntervalMinutes: 0,
      maxReminders: 0,
      notifyOnRecovery: false,
      timeZone: 'Asia/Shanghai',
      enabled: true,
      createdBy: 'usr_1',
      createdAt: '2026-11-01T00:00:00Z',
      updatedAt: '2026-11-01T00:00:00Z',
    }

    render(
      <TooltipProvider>
        <NotificationRuleDialog
          channels={[dummyChannel]}
          contextMode="global"
          open
          projects={undefined}
          projectsError
          rule={existingRule}
          saving={false}
          templates={undefined}
          templatesError
          onClose={vi.fn()}
          onRetryProjects={onRetryProjects}
          onRetryTemplates={onRetryTemplates}
          onSave={vi.fn()}
        />
      </TooltipProvider>,
    )

    expect(screen.queryByText(i18next.t('notificationsPage.ruleProjectsRequired'))).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: i18next.t('common.save') })).toBeDisabled()

    const retryButtons = screen.getAllByRole('button', { name: i18next.t('common.retry') })
    expect(retryButtons).toHaveLength(2)
    await user.click(retryButtons[0]!)
    await user.click(retryButtons[1]!)
    expect(onRetryTemplates).toHaveBeenCalledOnce()
    expect(onRetryProjects).toHaveBeenCalledOnce()
  })
})
