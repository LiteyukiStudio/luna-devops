import type { NotificationDelivery } from '@/api'
import { ExternalLink, RotateCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link, useInRouterContext } from 'react-router-dom'
import { DescriptionListItem } from '@/components/common/description-list-item'
import { StatusValueBadge } from '@/components/common/status-badge'
import { formatSmartDateTime } from '@/components/common/time-format'
import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { formatSnapshotBlock, notificationEventLabel } from './notification-models'

export function NotificationDeliveryDetailSheet({
  canRetry = false,
  delivery,
  retrying = false,
  showEventLink = true,
  onClose,
  onRetry,
}: {
  canRetry?: boolean
  delivery: NotificationDelivery | null
  retrying?: boolean
  showEventLink?: boolean
  onClose: () => void
  onRetry?: (delivery: NotificationDelivery) => void
}) {
  const { t } = useTranslation()
  const inRouter = useInRouterContext()
  const active = delivery?.status === 'pending' || delivery?.status === 'sending'
  const requestText = formatSnapshotBlock(delivery?.requestSnapshot)
  const responseText = formatSnapshotBlock(delivery?.responseSnippet)
  const sourceEventPath = delivery?.eventId
    ? `/events?eventId=${encodeURIComponent(delivery.eventId)}${delivery.projectId ? `&projectId=${encodeURIComponent(delivery.projectId)}` : ''}`
    : ''

  return (
    <Sheet open={Boolean(delivery)} onOpenChange={open => !open && onClose()}>
      <SheetContent className="flex w-full flex-col gap-0 overflow-hidden p-0 sm:max-w-xl">
        <SheetHeader className="border-b border-border px-5 py-4 pr-12">
          <div className="flex flex-wrap items-center gap-2">
            <SheetTitle>{t('notificationsPage.deliveryDetailTitle')}</SheetTitle>
            {delivery && <StatusValueBadge value={delivery.status} />}
          </div>
          <SheetDescription>{t('notificationsPage.deliveryDetailDescription')}</SheetDescription>
        </SheetHeader>

        {delivery && (
          <div className="grid flex-1 content-start gap-4 overflow-y-auto px-5 py-4">
            <div className="rounded-container border border-border bg-surface-subtle/60 p-3">
              <p className="text-sm font-medium text-foreground">
                {notificationEventLabel(delivery.eventType, t)}
              </p>
              <p className="mt-0.5 font-mono text-xs text-muted-foreground">{delivery.eventType}</p>
            </div>

            {delivery.errorMessage && (
              <div className="rounded-container border border-danger-border bg-danger-subtle p-3 text-sm text-danger">
                <p className="font-medium">{t('notificationsPage.error')}</p>
                <p className="mt-1 break-words text-xs">{delivery.errorMessage}</p>
              </div>
            )}

            <dl className="grid gap-3 sm:grid-cols-2">
              <DescriptionListItem
                label={t('notificationsPage.channels')}
                value={delivery.channelName || delivery.channelId || '-'}
              />
              <DescriptionListItem
                label={t('notificationsPage.recipientLabel')}
                value={delivery.recipientSummary || '-'}
              />
              {delivery.ruleName && (
                <DescriptionListItem
                  label={t('notificationsPage.ruleLabel')}
                  value={delivery.ruleName}
                />
              )}
              <DescriptionListItem
                label={t('notificationsPage.attempts')}
                value={t('notificationsPage.attemptsOfMax', { count: delivery.attemptCount })}
              />
              <DescriptionListItem
                label={t('notificationsPage.queuedAt')}
                value={formatSmartDateTime(delivery.queuedAt ?? delivery.createdAt, t)}
              />
              <DescriptionListItem
                label={t('notificationsPage.startedAt')}
                value={delivery.startedAt ? formatSmartDateTime(delivery.startedAt, t) : '-'}
              />
              <DescriptionListItem
                label={t('notificationsPage.finishedAt')}
                value={delivery.finishedAt ? formatSmartDateTime(delivery.finishedAt, t) : '-'}
              />
              <DescriptionListItem
                label={t('notificationsPage.durationLabel')}
                value={delivery.durationMillis > 0 ? t('notificationsPage.durationMs', { ms: delivery.durationMillis }) : '-'}
              />
              <DescriptionListItem
                label={t('notificationsPage.timeZone')}
                value={delivery.timeZone || 'UTC'}
              />
            </dl>

            {showEventLink && sourceEventPath && (
              <div>
                <Button asChild size="sm" variant="outline">
                  {inRouter
                    ? (
                        <Link to={sourceEventPath} onClick={onClose}>
                          <ExternalLink className="size-3.5" />
                          {t('notificationsPage.openSourceEvent')}
                        </Link>
                      )
                    : (
                        <a href={sourceEventPath}>
                          <ExternalLink className="size-3.5" />
                          {t('notificationsPage.openSourceEvent')}
                        </a>
                      )}
                </Button>
              </div>
            )}

            <div className="grid gap-1.5">
              <p className="text-xs font-medium text-muted-foreground">{t('notificationsPage.requestSnapshot')}</p>
              {requestText
                ? (
                    <pre className="max-h-56 overflow-auto rounded-control border border-border bg-surface-subtle p-3 font-mono text-xs leading-relaxed text-foreground">
                      {requestText}
                    </pre>
                  )
                : <p className="text-xs text-muted-foreground">{t('notificationsPage.noSnapshot')}</p>}
            </div>

            <div className="grid gap-1.5">
              <p className="text-xs font-medium text-muted-foreground">{t('notificationsPage.responseSnippet')}</p>
              {responseText
                ? (
                    <pre className="max-h-44 overflow-auto rounded-control border border-border bg-surface-subtle p-3 font-mono text-xs leading-relaxed text-foreground">
                      {responseText}
                    </pre>
                  )
                : <p className="text-xs text-muted-foreground">{t('notificationsPage.noSnapshot')}</p>}
            </div>
          </div>
        )}

        {delivery && canRetry && onRetry && (
          <SheetFooter className="border-t border-border px-5 py-3 sm:flex-row sm:justify-end">
            <Button
              disabled={retrying || active}
              type="button"
              onClick={() => onRetry(delivery)}
            >
              <RotateCw className="size-4" />
              {t('notificationsPage.retryDelivery')}
            </Button>
          </SheetFooter>
        )}
      </SheetContent>
    </Sheet>
  )
}
