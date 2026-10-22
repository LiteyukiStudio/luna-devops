import type { DeploymentTargetRuntimeSecretsPayload, DeploymentTargetRuntimeSecretsSummary } from '@/api'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { KeyRound, RefreshCw, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { FormField as Field } from '@/components/common/form-field'
import { StatusBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { runtimeSecretKeys } from '@/lib/runtime-environment'

/** 运行时密钥使用独立接口即时写入，不随部署配置的“保存”提交；替换值永不回显。 */
export function ApplicationDeploymentRuntimeSecretsEditor({ applicationId, canManage, projectId, targetId }: {
  applicationId: string
  canManage: boolean
  projectId: string
  targetId?: string
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [editingKey, setEditingKey] = useState<string | null>(null)
  const [replacementValue, setReplacementValue] = useState('')
  const [newKey, setNewKey] = useState('')
  const [newValue, setNewValue] = useState('')
  const [confirmAction, setConfirmAction] = useState<{ key: string, operation: 'clear' | 'generate' } | null>(null)
  const summary = useQuery({
    queryKey: ['deployment-target-runtime-secrets', projectId, applicationId, targetId],
    queryFn: () => api.getDeploymentTargetRuntimeSecretsSummary(projectId, applicationId, targetId!),
    enabled: Boolean(targetId),
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
      setConfirmAction(null)
      void queryClient.invalidateQueries({ queryKey: ['deployment-target-runtime-secrets', projectId, applicationId, targetId] })
      toast.success(t('deploymentsPage.runtimeSecretsUpdated'))
    },
    onError: error => toast.error(error instanceof Error ? error.message : t('deploymentsPage.runtimeSecretsUpdateFailed')),
  })

  const keys = runtimeSecretKeys((summary.data as DeploymentTargetRuntimeSecretsSummary | undefined)?.environmentVariables)
  const addSecret = () => {
    const key = newKey.trim()
    if (key && newValue)
      update.mutate({ items: [{ key, operation: 'set', value: newValue, valueMode: 'secret' }] })
  }
  const confirmPendingAction = () => {
    if (!confirmAction)
      return
    update.mutate({
      items: [confirmAction.operation === 'generate'
        ? { generation: { length: 32, encoding: 'base64' }, key: confirmAction.key, operation: 'generate', valueMode: 'secret' }
        : { key: confirmAction.key, operation: 'clear', valueMode: 'secret' }],
    })
  }
  const replaceSecret = () => {
    if (editingKey && replacementValue)
      update.mutate({ items: [{ key: editingKey, operation: 'set', value: replacementValue, valueMode: 'secret' }] })
  }

  return (
    <div className="grid gap-3 rounded-control bg-surface-inset/60 p-4" data-testid="runtime-secrets-editor">
      <div className="flex items-start gap-3">
        <KeyRound aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <h4 className="text-sm font-medium">{t('deploymentsPage.runtimeSecretsTitle')}</h4>
            {targetId && <StatusBadge tone="info">{t('deploymentTargetEditor.runtimeSecretsImmediate')}</StatusBadge>}
          </div>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">{t(canManage ? 'deploymentsPage.runtimeSecretsManageHint' : 'deploymentsPage.runtimeSecretsRoleHint')}</p>
        </div>
      </div>
      {!targetId && <p className="text-sm text-muted-foreground">{t('deploymentsPage.runtimeSecretsSaveTargetFirst')}</p>}
      {summary.isLoading && <p className="text-sm text-muted-foreground">{t('common.loading')}</p>}
      {summary.isError && <p className="text-sm text-destructive">{t('deploymentsPage.runtimeSecretsLoadFailed')}</p>}
      {targetId && !summary.isLoading && !summary.isError && keys.length === 0 && <p className="text-sm text-muted-foreground">{t('deploymentsPage.runtimeSecretsEmpty')}</p>}
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
                <Button aria-label={t('deploymentsPage.runtimeSecretGenerate', { key })} disabled={update.isPending} size="sm" type="button" variant="outline" onClick={() => setConfirmAction({ key, operation: 'generate' })}>
                  <RefreshCw className="size-4" />
                  {t('deploymentsPage.runtimeSecretGenerate')}
                </Button>
                <Button aria-label={t('deploymentsPage.runtimeSecretClear', { key })} disabled={update.isPending} size="sm" type="button" variant="ghost" onClick={() => setConfirmAction({ key, operation: 'clear' })}>
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
      {canManage && targetId && !editingKey && (
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
      <ConfirmDialog
        cancelText={t('common.cancel')}
        confirmText={confirmAction?.operation === 'generate' ? t('deploymentsPage.runtimeSecretGenerate') : t('deploymentsPage.runtimeSecretClear')}
        confirmVariant={confirmAction?.operation === 'clear' ? 'destructive' : 'default'}
        description={confirmAction?.operation === 'generate'
          ? t('deploymentTargetEditor.runtimeSecretGenerateConfirm', { key: confirmAction.key })
          : t('deploymentTargetEditor.runtimeSecretClearConfirm', { key: confirmAction?.key ?? '' })}
        open={Boolean(confirmAction)}
        pending={update.isPending}
        title={confirmAction?.operation === 'generate' ? t('deploymentTargetEditor.runtimeSecretGenerateConfirmTitle') : t('deploymentTargetEditor.runtimeSecretClearConfirmTitle')}
        onConfirm={confirmPendingAction}
        onOpenChange={open => !open && setConfirmAction(null)}
      />
    </div>
  )
}
