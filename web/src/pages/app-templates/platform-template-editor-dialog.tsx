import type { PlatformTemplateFormValues } from './platform-template-form'
import type { PlatformAppTemplate } from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { Controller, useFieldArray, useForm, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { ControlledCheckboxField } from '@/components/common/checkbox-field'
import { FormField as Field } from '@/components/common/form-field'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'
import {
  createPlatformTemplateFormSchema,
  defaultPlatformTemplateFormValues,
  emptyPlatformComponentFormValue,
  emptyPlatformParameterFormValue,
  PLATFORM_TEMPLATE_CATEGORIES,
  toPlatformTemplateWritePayload,
} from './platform-template-form'
import { templateCategoryLabel } from './template-text'

export function PlatformTemplateEditorDialog({
  open,
  template,
  onOpenChange,
}: {
  open: boolean
  template: PlatformAppTemplate | null
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const [saving, setSaving] = useState(false)

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (!saving)
          onOpenChange(nextOpen)
      }}
    >
      <DialogContent className="sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>
            {template ? t('appTemplatesPage.platformAdmin.editDialogTitle') : t('appTemplatesPage.platformAdmin.createDialogTitle')}
          </DialogTitle>
          <DialogDescription>
            {t('appTemplatesPage.platformAdmin.editorDialogDescription')}
          </DialogDescription>
        </DialogHeader>
        {open && (
          <PlatformTemplateEditorForm
            key={template?.id ?? 'create'}
            template={template}
            onCancel={() => onOpenChange(false)}
            onPendingChange={setSaving}
            onSaved={() => onOpenChange(false)}
          />
        )}
      </DialogContent>
    </Dialog>
  )
}

