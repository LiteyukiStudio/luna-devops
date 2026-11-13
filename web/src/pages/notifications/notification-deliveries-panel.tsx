import type { NotificationChannel, NotificationDelivery, PaginatedResponse } from '@/api'
import type { DataListColumn } from '@/components/common/data-list'
import { RotateCw, X } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { DataList } from '@/components/common/data-list'
import { StatusBadge, StatusValueBadge } from '@/components/common/status-badge'
import { formatSmartDateTime } from '@/components/common/time-format'
import { Button } from '@/components/ui/button'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { NotificationDeliveryDetailSheet } from './notification-delivery-detail-sheet'
import {
  NOTIFICATION_EVENT_GROUPS,
  NOTIFICATION_PAGE_SIZE,
  notificationEventGroupLabel,
  notificationEventLabel,
} from './notification-models'

export function NotificationDeliveriesPanel({
  canRetry = false,
  channelFilter,
  channels = [],
  data,
  eventIdFilter = '',
  eventTypeFilter,
  framed = true,
  loading = false,
  page,
  retrying = false,
  statusFilter,
  onChannelFilterChange,
  onClearEventIdFilter,
  onEventTypeFilterChange,
  onPageChange,
  onRetry,
  onStatusFilterChange,
}: {
  canRetry?: boolean
  channelFilter: string
  channels?: NotificationChannel[]
  data?: PaginatedResponse<NotificationDelivery>
  eventIdFilter?: string
  eventTypeFilter: string
  framed?: boolean
  loading?: boolean
  page: number
  retrying?: boolean
  statusFilter: string
  onChannelFilterChange: (channelId: string) => void
  onClearEventIdFilter?: () => void
  onEventTypeFilterChange: (eventType: string) => void
  onPageChange: (page: number) => void
  onRetry?: (delivery: NotificationDelivery) => void
  onStatusFilterChange: (status: string) => void
}) {
  const { t } = useTranslation()
  const [selectedDeliveryId, setSelectedDeliveryId] = useState<string | null>(null)
  const items = useMemo(() => data?.items ?? [], [data?.items])
  const selectedDelivery = useMemo(
    () => items.find(item => item.id === selectedDeliveryId) ?? null,
    [items, selectedDeliveryId],
  )
  const hasActiveFilters = Boolean(statusFilter || eventTypeFilter || channelFilter || eventIdFilter)

  const clearAllFilters = () => {
    onStatusFilterChange('')
    onEventTypeFilterChange('')
    onChannelFilterChange('')
    onClearEventIdFilter?.()
    onPageChange(1)
  }

  const columns = useMemo<DataListColumn<NotificationDelivery>[]>(() => {
    const base: DataListColumn<NotificationDelivery>[] = [
      {
        key: 'eventType',
        header: t('notificationsPage.eventType'),
        width: 'primary',
        render: item => (
          <div className="min-w-0">
            <p className="truncate font-medium text-foreground">{notificationEventLabel(item.eventType, t)}</p>
            <p className="mt-0.5 truncate text-xs text-muted-foreground">
              {item.ruleName || item.eventId}
            </p>
          </div>
        ),
      },
      {
        key: 'targetChannel',
        header: t('notificationsPage.targetChannel'),
        width: 'normal',
        render: item => (
          <div className="min-w-0">
            <p className="truncate text-sm font-medium">{item.channelName || item.channelId || '-'}</p>
            <p className="mt-0.5 truncate text-xs text-muted-foreground">{item.recipientSummary || '-'}</p>
          </div>
        ),
      },
      {
        key: 'status',
        header: t('common.status'),
        width: 'status',
        render: item => <StatusValueBadge value={item.status} />,
      },
      {
        key: 'attemptCount',
        header: t('notificationsPage.attempts'),
        width: 'compact',
        render: item => (
          <span className="tabular-nums text-xs text-muted-foreground">
            {t('notificationsPage.attemptsOfMax', { count: item.attemptCount })}
          </span>
        ),
      },
      {
        key: 'queuedAndDuration',
        header: t('notificationsPage.queuedAndDuration'),
        width: 'compact',
        render: item => (
          <div className="min-w-0">
            <p className="truncate text-xs text-foreground">
              {formatSmartDateTime(item.finishedAt ?? item.queuedAt ?? item.createdAt, t)}
            </p>
            <p className="mt-0.5 truncate tabular-nums text-xs text-muted-foreground">
              {item.durationMillis > 0 ? t('notificationsPage.durationMs', { ms: item.durationMillis }) : '-'}
            </p>
          </div>
        ),
      },
      {
        key: 'errorMessage',
        header: t('notificationsPage.error'),
        width: 'normal',
        render: item => item.errorMessage
          ? <span className="block max-w-60 truncate text-xs text-danger" title={item.errorMessage}>{item.errorMessage}</span>
          : <span className="text-xs text-muted-foreground">-</span>,
      },
    ]

    if (canRetry && onRetry) {
      base.push({
        key: 'actions',
        header: t('common.actions'),
        className: 'text-right',
        mobileActions: 'inline',
        sticky: 'right',
        width: 'actions',
        render: (item) => {
          const isActive = item.status === 'pending' || item.status === 'sending'
          return (
            <div className="flex justify-end">
              <Button
                aria-label={t('notificationsPage.retryDelivery')}
                disabled={retrying || isActive}
                size="sm"
                title={t('notificationsPage.retryDelivery')}
                type="button"
                variant="ghost"
                onClick={(event) => {
                  event.stopPropagation()
                  onRetry(item)
                }}
              >
                <RotateCw className="size-3.5" />
                <span className="sr-only sm:not-sr-only">{t('notificationsPage.retryDelivery')}</span>
              </Button>
            </div>
          )
        },
      })
    }
    return base
  }, [canRetry, onRetry, retrying, t])

  const total = data?.total ?? 0
  const pagination = total > 0
    ? {
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
    : undefined

  return (
    <>
      <DataList
        columns={columns}
        emptyActions={hasActiveFilters
          ? (
              <Button size="sm" type="button" variant="outline" onClick={clearAllFilters}>
                {t('notificationsPage.clearFilters')}
              </Button>
            )
          : undefined}
        emptyMode={hasActiveFilters ? 'filtered' : 'actionable'}
        emptyTitle={hasActiveFilters ? t('notificationsPage.emptyFilteredDeliveries') : t('notificationsPage.emptyDeliveries')}
        framed={framed}
        items={items}
        loading={loading}
        pagination={pagination}
        rowKey={item => item.id}
        toolbar={(
          <div className="flex flex-wrap items-center gap-2">
            <Select
              aria-label={t('common.status')}
              className="h-8 w-auto min-w-32 text-xs"
              value={statusFilter}
              onChange={(event) => {
                onStatusFilterChange(event.target.value)
                onPageChange(1)
              }}
            >
              <option value="">{t('notificationsPage.allStatuses')}</option>
              <option value="pending">{t('notificationsPage.deliveryStatus.pending')}</option>
              <option value="sending">{t('notificationsPage.deliveryStatus.sending')}</option>
              <option value="delivered">{t('notificationsPage.deliveryStatus.delivered')}</option>
              <option value="failed">{t('notificationsPage.deliveryStatus.failed')}</option>
            </Select>

            <Select
              aria-label={t('notificationsPage.eventType')}
              className="h-8 w-auto min-w-44 text-xs"
              value={eventTypeFilter}
              onChange={(event) => {
                onEventTypeFilterChange(event.target.value)
                onPageChange(1)
              }}
            >
              <option value="">{t('notificationsPage.allEventTypes')}</option>
              {NOTIFICATION_EVENT_GROUPS.map(group => (
                <optgroup key={group.key} label={notificationEventGroupLabel(group.key, t)}>
                  {group.events.map(eventType => (
                    <option key={eventType} value={eventType}>
                      {notificationEventLabel(eventType, t)}
                    </option>
                  ))}
                </optgroup>
              ))}
            </Select>

            {channels.length > 0 && (
              <Select
                aria-label={t('notificationsPage.channels')}
                className="h-8 w-auto min-w-40 text-xs"
                value={channelFilter}
                onChange={(event) => {
                  onChannelFilterChange(event.target.value)
                  onPageChange(1)
                }}
              >
                <option value="">{t('notificationsPage.allChannels')}</option>
                {channels.map(channel => (
                  <option key={channel.id} value={channel.id}>{channel.name}</option>
                ))}
              </Select>
            )}

            {eventIdFilter && (
              <StatusBadge tone="info">
                <span className="inline-flex items-center gap-1">
                  {t('notificationsPage.filteredByEvent', { eventId: eventIdFilter })}
                  {onClearEventIdFilter && (
                    <button
                      aria-label={t('notificationsPage.clearFilters')}
                      className="rounded-sm p-0.5 hover:bg-primary-subtle-hover"
                      type="button"
                      onClick={() => {
                        onClearEventIdFilter()
                        onPageChange(1)
                      }}
                    >
                      <X className="size-3" />
                    </button>
                  )}
                </span>
              </StatusBadge>
            )}

            {hasActiveFilters && (
              <Button className="h-8 text-xs" size="sm" type="button" variant="ghost" onClick={clearAllFilters}>
                {t('notificationsPage.clearFilters')}
              </Button>
            )}
          </div>
        )}
        onRowClick={item => setSelectedDeliveryId(item.id)}
      />

      <NotificationDeliveryDetailSheet
        canRetry={canRetry}
        delivery={selectedDelivery}
        retrying={retrying}
        onClose={() => setSelectedDeliveryId(null)}
        onRetry={onRetry}
      />
    </>
  )
}
