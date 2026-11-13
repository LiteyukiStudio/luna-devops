import type { NotificationChannelMode } from './notification-models'
import type { NotificationChannel, NotificationChannelPayload } from '@/api'
import type { DataListColumn } from '@/components/common/data-list'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Bell, FlaskConical, MoreHorizontal, Pencil, Plus, Trash2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { useSession } from '@/app/session-context'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { DataList } from '@/components/common/data-list'
import { FormField as Field } from '@/components/common/form-field'
import { HelpTip } from '@/components/common/help-tip'
import { PageChromeHeading, PageChromeTools } from '@/components/common/page-chrome'
import { PageShell } from '@/components/common/page-shell'
import { StatusBadge, StatusValueBadge } from '@/components/common/status-badge'
import { formatSmartDateTime } from '@/components/common/time-format'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'
import { isPlatformAdmin } from '@/lib/roles'
import { NotificationChannelDialog } from './notification-channel-dialog'
import { NOTIFICATION_PAGE_SIZE } from './notification-models'

export function IntegrationNotificationChannelsPage() {
  const { t } = useTranslation()
  const { user } = useSession()
  const queryClient = useQueryClient()
  const canViewAll = isPlatformAdmin(user?.role)

  const [page, setPage] = useState(1)
  const [search, setSearch] = useState('')
  const [scopeFilter, setScopeFilter] = useState<'all' | 'project' | 'global'>('all')
  const [dialogState, setDialogState] = useState<{
    channel?: NotificationChannel
    mode: NotificationChannelMode
  } | null>(null)
  const [testTarget, setTestTarget] = useState<NotificationChannel | null>(null)
  const [testRecipient, setTestRecipient] = useState('')
  const [deleteTarget, setDeleteTarget] = useState<NotificationChannel | null>(null)

  const presets = useQuery({
    queryKey: ['scoped-notification-presets'],
    queryFn: api.listScopedNotificationPresets,
  })
  const projects = useQuery({
    queryKey: ['projects', 'options', canViewAll ? 'all' : 'member', 'notification-channels'],
    queryFn: () => api.listProjects(canViewAll ? 'all' : undefined),
  })
  const channels = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['scoped-notification-channels', page, scopeFilter, search],
    queryFn: () => api.listScopedNotificationChannels({
      page,
      pageSize: NOTIFICATION_PAGE_SIZE,
      scope: scopeFilter === 'all' ? undefined : scopeFilter,
      search: search.trim() || undefined,
      sortBy: 'createdAt',
      sortOrder: 'desc',
    }),
  })

  const projectNameById = useMemo(
    () => new Map((projects.data ?? []).map(project => [project.id, project.name])),
    [projects.data],
  )
  const presetNameById = useMemo(
    () => new Map((presets.data ?? []).map(preset => [preset.id, preset.name])),
    [presets.data],
  )

  const refreshAll = () => {
    void queryClient.invalidateQueries({ queryKey: ['scoped-notification-channels'] })
    void queryClient.invalidateQueries({ queryKey: ['project-notification-channels'] })
    void queryClient.invalidateQueries({ queryKey: ['notifications'] })
  }

  const saveChannel = useMutation({
    mutationFn: ({ channelId, payload }: { channelId?: string, payload: NotificationChannelPayload }) =>
      channelId
        ? api.updateScopedNotificationChannel(channelId, payload)
        : api.createScopedNotificationChannel(payload),
    onSuccess: () => {
      toast.success(t('notificationsPage.saved'))
      setDialogState(null)
      refreshAll()
    },
    onError: error => toast.error(error.message),
  })

  const deleteChannel = useMutation({
    mutationFn: api.deleteScopedNotificationChannel,
    onSuccess: () => {
      setDeleteTarget(null)
      refreshAll()
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
      refreshAll()
    },
    onError: error => toast.error(error.message),
  })

  const canCreate = canViewAll || (projects.data?.length ?? 0) > 0

  const columns = useMemo<DataListColumn<NotificationChannel>[]>(() => [
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
        const boundIds = item.projectIds ?? []
        const namedProjects = boundIds.map(id => projectNameById.get(id)).filter((name): name is string => Boolean(name))
        return (
          <div className="flex flex-wrap items-center gap-1.5">
            <StatusBadge tone="neutral">
              {namedProjects.length > 0
                ? namedProjects.slice(0, 2).join(', ') + (boundIds.length > 2 ? ` +${boundIds.length - 2}` : '')
                : t('notificationsPage.boundProjectsCount', { count: boundIds.length })}
            </StatusBadge>
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
    {
      key: 'actions',
      header: t('common.actions'),
      className: 'text-right',
      mobileActions: 'inline',
      sticky: 'right',
      width: 'actions',
      render: (item) => {
        const canEditOrDelete = item.canManage !== false
        const canTest = canEditOrDelete || item.scope === 'project'
        if (!canTest)
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
                <DropdownMenuItem
                  onSelect={() => {
                    setTestRecipient('')
                    setTestTarget(item)
                  }}
                >
                  <FlaskConical className="size-4" />
                  {t('notificationsPage.testChannel')}
                </DropdownMenuItem>
                {canEditOrDelete && (
                  <>
                    <DropdownMenuItem
                      onSelect={() => setDialogState({
                        channel: item,
                        mode: item.presetId ? 'preset' : (item.adapterKind === 'smtp' ? 'smtp' : 'webhook'),
                      })}
                    >
                      <Pencil className="size-4" />
                      {t('common.edit')}
                    </DropdownMenuItem>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem variant="destructive" onSelect={() => setDeleteTarget(item)}>
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
    },
  ], [presetNameById, projectNameById, t])

  const total = channels.data?.total ?? 0
  const pagination = total > 0
    ? {
        page: channels.data?.page ?? page,
        pageSize: channels.data?.pageSize ?? NOTIFICATION_PAGE_SIZE,
        total,
        totalPages: channels.data?.totalPages ?? 1,
        pageInfoLabel: t('notificationsPage.pageInfo', {
          page: channels.data?.page ?? page,
          totalPages: channels.data?.totalPages ?? 1,
          total,
        }),
        onPageChange: setPage,
      }
    : undefined

  return (
    <PageShell>
      <PageChromeHeading description={t('notificationsPage.channelsPageDescription')} title={t('notificationChannels')} />
      {canCreate && (
        <PageChromeTools>
          <Button type="button" variant="outline" onClick={() => setDialogState({ mode: 'preset' })}>
            <Bell className="size-4" />
            {t('notificationsPage.fromPreset')}
          </Button>
          <Button type="button" onClick={() => setDialogState({ mode: 'smtp' })}>
            <Plus className="size-4" />
            {t('notificationsPage.createChannel')}
          </Button>
        </PageChromeTools>
      )}

      <DataList
        columns={columns}
        emptyDescription={t('notificationsPage.emptyChannelsDescription')}
        emptyTitle={t('notificationsPage.emptyChannels')}
        items={channels.data?.items ?? []}
        loading={channels.isLoading}
        pagination={pagination}
        rowKey={item => item.id}
        search={{
          value: search,
          placeholder: t('notificationsPage.searchChannelsPlaceholder'),
          onChange: (next) => {
            setSearch(next)
            setPage(1)
          },
        }}
        toolbar={(
          <Select
            aria-label={t('notificationsPage.scope')}
            className="h-8 w-auto min-w-36 text-xs"
            value={scopeFilter}
            onChange={(event) => {
              setScopeFilter(event.target.value as 'all' | 'project' | 'global')
              setPage(1)
            }}
          >
            <option value="all">{t('notificationsPage.allScopes')}</option>
            <option value="project">{t('notificationsPage.scopeProject')}</option>
            <option value="global">{t('notificationsPage.scopeGlobal')}</option>
          </Select>
        )}
      />

      <NotificationChannelDialog
        allowGlobalScope={canViewAll}
        channel={dialogState?.channel}
        defaultMode={dialogState?.mode ?? 'preset'}
        defaultProjectIds={projects.data?.[0] ? [projects.data[0].id] : []}
        defaultScope="project"
        manageableProjects={projects.data ?? []}
        open={Boolean(dialogState)}
        presets={presets.data ?? []}
        saving={saveChannel.isPending}
        onClose={() => setDialogState(null)}
        onSave={(payload, channelId) => saveChannel.mutate({ channelId, payload })}
      />

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
        confirmText={t('common.delete')}
        description={t('notificationsPage.deleteDescription', { name: deleteTarget?.name ?? '' })}
        open={Boolean(deleteTarget)}
        pending={deleteChannel.isPending}
        title={t('notificationsPage.deleteChannelTitle')}
        onConfirm={() => deleteTarget && deleteChannel.mutate(deleteTarget.id)}
        onOpenChange={open => !open && setDeleteTarget(null)}
      />
    </PageShell>
  )
}
