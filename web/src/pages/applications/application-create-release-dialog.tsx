import type { UseFormReturn } from 'react-hook-form'
import type { ReleaseForm } from './application-deployments-panel-utils'
import type { BuildRun, DeploymentTarget } from '@/api'
import { useTranslation } from 'react-i18next'
import { buildRunOptionLabel } from '@/components/common/deployment-build-runs'
import { FormField as Field } from '@/components/common/form-field'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'

interface ApplicationCreateReleaseDialogProps {
  form: UseFormReturn<ReleaseForm>
  open: boolean
  pending: boolean
  releaseReadyTargets: DeploymentTarget[]
  selectableBuildRuns: BuildRun[]
  selectedTarget?: DeploymentTarget
  onOpenChange: (open: boolean) => void
  onSubmit: (values: ReleaseForm) => void
}

export function ApplicationCreateReleaseDialog({
  form,
  onOpenChange,
  onSubmit,
  open,
  pending,
  releaseReadyTargets,
  selectableBuildRuns,
  selectedTarget,
}: ApplicationCreateReleaseDialogProps) {
  const { t } = useTranslation()

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('deploymentsPage.createRelease')}</DialogTitle>
          <DialogDescription>{t('deploymentsPage.releaseDialogDescription')}</DialogDescription>
        </DialogHeader>
        <form className="grid gap-3" onSubmit={form.handleSubmit(onSubmit)}>
          {selectedTarget?.sourceType !== 'image' && (
            <Field hint={t('deploymentsPage.buildRunHint')} label={t('deploymentsPage.buildRun')} required>
              <Select {...form.register('buildRunId', { required: true })}>
                <option value="">{t('common.select')}</option>
                {selectableBuildRuns.map(run => <option key={run.id} value={run.id}>{buildRunOptionLabel(run)}</option>)}
              </Select>
            </Field>
          )}
          <Field label={t('buildsPage.buildConfig')}>
            <Select {...form.register('deploymentTargetId', { required: true })}>
              <option value="">{t('common.select')}</option>
              {releaseReadyTargets.map(target => <option key={target.id} value={target.id}>{target.name}</option>)}
            </Select>
          </Field>
          <Field label={t('deploymentsPage.image')} required>
            <Input {...form.register('imageRef', { required: true })} />
          </Field>
          <DialogFooter>
            <Button disabled={!form.formState.isValid || pending} type="submit">{t('common.save')}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
