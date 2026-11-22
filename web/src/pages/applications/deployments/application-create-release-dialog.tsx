import type { UseFormReturn } from 'react-hook-form'
import type { ReleaseForm } from './application-deployments-panel-utils'
import type { BuildRun, DeploymentTarget, ReleaseImageCandidate } from '@/api'
import { useQuery } from '@tanstack/react-query'
import { useMemo } from 'react'
import { Controller } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { api } from '@/api'
import { CopyableHoverText } from '@/components/common/copyable-hover-text'
import { buildRunImageRef, buildRunOptionLabel } from '@/components/common/deployment-build-runs'
import { FormField as Field } from '@/components/common/form-field'
import { ProgressiveSection } from '@/components/common/progressive-section'
import { Button } from '@/components/ui/button'
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { shortImageRef } from '@/lib/image-ref'

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
  const candidatesLoadingEmpty = imageCandidates.isLoading && candidates.length === 0
  const registryHint = imageCandidates.isLoading
    ? t('deploymentsPage.releaseImageCandidateLoadingHint')
    : imageCandidates.data?.registryAvailable
      ? t('deploymentsPage.releaseImageCandidateRegistryHint')
      : imageCandidates.data?.fallbackUsed || imageCandidates.isError
        ? t('deploymentsPage.releaseImageCandidateFallbackHint')
        : undefined

  const applyCandidate = (key: string) => {
    const candidate = candidates.find(item => item.key === key)
    if (!candidate)
      return
    form.setValue('buildRunId', candidate.buildRunId || '', { shouldDirty: true, shouldValidate: true })
    form.setValue('imageRef', candidate.imageRef, { shouldDirty: true, shouldValidate: true })
    form.setValue('applicationId', applicationId, { shouldDirty: true, shouldValidate: true })
    if (selectedTarget) {
      form.setValue('deploymentTargetId', selectedTarget.id, { shouldDirty: true, shouldValidate: true })
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('deploymentsPage.createRelease')}</DialogTitle>
          <DialogDescription>{t('deploymentsPage.releaseDialogDescription')}</DialogDescription>
        </DialogHeader>
        <form className="grid gap-3" onSubmit={form.handleSubmit(onSubmit)}>
          <input type="hidden" {...form.register('imageRef', { required: t('common.required') })} />
          {selectedTarget?.sourceType !== 'image' && (
            <Field error={form.formState.errors.imageRef?.message} hint={registryHint} label={t('deploymentsPage.releaseImageCandidate')} required>
              {fieldControl => (
                <div className="grid gap-1.5">
                  <Select
                    disabled={candidatesLoadingEmpty}
                    value={selectedCandidateKey || undefined}
                    onValueChange={applyCandidate}
                  >
                    <SelectTrigger {...fieldControl} className="w-full">
                      <SelectValue
                        placeholder={
                          candidatesLoadingEmpty
                            ? t('common.loading')
                            : candidates.length === 0
                              ? t('common.noOptions')
                              : t('common.select')
                        }
                      />
                    </SelectTrigger>
                    <SelectContent>
                      {candidatesLoadingEmpty
                        ? <SelectItem disabled value="__loading__">{t('common.loading')}</SelectItem>
                        : candidates.length === 0
                          ? <SelectItem disabled value="__empty__">{t('common.noOptions')}</SelectItem>
                          : candidates.map(candidate => (
                              <SelectItem key={candidate.key} value={candidate.key}>{candidate.label}</SelectItem>
                            ))}
                    </SelectContent>
                  </Select>
                  {imageCandidates.isError && (
                    <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground" role="alert">
                      <span>{t('common.unavailable')}</span>
                      <Button disabled={imageCandidates.isFetching} size="sm" type="button" variant="outline" onClick={() => void imageCandidates.refetch()}>
                        {t('common.retry')}
                      </Button>
                    </div>
                  )}
                </div>
              )}
            </Field>
          )}
          {selectedTarget && <input type="hidden" {...form.register('deploymentTargetId', { required: t('common.required') })} />}
          <Field error={form.formState.errors.deploymentTargetId?.message} hint={selectedTarget ? t('deploymentsPage.releaseTargetLockedHint') : undefined} label={t('buildsPage.buildConfig')} required>
            {fieldControl => selectedTarget
              ? (
                  <Input
                    {...fieldControl}
                    disabled
                    value={`${selectedTarget.name} · ${t(`deploymentsPage.stageLabels.${selectedTarget.stage}`, { defaultValue: selectedTarget.stage })}`}
                  />
                )
              : (
                  <Controller
                    control={form.control}
                    name="deploymentTargetId"
                    rules={{ required: t('common.required') }}
                    render={({ field }) => (
                      <Select
                        value={field.value || undefined}
                        onValueChange={(targetId) => {
                          field.onChange(targetId)
                          const nextTarget = releaseReadyTargets.find(item => item.id === targetId)
                          const matchedRun = targetId ? selectableBuildRuns.find(run => run.deploymentTargetId === targetId) : undefined
                          form.setValue('applicationId', matchedRun?.applicationId ?? applicationId, { shouldDirty: true, shouldValidate: true })
                          form.setValue('buildRunId', matchedRun?.id ?? '', { shouldDirty: true, shouldValidate: true })
                          form.setValue('imageRef', matchedRun ? buildRunImageRef(matchedRun) : nextTarget?.imageRef ?? '', { shouldDirty: true, shouldValidate: true })
                        }}
                      >
                        <SelectTrigger {...fieldControl} className="w-full">
                          <SelectValue placeholder={t('common.select')} />
                        </SelectTrigger>
                        <SelectContent>
                          {releaseReadyTargets.map(target => (
                            <SelectItem key={target.id} value={target.id}>{target.name}</SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    )}
                  />
                )}
          </Field>
          <Field hint={t('deploymentsPage.releaseImageSummaryHint')} label={t('deploymentsPage.imageSummary')} required>
            <CopyableHoverText
              className="max-w-full font-mono text-sm"
              display={imageSummary ? shortImageRef(imageSummary) : t('common.select')}
              value={imageSummary}
            />
          </Field>
          <ProgressiveSection
            description={t('deploymentsPage.releaseImageOverrideDescription')}
            summary={imageSummary ? shortImageRef(imageSummary) : t('common.select')}
            title={t('deploymentsPage.releaseImageOverride')}
          >
            <Field error={form.formState.errors.imageRef?.message} hint={t('deploymentsPage.releaseImageOverrideHint')} label={t('deploymentsPage.image')} required>
              <Input
                aria-invalid={Boolean(form.formState.errors.imageRef)}
                value={imageRef}
                onChange={event => form.setValue('imageRef', event.target.value, { shouldDirty: true, shouldValidate: true })}
              />
            </Field>
          </ProgressiveSection>
          <DialogFooter>
            <DialogClose asChild>
              <Button disabled={pending} type="button" variant="outline">{t('common.cancel')}</Button>
            </DialogClose>
            <Button disabled={!form.formState.isValid || pending} type="submit">{t('deploymentsPage.createRelease')}</Button>
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
