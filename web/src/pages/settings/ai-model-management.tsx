import type { ModelFormValues } from './ai-model-management-utils'
import type { AIModelConfig } from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Pencil, Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { FormField as Field } from '@/components/common/form-field'
import { StatusBadge } from '@/components/common/status-badge'
import { Surface } from '@/components/common/surface'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog'
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
      <Surface className="mt-6 grid gap-4 rounded-xl p-6" variant="bordered">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="grid gap-1">
            <h2 className="text-base font-semibold">{t('settings.ai.models.title')}</h2>
            <p className="text-sm text-muted-foreground">{t('settings.ai.models.description')}</p>
          </div>
          <Button type="button" onClick={startCreate}>
            <Plus className="size-4" />
            {t('settings.ai.models.add')}
          </Button>
        </div>
        {models.isError && <p className="text-sm text-destructive">{t('settings.ai.models.loadFailed')}</p>}
        {!models.isLoading && models.data?.length === 0 && <p className="rounded-control bg-surface-subtle p-4 text-sm text-muted-foreground">{t('settings.ai.models.empty')}</p>}
        {models.data && models.data.length > 0 && (
          <div className="grid gap-2">
            {models.data.map(model => (
              <div className="flex flex-wrap items-center gap-3 rounded-control bg-surface-subtle px-4 py-3" key={model.id}>
                <div className="min-w-0 flex-1">
                  <p className="truncate font-medium">{model.name}</p>
                  <p className="text-xs text-muted-foreground">{t('settings.ai.models.priceSummary', { input: model.inputCreditsPerMillion, output: model.outputCreditsPerMillion })}</p>
                  <p className="text-xs text-muted-foreground">{t('settings.ai.models.capabilitySummary', { contextTokens: model.maxContextTokens, outputTokens: model.maxOutputTokens })}</p>
                </div>
                <StatusBadge tone={model.enabled ? 'success' : 'neutral'}>{t(model.enabled ? 'settings.ai.models.enabled' : 'settings.ai.models.disabled')}</StatusBadge>
                <Button aria-label={t('settings.ai.models.edit')} size="icon" type="button" variant="ghost" onClick={() => startEdit(model)}><Pencil className="size-4" /></Button>
                <Button aria-label={t('settings.ai.models.delete')} size="icon" type="button" variant="ghost" onClick={() => setDeleting(model)}><Trash2 className="size-4" /></Button>
              </div>
            ))}
          </div>
        )}
      </Surface>
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
            <label className="flex items-center gap-2 text-sm">
              <input type="checkbox" {...form.register('enabled')} />
              {t('settings.ai.models.enabled')}
            </label>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setOpen(false)}>{t('common.cancel')}</Button>
              <Button disabled={!form.formState.isValid || save.isPending} type="submit">{t('common.save')}</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <AlertDialog open={deleting !== null} onOpenChange={value => !value && setDeleting(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('settings.ai.models.deleteTitle')}</AlertDialogTitle>
            <AlertDialogDescription>{t('settings.ai.models.deleteDescription', { name: deleting?.name ?? '' })}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction disabled={remove.isPending} onClick={() => deleting && remove.mutate(deleting.id)}>
              {t('settings.ai.models.deleteConfirm')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
