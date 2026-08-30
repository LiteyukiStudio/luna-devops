import type { DeploymentTargetRuntimeSecretsPayload, DeploymentTargetRuntimeSecretsSummary } from '@/api'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { KeyRound, RefreshCw, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { FormField as Field } from '@/components/common/form-field'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { runtimeSecretKeys } from '@/lib/runtime-environment'

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
  const [replacementValue, setReplacementValue] = useState('')
  const [newKey, setNewKey] = useState('')
  const [newValue, setNewValue] = useState('')
  const summary = useQuery({
    queryKey: ['deployment-target-runtime-secrets', projectId, applicationId, targetId],
    queryFn: () => api.getDeploymentTargetRuntimeSecretsSummary(projectId, applicationId, targetId!),
    enabled: Boolean(open && targetId),
  })

  const clearEditorState = () => {
    setEditingKey(null)
    setReplacementValue('')
    setNewKey('')
    setNewValue('')
  }

  const update = useMutation({
    mutationFn: (payload: DeploymentTargetRuntimeSecretsPayload) =>
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

  const keys = runtimeSecretKeys((summary.data as DeploymentTargetRuntimeSecretsSummary | undefined)?.environmentVariables)
  const addSecret = () => {
    const key = newKey.trim()
    if (key && newValue)
      update.mutate({ items: [{ key, operation: 'set', value: newValue, valueMode: 'secret' }] })
  }
  const replaceSecret = () => {
    if (editingKey && replacementValue)
      update.mutate({ items: [{ key: editingKey, operation: 'set', value: replacementValue, valueMode: 'secret' }] })
  }

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
                    setReplacementValue('')
                  }}
                >
                  {t('deploymentsPage.runtimeSecretReplace')}
                </Button>
                <Button aria-label={t('deploymentsPage.runtimeSecretGenerate', { key })} disabled={update.isPending} size="sm" type="button" variant="outline" onClick={() => update.mutate({ items: [{ generation: { length: 32, encoding: 'base64' }, key, operation: 'generate', valueMode: 'secret' }] })}>
                  <RefreshCw className="size-4" />
                  {t('deploymentsPage.runtimeSecretGenerate')}
                </Button>
                <Button aria-label={t('deploymentsPage.runtimeSecretClear', { key })} disabled={update.isPending} size="sm" type="button" variant="ghost" onClick={() => update.mutate({ items: [{ key, operation: 'clear', valueMode: 'secret' }] })}>
                  <Trash2 className="size-4" />
                  {t('deploymentsPage.runtimeSecretClear')}
                </Button>
              </>
            )}
          </div>
        </div>
      ))}
      {editingKey && (
        <div className="grid gap-3 rounded-control bg-background p-3">
          <Field label={t('deploymentsPage.runtimeSecretReplaceLabel', { key: editingKey })}>
            <Input
              aria-label={t('deploymentsPage.runtimeSecretReplaceLabel', { key: editingKey })}
              autoComplete="new-password"
              type="password"
              value={replacementValue}
              onChange={event => setReplacementValue(event.target.value)}
            />
          </Field>
          <div className="flex justify-end gap-2">
            <Button
              type="button"
              variant="ghost"
              onClick={() => {
                setEditingKey(null)
                setReplacementValue('')
              }}
            >
              {t('common.cancel')}
            </Button>
            <Button disabled={update.isPending || !replacementValue} type="button" onClick={replaceSecret}>{t('common.save')}</Button>
          </div>
        </div>
      )}
      {canManage && !editingKey && (
        <div className="grid gap-3 rounded-control bg-background p-3 md:grid-cols-2">
          <Field label={t('deploymentsPage.runtimeSecretNewKey')}>
            <Input aria-label={t('deploymentsPage.runtimeSecretNewKey')} value={newKey} onChange={event => setNewKey(event.target.value)} />
          </Field>
          <Field label={t('deploymentsPage.runtimeSecretNewValue')}>
            <Input aria-label={t('deploymentsPage.runtimeSecretNewValue')} autoComplete="new-password" type="password" value={newValue} onChange={event => setNewValue(event.target.value)} />
          </Field>
          <div className="md:col-span-2 flex justify-end">
            <Button disabled={update.isPending || !newKey.trim() || !newValue} type="button" onClick={addSecret}>{t('deploymentsPage.runtimeSecretAdd')}</Button>
          </div>
        </div>
      )}
    </div>
  )
}
