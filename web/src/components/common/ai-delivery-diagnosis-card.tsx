import type { TFunction } from 'i18next'
import type { AIDeliveryDiagnosisView } from '@/api'
import { useMutation, useQuery } from '@tanstack/react-query'
import { AlertTriangle, ExternalLink, LoaderCircle, Play, Sparkles } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'
import { api } from '@/api'
import { toolDisplayName } from '@/components/common/ai-assistant/tool-display-name'
import { ErrorState } from '@/components/common/error-state'
import { StatusBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { cn } from '@/lib/utils'

function formatScalar(value: unknown): string {
  if (value === undefined)
    return '-'
  if (typeof value === 'string')
    return value || '""'
  if (typeof value === 'number' || typeof value === 'boolean' || value === null)
    return String(value)
  return JSON.stringify(value)
}

function formatParameterEntries(args: Record<string, unknown>): Array<{ key: string, value: string }> {
  return Object.entries(args)
    .filter(([, value]) => value !== undefined)
    .map(([key, value]) => ({
      key,
      value: formatScalar(value),
    }))
}

function diagnosisFailureLabel(t: TFunction, errorCode?: string): string {
  switch (errorCode) {
    case 'ai.wallet_balance_insufficient':
      return t('aiAssistant.deliveryDiagnosis.failedWalletInsufficient')
    case 'ai.disabled':
      return t('errors.ai.disabled')
    case 'ai.agent_unavailable':
      return t('errors.ai.agent_unavailable')
    case 'ai.not_configured':
      return t('errors.ai.not_configured')
    case 'ai.provider_unavailable':
      return t('errors.ai.provider_unavailable')
    case 'ai.timeout':
    case 'ai.provider_timeout':
      return t('errors.ai.provider_timeout')
    case 'ai.provider_rate_limited':
    case 'ai.rate_limited':
      return t('errors.ai.provider_rate_limited')
    case 'ai.model_not_available':
      return t('errors.ai.model_not_available')
    case 'ai.user_not_allowed':
      return t('errors.ai.user_not_allowed')
    case 'ai.structured_output_invalid':
      return t('errors.ai.structured_output_invalid')
    case 'auth.account.disabled':
      return t('errors.auth.account.disabled')
    default:
      return t('aiAssistant.deliveryDiagnosis.failed')
  }
}

export function AIDeliveryDiagnosisCard({
  applicationId,
  className,
  deploymentTargetId,
  diagnosis,
  onActionExecuted,
  onNavigateEditTarget,
  projectId,
}: {
  applicationId: string
  className?: string
  deploymentTargetId?: string
  diagnosis?: AIDeliveryDiagnosisView
  onActionExecuted?: () => void
  onNavigateEditTarget?: () => void
  projectId: string
}) {
  const { t } = useTranslation()
  const [previewOpen, setPreviewOpen] = useState(false)

  const proposedAction = diagnosis?.proposedAction
  const effectiveTargetId = (deploymentTargetId || diagnosis?.deploymentTargetId || '').trim()
  const editHref = projectId && applicationId && effectiveTargetId
    ? `/projects/${projectId}/apps/${applicationId}/deployments/${effectiveTargetId}/edit`
    : ''

  const previewQuery = useQuery({
    queryKey: [
      'ai-action-proposal-preview',
      projectId,
      diagnosis?.id ?? '',
      proposedAction?.toolName ?? '',
      JSON.stringify(proposedAction?.arguments ?? {}),
    ],
    queryFn: ({ signal }) => api.previewProjectAIActionProposal(
      projectId,
      {
        toolName: proposedAction?.toolName ?? '',
        arguments: proposedAction?.arguments ?? {},
      },
      signal,
    ),
    enabled: Boolean(previewOpen && projectId && proposedAction?.toolName),
    gcTime: 0,
    staleTime: 0,
    retry: false,
  })

  const executeMutation = useMutation({
    mutationFn: () => {
      if (!proposedAction || !previewQuery.data?.previewLock)
        throw new Error(t('common.unavailable'))
      return api.executeProjectAIActionProposal(projectId, {
        toolName: proposedAction.toolName,
        arguments: proposedAction.arguments,
        previewLock: previewQuery.data.previewLock,
      })
    },
    onSuccess: () => {
      toast.success(t('aiAssistant.deliveryDiagnosis.executedToast'))
      setPreviewOpen(false)
      onActionExecuted?.()
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : t('common.unavailable'))
      void previewQuery.refetch()
    },
  })

  if (!diagnosis)
    return null

  if (diagnosis.status === 'running') {
    return (
      <div
        className={cn('flex items-center gap-2.5 rounded-container border border-border bg-surface-subtle/70 px-3.5 py-2.5 text-xs text-muted-foreground', className)}
        data-slot="ai-delivery-diagnosis"
        data-status="running"
        role="status"
      >
        <LoaderCircle aria-hidden="true" className="size-4 shrink-0 animate-spin text-primary motion-reduce:animate-none" />
        <span>{t('aiAssistant.deliveryDiagnosis.running')}</span>
      </div>
    )
  }

  if (diagnosis.status === 'failed') {
    return (
      <div
        className={cn('flex items-center gap-2.5 rounded-container border border-border bg-surface-subtle/60 px-3.5 py-2.5 text-xs text-muted-foreground', className)}
        data-slot="ai-delivery-diagnosis"
        data-status="failed"
      >
        <AlertTriangle aria-hidden="true" className="size-4 shrink-0 text-warning" />
        <span>{diagnosisFailureLabel(t, diagnosis.errorCode)}</span>
      </div>
    )
  }

  const confidenceLabel = diagnosis.confidence === 'high'
    ? t('aiAssistant.deliveryDiagnosis.confidenceHigh')
    : diagnosis.confidence === 'medium'
      ? t('aiAssistant.deliveryDiagnosis.confidenceMedium')
      : t('aiAssistant.deliveryDiagnosis.confidenceLow')
  const confidenceTone = diagnosis.confidence === 'high'
    ? 'success'
    : diagnosis.confidence === 'medium'
      ? 'info'
      : 'neutral'

  const preview = previewQuery.data?.approvalPreview
  const parameterEntries = proposedAction ? formatParameterEntries(proposedAction.arguments) : []

  return (
    <>
      <section
        className={cn('grid gap-3 rounded-container border border-border bg-surface-raised p-3.5 text-xs shadow-surface-sm', className)}
        data-slot="ai-delivery-diagnosis"
        data-status="succeeded"
      >
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <Sparkles aria-hidden="true" className="size-4 shrink-0 text-primary" />
            <h3 className="text-sm font-semibold text-foreground">{t('aiAssistant.deliveryDiagnosis.title')}</h3>
            <StatusBadge tone={confidenceTone}>{confidenceLabel}</StatusBadge>
          </div>
          {editHref && (
            <Button asChild className="h-7 px-2.5 text-xs" size="sm" variant="outline" onClick={onNavigateEditTarget}>
              <Link to={editHref}>
                <ExternalLink aria-hidden="true" className="size-3.5" />
                {t('aiAssistant.deliveryDiagnosis.editTargetConfig')}
              </Link>
            </Button>
          )}
        </div>

        <p className="leading-5 text-foreground">{diagnosis.rootCauseSummary}</p>

        {diagnosis.suggestions.length > 0 && (
          <div className="grid gap-1.5">
            <span className="font-medium text-muted-foreground">{t('aiAssistant.deliveryDiagnosis.suggestionsTitle')}</span>
            <ul className="grid list-disc gap-1 pl-4 leading-5 text-foreground">
              {diagnosis.suggestions.map(item => (
                <li key={item}>{item}</li>
              ))}
            </ul>
          </div>
        )}

        {proposedAction && (
          <div className="flex flex-wrap items-center justify-between gap-2.5 rounded-control border border-border bg-surface-subtle/60 px-3 py-2">
            <div className="grid min-w-0 flex-1 gap-0.5">
              <div className="flex flex-wrap items-center gap-1.5">
                <span className="font-medium text-foreground">{t('aiAssistant.deliveryDiagnosis.proposedActionTitle')}</span>
                <StatusBadge tone="info">{toolDisplayName(t, proposedAction.toolName)}</StatusBadge>
              </div>
              <p className="leading-5 text-muted-foreground">{proposedAction.summary}</p>
            </div>
            <Button
              className="h-8 shrink-0 px-3 text-xs"
              size="sm"
              type="button"
              onClick={() => setPreviewOpen(true)}
            >
              <Play aria-hidden="true" className="size-3.5" />
              {t('aiAssistant.deliveryDiagnosis.previewAndExecute')}
            </Button>
          </div>
        )}
      </section>

      {proposedAction && (
        <Dialog open={previewOpen} onOpenChange={open => !executeMutation.isPending && setPreviewOpen(open)}>
          <DialogContent className="sm:max-w-lg" scrollable>
            <DialogHeader>
              <DialogTitle>{t('aiAssistant.deliveryDiagnosis.previewDialogTitle')}</DialogTitle>
              <DialogDescription>{t('aiAssistant.deliveryDiagnosis.previewDialogDescription')}</DialogDescription>
            </DialogHeader>
            <DialogBody className="grid gap-3 text-xs">
              {previewQuery.isPending && (
                <div className="flex items-center gap-2 py-4 text-sm text-muted-foreground" role="status">
                  <LoaderCircle aria-hidden="true" className="size-4 animate-spin text-primary motion-reduce:animate-none" />
                  <span>{t('aiAssistant.deliveryDiagnosis.previewLoading')}</span>
                </div>
              )}

              {previewQuery.isError && (
                <ErrorState
                  description={previewQuery.error instanceof Error ? previewQuery.error.message : t('common.unavailable')}
                  retrying={previewQuery.isFetching}
                  title={t('common.unavailable')}
                  onRetry={() => void previewQuery.refetch()}
                />
              )}

              {previewQuery.data && (
                <div className="grid gap-2.5">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="text-sm font-medium text-foreground">{toolDisplayName(t, previewQuery.data.toolName)}</span>
                    <StatusBadge tone={previewQuery.data.approvalTier === 'L2' ? 'warning' : 'info'}>
                      {previewQuery.data.approvalTier === 'L2' ? t('aiAssistant.approval.tierL2') : t('aiAssistant.approval.tierL1')}
                    </StatusBadge>
                  </div>
                  {(preview?.summary || proposedAction.summary) && (
                    <p className="leading-5 text-foreground">{preview?.summary || proposedAction.summary}</p>
                  )}
                  {preview?.changes && preview.changes.length > 0 && (
                    <div className="grid gap-1 rounded-control border border-border bg-surface-subtle/50 px-3 py-2">
                      <span className="font-medium text-muted-foreground">{t('aiAssistant.approval.changesTitle')}</span>
                      <div className="grid max-h-44 gap-1 overflow-y-auto">
                        {preview.changes.map(change => (
                          <div key={`${change.field}-${formatScalar(change.after)}`} className="flex flex-wrap items-baseline justify-between gap-2 font-mono text-[11px]">
                            <span className="text-muted-foreground">{change.field}</span>
                            <span className="min-w-0 break-all text-right text-foreground">
                              {change.before !== undefined ? `${formatScalar(change.before)} -> ${formatScalar(change.after)}` : formatScalar(change.after)}
                            </span>
                          </div>
                        ))}
                      </div>
                    </div>
                  )}
                  {parameterEntries.length > 0 && (
                    <div className="grid gap-1 rounded-control border border-border bg-surface-subtle/50 px-3 py-2">
                      <span className="font-medium text-muted-foreground">{t('aiAssistant.deliveryDiagnosis.parametersTitle')}</span>
                      <dl className="grid max-h-44 gap-1 overflow-y-auto">
                        {parameterEntries.map(entry => (
                          <div key={entry.key} className="flex items-baseline justify-between gap-3">
                            <dt className="shrink-0 font-mono text-[11px] text-muted-foreground">{entry.key}</dt>
                            <dd className="min-w-0 break-all text-right font-mono text-[11px] text-foreground">{entry.value}</dd>
                          </div>
                        ))}
                      </dl>
                    </div>
                  )}
                  {preview?.warnings && preview.warnings.length > 0 && (
                    <div className="grid gap-1 rounded-control border border-warning/40 bg-warning-subtle/80 px-3 py-2">
                      <span className="font-medium text-warning">{t('aiAssistant.approval.warningsTitle')}</span>
                      {preview.warnings.map(line => (
                        <span key={line} className="text-foreground">{line}</span>
                      ))}
                    </div>
                  )}
                </div>
              )}
            </DialogBody>
            <DialogFooter>
              <Button
                disabled={executeMutation.isPending}
                type="button"
                variant="outline"
                onClick={() => setPreviewOpen(false)}
              >
                {t('common.cancel')}
              </Button>
              <Button
                disabled={!previewQuery.data?.previewLock || previewQuery.isFetching || executeMutation.isPending}
                type="button"
                onClick={() => executeMutation.mutate()}
              >
                {executeMutation.isPending && <LoaderCircle aria-hidden="true" className="size-4 animate-spin motion-reduce:animate-none" />}
                {t('aiAssistant.deliveryDiagnosis.confirmExecute')}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      )}
    </>
  )
}
