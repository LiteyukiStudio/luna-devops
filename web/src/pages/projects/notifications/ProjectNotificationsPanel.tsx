import type { NotificationChannel, NotificationChannelPayload, NotificationDelivery, NotificationRule, NotificationRulePayload } from '@/api'
import type { DataListColumn } from '@/components/common/data-list'
import type { NotificationChannelMode } from '@/pages/notifications/notification-models'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Bell, FlaskConical, Info, Link2, MoreHorizontal, Pencil, Plus, RefreshCw, Trash2, Unlink, Webhook } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useSearchParams } from 'react-router-dom'
import { toast } from 'sonner'
import { api } from '@/api'
import { useSession } from '@/app/session-context'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { ContentTabs } from '@/components/common/content-tabs'
import { DataList } from '@/components/common/data-list'
import { EmptyState } from '@/components/common/empty-state'
import { FormField as Field } from '@/components/common/form-field'
import { HelpTip } from '@/components/common/help-tip'
import { StatusBadge, StatusValueBadge } from '@/components/common/status-badge'
import { formatSmartDateTime } from '@/components/common/time-format'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { TabsContent } from '@/components/ui/tabs'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'
import { isPlatformAdmin, ProjectAction, projectActionAllowed } from '@/lib/roles'
import { cn } from '@/lib/utils'
import { NotificationChannelDialog } from '@/pages/notifications/notification-channel-dialog'
import { NotificationDeliveriesPanel } from '@/pages/notifications/notification-deliveries-panel'
import {
  decodeStringList,
  NOTIFICATION_PAGE_SIZE,
  notificationEventLabel,
} from '@/pages/notifications/notification-models'
import { NotificationRuleDialog } from '@/pages/notifications/notification-rule-dialog'

type ProjectNotificationSubtab = 'rules' | 'channels' | 'deliveries'