function PlatformTemplateEditorForm({
  template,
  onCancel,
  onPendingChange,
  onSaved,
}: {
  template: PlatformAppTemplate | null
  onCancel: () => void
  onPendingChange: (pending: boolean) => void
  onSaved: () => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [defaultValues] = useState(() => defaultPlatformTemplateFormValues(template))
  const form = useForm<PlatformTemplateFormValues>({
    defaultValues,
    resolver: zodResolver(createPlatformTemplateFormSchema(t)),
  })
  const { control, formState: { errors }, register, setValue } = form
  const workloadMode = useWatch({ control, name: 'workloadMode' })
  const watchedValues = useWatch({ control, name: 'values' })

  const componentsFieldArray = useFieldArray({ control, name: 'components' })
  const parametersFieldArray = useFieldArray({ control, name: 'values' })

  const saveMutation = useMutation({
    mutationFn: async (values: PlatformTemplateFormValues) => {
      const payload = toPlatformTemplateWritePayload(values)
      if (template)
        return api.updatePlatformAppTemplate(template.id, payload)
      return api.createPlatformAppTemplate(payload)
    },
    onMutate: () => onPendingChange(true),
    onSettled: () => onPendingChange(false),
    onSuccess: async () => {
      toast.success(template ? t('appTemplatesPage.platformAdmin.updatedToast') : t('appTemplatesPage.platformAdmin.createdToast'))
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['app-templates'] }),
        queryClient.invalidateQueries({ queryKey: ['app-template'] }),
        queryClient.invalidateQueries({ queryKey: ['platform-app-templates'] }),
      ])
      onSaved()
    },
    onError: error => toast.error(error.message),
  })

  const pending = saveMutation.isPending
  const submit = form.handleSubmit(values => saveMutation.mutate(values))

  return (
    <form className="grid gap-5" noValidate onSubmit={submit}>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field
          error={errors.slug?.message}
          hint={t('appTemplatesPage.platformAdmin.slugHint')}
          label={t('appTemplatesPage.platformAdmin.slug')}
          required
        >
          <Input disabled={pending} placeholder="my-app" {...register('slug')} />
        </Field>
        <Field error={errors.name?.message} label={t('appTemplatesPage.platformAdmin.name')} required>
          <Input disabled={pending} {...register('name')} />
        </Field>
      </div>

      <Field error={errors.description?.message} label={t('appTemplatesPage.platformAdmin.description')}>
        <Textarea className="min-h-16" disabled={pending} rows={2} {...register('description')} />
      </Field>

      <div className="grid gap-4 sm:grid-cols-4">
        <Field label={t('appTemplatesPage.platformAdmin.category')} required>
          <Select disabled={pending} {...register('category')}>
            {PLATFORM_TEMPLATE_CATEGORIES.map(category => (
              <option key={category} value={category}>{templateCategoryLabel(category, t)}</option>
            ))}
          </Select>
        </Field>
        <Field label={t('appTemplatesPage.platformAdmin.version')}>
          <Input disabled={pending} placeholder="1.0" {...register('version')} />
        </Field>
        <Field label={t('appTemplatesPage.platformAdmin.popularityWeight')}>
          <Input disabled={pending} min={0} type="number" {...register('popularityWeight', { valueAsNumber: true })} />
        </Field>
        <Field label={t('appTemplatesPage.platformAdmin.icon')}>
          <Input disabled={pending} placeholder="/app-templates/icons/fallback.svg" {...register('icon')} />
        </Field>
      </div>

      <div className="grid gap-4 sm:grid-cols-2">
        <Field error={errors.officialWebsite?.message} label={t('appTemplatesPage.platformAdmin.officialWebsite')}>
          <Input disabled={pending} placeholder="https://example.com" {...register('officialWebsite')} />
        </Field>
        <Field error={errors.officialRepository?.message} label={t('appTemplatesPage.platformAdmin.officialRepository')}>
          <Input disabled={pending} placeholder="https://github.com/org/repo" {...register('officialRepository')} />
        </Field>
      </div>

      <div className="grid gap-3 rounded-container border border-border bg-surface-subtle/50 p-3 sm:grid-cols-2">
        <Controller
          control={control}
          name="enabled"
          render={({ field }) => (
            <ControlledCheckboxField
              description={t('appTemplatesPage.platformAdmin.enabledDescription')}
              disabled={pending}
              field={field}
            >
              {t('appTemplatesPage.platformAdmin.enabledLabel')}
            </ControlledCheckboxField>
          )}
        />
        <Controller
          control={control}
          name="featured"
          render={({ field }) => (
            <ControlledCheckboxField
              description={t('appTemplatesPage.platformAdmin.featuredDescription')}
              disabled={pending}
              field={field}
            >
              {t('appTemplatesPage.platformAdmin.featuredLabel')}
            </ControlledCheckboxField>
          )}
        />
      </div>

      <div className="grid gap-4 border-t border-border pt-4">
        <Field label={t('appTemplatesPage.platformAdmin.workloadMode')}>
          <Select disabled={pending} {...register('workloadMode')}>
            <option value="single">{t('appTemplatesPage.platformAdmin.modeSingle')}</option>
            <option value="multi">{t('appTemplatesPage.platformAdmin.modeMulti')}</option>
          </Select>
        </Field>

        {workloadMode === 'single'
          ? (
              <div className="grid gap-4 rounded-container border border-border bg-surface-raised p-4">
                <div className="grid gap-4 sm:grid-cols-3">
                  <Field className="sm:col-span-2" error={errors.image?.message} label={t('appTemplatesPage.imageRef')} required>
                    <Input disabled={pending} placeholder="nginx:1.27-alpine" {...register('image')} />
                  </Field>
                  <Field error={errors.servicePort?.message} label={t('appTemplatesPage.platformAdmin.servicePort')} required>
                    <Input disabled={pending} max={65535} min={1} type="number" {...register('servicePort', { valueAsNumber: true })} />
                  </Field>
                </div>
                <div className="grid gap-4 sm:grid-cols-4">
                  <Field error={errors.defaultReplicas?.message} label={t('appTemplatesPage.replicas')}>
                    <Input disabled={pending} min={1} type="number" {...register('defaultReplicas', { valueAsNumber: true })} />
                  </Field>
                  <Field label={t('appTemplatesPage.cpu')}>
                    <Input disabled={pending} placeholder="250m" {...register('defaultCPU')} />
                  </Field>
                  <Field label={t('appTemplatesPage.memory')}>
                    <Input disabled={pending} placeholder="256Mi" {...register('defaultMemory')} />
                  </Field>
                  <Field
                    hint={t('appTemplatesPage.platformAdmin.persistentMountPathHint')}
                    label={t('appTemplatesPage.platformAdmin.persistentMountPath')}
                  >
                    <Input disabled={pending} placeholder="/data" {...register('persistentMountPath')} />
                  </Field>
                </div>
                <div className="grid gap-4 sm:grid-cols-2">
                  <Field label={t('appTemplatesPage.platformAdmin.containerCommand')}>
                    <Input disabled={pending} {...register('containerCommand')} />
                  </Field>
                  <Field label={t('appTemplatesPage.platformAdmin.containerArgs')}>
                    <Input disabled={pending} {...register('containerArgs')} />
                  </Field>
                </div>
                <div className="grid gap-4 sm:grid-cols-2">
                  <Field
                    error={errors.envLines?.message}
                    hint={t('appTemplatesPage.platformAdmin.envLinesHint')}
                    label={t('appTemplatesPage.platformAdmin.envLines')}
                  >
                    <Textarea className="font-mono text-xs" disabled={pending} placeholder="NODE_ENV=production" rows={3} {...register('envLines')} />
                  </Field>
                  <Field
                    error={errors.secretEnvLines?.message}
                    hint={t('appTemplatesPage.platformAdmin.secretEnvLinesHint')}
                    label={t('appTemplatesPage.platformAdmin.secretEnvLines')}
                  >
                    <Textarea className="font-mono text-xs" disabled={pending} placeholder="APP_SECRET={{appSecret}}" rows={3} {...register('secretEnvLines')} />
                  </Field>
                </div>
              </div>
            )
          : (
              <div className="grid gap-3">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <span className="text-sm font-medium text-foreground">{t('appTemplatesPage.components')}</span>
                  <Button
                    disabled={pending}
                    size="sm"
                    type="button"
                    variant="outline"
                    onClick={() => componentsFieldArray.append(emptyPlatformComponentFormValue(componentsFieldArray.fields.length + 1))}
                  >
                    <Plus className="size-3.5" />
                    {t('appTemplatesPage.platformAdmin.addComponent')}
                  </Button>
                </div>
                {errors.components?.root?.message && (
                  <p className="text-xs text-danger">{errors.components.root.message}</p>
                )}
                <div className="grid gap-3">
                  {componentsFieldArray.fields.map((item, index) => (
                    <div key={item.id} className="grid gap-3 rounded-container border border-border bg-surface-raised p-4">
                      <div className="flex items-center justify-between gap-2">
                        <span className="text-xs font-medium text-muted-foreground">{`#${index + 1}`}</span>
                        {componentsFieldArray.fields.length > 2 && (
                          <Button
                            aria-label={t('appTemplatesPage.platformAdmin.removeComponent')}
                            className="size-7"
                            disabled={pending}
                            size="icon"
                            type="button"
                            variant="ghost"
                            onClick={() => componentsFieldArray.remove(index)}
                          >
                            <Trash2 className="size-3.5 text-danger" />
                          </Button>
                        )}
                      </div>
                      <div className="grid gap-3 sm:grid-cols-4">
                        <Field
                          error={errors.components?.[index]?.key?.message}
                          label={t('appTemplatesPage.platformAdmin.componentKey')}
                          required
                        >
                          <Input disabled={pending} placeholder="web" {...register(`components.${index}.key`)} />
                        </Field>
                        <Field
                          error={errors.components?.[index]?.name?.message}
                          label={t('appTemplatesPage.platformAdmin.componentName')}
                          required
                        >
                          <Input disabled={pending} placeholder="Frontend" {...register(`components.${index}.name`)} />
                        </Field>
                        <Field
                          error={errors.components?.[index]?.image?.message}
                          label={t('appTemplatesPage.imageRef')}
                          required
                        >
                          <Input disabled={pending} placeholder="nginx:1.27-alpine" {...register(`components.${index}.image`)} />
                        </Field>
                        <Field
                          error={errors.components?.[index]?.servicePort?.message}
                          label={t('appTemplatesPage.platformAdmin.servicePort')}
                          required
                        >
                          <Input disabled={pending} max={65535} min={1} type="number" {...register(`components.${index}.servicePort`, { valueAsNumber: true })} />
                        </Field>
                      </div>
                      <div className="grid gap-3 sm:grid-cols-4">
                        <Field error={errors.components?.[index]?.defaultReplicas?.message} label={t('appTemplatesPage.replicas')}>
                          <Input disabled={pending} min={1} type="number" {...register(`components.${index}.defaultReplicas`, { valueAsNumber: true })} />
                        </Field>
                        <Field label={t('appTemplatesPage.cpu')}>
                          <Input disabled={pending} {...register(`components.${index}.defaultCPU`)} />
                        </Field>
                        <Field label={t('appTemplatesPage.memory')}>
                          <Input disabled={pending} {...register(`components.${index}.defaultMemory`)} />
                        </Field>
                        <Field label={t('appTemplatesPage.platformAdmin.persistentMountPath')}>
                          <Input disabled={pending} placeholder="/var/lib/postgresql/data" {...register(`components.${index}.persistentMountPath`)} />
                        </Field>
                      </div>
                      <div className="grid gap-3 sm:grid-cols-2">
                        <Field label={t('appTemplatesPage.platformAdmin.containerCommand')}>
                          <Input disabled={pending} {...register(`components.${index}.containerCommand`)} />
                        </Field>
                        <Field label={t('appTemplatesPage.platformAdmin.containerArgs')}>
                          <Input disabled={pending} {...register(`components.${index}.containerArgs`)} />
                        </Field>
                      </div>
                      <div className="grid gap-3 sm:grid-cols-2">
                        <Field
                          error={errors.components?.[index]?.envLines?.message}
                          hint={t('appTemplatesPage.platformAdmin.envLinesHint')}
                          label={t('appTemplatesPage.platformAdmin.envLines')}
                        >
                          <Textarea className="font-mono text-xs" disabled={pending} rows={2} {...register(`components.${index}.envLines`)} />
                        </Field>
                        <Field
                          error={errors.components?.[index]?.secretEnvLines?.message}
                          hint={t('appTemplatesPage.platformAdmin.secretEnvLinesHint')}
                          label={t('appTemplatesPage.platformAdmin.secretEnvLines')}
                        >
                          <Textarea className="font-mono text-xs" disabled={pending} rows={2} {...register(`components.${index}.secretEnvLines`)} />
                        </Field>
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            )}
      </div>

      <div className="grid gap-3 border-t border-border pt-4">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <span className="text-sm font-medium text-foreground">{t('appTemplatesPage.templateParameters')}</span>
          <Button
            disabled={pending}
            size="sm"
            type="button"
            variant="outline"
            onClick={() => parametersFieldArray.append(emptyPlatformParameterFormValue())}
          >
            <Plus className="size-3.5" />
            {t('appTemplatesPage.platformAdmin.addParameter')}
          </Button>
        </div>
        {parametersFieldArray.fields.length === 0
          ? (
              <p className="text-xs text-muted-foreground">{t('appTemplatesPage.platformAdmin.parametersEmpty')}</p>
            )
          : (
              <div className="grid gap-3">
                {parametersFieldArray.fields.map((item, index) => {
                  const isSecret = Boolean(watchedValues?.[index]?.secret)
                  return (
                    <div key={item.id} className="grid gap-3 rounded-container border border-border bg-surface-raised p-3">
                      <div className="flex items-start gap-3">
                        <div className="grid min-w-0 flex-1 gap-3 sm:grid-cols-4">
                          <Field
                            error={errors.values?.[index]?.key?.message}
                            label={t('appTemplatesPage.platformAdmin.paramKey')}
                            required
                          >
                            <Input disabled={pending} placeholder="databasePassword" {...register(`values.${index}.key`)} />
                          </Field>
                          <Field label={t('appTemplatesPage.platformAdmin.paramLabel')}>
                            <Input disabled={pending} {...register(`values.${index}.label`)} />
                          </Field>
                          <Field label={t('appTemplatesPage.platformAdmin.paramDescription')}>
                            <Input disabled={pending} {...register(`values.${index}.description`)} />
                          </Field>
                          <Field
                            error={errors.values?.[index]?.defaultValue?.message}
                            label={t('appTemplatesPage.platformAdmin.paramDefault')}
                          >
                            <Input
                              disabled={pending || isSecret}
                              placeholder={isSecret ? t('appTemplatesPage.autoGeneratePlaceholder') : ''}
                              {...register(`values.${index}.defaultValue`)}
                            />
                          </Field>
                        </div>
                        <Button
                          aria-label={t('appTemplatesPage.platformAdmin.removeParameter')}
                          className="mt-6 size-7"
                          disabled={pending}
                          size="icon"
                          type="button"
                          variant="ghost"
                          onClick={() => parametersFieldArray.remove(index)}
                        >
                          <Trash2 className="size-3.5 text-danger" />
                        </Button>
                      </div>
                      <div className="flex flex-wrap items-center gap-5">
                        <Controller
                          control={control}
                          name={`values.${index}.required`}
                          render={({ field }) => (
                            <ControlledCheckboxField disabled={pending} field={field}>
                              {t('appTemplatesPage.platformAdmin.paramRequired')}
                            </ControlledCheckboxField>
                          )}
                        />
                        <Controller
                          control={control}
                          name={`values.${index}.secret`}
                          render={({ field }) => (
                            <ControlledCheckboxField
                              disabled={pending}
                              field={{
                                ...field,
                                onChange: (checked) => {
                                  field.onChange(checked)
                                  if (checked)
                                    setValue(`values.${index}.defaultValue`, '')
                                  else
                                    setValue(`values.${index}.autoGenerate`, false)
                                },
                              }}
                            >
                              {t('appTemplatesPage.platformAdmin.paramSecret')}
                            </ControlledCheckboxField>
                          )}
                        />
                        {isSecret && (
                          <Controller
                            control={control}
                            name={`values.${index}.autoGenerate`}
                            render={({ field }) => (
                              <ControlledCheckboxField disabled={pending} field={field}>
                                {t('appTemplatesPage.platformAdmin.paramAutoGenerate')}
                              </ControlledCheckboxField>
                            )}
                          />
                        )}
                      </div>
                    </div>
                  )
                })}
              </div>
            )}
      </div>

      <DialogFooter>
        <Button disabled={pending} type="button" variant="outline" onClick={onCancel}>
          {t('common.cancel')}
        </Button>
        <Button disabled={pending} type="submit">
          {pending ? t('appTemplatesPage.platformAdmin.saving') : t('appTemplatesPage.platformAdmin.save')}
        </Button>
      </DialogFooter>
    </form>
  )
}
