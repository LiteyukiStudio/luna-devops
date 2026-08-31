import type { UseFormReturn } from 'react-hook-form'
import type { DeploymentTargetPayload } from '@/api'
import { useTranslation } from 'react-i18next'
import { FormField as Field } from '@/components/common/form-field'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'

interface KubernetesAdvancedFieldsProps {
  form: UseFormReturn<DeploymentTargetPayload>
}

export function KubernetesAdvancedFields({ form }: KubernetesAdvancedFieldsProps) {
  const { t } = useTranslation()

  return (
    <div className="grid gap-4">
      <div className="grid gap-3 rounded-md border border-dashed border-border p-3">
        <p className="text-sm font-medium text-foreground">{t('deploymentsPage.kubernetesAdvancedWorkload')}</p>
        <div className="grid gap-3 md:grid-cols-2">
          <Field hint={t('deploymentsPage.workloadTypeHint')} label={t('deploymentsPage.workloadType')}>
            <Select {...form.register('workloadType')}>
              <option value="Deployment">{t('deploymentsPage.kubernetesValues.Deployment')}</option>
              <option value="StatefulSet">{t('deploymentsPage.kubernetesValues.StatefulSet')}</option>
            </Select>
          </Field>
        </div>
      </div>

      <div className="grid gap-3 rounded-md border border-dashed border-border p-3">
        <p className="text-sm font-medium text-foreground">{t('deploymentsPage.kubernetesAdvancedContainer')}</p>
        <div className="grid gap-3 md:grid-cols-2">
          <Field hint={t('deploymentsPage.imagePullPolicyHint')} label={t('deploymentsPage.imagePullPolicy')}>
            <Select {...form.register('imagePullPolicy')}>
              <option value="">{t('deploymentsPage.kubernetesDefault')}</option>
              <option value="IfNotPresent">{t('deploymentsPage.kubernetesValues.IfNotPresent')}</option>
              <option value="Always">{t('deploymentsPage.kubernetesValues.Always')}</option>
              <option value="Never">{t('deploymentsPage.kubernetesValues.Never')}</option>
            </Select>
          </Field>
          <Field hint={t('deploymentsPage.priorityClassNameHint')} label={t('deploymentsPage.priorityClassName')}>
            <Input {...form.register('priorityClassName')} placeholder={t('deploymentsPage.priorityClassNamePlaceholder')} />
          </Field>
        </div>
        <div className="grid gap-3 md:grid-cols-2">
          <Field hint={t('deploymentsPage.containerCommandHint')} label={t('deploymentsPage.containerCommand')}>
            <Textarea className="min-h-24" {...form.register('containerCommand')} placeholder={t('deploymentsPage.containerCommandPlaceholder')} />
          </Field>
          <Field hint={t('deploymentsPage.containerArgsHint')} label={t('deploymentsPage.containerArgs')}>
            <Textarea className="min-h-24" {...form.register('containerArgs')} placeholder={t('deploymentsPage.containerArgsPlaceholder')} />
          </Field>
        </div>
        <Field hint={t('deploymentsPage.lifecycleHint')} label={t('deploymentsPage.lifecycle')}>
          <Textarea className="min-h-24" {...form.register('lifecycle')} placeholder={t('deploymentsPage.lifecyclePlaceholder')} />
        </Field>
        <div className="grid gap-3 md:grid-cols-3">
          <Field hint={t('deploymentsPage.readinessProbeHint')} label={t('deploymentsPage.readinessProbe')}>
            <Textarea className="min-h-24" {...form.register('readinessProbe')} placeholder={t('deploymentsPage.probePlaceholder')} />
          </Field>
          <Field hint={t('deploymentsPage.livenessProbeHint')} label={t('deploymentsPage.livenessProbe')}>
            <Textarea className="min-h-24" {...form.register('livenessProbe')} placeholder={t('deploymentsPage.probePlaceholder')} />
          </Field>
          <Field hint={t('deploymentsPage.startupProbeHint')} label={t('deploymentsPage.startupProbe')}>
            <Textarea className="min-h-24" {...form.register('startupProbe')} placeholder={t('deploymentsPage.probePlaceholder')} />
          </Field>
        </div>
      </div>

      <div className="grid gap-3 rounded-md border border-dashed border-border p-3">
        <p className="text-sm font-medium text-foreground">{t('deploymentsPage.kubernetesAdvancedAutoScaling')}</p>
        <div className="grid gap-3 md:grid-cols-5">
          <Field hint={t('deploymentsPage.autoScalingEnabledHint')} label={t('deploymentsPage.autoScalingEnabled')}>
            <Select {...form.register('autoScalingEnabled')}>
              <option value="false">{t('common.disabled')}</option>
              <option value="true">{t('common.enabled')}</option>
            </Select>
          </Field>
          <Field hint={t('deploymentsPage.autoScalingMinReplicasHint')} label={t('deploymentsPage.autoScalingMinReplicas')}>
            <Input {...form.register('autoScalingMinReplicas', { valueAsNumber: true })} min={0} type="number" />
          </Field>
          <Field hint={t('deploymentsPage.autoScalingMaxReplicasHint')} label={t('deploymentsPage.autoScalingMaxReplicas')}>
            <Input {...form.register('autoScalingMaxReplicas', { valueAsNumber: true })} min={1} type="number" />
          </Field>
          <Field hint={t('deploymentsPage.autoScalingCpuPercentHint')} label={t('deploymentsPage.autoScalingCpuPercent')}>
            <Input {...form.register('autoScalingCpuPercent', { valueAsNumber: true })} min={0} type="number" />
          </Field>
          <Field hint={t('deploymentsPage.autoScalingMemoryPercentHint')} label={t('deploymentsPage.autoScalingMemoryPercent')}>
            <Input {...form.register('autoScalingMemoryPercent', { valueAsNumber: true })} min={0} type="number" />
          </Field>
        </div>
        <Field hint={t('deploymentsPage.autoScalingBehaviorHint')} label={t('deploymentsPage.autoScalingBehavior')}>
          <Textarea className="min-h-24" {...form.register('autoScalingBehavior')} placeholder={t('deploymentsPage.autoScalingBehaviorPlaceholder')} />
        </Field>
      </div>

      <div className="grid gap-3 rounded-md border border-dashed border-border p-3">
        <p className="text-sm font-medium text-foreground">{t('deploymentsPage.kubernetesAdvancedSecurity')}</p>
        <Field hint={t('deploymentsPage.webConsoleOverrideHint')} label={t('deploymentsPage.webConsoleOverride')}>
          <Select {...form.register('webConsoleEnabled')}>
            <option value="">{t('deploymentsPage.webConsoleInheritProject')}</option>
            <option value="false">{t('deploymentsPage.webConsoleDisableForTarget')}</option>
          </Select>
        </Field>
        <div className="grid gap-3 md:grid-cols-3">
          <Field hint={t('deploymentsPage.runAsUserHint')} label={t('deploymentsPage.runAsUser')}>
            <Input {...form.register('runAsUser')} inputMode="numeric" placeholder="1001" />
          </Field>
          <Field hint={t('deploymentsPage.runAsGroupHint')} label={t('deploymentsPage.runAsGroup')}>
            <Input {...form.register('runAsGroup')} inputMode="numeric" placeholder="1001" />
          </Field>
          <Field hint={t('deploymentsPage.fsGroupHint')} label={t('deploymentsPage.fsGroup')}>
            <Input {...form.register('fsGroup')} inputMode="numeric" placeholder="1001" />
          </Field>
          <Field hint={t('deploymentsPage.fsGroupChangePolicyHint')} label={t('deploymentsPage.fsGroupChangePolicy')}>
            <Select {...form.register('fsGroupChangePolicy')}>
              <option value="">{t('deploymentsPage.kubernetesDefault')}</option>
              <option value="OnRootMismatch">{t('deploymentsPage.kubernetesValues.OnRootMismatch')}</option>
              <option value="Always">{t('deploymentsPage.kubernetesValues.Always')}</option>
            </Select>
          </Field>
          <Field hint={t('deploymentsPage.readOnlyRootFilesystemHint')} label={t('deploymentsPage.readOnlyRootFilesystem')}>
            <Select {...form.register('readOnlyRootFilesystem')}>
              <option value="false">{t('common.disabled')}</option>
              <option value="true">{t('common.enabled')}</option>
            </Select>
          </Field>
        </div>
        <div className="grid gap-3">
          <Field hint={t('deploymentsPage.capabilityDropHint')} label={t('deploymentsPage.capabilityDrop')}>
            <Textarea className="min-h-24" {...form.register('capabilityDrop')} placeholder={t('deploymentsPage.capabilityDropPlaceholder')} />
          </Field>
        </div>
      </div>

      <div className="grid gap-3 rounded-md border border-dashed border-border p-3">
        <p className="text-sm font-medium text-foreground">{t('deploymentsPage.kubernetesAdvancedScheduling')}</p>
        <Field hint={t('deploymentsPage.nodeSelectorHint')} label={t('deploymentsPage.nodeSelector')}>
          <Textarea className="min-h-24" {...form.register('nodeSelector')} placeholder={t('deploymentsPage.nodeSelectorPlaceholder')} />
        </Field>
        <div className="grid gap-3 md:grid-cols-3">
          <Field hint={t('deploymentsPage.tolerationsHint')} label={t('deploymentsPage.tolerations')}>
            <Textarea className="min-h-24" {...form.register('tolerations')} placeholder={t('deploymentsPage.tolerationsPlaceholder')} />
          </Field>
          <Field hint={t('deploymentsPage.affinityHint')} label={t('deploymentsPage.affinity')}>
            <Textarea className="min-h-24" {...form.register('affinity')} placeholder={t('deploymentsPage.affinityPlaceholder')} />
          </Field>
          <Field hint={t('deploymentsPage.topologySpreadConstraintsHint')} label={t('deploymentsPage.topologySpreadConstraints')}>
            <Textarea className="min-h-24" {...form.register('topologySpreadConstraints')} placeholder={t('deploymentsPage.topologySpreadConstraintsPlaceholder')} />
          </Field>
        </div>
      </div>

      <div className="grid gap-3 rounded-md border border-dashed border-border p-3">
        <p className="text-sm font-medium text-foreground">{t('deploymentsPage.kubernetesAdvancedService')}</p>
        <div className="grid gap-3">
          <Field hint={t('deploymentsPage.serviceSessionAffinityHint')} label={t('deploymentsPage.serviceSessionAffinity')}>
            <Select {...form.register('serviceSessionAffinity')}>
              <option value="">{t('deploymentsPage.kubernetesDefaultNone')}</option>
              <option value="None">{t('deploymentsPage.kubernetesValues.None')}</option>
              <option value="ClientIP">{t('deploymentsPage.kubernetesValues.ClientIP')}</option>
            </Select>
          </Field>
        </div>
        <Field hint={t('deploymentsPage.serviceAnnotationsHint')} label={t('deploymentsPage.serviceAnnotations')}>
          <Textarea className="min-h-24" {...form.register('serviceAnnotations')} placeholder={t('deploymentsPage.serviceAnnotationsPlaceholder')} />
        </Field>
      </div>

      <div className="grid gap-3 rounded-md border border-dashed border-border p-3">
        <p className="text-sm font-medium text-foreground">{t('deploymentsPage.kubernetesAdvancedMultiContainer')}</p>
        <div className="grid gap-3 md:grid-cols-2">
          <Field hint={t('deploymentsPage.initContainersHint')} label={t('deploymentsPage.initContainers')}>
            <Textarea className="min-h-24" {...form.register('initContainers')} placeholder={t('deploymentsPage.auxContainersPlaceholder')} />
          </Field>
          <Field hint={t('deploymentsPage.sidecarContainersHint')} label={t('deploymentsPage.sidecarContainers')}>
            <Textarea className="min-h-24" {...form.register('sidecarContainers')} placeholder={t('deploymentsPage.auxContainersPlaceholder')} />
          </Field>
        </div>
      </div>
    </div>
  )
}
