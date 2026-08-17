import type { DeploymentTargetRuntimeSecretsSummary } from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { KeyRound, RefreshCw, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api'
import { FormField as Field } from '@/components/common/form-field'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

const replaceSchema = z.object({ key: z.string().trim().min(1), value: z.string().min(1) })
type ReplaceForm = z.infer<typeof replaceSchema>

export function ApplicationDeploymentRuntimeSecretsEditor({ applicationId, canManage, open, projectId, targetId }: {
  applicationId: string
  canManage: boolean
  open: boolean
  projectId: string
  targetId?: string
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [editingKey, setEditingKey] = useState<string | null>(null)
  const replaceForm = useForm<ReplaceForm>({ resolver: zodResolver(replaceSchema), defaultValues: { key: '', value: '' } })
  const summary = useQuery({
    queryKey: ['deployment-target-runtime-secrets', projectId, applicationId, targetId],
    queryFn: () => api.getDeploymentTargetRuntimeSecretsSummary(projectId, applicationId, targetId!),
    enabled: Boolean(open && targetId),
  })

  const clearEditorState = () => {
    setEditingKey(null)
    replaceForm.reset({ key: '', value: '' })
  }

  const update = useMutation({
    mutationFn: (payload: { values?: Record<string, string>, generate?: Record<string, { length: number, encoding: 'base64' }>, clear?: string[] }) =>
      api.updateDeploymentTargetRuntimeSecrets(projectId, applicationId, targetId!, payload),
    onSuccess: () => {
      clearEditorState()
      void queryClient.invalidateQueries({ queryKey: ['deployment-target-runtime-secrets', projectId, applicationId, targetId] })
      toast.success(t('deploymentsPage.runtimeSecretsUpdated'))
    },
    onError: error => toast.error(error instanceof Error ? error.message : t('deploymentsPage.runtimeSecretsUpdateFailed')),
  })

  if (!targetId)
    return <p className="text-sm text-muted-foreground">{t('deploymentsPage.runtimeSecretsSaveTargetFirst')}</p>

  const keys = (summary.data as DeploymentTargetRuntimeSecretsSummary | undefined)?.secretKeys ?? []
  const submitReplace = replaceForm.handleSubmit(({ key, value }) => {
    const normalizedKey = editingKey === '__new__' ? key.trim() : (editingKey ?? key.trim())
    if (normalizedKey)
      update.mutate({ values: { [normalizedKey]: value } })
  })

  return (
    <div className="grid gap-3 rounded-control bg-surface-inset/60 p-4" data-testid="runtime-secrets-editor">
      <div className="flex items-start gap-3">
        <KeyRound className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
        <div className="min-w-0">
          <p className="text-sm font-medium">{t('deploymentsPage.runtimeSecretsTitle')}</p>
          <p className="text-sm text-muted-foreground">{t(canManage ? 'deploymentsPage.runtimeSecretsManageHint' : 'deploymentsPage.runtimeSecretsRoleHint')}</p>
        </div>
      </div>
      {summary.isLoading && <p className="text-sm text-muted-foreground">{t('common.loading')}</p>}
      {summary.isError && <p className="text-sm text-destructive">{t('deploymentsPage.runtimeSecretsLoadFailed')}</p>}
      {!summary.isLoading && !summary.isError && keys.length === 0 && <p className="text-sm text-muted-foreground">{t('deploymentsPage.runtimeSecretsEmpty')}</p>}
      {keys.map(key => (
        <div className="grid gap-3 rounded-control bg-background p-3 md:grid-cols-[minmax(0,1fr)_auto] md:items-center" key={key}>
          <div className="min-w-0">
            <p className="truncate text-sm font-medium" title={key}>{key}</p>
            <p className="font-mono text-sm text-muted-foreground" data-testid={`runtime-secret-value-${key}`}>{t('common.secretValueMasked')}</p>
          </div>
          <div className="flex flex-wrap gap-2">
            {canManage && (
              <>
                <Button
                  size="sm"
                  type="button"
                  variant="outline"
                  onClick={() => {
                    setEditingKey(key)
                    replaceForm.reset({ key, value: '' })
                  }}
                >
                  {t('deploymentsPage.runtimeSecretReplace')}
                </Button>
                <Button aria-label={t('deploymentsPage.runtimeSecretGenerate', { key })} disabled={update.isPending} size="sm" type="button" variant="outline" onClick={() => update.mutate({ generate: { [key]: { length: 32, encoding: 'base64' } } })}>
                  <RefreshCw className="size-4" />
                  {t('deploymentsPage.runtimeSecretGenerate')}
                </Button>
                <Button aria-label={t('deploymentsPage.runtimeSecretClear', { key })} disabled={update.isPending} size="sm" type="button" variant="ghost" onClick={() => update.mutate({ clear: [key] })}>
                  <Trash2 className="size-4" />
                  {t('deploymentsPage.runtimeSecretClear')}
                </Button>
              </>
            )}
          </div>
        </div>
      ))}
      {editingKey && (
        <form className="grid gap-3 rounded-control bg-background p-3" onSubmit={submitReplace}>
          <Field error={replaceForm.formState.errors.value ? t('deploymentsPage.runtimeSecretValueRequired') : undefined} label={t('deploymentsPage.runtimeSecretReplaceLabel', { key: editingKey })}>
            <Input autoComplete="new-password" type="password" {...replaceForm.register('value')} />
          </Field>
          <div className="flex justify-end gap-2">
            <Button type="button" variant="ghost" onClick={() => setEditingKey(null)}>{t('common.cancel')}</Button>
            <Button disabled={update.isPending} type="submit">{t('common.save')}</Button>
          </div>
        </form>
      )}
      {canManage && !editingKey && (
        <form className="grid gap-3 rounded-control bg-background p-3 md:grid-cols-2" onSubmit={submitReplace}>
          <Field label={t('deploymentsPage.runtimeSecretNewKey')}>
            <Input {...replaceForm.register('key')} />
          </Field>
          <Field label={t('deploymentsPage.runtimeSecretNewValue')}>
            <Input autoComplete="new-password" type="password" {...replaceForm.register('value')} />
          </Field>
          <div className="md:col-span-2 flex justify-end">
            <Button disabled={update.isPending} type="submit">{t('deploymentsPage.runtimeSecretAdd')}</Button>
          </div>
        </form>
      )}
    </div>
  )
}
