import type { AiInferDraftPlan } from './ai-infer-config-utils'
import type { DeploymentTargetConfigInferenceOutput } from '@/api'
import { ArrowRight, FileCode2, Sparkles } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { StatusBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { aiInferFieldLabel } from './ai-infer-config-utils'

function confidenceTone(confidence: DeploymentTargetConfigInferenceOutput['confidence']) {
  switch (confidence) {
    case 'high':
      return 'success' as const
    case 'medium':
      return 'warning' as const
    case 'low':
      return 'neutral' as const
  }
}

export function AiInferConfigDialog({
  inferred,
  open,
  plan,
  onApply,
  onOpenChange,
}: {
  inferred: DeploymentTargetConfigInferenceOutput | null
  open: boolean
  plan: AiInferDraftPlan | null
  onApply: () => void
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()

  const confidenceLabel = inferred
    ? inferred.confidence === 'high'
      ? t('deploymentTargetEditor.aiInfer.confidenceHigh')
      : inferred.confidence === 'medium'
        ? t('deploymentTargetEditor.aiInfer.confidenceMedium')
        : t('deploymentTargetEditor.aiInfer.confidenceLow')
    : ''

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[85dvh] flex-col sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle className="flex flex-wrap items-center gap-2">
            <Sparkles className="size-4 text-primary" />
            <span>{t('deploymentTargetEditor.aiInfer.title')}</span>
            {inferred && <StatusBadge tone={confidenceTone(inferred.confidence)}>{confidenceLabel}</StatusBadge>}
          </DialogTitle>
          <DialogDescription>
            {t('deploymentTargetEditor.aiInfer.description')}
          </DialogDescription>
        </DialogHeader>

        {inferred && plan && (
          <div className="grid min-h-0 flex-1 gap-4 overflow-y-auto pr-1">
            {inferred.summary && (
              <div className="rounded-control border border-border bg-surface-subtle px-3.5 py-2.5 text-sm leading-6 text-foreground">
                {inferred.summary}
              </div>
            )}

            {inferred.inspectedFiles.length > 0 && (
              <div className="grid gap-1.5">
                <h4 className="text-xs font-medium text-muted-foreground">
                  {t('deploymentTargetEditor.aiInfer.inspectedFilesTitle')}
                </h4>
                <div className="flex flex-wrap gap-1.5">
                  {inferred.inspectedFiles.map(file => (
                    <span
                      key={file}
                      className="inline-flex items-center gap-1 rounded-control border border-border bg-surface-subtle px-2 py-0.5 font-mono text-xs text-foreground"
                    >
                      <FileCode2 className="size-3 text-muted-foreground" />
                      {file}
                    </span>
                  ))}
                </div>
              </div>
            )}

            {inferred.notes.length > 0 && (
              <div className="grid gap-1.5">
                <h4 className="text-xs font-medium text-muted-foreground">
                  {t('deploymentTargetEditor.aiInfer.notesTitle')}
                </h4>
                <ul className="grid list-disc gap-1 pl-4 text-xs leading-5 text-muted-foreground">
                  {inferred.notes.map(note => <li key={note}>{note}</li>)}
                </ul>
              </div>
            )}

            <div className="grid gap-2">
              <h4 className="text-xs font-medium text-muted-foreground">
                {t('deploymentTargetEditor.aiInfer.diffTitle')}
              </h4>
              {plan.diffItems.length === 0
                ? (
                    <p className="rounded-control border border-border bg-surface-subtle px-3 py-2.5 text-sm text-muted-foreground">
                      {t('deploymentTargetEditor.aiInfer.diffEmpty')}
                    </p>
                  )
                : (
                    <div className="grid gap-2">
                      {plan.diffItems.map(item => (
                        <div key={item.field} className="grid gap-1.5 rounded-control border border-border bg-surface px-3 py-2.5">
                          <span className="text-xs font-semibold text-foreground">{aiInferFieldLabel(item.field, t)}</span>
                          <div className="grid items-start gap-2 text-xs sm:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)]">
                            <div className="grid gap-0.5 rounded-control bg-surface-subtle px-2.5 py-1.5">
                              <span className="text-[11px] text-muted-foreground">{t('deploymentTargetEditor.aiInfer.beforeLabel')}</span>
                              <pre className="m-0 overflow-x-auto font-mono whitespace-pre-wrap break-all text-muted-foreground">{item.before}</pre>
                            </div>
                            <ArrowRight aria-hidden="true" className="hidden size-3.5 self-center text-muted-foreground sm:block" />
                            <div className="grid gap-0.5 rounded-control border border-border bg-surface-subtle px-2.5 py-1.5">
                              <span className="text-[11px] text-muted-foreground">{t('deploymentTargetEditor.aiInfer.afterLabel')}</span>
                              <pre className="m-0 overflow-x-auto font-mono whitespace-pre-wrap break-all text-foreground">{item.after}</pre>
                            </div>
                          </div>
                        </div>
                      ))}
                    </div>
                  )}
            </div>
          </div>
        )}

        <DialogFooter>
          <DialogClose asChild>
            <Button type="button" variant="outline">
              {t('common.cancel')}
            </Button>
          </DialogClose>
          <Button
            disabled={!plan || plan.diffItems.length === 0}
            type="button"
            onClick={onApply}
          >
            {t('deploymentTargetEditor.aiInfer.applyDraft')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
