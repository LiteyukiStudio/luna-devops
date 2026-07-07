import type { UseFormReturn } from 'react-hook-form'
import type { ReleaseForm } from './application-deployments-panel-utils'
import type { BuildRun, DeploymentTarget, ReleaseImageCandidate } from '@/api'
import { useQuery } from '@tanstack/react-query'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '@/api'
import { CopyableHoverText } from '@/components/common/copyable-hover-text'
import { buildRunImageRef, buildRunOptionLabel } from '@/components/common/deployment-build-runs'
import { FormField as Field } from '@/components/common/form-field'
import { ProgressiveSection } from '@/components/common/progressive-section'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { shortImageRef } from './application-deployments-panel-utils'

interface ApplicationCreateReleaseDialogProps {
  applicationId: string
  form: UseFormReturn<ReleaseForm>
  open: boolean
  pending: boolean
  projectId: string
  releaseReadyTargets: DeploymentTarget[]
  selectableBuildRuns: BuildRun[]
  selectedTarget?: DeploymentTarget
  onOpenChange: (open: boolean) => void
  onSubmit: (values: ReleaseForm) => void
}

export function ApplicationCreateReleaseDialog({
  applicationId,
  form,
  onOpenChange,
  onSubmit,
  open,
  pending,
  projectId,
  releaseReadyTargets,
  selectableBuildRuns,
  selectedTarget,
}: ApplicationCreateReleaseDialogProps) {
  const { t } = useTranslation()
  const imageRef = form.watch('imageRef')
  const selectedBuildRun = selectableBuildRuns.find(run => run.id === form.watch('buildRunId'))
  const imageCandidates = useQuery({
    queryKey: ['release-image-candidates', projectId, applicationId, selectedTarget?.id],
    queryFn: () => api.listReleaseImageCandidates(projectId, applicationId, selectedTarget!.id),
    enabled: open && Boolean(projectId && applicationId && selectedTarget?.id),
  })
  const fallbackCandidates = useMemo(
    () => selectableBuildRuns.map(buildRunCandidate),
    [selectableBuildRuns],
  )
  const candidates = imageCandidates.data?.items.length ? imageCandidates.data.items : fallbackCandidates
  const selectedCandidateKey = selectedCandidateValue(candidates, form.watch('buildRunId'), imageRef)
  const imageSummary = imageRef || (selectedBuildRun ? buildRunImageRef(selectedBuildRun) : '')
  const registryHint = imageCandidates.data?.registryAvailable
    ? t('deploymentsPage.releaseImageCandidateRegistryHint')
    : imageCandidates.data?.fallbackUsed || imageCandidates.isError
      ? t('deploymentsPage.releaseImageCandidateFallbackHint')
      : t('deploymentsPage.releaseImageCandidateLoadingHint')

  const applyCandidate = (key: string) => {
    const candidate = candidates.find(item => item.key === key)
    if (!candidate)
      return
    form.setValue('buildRunId', candidate.buildRunId || '', { shouldDirty: true, shouldValidate: true })
    form.setValue('imageRef', candidate.imageRef, { shouldDirty: true, shouldValidate: true })
    form.setValue('applicationId', applicationId, { shouldDirty: true, shouldValidate: true })
    if (selectedTarget) {
      form.setValue('deploymentTargetId', selectedTarget.id, { shouldDirty: true, shouldValidate: true })
      form.setValue('environmentId', selectedTarget.environmentId, { shouldDirty: true, shouldValidate: true })
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>{t('deploymentsPage.createRelease')}</DialogTitle>
          <DialogDescription>{t('deploymentsPage.releaseDialogDescription')}</DialogDescription>
        </DialogHeader>
        <form className="grid gap-3" onSubmit={form.handleSubmit(onSubmit)}>
          <input type="hidden" {...form.register('imageRef', { required: true })} />
          {selectedTarget?.sourceType !== 'image' && (
            <Field hint={registryHint} label={t('deploymentsPage.releaseImageCandidate')} required>
              <Select value={selectedCandidateKey} onChange={event => applyCandidate(event.target.value)}>
                <option value="">{t('common.select')}</option>
                {candidates.map(candidate => <option key={candidate.key} value={candidate.key}>{candidate.label}</option>)}
              </Select>
            </Field>
          )}
          <Field hint={selectedTarget ? t('deploymentsPage.releaseTargetLockedHint') : undefined} label={t('buildsPage.buildConfig')} required>
            {selectedTarget
              ? (
                  <>
                    <input type="hidden" {...form.register('deploymentTargetId', { required: true })} />
                    <div className="rounded-md border border-border bg-muted/40 px-3 py-2 text-sm">
                      <span className="font-medium text-foreground">{selectedTarget.name}</span>
                      <span className="ml-2 text-muted-foreground">{t(`deploymentsPage.stageLabels.${selectedTarget.stage}`, { defaultValue: selectedTarget.stage })}</span>
                    </div>
                  </>
                )
              : (
                  <Select {...form.register('deploymentTargetId', { required: true })}>
                    <option value="">{t('common.select')}</option>
                    {releaseReadyTargets.map(target => <option key={target.id} value={target.id}>{target.name}</option>)}
                  </Select>
                )}
          </Field>
          <Field hint={t('deploymentsPage.releaseImageSummaryHint')} label={t('deploymentsPage.imageSummary')} required>
            <div className="rounded-md border border-border bg-muted/40 px-3 py-2">
              <CopyableHoverText
                className="max-w-full font-mono text-sm"
                display={imageSummary ? shortImageRef(imageSummary) : t('common.select')}
                value={imageSummary}
              />
            </div>
          </Field>
          <ProgressiveSection
            description={t('deploymentsPage.releaseImageOverrideDescription')}
            summary={imageSummary ? shortImageRef(imageSummary) : t('common.select')}
            title={t('deploymentsPage.releaseImageOverride')}
          >
            <Field hint={t('deploymentsPage.releaseImageOverrideHint')} label={t('deploymentsPage.image')} required>
              <Input
                value={imageRef}
                onChange={event => form.setValue('imageRef', event.target.value, { shouldDirty: true, shouldValidate: true })}
              />
            </Field>
          </ProgressiveSection>
          <DialogFooter>
            <Button disabled={!form.formState.isValid || pending} type="submit">{t('common.save')}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function buildRunCandidate(run: BuildRun): ReleaseImageCandidate {
  const imageRef = buildRunImageRef(run)
  return {
    buildRunId: run.id,
    createdAt: run.finishedAt || run.createdAt || '',
    digest: run.imageDigest,
    imageRef,
    key: `build:${run.id}`,
    label: buildRunOptionLabel(run),
    source: 'build',
    sourceCommit: run.sourceCommit,
    tag: run.targetTag,
  }
}

function selectedCandidateValue(candidates: ReleaseImageCandidate[], buildRunId: string, imageRef: string) {
  if (buildRunId) {
    const byBuild = candidates.find(candidate => candidate.buildRunId === buildRunId)
    if (byBuild)
      return byBuild.key
  }
  if (imageRef) {
    const byImage = candidates.find(candidate => candidate.imageRef === imageRef)
    if (byImage)
      return byImage.key
  }
  return ''
}