export function ProjectNotificationsPanel({
  currentUserActions,
  projectId,
}: {
  currentUserActions?: string[]
  projectId: string
}) {
  const { t } = useTranslation()
  const { user } = useSession()
  const queryClient = useQueryClient()
  const [searchParams, setSearchParams] = useSearchParams()
  const isAdmin = isPlatformAdmin(user?.role)
  const canRead = isAdmin || projectActionAllowed(currentUserActions, ProjectAction.NotificationRead)
  const canManage = isAdmin || projectActionAllowed(currentUserActions, ProjectAction.NotificationManage)

  const eventIdFilter = searchParams.get('eventId') ?? ''
  const rawSubtab = searchParams.get('subtab')
  const activeTab: ProjectNotificationSubtab = rawSubtab === 'channels' || rawSubtab === 'deliveries' || rawSubtab === 'rules'
    ? rawSubtab
    : (eventIdFilter ? 'deliveries' : 'rules')

  const [rulePage, setRulePage] = useState(1)
  const [channelPage, setChannelPage] = useState(1)
  const [deliveryPage, setDeliveryPage] = useState(1)
  const [deliveryStatus, setDeliveryStatus] = useState('')
  const [deliveryEventType, setDeliveryEventType] = useState('')
  const [deliveryChannelId, setDeliveryChannelId] = useState('')

  const [channelDialog, setChannelDialog] = useState<{
    channel?: NotificationChannel
    mode: NotificationChannelMode
  } | null>(null)
  const [bindDialogOpen, setBindDialogOpen] = useState(false)
  const [selectedBindChannelId, setSelectedBindChannelId] = useState('')
  const [ruleDialog, setRuleDialog] = useState<{ rule?: NotificationRule } | null>(null)
  const [testTarget, setTestTarget] = useState<NotificationChannel | null>(null)
  const [testRecipient, setTestRecipient] = useState('')
  const [unbindTarget, setUnbindTarget] = useState<NotificationChannel | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<
    | { kind: 'channel', item: NotificationChannel }
    | { kind: 'rule', item: NotificationRule }
    | null
  >(null)

  const changeTab = (nextValue: string) => {
    const nextTab: ProjectNotificationSubtab = nextValue === 'channels' || nextValue === 'deliveries' ? nextValue : 'rules'
    setSearchParams((current) => {
      const next = new URLSearchParams(current)
      if (nextTab === 'rules' && !next.get('eventId'))
        next.delete('subtab')
      else
        next.set('subtab', nextTab)
      return next
    }, { replace: true })
  }

  const clearEventIdFilter = () => {
    setSearchParams((current) => {
      const next = new URLSearchParams(current)
      next.delete('eventId')
      return next
    }, { replace: true })
  }

  const presets = useQuery({
    queryKey: ['scoped-notification-presets'],
    queryFn: api.listScopedNotificationPresets,
    enabled: Boolean(projectId && canRead),
  })
  const projects = useQuery({
    queryKey: ['projects', 'options', isAdmin ? 'all' : 'member', 'project-notifications'],
    queryFn: () => api.listProjects(isAdmin ? 'all' : undefined),
    enabled: Boolean(projectId && canManage),
  })
  const rules = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['project-notification-rules', projectId, rulePage],
    queryFn: () => api.listProjectNotificationRules(projectId, {
      page: rulePage,
      pageSize: NOTIFICATION_PAGE_SIZE,
      sortBy: 'createdAt',
      sortOrder: 'desc',
    }),
    enabled: Boolean(projectId && canRead),
  })
  const channels = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['project-notification-channels', projectId, channelPage],
    queryFn: () => api.listProjectNotificationChannels(projectId, {
      page: channelPage,
      pageSize: NOTIFICATION_PAGE_SIZE,
      sortBy: 'createdAt',
      sortOrder: 'desc',
    }),
    enabled: Boolean(projectId && canRead),
  })
  const channelOptions = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['project-notification-channels', projectId, 'options'],
    queryFn: () => api.listProjectNotificationChannels(projectId, {
      page: 1,
      pageSize: 100,
      sortBy: 'name',
      sortOrder: 'asc',
    }),
    enabled: Boolean(projectId && canRead),
  })
  const bindableChannelsQuery = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['scoped-notification-channels', 'bindable', projectId],
    queryFn: () => api.listScopedNotificationChannels({
      page: 1,
      pageSize: 100,
      scope: 'project',
      sortBy: 'name',
      sortOrder: 'asc',
    }),
    enabled: Boolean(projectId && canManage && bindDialogOpen),
  })
  const applications = useQuery({
    queryKey: ['applications', projectId],
    queryFn: () => api.listApplications(projectId),
    enabled: Boolean(projectId && canManage && Boolean(ruleDialog)),
  })
  const deploymentTargets = useQuery({
    queryKey: ['project-deployment-targets', projectId, (applications.data ?? []).map(app => app.id).join(',')],
    queryFn: async () => {
      const lists = await Promise.all((applications.data ?? []).map(app => api.listDeploymentTargets(projectId, app.id)))
      return lists.flat()
    },
    enabled: Boolean(projectId && canManage && Boolean(ruleDialog) && (applications.data?.length ?? 0) > 0),
  })
  const deliveries = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['project-notification-deliveries', projectId, deliveryPage, deliveryStatus, deliveryEventType, deliveryChannelId, eventIdFilter],
    queryFn: () => api.listProjectNotificationDeliveries(projectId, {
      page: deliveryPage,
      pageSize: NOTIFICATION_PAGE_SIZE,
      status: deliveryStatus || undefined,
      eventType: deliveryEventType || undefined,
      channelId: deliveryChannelId || undefined,
      eventId: eventIdFilter || undefined,
      sortBy: 'createdAt',
      sortOrder: 'desc',
    }),
    enabled: Boolean(projectId && canRead && activeTab === 'deliveries'),
  })

  const bindableChannels = useMemo(
    () => (bindableChannelsQuery.data?.items ?? []).filter(
      channel => channel.canManage !== false && !(channel.projectIds ?? []).includes(projectId),
    ),
    [bindableChannelsQuery.data?.items, projectId],
  )
  const presetNameById = useMemo(
    () => new Map((presets.data ?? []).map(preset => [preset.id, preset.name])),
    [presets.data],
  )

  const refreshProjectNotifications = () => {
    void queryClient.invalidateQueries({ queryKey: ['project-notification-rules', projectId] })
    void queryClient.invalidateQueries({ queryKey: ['project-notification-channels', projectId] })
    void queryClient.invalidateQueries({ queryKey: ['project-notification-deliveries', projectId] })
    void queryClient.invalidateQueries({ queryKey: ['scoped-notification-channels'] })
  }

  const saveChannel = useMutation({
    mutationFn: ({ channelId, payload }: { channelId?: string, payload: NotificationChannelPayload }) =>
      channelId
        ? api.updateScopedNotificationChannel(channelId, payload)
        : api.createScopedNotificationChannel(payload),
    onSuccess: () => {
      toast.success(t('notificationsPage.saved'))
      setChannelDialog(null)
      refreshProjectNotifications()
    },
    onError: error => toast.error(error.message),
  })

  const bindChannel = useMutation({
    mutationFn: (channelId: string) => api.bindProjectNotificationChannel(projectId, channelId),
    onSuccess: () => {
      toast.success(t('notificationsPage.channelBound'))
      setBindDialogOpen(false)
      setSelectedBindChannelId('')
      refreshProjectNotifications()
    },
    onError: error => toast.error(error.message),
  })

  const unbindChannel = useMutation({
    mutationFn: (channelId: string) => api.unbindProjectNotificationChannel(projectId, channelId),
    onSuccess: () => {
      toast.success(t('notificationsPage.channelUnbound'))
      setUnbindTarget(null)
      refreshProjectNotifications()
    },
    onError: error => toast.error(error.message),
  })

  const deleteChannel = useMutation({
    mutationFn: api.deleteScopedNotificationChannel,
    onSuccess: () => {
      setDeleteTarget(null)
      refreshProjectNotifications()
    },
    onError: error => toast.error(error.message),
  })

  const testChannel = useMutation({
    mutationFn: ({ channelId, recipient }: { channelId: string, recipient: string }) =>
      api.testScopedNotificationChannel(channelId, recipient.trim() || undefined),
    onSuccess: () => {
      toast.success(t('notificationsPage.testSent'))
      setTestTarget(null)
      setTestRecipient('')
      refreshProjectNotifications()
    },
    onError: error => toast.error(error.message),
  })

  const saveRule = useMutation({
    mutationFn: ({ payload, ruleId }: { payload: NotificationRulePayload, ruleId?: string }) =>
      ruleId
        ? api.updateProjectNotificationRule(projectId, ruleId, payload)
        : api.createProjectNotificationRule(projectId, payload),
    onSuccess: () => {
      toast.success(t('notificationsPage.saved'))
      setRuleDialog(null)
      refreshProjectNotifications()
    },
    onError: error => toast.error(error.message),
  })

  const deleteRule = useMutation({
    mutationFn: (ruleId: string) => api.deleteProjectNotificationRule(projectId, ruleId),
    onSuccess: () => {
      setDeleteTarget(null)
      refreshProjectNotifications()
    },
    onError: error => toast.error(error.message),
  })

  const retryDelivery = useMutation({
    mutationFn: (delivery: NotificationDelivery) => api.retryProjectNotificationDelivery(projectId, delivery.id),
    onSuccess: () => {
      toast.success(t('notificationsPage.retryQueued'))
      void queryClient.invalidateQueries({ queryKey: ['project-notification-deliveries', projectId] })
    },
    onError: error => toast.error(error.message),
  })

  const ruleColumns = useMemo<DataListColumn<NotificationRule>[]>(() => {
    const cols: DataListColumn<NotificationRule>[] = [
      {
        key: 'name',
        header: t('notificationsPage.name'),
        width: 'primary',
        render: (item) => {
          const events = decodeStringList(item.eventTypesJson)
          const localizedPreview = events.slice(0, 3).map(ev => notificationEventLabel(ev, t)).join(' · ')
          return (
            <div className="min-w-0">
              <p className="truncate font-medium text-foreground">{item.name}</p>
              <p className="mt-0.5 truncate text-xs text-muted-foreground">
                {localizedPreview}
                {events.length > 3 ? ` +${events.length - 3}` : ''}
              </p>
            </div>
          )
        },
      },
      {
        key: 'throttling',
        header: t('notificationsPage.throttlingSectionTitle'),
        width: 'normal',
        render: (item) => {
          const max = item.maxReminders ?? 3
          const interval = item.remindIntervalMinutes ?? 60
          return (
            <div className="flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
              <span>
                {max === 0 || interval === 0
                  ? t('notificationsPage.throttleOnceSummary')
                  : t('notificationsPage.throttleRepeatSummary', { max, minutes: interval })}
              </span>
              {item.notifyOnRecovery !== false && (
                <StatusBadge tone="info">{t('notificationsPage.recoveryBadge')}</StatusBadge>
              )}
            </div>
          )
        },
      },
      {
        key: 'channels',
        header: t('notificationsPage.channels'),
        width: 'compact',
        render: item => decodeStringList(item.channelIdsJson).length,
      },
      {
        key: 'enabled',
        header: t('common.status'),
        width: 'status',
        render: item => (
          <StatusBadge tone={item.enabled ? 'success' : 'neutral'}>
            {item.enabled ? t('common.enabled') : t('common.disabled')}
          </StatusBadge>
        ),
      },
    ]

    if (canManage) {
      cols.push({
        key: 'actions',
        header: t('common.actions'),
        className: 'text-right',
        mobileActions: 'inline',
        sticky: 'right',
        width: 'actions',
        render: item => (
          <div className="flex justify-end">
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button aria-label={t('notificationsPage.rowActions', { name: item.name })} size="icon" variant="ghost">
                  <MoreHorizontal className="size-4" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem onSelect={() => setRuleDialog({ rule: item })}>
                  <Pencil className="size-4" />
                  {t('common.edit')}
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem variant="destructive" onSelect={() => setDeleteTarget({ item, kind: 'rule' })}>
                  <Trash2 className="size-4" />
                  {t('common.delete')}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        ),
      })
    }
    return cols
  }, [canManage, t])

  const channelColumns = useMemo<DataListColumn<NotificationChannel>[]>(() => {
    const cols: DataListColumn<NotificationChannel>[] = [
      {
        key: 'name',
        header: t('notificationsPage.name'),
        width: 'primary',
        render: (item) => {
          const sub = item.presetId
            ? (presetNameById.get(item.presetId) ?? item.presetId)
            : (item.adapterKind === 'smtp' ? t('notificationsPage.adapters.smtp') : t('notificationsPage.adapters.webhook'))
          return (
            <div className="min-w-0">
              <p className="truncate font-medium text-foreground">{item.name}</p>
              <p className="mt-0.5 truncate text-xs text-muted-foreground">{sub}</p>
            </div>
          )
        },
      },
      {
        key: 'scope',
        header: t('notificationsPage.scope'),
        width: 'normal',
        render: (item) => {
          if (item.scope === 'global')
            return <StatusBadge tone="info">{t('notificationsPage.scopeGlobal')}</StatusBadge>
          const count = item.projectIds?.length ?? 1
          return (
            <div className="flex flex-wrap items-center gap-1.5">
              <StatusBadge tone="neutral">{t('notificationsPage.boundProjectsCount', { count })}</StatusBadge>
              {item.canManage === false && (
                <span className="inline-flex items-center gap-1">
                  <StatusBadge tone="warning">{t('notificationsPage.sharedReadOnlyBadge')}</StatusBadge>
                  <HelpTip label={t('notificationsPage.sharedReadOnlyBadge')}>
                    {t('notificationsPage.sharedReadOnlyHint')}
                  </HelpTip>
                </span>
              )}
            </div>
          )
        },
      },
      {
        key: 'enabled',
        header: t('common.status'),
        width: 'status',
        render: item => (
          <StatusBadge tone={item.enabled ? 'success' : 'neutral'}>
            {item.enabled ? t('common.enabled') : t('common.disabled')}
          </StatusBadge>
        ),
      },
      {
        key: 'lastDeliveryStatus',
        header: t('notificationsPage.lastDelivery'),
        width: 'normal',
        render: item => item.lastDeliveryStatus
          ? (
              <div className="min-w-0">
                <StatusValueBadge value={item.lastDeliveryStatus} />
                {item.lastDeliveredAt && (
                  <p className="mt-0.5 truncate text-xs text-muted-foreground">
                    {formatSmartDateTime(item.lastDeliveredAt, t)}
                  </p>
                )}
              </div>
            )
          : '-',
      },
    ]

    if (canManage) {
      cols.push({
        key: 'actions',
        header: t('common.actions'),
        className: 'text-right',
        mobileActions: 'inline',
        sticky: 'right',
        width: 'actions',
        render: (item) => {
          const isProjectScope = item.scope !== 'global'
          const canEditChannel = item.canManage !== false && (isProjectScope || isAdmin)
          const canTestChannel = isProjectScope || isAdmin
          if (!canTestChannel && !isProjectScope)
            return null
          return (
            <div className="flex justify-end">
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button aria-label={t('notificationsPage.rowActions', { name: item.name })} size="icon" variant="ghost">
                    <MoreHorizontal className="size-4" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  {canTestChannel && (
                    <DropdownMenuItem
                      onSelect={() => {
                        setTestRecipient('')
                        setTestTarget(item)
                      }}
                    >
                      <FlaskConical className="size-4" />
                      {t('notificationsPage.testChannel')}
                    </DropdownMenuItem>
                  )}
                  {canEditChannel && (
                    <DropdownMenuItem
                      onSelect={() => setChannelDialog({
                        channel: item,
                        mode: item.presetId ? 'preset' : (item.adapterKind === 'smtp' ? 'smtp' : 'webhook'),
                      })}
                    >
                      <Pencil className="size-4" />
                      {t('common.edit')}
                    </DropdownMenuItem>
                  )}
                  {isProjectScope && (item.projectIds?.length ?? 1) > 1 && (
                    <DropdownMenuItem onSelect={() => setUnbindTarget(item)}>
                      <Unlink className="size-4" />
                      {t('notificationsPage.unbindChannel')}
                    </DropdownMenuItem>
                  )}
                  {canEditChannel && (
                    <>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem variant="destructive" onSelect={() => setDeleteTarget({ item, kind: 'channel' })}>
                        <Trash2 className="size-4" />
                        {t('common.delete')}
                      </DropdownMenuItem>
                    </>
                  )}
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          )
        },
      })
    }
    return cols
  }, [canManage, isAdmin, presetNameById, t])

  if (!canRead) {
    return (
      <EmptyState
        description={t('notificationsPage.projectReadOnlyHint')}
        icon={<Bell className="size-5" />}
        title={t('notificationsPage.title')}
      />
    )
  }

  const hasAvailableChannels = (channelOptions.data?.items.length ?? 0) > 0

  const tools = (
    <>
      {canManage && activeTab === 'rules' && (
        <Button type="button" onClick={() => setRuleDialog({})}>
          <Plus className="size-4" />
          {t('notificationsPage.createRule')}
        </Button>
      )}
      {canManage && activeTab === 'channels' && (
        <>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              setSelectedBindChannelId('')
              setBindDialogOpen(true)
            }}
          >
            <Link2 className="size-4" />
            {t('notificationsPage.bindExistingChannel')}
          </Button>
          <Button type="button" onClick={() => setChannelDialog({ mode: 'preset' })}>
            <Plus className="size-4" />
            {t('notificationsPage.createChannel')}
          </Button>
        </>
      )}
      {activeTab === 'deliveries' && (
        <Button
          disabled={deliveries.isFetching}
          type="button"
          variant="outline"
          onClick={() => void deliveries.refetch()}
        >
          <RefreshCw className={cn('size-4', deliveries.isFetching && 'animate-spin')} />
          {t('common.refresh')}
        </Button>
      )}
    </>
  )

  return (
    <div className="grid gap-4">
      {!canManage && (
        <div
          className="flex items-center gap-2.5 rounded-control border border-border bg-surface-subtle/60 px-3.5 py-2.5 text-xs text-muted-foreground"
          role="status"
        >
          <Info className="size-4 shrink-0 text-muted-foreground" />
          <span>{t('notificationsPage.projectReadOnlyHint')}</span>
        </div>
      )}

      <ContentTabs
        hashRouting={false}
        tabs={[
          { label: t('notificationsPage.rules'), value: 'rules' },
          { label: t('notificationsPage.channels'), value: 'channels' },
          { label: t('notificationsPage.deliveries'), value: 'deliveries' },
        ]}
        tools={tools}
        value={activeTab}
        onValueChange={changeTab}
      >
        <TabsContent value="rules">
          <DataList
            columns={ruleColumns}
            emptyActions={canManage && !channelOptions.isLoading && !hasAvailableChannels
              ? (
                  <Button size="sm" type="button" variant="outline" onClick={() => changeTab('channels')}>
                    {t('notificationsPage.setupChannelFirst')}
                  </Button>
                )
              : undefined}
            emptyDescription={t('notificationsPage.emptyRulesDescription')}
            emptyIcon={<Bell className="size-5" />}
            emptyTitle={t('notificationsPage.emptyRules')}
            error={rules.isError
              ? {
                  description: t('notificationsPage.loadFailedDescription'),
                  retrying: rules.isFetching,
                  title: t('notificationsPage.loadFailedTitle'),
                  onRetry: () => void rules.refetch(),
                }
              : undefined}
            items={rules.data?.items ?? []}
            loading={rules.isLoading}
            pagination={pageFor(rules.data, rulePage, setRulePage, t)}
            rowKey={item => item.id}
          />
        </TabsContent>

        <TabsContent value="channels">
          <DataList
            columns={channelColumns}
            emptyDescription={t('notificationsPage.emptyChannelsDescription')}
            emptyIcon={<Webhook className="size-5" />}
            emptyTitle={t('notificationsPage.emptyChannels')}
            error={channels.isError
              ? {
                  description: t('notificationsPage.loadFailedDescription'),
                  retrying: channels.isFetching,
                  title: t('notificationsPage.loadFailedTitle'),
                  onRetry: () => void channels.refetch(),
                }
              : undefined}
            items={channels.data?.items ?? []}
            loading={channels.isLoading}
            pagination={pageFor(channels.data, channelPage, setChannelPage, t)}
            rowKey={item => item.id}
          />
        </TabsContent>

        <TabsContent value="deliveries">
          <NotificationDeliveriesPanel
            canRetry={canManage}
            channelFilter={deliveryChannelId}
            channels={channelOptions.data?.items ?? []}
            data={deliveries.data}
            error={deliveries.isError}
            eventIdFilter={eventIdFilter}
            eventTypeFilter={deliveryEventType}
            loading={deliveries.isLoading}
            page={deliveryPage}
            retrying={retryDelivery.isPending}
            retryingLoad={deliveries.isFetching}
            statusFilter={deliveryStatus}
            onChannelFilterChange={setDeliveryChannelId}
            onClearEventIdFilter={clearEventIdFilter}
            onEventTypeFilterChange={setDeliveryEventType}
            onPageChange={setDeliveryPage}
            onRetry={delivery => retryDelivery.mutate(delivery)}
            onRetryLoad={() => void deliveries.refetch()}
            onStatusFilterChange={setDeliveryStatus}
          />
        </TabsContent>
      </ContentTabs>

      <NotificationChannelDialog
        allowGlobalScope={false}
        channel={channelDialog?.channel}
        defaultMode={channelDialog?.mode ?? 'preset'}
        defaultProjectIds={[projectId]}
        defaultScope="project"
        manageableProjects={projects.data ?? []}
        open={Boolean(channelDialog)}
        presets={presets.data ?? []}
        saving={saveChannel.isPending}
        onClose={() => setChannelDialog(null)}
        onSave={(payload, channelId) => saveChannel.mutate({ channelId, payload })}
      />

      <NotificationRuleDialog
        applications={applications.data ?? []}
        channels={channelOptions.data?.items ?? []}
        contextMode="project"
        deploymentTargets={deploymentTargets.data ?? []}
        fixedProjectId={projectId}
        open={Boolean(ruleDialog)}
        rule={ruleDialog?.rule}
        saving={saveRule.isPending}
        onClose={() => setRuleDialog(null)}
        onSave={(payload, ruleId) => saveRule.mutate({ payload, ruleId })}
      />

      <Dialog open={bindDialogOpen} onOpenChange={setBindDialogOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t('notificationsPage.bindChannelTitle')}</DialogTitle>
            <DialogDescription>{t('notificationsPage.bindChannelDescription')}</DialogDescription>
          </DialogHeader>
          {bindableChannels.length === 0
            ? (
                <p className="py-2 text-sm text-muted-foreground">
                  {t('notificationsPage.noBindableChannels')}
                </p>
              )
            : (
                <Field label={t('notificationsPage.selectChannelToBind')} required>
                  <Select
                    value={selectedBindChannelId || bindableChannels[0]?.id || ''}
                    onChange={event => setSelectedBindChannelId(event.target.value)}
                  >
                    {bindableChannels.map(channel => (
                      <option key={channel.id} value={channel.id}>
                        {`${channel.name} (${channel.adapterKind === 'smtp' ? t('notificationsPage.adapters.smtp') : t('notificationsPage.adapters.webhook')})`}
                      </option>
                    ))}
                  </Select>
                </Field>
              )}
          <DialogFooter>
            <Button disabled={bindChannel.isPending} type="button" variant="outline" onClick={() => setBindDialogOpen(false)}>
              {t('common.cancel')}
            </Button>
            <Button
              disabled={bindChannel.isPending || bindableChannels.length === 0}
              type="button"
              onClick={() => {
                const targetId = selectedBindChannelId || bindableChannels[0]?.id
                if (targetId)
                  bindChannel.mutate(targetId)
              }}
            >
              {t('notificationsPage.bindChannelAction')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        cancelText={t('common.cancel')}
        confirmText={t('notificationsPage.sendTest')}
        confirmVariant="default"
        content={testTarget?.adapterKind === 'smtp'
          ? (
              <Field label={t('notificationsPage.testRecipientLabel')}>
                <Input
                  placeholder={t('notificationsPage.testRecipientPlaceholder')}
                  type="email"
                  value={testRecipient}
                  onChange={event => setTestRecipient(event.target.value)}
                />
              </Field>
            )
          : undefined}
        description={t('notificationsPage.testChannelDescription', { name: testTarget?.name ?? '' })}
        open={Boolean(testTarget)}
        pending={testChannel.isPending}
        title={t('notificationsPage.testChannelTitle')}
        onConfirm={() => testTarget && testChannel.mutate({ channelId: testTarget.id, recipient: testRecipient })}
        onOpenChange={(open) => {
          if (!open) {
            setTestTarget(null)
            setTestRecipient('')
          }
        }}
      />

      <ConfirmDialog
        cancelText={t('common.cancel')}
        closeOnConfirm={false}
        confirmText={t('notificationsPage.unbindChannel')}
        description={t('notificationsPage.unbindChannelDescription', { name: unbindTarget?.name ?? '' })}
        open={Boolean(unbindTarget)}
        pending={unbindChannel.isPending}
        title={t('notificationsPage.unbindChannelTitle')}
        onConfirm={() => unbindTarget && unbindChannel.mutate(unbindTarget.id)}
        onOpenChange={open => !open && setUnbindTarget(null)}
      />

      <ConfirmDialog
        cancelText={t('common.cancel')}
        closeOnConfirm={false}
        confirmText={t('common.delete')}
        description={t('notificationsPage.deleteDescription', { name: deleteTarget?.item.name ?? '' })}
        open={Boolean(deleteTarget)}
        pending={deleteChannel.isPending || deleteRule.isPending}
        title={deleteTarget?.kind === 'rule' ? t('notificationsPage.deleteRuleTitle') : t('notificationsPage.deleteChannelTitle')}
        onConfirm={() => {
          if (deleteTarget?.kind === 'channel')
            deleteChannel.mutate(deleteTarget.item.id)
          else if (deleteTarget?.kind === 'rule')
            deleteRule.mutate(deleteTarget.item.id)
        }}
        onOpenChange={open => !open && setDeleteTarget(null)}
      />
    </div>
  )
}

function pageFor(
  data: { page?: number, pageSize?: number, total?: number, totalPages?: number } | undefined,
  page: number,
  onPageChange: (page: number) => void,
  t: ReturnType<typeof useTranslation>['t'],
) {
  const total = data?.total ?? 0
  if (total === 0)
    return undefined
  return {
    page: data?.page ?? page,
    pageSize: data?.pageSize ?? NOTIFICATION_PAGE_SIZE,
    total,
    totalPages: data?.totalPages ?? 1,
    pageInfoLabel: t('notificationsPage.pageInfo', {
      page: data?.page ?? page,
      totalPages: data?.totalPages ?? 1,
      total,
    }),
    onPageChange,
  }
}
