import type { ModelFormValues } from './ai-model-management-utils'
import type { AIModelConfig } from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Pencil, Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { ControlledCheckboxField } from '@/components/common/checkbox-field'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { DataList } from '@/components/common/data-list'
import { ErrorState } from '@/components/common/error-state'
import { FormField as Field } from '@/components/common/form-field'
import { StatusValueBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { findSuggestedModelPreset, listSuggestedModelPresets } from '@/lib/ai-model-suggested-prices'
import { aiModelFormSchema, emptyModel, modelDeleteErrorMessage, modelFormValues, modelSaveErrorMessage } from './ai-model-management-utils'

const suggestedPresets = listSuggestedModelPresets()
const presetDatalistId = 'ai-model-name-presets'

export function AIModelManagement() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [editing, setEditing] = useState<AIModelConfig | null>(null)
  const [deleting, setDeleting] = useState<AIModelConfig | null>(null)
  const [open, setOpen] = useState(false)
  const models = useQuery({ queryKey: ['configs', 'ai', 'models'], queryFn: api.listAIModelConfigs })
  const form = useForm<ModelFormValues>({ resolver: zodResolver(aiModelFormSchema), defaultValues: emptyModel, mode: 'onChange' })
  const save = useMutation({
    mutationFn: (values: ModelFormValues) => editing
      ? api.updateAIModel(editing.id, values)
      : api.createAIModel(values),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['configs', 'ai', 'models'] })
      await queryClient.invalidateQueries({ queryKey: ['ai', 'models'] })
      setOpen(false)
      toast.success(t('settings.ai.models.saved'))
    },
    onError: error => toast.error(modelSaveErrorMessage(error, t('settings.ai.models.saveFailed'), t)),
  })
  const remove = useMutation({
    mutationFn: (id: string) => api.deleteAIModel(id),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['configs', 'ai', 'models'] })
      await queryClient.invalidateQueries({ queryKey: ['ai', 'models'] })
      setDeleting(null)
      toast.success(t('settings.ai.models.deleted'))
    },
    onError: error => toast.error(modelDeleteErrorMessage(error, t('settings.ai.models.deleteFailed'), t)),
  })

  const startCreate = () => {
    setEditing(null)
    form.reset(emptyModel)
    setOpen(true)
  }
  const startEdit = (model: AIModelConfig) => {
    setEditing(model)
    form.reset(modelFormValues(model))
    setOpen(true)
  }
  const errors = form.formState.errors
  const watchedName = form.watch('name')
  const suggestedPreset = findSuggestedModelPreset(watchedName ?? '')
  const applySuggestedPreset = () => {
    if (!suggestedPreset) {
      return
    }
    form.setValue('name', suggestedPreset.displayName, { shouldValidate: true })
    form.setValue('maxContextTokens', suggestedPreset.maxContextTokens, { shouldValidate: true })
    form.setValue('maxOutputTokens', suggestedPreset.maxOutputTokens, { shouldValidate: true })
    form.setValue('inputCreditsPerMillion', suggestedPreset.prices.input, { shouldValidate: true })
    form.setValue('outputCreditsPerMillion', suggestedPreset.prices.output, { shouldValidate: true })
    form.setValue('cachedInputCreditsPerMillion', suggestedPreset.prices.cachedInput, { shouldValidate: true })
  }
  return (
    <div className="max-w-3xl">
      <div className="mt-6 grid gap-4">
        <p className="text-sm text-muted-foreground">{t('settings.ai.models.description')}</p>
        {models.isError
          ? <ErrorState title={t('settings.ai.models.loadFailed')} />
          : (
              <DataList
                columns={[
                  {
                    key: 'name',
                    header: t('settings.ai.models.name'),
                    render: model => (
                      <div className="min-w-0">
                        <p className="truncate font-medium">{model.name}</p>
                        <p className="text-xs text-muted-foreground">{t('settings.ai.models.priceSummary', { input: model.inputCreditsPerMillion, output: model.outputCreditsPerMillion })}</p>
                        <p className="text-xs text-muted-foreground">{t('settings.ai.models.capabilitySummary', { contextTokens: model.maxContextTokens, outputTokens: model.maxOutputTokens })}</p>
                      </div>
                    ),
                  },
                  {
                    key: 'status',
                    header: t('common.status'),
                    render: model => <StatusValueBadge labelKeyPrefix="settings.ai.models" value={model.enabled ? 'enabled' : 'disabled'} />,
                  },
                  {
                    key: 'actions',
                    header: t('common.actions'),
                    mobileActions: 'inline',
                    render: model => (
                      <div className="flex justify-end gap-1">
                        <Button aria-label={t('settings.ai.models.edit')} size="icon" type="button" variant="ghost" onClick={() => startEdit(model)}><Pencil className="size-4" /></Button>
                        <Button aria-label={t('settings.ai.models.delete')} size="icon" type="button" variant="ghost" onClick={() => setDeleting(model)}><Trash2 className="size-4" /></Button>
                      </div>
                    ),
                  },
                ]}
                emptyActions={(
                  <Button type="button" variant="outline" onClick={startCreate}>
                    <Plus className="size-4" />
                    {t('settings.ai.models.add')}
                  </Button>
                )}
                emptyTitle={t('settings.ai.models.empty')}
                items={models.data ?? []}
                loading={models.isLoading}
                rowKey={model => model.id}
                title={t('settings.ai.models.title')}
                toolbarActions={(
                  <Button type="button" onClick={startCreate}>
                    <Plus className="size-4" />
                    {t('settings.ai.models.add')}
                  </Button>
                )}
              />
            )}
      </div>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t(editing ? 'settings.ai.models.editTitle' : 'settings.ai.models.addTitle')}</DialogTitle>
            <DialogDescription>{t('settings.ai.models.formDescription')}</DialogDescription>
          </DialogHeader>
          <form className="grid gap-4" onSubmit={form.handleSubmit(values => save.mutate(values))}>
            <Field error={errors.name?.message} hint={t('settings.ai.models.namePresetHint')} label={t('settings.ai.models.name')} required>
              <Input list={presetDatalistId} {...form.register('name')} />
            </Field>
            <datalist id={presetDatalistId}>
              {suggestedPresets.map(preset => (
                <option key={preset.displayName} value={preset.displayName}>
                  {t('settings.ai.models.presetOption', { contextTokens: preset.maxContextTokens, outputTokens: preset.maxOutputTokens })}
                </option>
              ))}
            </datalist>
            {suggestedPreset && (
              <div className="flex flex-wrap items-center justify-between gap-2 rounded-control bg-surface-subtle px-4 py-3 text-sm">
                <span className="text-muted-foreground">
                  {t('settings.ai.models.suggestedPreset', {
                    contextTokens: suggestedPreset.maxContextTokens,
                    outputTokens: suggestedPreset.maxOutputTokens,
                    input: suggestedPreset.prices.input,
                    output: suggestedPreset.prices.output,
                    cachedInput: suggestedPreset.prices.cachedInput,
                  })}
                </span>
                <Button size="sm" type="button" variant="outline" onClick={applySuggestedPreset}>
                  {t('settings.ai.models.applySuggestedPreset')}
                </Button>
              </div>
            )}
            <div className="grid gap-4 sm:grid-cols-2">
              <Field error={errors.maxContextTokens?.message} hint={t('settings.ai.models.contextLimitHint')} label={t('settings.ai.models.contextLimit')} required><Input max={2097152} min={4096} step={1} type="number" {...form.register('maxContextTokens', { valueAsNumber: true })} /></Field>
              <Field error={errors.maxOutputTokens?.message} hint={t('settings.ai.models.outputLimitHint')} label={t('settings.ai.models.outputLimit')} required><Input max={262144} min={256} step={1} type="number" {...form.register('maxOutputTokens', { valueAsNumber: true })} /></Field>
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field error={errors.inputCreditsPerMillion?.message} hint={t('settings.ai.models.priceHint')} label={t('settings.ai.models.inputPrice')} required><Input inputMode="decimal" {...form.register('inputCreditsPerMillion')} /></Field>
              <Field error={errors.outputCreditsPerMillion?.message} hint={t('settings.ai.models.priceHint')} label={t('settings.ai.models.outputPrice')} required><Input inputMode="decimal" {...form.register('outputCreditsPerMillion')} /></Field>
              <Field error={errors.cachedInputCreditsPerMillion?.message} hint={t('settings.ai.models.priceHint')} label={t('settings.ai.models.cachedInputPrice')} required><Input inputMode="decimal" {...form.register('cachedInputCreditsPerMillion')} /></Field>
            </div>
            <Controller
              control={form.control}
              name="enabled"
              render={({ field }) => <ControlledCheckboxField field={field}>{t('settings.ai.models.enabled')}</ControlledCheckboxField>}
            />
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setOpen(false)}>{t('common.cancel')}</Button>
              <Button disabled={!form.formState.isValid || save.isPending} type="submit">{t('common.save')}</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <ConfirmDialog
        cancelText={t('common.cancel')}
        closeOnConfirm={false}
        confirmText={t('settings.ai.models.deleteConfirm')}
        description={t('settings.ai.models.deleteDescription', { name: deleting?.name ?? '' })}
        open={deleting !== null}
        pending={remove.isPending}
        title={t('settings.ai.models.deleteTitle')}
        onConfirm={() => deleting ? remove.mutateAsync(deleting.id) : undefined}
        onOpenChange={value => !value && setDeleting(null)}
      />
    </div>
  )
}
