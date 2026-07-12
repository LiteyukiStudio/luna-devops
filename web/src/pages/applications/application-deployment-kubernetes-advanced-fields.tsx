import type { UseFormReturn } from 'react-hook-form'
import type { DeploymentTargetPayload } from '@/api'
import { useTranslation } from 'react-i18next'
import { FormField as Field } from '@/components/common/form-field'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'

interface KubernetesAdvancedFieldsProps {
  dataRetentionEnabled: boolean
  form: UseFormReturn<DeploymentTargetPayload>
}

const textareaClassName = 'min-h-24 rounded-md border border-input bg-background px-3 py-2 text-sm outline-none transition focus-visible:border-primary/60 focus-visible:ring-2 focus-visible:ring-primary/20'

export function KubernetesAdvancedFields({ dataRetentionEnabled, form }: KubernetesAdvancedFieldsProps) {
  const { t } = useTranslation()

  return (
    <div className="grid gap-4">
      <div className="grid gap-3 rounded-md border border-dashed border-border p-3">
        <p className="text-sm font-medium text-foreground">{t('deploymentsPage.kubernetesAdvancedWorkload')}</p>
        <div className="grid gap-3 md:grid-cols-2">
          <Field hint={t('deploymentsPage.workloadTypeHint')} label={t('deploymentsPage.workloadType')}>
            <Select {...form.register('workloadType')}>
              <option value="Deployment">Deployment</option>
              <option value="StatefulSet">StatefulSet</option>
            </Select>
          </Field>
        </div>
      </div>

      <div className="grid gap-3 rounded-md border border-dashed border-border p-3">
        <p className="text-sm font-medium text-foreground">{t('deploymentsPage.kubernetesAdvancedContainer')}</p>
        <div className="grid gap-3 md:grid-cols-2">
          <Field hint={t('deploymentsPage.cpuLimitHint')} label={t('deploymentsPage.cpuLimit')}>
            <Input {...form.register('cpuLimit')} placeholder={t('deploymentsPage.cpuLimitPlaceholder')} />
          </Field>
          <Field hint={t('deploymentsPage.memoryLimitHint')} label={t('deploymentsPage.memoryLimit')}>
            <Input {...form.register('memoryLimit')} placeholder={t('deploymentsPage.memoryLimitPlaceholder')} />
          </Field>
          <Field hint={t('deploymentsPage.imagePullPolicyHint')} label={t('deploymentsPage.imagePullPolicy')}>
            <Select {...form.register('imagePullPolicy')}>
              <option value="">{t('deploymentsPage.kubernetesDefault')}</option>
              <option value="IfNotPresent">IfNotPresent</option>
              <option value="Always">Always</option>
              <option value="Never">Never</option>
            </Select>
          </Field>
          <Field hint={t('deploymentsPage.priorityClassNameHint')} label={t('deploymentsPage.priorityClassName')}>
            <Input {...form.register('priorityClassName')} placeholder={t('deploymentsPage.priorityClassNamePlaceholder')} />
          </Field>
        </div>
        <div className="grid gap-3 md:grid-cols-2">
          <Field hint={t('deploymentsPage.containerCommandHint')} label={t('deploymentsPage.containerCommand')}>
            <textarea className={textareaClassName} {...form.register('containerCommand')} placeholder={t('deploymentsPage.containerCommandPlaceholder')} />
          </Field>
          <Field hint={t('deploymentsPage.containerArgsHint')} label={t('deploymentsPage.containerArgs')}>
            <textarea className={textareaClassName} {...form.register('containerArgs')} placeholder={t('deploymentsPage.containerArgsPlaceholder')} />
          </Field>
        </div>
        <Field hint={t('deploymentsPage.lifecycleHint')} label={t('deploymentsPage.lifecycle')}>
          <textarea className={textareaClassName} {...form.register('lifecycle')} placeholder={t('deploymentsPage.lifecyclePlaceholder')} />
        </Field>
        <div className="grid gap-3 md:grid-cols-3">
          <Field hint={t('deploymentsPage.readinessProbeHint')} label={t('deploymentsPage.readinessProbe')}>
            <textarea className={textareaClassName} {...form.register('readinessProbe')} placeholder={t('deploymentsPage.probePlaceholder')} />
          </Field>
          <Field hint={t('deploymentsPage.livenessProbeHint')} label={t('deploymentsPage.livenessProbe')}>
            <textarea className={textareaClassName} {...form.register('livenessProbe')} placeholder={t('deploymentsPage.probePlaceholder')} />
          </Field>
          <Field hint={t('deploymentsPage.startupProbeHint')} label={t('deploymentsPage.startupProbe')}>
            <textarea className={textareaClassName} {...form.register('startupProbe')} placeholder={t('deploymentsPage.probePlaceholder')} />
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
            <Input {...form.register('autoScalingMinReplicas', { valueAsNumber: true })} min={1} type="number" />
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
          <textarea className={textareaClassName} {...form.register('autoScalingBehavior')} placeholder={t('deploymentsPage.autoScalingBehaviorPlaceholder')} />
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
              <option value="OnRootMismatch">OnRootMismatch</option>
              <option value="Always">Always</option>
            </Select>
          </Field>
          <Field hint={t('deploymentsPage.readOnlyRootFilesystemHint')} label={t('deploymentsPage.readOnlyRootFilesystem')}>
            <Select {...form.register('readOnlyRootFilesystem')}>
              <option value="false">{t('common.disabled')}</option>
              <option value="true">{t('common.enabled')}</option>
            </Select>
          </Field>
          <Field hint={t('deploymentsPage.allowPrivilegeEscalationHint')} label={t('deploymentsPage.allowPrivilegeEscalation')}>
            <Select {...form.register('allowPrivilegeEscalation')}>
              <option value="">{t('deploymentsPage.kubernetesDefault')}</option>
              <option value="false">{t('common.disabled')}</option>
              <option value="true">{t('common.enabled')}</option>
            </Select>
          </Field>
        </div>
        <div className="grid gap-3 md:grid-cols-2">
          <Field hint={t('deploymentsPage.capabilityAddHint')} label={t('deploymentsPage.capabilityAdd')}>
            <textarea className={textareaClassName} {...form.register('capabilityAdd')} placeholder={t('deploymentsPage.capabilityPlaceholder')} />
          </Field>
          <Field hint={t('deploymentsPage.capabilityDropHint')} label={t('deploymentsPage.capabilityDrop')}>
            <textarea className={textareaClassName} {...form.register('capabilityDrop')} placeholder={t('deploymentsPage.capabilityDropPlaceholder')} />
          </Field>
        </div>
      </div>

      <div className="grid gap-3 rounded-md border border-dashed border-border p-3">
        <p className="text-sm font-medium text-foreground">{t('deploymentsPage.kubernetesAdvancedScheduling')}</p>
        <Field hint={t('deploymentsPage.nodeSelectorHint')} label={t('deploymentsPage.nodeSelector')}>
          <textarea className={textareaClassName} {...form.register('nodeSelector')} placeholder={t('deploymentsPage.nodeSelectorPlaceholder')} />
        </Field>
        <div className="grid gap-3 md:grid-cols-3">
          <Field hint={t('deploymentsPage.tolerationsHint')} label={t('deploymentsPage.tolerations')}>
            <textarea className={textareaClassName} {...form.register('tolerations')} placeholder={t('deploymentsPage.tolerationsPlaceholder')} />
          </Field>
          <Field hint={t('deploymentsPage.affinityHint')} label={t('deploymentsPage.affinity')}>
            <textarea className={textareaClassName} {...form.register('affinity')} placeholder={t('deploymentsPage.affinityPlaceholder')} />
          </Field>
          <Field hint={t('deploymentsPage.topologySpreadConstraintsHint')} label={t('deploymentsPage.topologySpreadConstraints')}>
            <textarea className={textareaClassName} {...form.register('topologySpreadConstraints')} placeholder={t('deploymentsPage.topologySpreadConstraintsPlaceholder')} />
          </Field>
        </div>
      </div>

      <div className="grid gap-3 rounded-md border border-dashed border-border p-3">
        <p className="text-sm font-medium text-foreground">{t('deploymentsPage.kubernetesAdvancedService')}</p>
        <div className="grid gap-3 md:grid-cols-3">
          <Field hint={t('deploymentsPage.serviceTypeHint')} label={t('deploymentsPage.serviceType')}>
            <Select {...form.register('serviceType')}>
              <option value="">{t('deploymentsPage.kubernetesDefaultClusterIP')}</option>
              <option value="ClusterIP">ClusterIP</option>
              <option value="NodePort">NodePort</option>
              <option value="LoadBalancer">LoadBalancer</option>
            </Select>
          </Field>
          <Field hint={t('deploymentsPage.serviceExternalTrafficPolicyHint')} label={t('deploymentsPage.serviceExternalTrafficPolicy')}>
            <Select {...form.register('serviceExternalTrafficPolicy')}>
              <option value="">{t('deploymentsPage.kubernetesDefault')}</option>
              <option value="Cluster">Cluster</option>
              <option value="Local">Local</option>
            </Select>
          </Field>
          <Field hint={t('deploymentsPage.serviceSessionAffinityHint')} label={t('deploymentsPage.serviceSessionAffinity')}>
            <Select {...form.register('serviceSessionAffinity')}>
              <option value="">{t('deploymentsPage.kubernetesDefaultNone')}</option>
              <option value="None">None</option>
              <option value="ClientIP">ClientIP</option>
            </Select>
          </Field>
        </div>
        <Field hint={t('deploymentsPage.serviceAnnotationsHint')} label={t('deploymentsPage.serviceAnnotations')}>
          <textarea className={textareaClassName} {...form.register('serviceAnnotations')} placeholder={t('deploymentsPage.serviceAnnotationsPlaceholder')} />
        </Field>
      </div>

      {dataRetentionEnabled && (
        <div className="grid gap-3 rounded-md border border-dashed border-border p-3">
          <p className="text-sm font-medium text-foreground">{t('deploymentsPage.kubernetesAdvancedStorage')}</p>
          <div className="grid gap-3 md:grid-cols-3">
            <Field hint={t('deploymentsPage.dataStorageClassNameHint')} label={t('deploymentsPage.dataStorageClassName')}>
              <Input {...form.register('dataStorageClassName')} placeholder={t('deploymentsPage.dataStorageClassNamePlaceholder')} />
            </Field>
            <Field hint={t('deploymentsPage.dataAccessModeHint')} label={t('deploymentsPage.dataAccessMode')}>
              <Select {...form.register('dataAccessMode')}>
                <option value="">{t('deploymentsPage.kubernetesDefaultReadWriteOnce')}</option>
                <option value="ReadWriteOnce">ReadWriteOnce</option>
                <option value="ReadWriteMany">ReadWriteMany</option>
                <option value="ReadOnlyMany">ReadOnlyMany</option>
              </Select>
            </Field>
            <Field hint={t('deploymentsPage.dataVolumeModeHint')} label={t('deploymentsPage.dataVolumeMode')}>
              <Select {...form.register('dataVolumeMode')}>
                <option value="">{t('deploymentsPage.kubernetesDefaultFilesystem')}</option>
                <option value="Filesystem">Filesystem</option>
                <option value="Block">Block</option>
              </Select>
            </Field>
          </div>
        </div>
      )}

      <div className="grid gap-3 rounded-md border border-dashed border-border p-3">
        <p className="text-sm font-medium text-foreground">{t('deploymentsPage.kubernetesAdvancedMultiContainer')}</p>
        <div className="grid gap-3 md:grid-cols-2">
          <Field hint={t('deploymentsPage.initContainersHint')} label={t('deploymentsPage.initContainers')}>
            <textarea className={textareaClassName} {...form.register('initContainers')} placeholder={t('deploymentsPage.auxContainersPlaceholder')} />
          </Field>
          <Field hint={t('deploymentsPage.sidecarContainersHint')} label={t('deploymentsPage.sidecarContainers')}>
            <textarea className={textareaClassName} {...form.register('sidecarContainers')} placeholder={t('deploymentsPage.auxContainersPlaceholder')} />
          </Field>
        </div>
      </div>
    </div>
  )
}
