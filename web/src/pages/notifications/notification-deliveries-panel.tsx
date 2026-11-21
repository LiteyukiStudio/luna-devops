import type { NotificationChannel, NotificationDelivery, PaginatedResponse } from '@/api'
import type { DataListColumn } from '@/components/common/data-list'
import { RotateCw, SearchX, Send, X } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { DataList } from '@/components/common/data-list'
import { StatusBadge, StatusValueBadge } from '@/components/common/status-badge'
import { formatSmartDateTime } from '@/components/common/time-format'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectGroup, SelectItem, SelectLabel, SelectTrigger, SelectValue } from '@/components/ui/select'
import { EMPTY_SELECT_VALUE, fromSelectValue, toSelectValue } from '@/components/ui/select-value'
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
  error = false,
  eventIdFilter = '',
  eventTypeFilter,
  framed = true,
  loading = false,
  page,
  retrying = false,
  retryingLoad = false,
  statusFilter,
  onChannelFilterChange,
  onClearEventIdFilter,
  onEventTypeFilterChange,
  onPageChange,
  onRetry,
  onRetryLoad,
  onStatusFilterChange,
}: {
  canRetry?: boolean
  channelFilter: string
  channels?: NotificationChannel[]
  data?: PaginatedResponse<NotificationDelivery>
  error?: boolean
  eventIdFilter?: string
  eventTypeFilter: string
  framed?: boolean
  loading?: boolean
  page: number
  retrying?: boolean
  retryingLoad?: boolean
  statusFilter: string
  onChannelFilterChange: (channelId: string) => void
  onClearEventIdFilter?: () => void
  onEventTypeFilterChange: (eventType: string) => void
  onPageChange: (page: number) => void
  onRetry?: (delivery: NotificationDelivery) => void
  onRetryLoad?: () => void
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
        emptyDescription={hasActiveFilters ? t('notificationsPage.emptyFilteredDeliveriesDescription') : t('notificationsPage.emptyDeliveriesDescription')}
        emptyIcon={hasActiveFilters ? <SearchX className="size-5" /> : <Send className="size-5" />}
        emptyMode={hasActiveFilters ? 'filtered' : 'actionable'}
        emptyTitle={hasActiveFilters ? t('notificationsPage.emptyFilteredDeliveries') : t('notificationsPage.emptyDeliveries')}
        error={error
          ? {
              description: t('notificationsPage.loadFailedDescription'),
              retrying: retryingLoad,
              title: t('notificationsPage.loadFailedTitle'),
              onRetry: onRetryLoad,
            }
          : undefined}
        framed={framed}
        items={items}
        loading={loading}
        pagination={pagination}
        rowActionLabel={() => t('notificationsPage.deliveryDetailTitle')}
        rowKey={item => item.id}
        toolbar={(
          <>
            <Select
              value={toSelectValue(statusFilter)}
              onValueChange={(next) => {
                onStatusFilterChange(fromSelectValue(next))
                onPageChange(1)
              }}
            >
              <SelectTrigger aria-label={t('common.status')} className="w-full text-xs @lg/data-list:w-36">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={EMPTY_SELECT_VALUE}>{t('notificationsPage.allStatuses')}</SelectItem>
                <SelectItem value="pending">{t('notificationsPage.deliveryStatus.pending')}</SelectItem>
                <SelectItem value="sending">{t('notificationsPage.deliveryStatus.sending')}</SelectItem>
                <SelectItem value="delivered">{t('notificationsPage.deliveryStatus.delivered')}</SelectItem>
                <SelectItem value="failed">{t('notificationsPage.deliveryStatus.failed')}</SelectItem>
              </SelectContent>
            </Select>

            <Select
              value={toSelectValue(eventTypeFilter)}
              onValueChange={(next) => {
                onEventTypeFilterChange(fromSelectValue(next))
                onPageChange(1)
              }}
            >
              <SelectTrigger aria-label={t('notificationsPage.eventType')} className="w-full text-xs @lg/data-list:w-52">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={EMPTY_SELECT_VALUE}>{t('notificationsPage.allEventTypes')}</SelectItem>
                {NOTIFICATION_EVENT_GROUPS.map(group => (
                  <SelectGroup key={group.key}>
                    <SelectLabel>{notificationEventGroupLabel(group.key, t)}</SelectLabel>
                    {group.events.map(eventType => (
                      <SelectItem key={eventType} value={eventType}>
                        {notificationEventLabel(eventType, t)}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                ))}
              </SelectContent>
            </Select>

            <Select
              disabled={channels.length === 0 && !channelFilter}
              value={toSelectValue(channelFilter)}
              onValueChange={(next) => {
                onChannelFilterChange(fromSelectValue(next))
                onPageChange(1)
              }}
            >
              <SelectTrigger aria-label={t('notificationsPage.channels')} className="w-full text-xs @lg/data-list:w-44">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={EMPTY_SELECT_VALUE}>{t('notificationsPage.allChannels')}</SelectItem>
                {channels.map(channel => (
                  <SelectItem key={channel.id} value={channel.id}>{channel.name}</SelectItem>
                ))}
              </SelectContent>
            </Select>

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
          </>
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
