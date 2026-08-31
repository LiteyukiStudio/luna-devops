import type { TFunction } from 'i18next'
import type { Control } from 'react-hook-form'
import type { ClusterKubeGatewayFormValues } from './cluster-kube-gateway-form'
import { Plus, Trash2 } from 'lucide-react'
import { Controller, useFieldArray } from 'react-hook-form'
import { CheckboxField } from '@/components/common/checkbox-field'
import { FormField as Field } from '@/components/common/form-field'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
import {
  createEmptyClusterKubeGatewayRule,
  kubeGatewayActionOptions,
  kubeGatewayVerbOptions,
} from './cluster-kube-gateway-form'

export function ClusterKubeGatewayRulesEditor({
  control,
  disabled,
  t,
}: {
  control: Control<ClusterKubeGatewayFormValues>
  disabled?: boolean
  t: TFunction
}) {
  const rules = useFieldArray({
    control,
    name: 'extraResourceRules',
  })

  return (
    <div className="grid gap-3">
      {rules.fields.length === 0 && (
        <div className="rounded-container border border-dashed border-border bg-surface-inset p-4 text-sm text-muted-foreground">
          {t('kubectlAccess.gatewayRules.empty')}
        </div>
      )}
      {rules.fields.map((field, index) => (
        <div key={field.id} className="grid gap-3 rounded-container border border-border bg-surface-inset p-4">
          <div className="flex items-center justify-between gap-3">
            <p className="font-medium">{t('kubectlAccess.gatewayRules.ruleLabel', { index: index + 1 })}</p>
            <Button disabled={disabled} size="sm" type="button" variant="ghost" onClick={() => rules.remove(index)}>
              <Trash2 className="size-4" />
              {t('common.remove')}
            </Button>
          </div>
          <div className="grid gap-3 md:grid-cols-2">
            <Field label={t('kubectlAccess.gatewayRules.apiGroup')} required>
              <Controller
                control={control}
                name={`extraResourceRules.${index}.apiGroup`}
                render={({ field: controllerField }) => (
                  <Input {...controllerField} disabled={disabled} placeholder={t('kubectlAccess.gatewayRules.apiGroupPlaceholder')} />
                )}
              />
            </Field>
            <Field label={t('kubectlAccess.gatewayRules.apiVersion')} required>
              <Controller
                control={control}
                name={`extraResourceRules.${index}.apiVersion`}
                render={({ field: controllerField }) => (
                  <Input {...controllerField} disabled={disabled} placeholder={t('kubectlAccess.gatewayRules.apiVersionPlaceholder')} />
                )}
              />
            </Field>
            <Field label={t('kubectlAccess.gatewayRules.resource')} required>
              <Controller
                control={control}
                name={`extraResourceRules.${index}.resource`}
                render={({ field: controllerField }) => (
                  <Input {...controllerField} disabled={disabled} placeholder={t('kubectlAccess.gatewayRules.resourcePlaceholder')} />
                )}
              />
            </Field>
            <Field hint={t('kubectlAccess.gatewayRules.subresourcesHint')} label={t('kubectlAccess.gatewayRules.subresources')}>
              <Controller
                control={control}
                name={`extraResourceRules.${index}.subresourcesText`}
                render={({ field: controllerField }) => (
                  <Input {...controllerField} disabled={disabled} placeholder={t('kubectlAccess.gatewayRules.subresourcesPlaceholder')} />
                )}
              />
            </Field>
            <Field label={t('kubectlAccess.gatewayRules.action')} required>
              <Controller
                control={control}
                name={`extraResourceRules.${index}.action`}
                render={({ field: controllerField }) => (
                  <Select {...controllerField} disabled={disabled}>
                    {kubeGatewayActionOptions.map(action => (
                      <option key={action} value={action}>{t(`kubectlAccess.gatewayActions.${action}`)}</option>
                    ))}
                  </Select>
                )}
              />
            </Field>
          </div>
          <Field hint={t('kubectlAccess.gatewayRules.verbsHint')} label={t('kubectlAccess.gatewayRules.verbs')} required>
            <Controller
              control={control}
              name={`extraResourceRules.${index}.verbs`}
              render={({ field: controllerField }) => (
                <div className="grid gap-2 rounded-md border border-border bg-card p-3 sm:grid-cols-2">
                  {kubeGatewayVerbOptions.map(verb => (
                    <CheckboxField
                      key={verb}
                      checked={(controllerField.value ?? []).includes(verb)}
                      disabled={disabled}
                      onCheckedChange={(checked) => {
                        const current = new Set(controllerField.value ?? [])
                        if (checked === true)
                          current.add(verb)
                        else
                          current.delete(verb)
                        controllerField.onChange([...current])
                      }}
                    >
                      {t(`kubectlAccess.gatewayVerbs.${verb}`)}
                    </CheckboxField>
                  ))}
                </div>
              )}
            />
          </Field>
        </div>
      ))}
      <div className="flex justify-start">
        <Button disabled={disabled || rules.fields.length >= 50} size="sm" type="button" variant="outline" onClick={() => rules.append(createEmptyClusterKubeGatewayRule())}>
          <Plus className="size-4" />
          {t('kubectlAccess.gatewayRules.add')}
        </Button>
      </div>
    </div>
  )
}
