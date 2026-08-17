import type { AIModelConfig } from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import i18next from 'i18next'
import { Pencil, Plus } from 'lucide-react'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'
import { api, ApiError } from '@/api'
import { FormField as Field } from '@/components/common/form-field'
import { StatusBadge } from '@/components/common/status-badge'
import { Surface } from '@/components/common/surface'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'

const price = () => z.string().trim().regex(/^\d+(?:\.\d{1,8})?$/, { message: i18next.t('settings.ai.models.priceInvalid') })
const modelSchema = z.object({
  name: z.string().trim().min(1, { message: i18next.t('settings.ai.models.nameRequired') }),
  inputCreditsPerMillion: price(),
  outputCreditsPerMillion: price(),
  cachedInputCreditsPerMillion: price(),
  cachedOutputCreditsPerMillion: price(),
  enabled: z.boolean(),
})
type ModelFormValues = z.infer<typeof modelSchema>

function modelSaveErrorMessage(error: unknown, fallback: string, translate: (key: string) => string) {
  if (!(error instanceof ApiError))
    return fallback
  switch (error.code) {
    case 'ai.model_name_conflict':
      return translate('settings.ai.models.errors.nameConflict')
    case 'ai.last_model_cannot_be_disabled':
      return translate('settings.ai.models.errors.lastEnabled')
    case 'ai.model_not_found':
      return translate('settings.ai.models.errors.notFound')
    default:
      return fallback
  }
}

const emptyModel: ModelFormValues = {
  name: '',
  inputCreditsPerMillion: '0',
  outputCreditsPerMillion: '0',
  cachedInputCreditsPerMillion: '0',
  cachedOutputCreditsPerMillion: '0',
  enabled: true,
}

export function AIModelManagement() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [editing, setEditing] = useState<AIModelConfig | null>(null)
  const [open, setOpen] = useState(false)
  const models = useQuery({ queryKey: ['configs', 'ai', 'models'], queryFn: api.listAIModelConfigs })
  const form = useForm<ModelFormValues>({ resolver: zodResolver(modelSchema), defaultValues: emptyModel, mode: 'onChange' })
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

  const startCreate = () => {
    setEditing(null)
    form.reset(emptyModel)
    setOpen(true)
  }
  const startEdit = (model: AIModelConfig) => {
    setEditing(model)
    form.reset({
      name: model.name,
      inputCreditsPerMillion: model.inputCreditsPerMillion,
      outputCreditsPerMillion: model.outputCreditsPerMillion,
      cachedInputCreditsPerMillion: model.cachedInputCreditsPerMillion,
      cachedOutputCreditsPerMillion: model.cachedOutputCreditsPerMillion,
      enabled: model.enabled,
    })
    setOpen(true)
  }
  const errors = form.formState.errors
  return (
    <>
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
                </div>
                <StatusBadge tone={model.enabled ? 'success' : 'neutral'}>{t(model.enabled ? 'settings.ai.models.enabled' : 'settings.ai.models.disabled')}</StatusBadge>
                <Button aria-label={t('settings.ai.models.edit')} size="icon" type="button" variant="ghost" onClick={() => startEdit(model)}><Pencil className="size-4" /></Button>
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
            <Field error={errors.name?.message} label={t('settings.ai.models.name')} required><Input {...form.register('name')} /></Field>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field error={errors.inputCreditsPerMillion?.message} hint={t('settings.ai.models.priceHint')} label={t('settings.ai.models.inputPrice')} required><Input inputMode="decimal" {...form.register('inputCreditsPerMillion')} /></Field>
              <Field error={errors.outputCreditsPerMillion?.message} hint={t('settings.ai.models.priceHint')} label={t('settings.ai.models.outputPrice')} required><Input inputMode="decimal" {...form.register('outputCreditsPerMillion')} /></Field>
              <Field error={errors.cachedInputCreditsPerMillion?.message} hint={t('settings.ai.models.priceHint')} label={t('settings.ai.models.cachedInputPrice')} required><Input inputMode="decimal" {...form.register('cachedInputCreditsPerMillion')} /></Field>
              <Field error={errors.cachedOutputCreditsPerMillion?.message} hint={t('settings.ai.models.priceHint')} label={t('settings.ai.models.cachedOutputPrice')} required><Input inputMode="decimal" {...form.register('cachedOutputCreditsPerMillion')} /></Field>
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
    </>
  )
}
