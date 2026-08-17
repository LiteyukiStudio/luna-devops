import type { DeploymentTargetRuntimeSecretsPayload, ProjectRuntimeConfigSet } from '@/api'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { KeyRound, RefreshCw, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { FormField as Field } from '@/components/common/form-field'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { runtimeSecretKeys } from '@/lib/runtime-environment'

export function RuntimeConfigSetSecretsEditor({ projectId, set }: { projectId: string, set: ProjectRuntimeConfigSet | null }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [key, setKey] = useState('')
  const [value, setValue] = useState('')
  const keys = runtimeSecretKeys(set?.environmentVariables)
  const update = useMutation({
    mutationFn: (payload: DeploymentTargetRuntimeSecretsPayload) => {
      if (!set)
        throw new Error(t('runtimeConfigSets.saveBeforeSecrets'))
      return api.updateProjectRuntimeConfigSetRuntimeSecrets(projectId, set.id, payload)
    },
    onSuccess: () => {
      setKey('')
      setValue('')
      void queryClient.invalidateQueries({ queryKey: ['runtime-config-sets', projectId] })
      toast.success(t('runtimeConfigSets.secretsUpdated'))
    },
    onError: error => toast.error(error instanceof Error ? error.message : t('runtimeConfigSets.secretsUpdateFailed')),
  })

  if (!set)
    return <p className="text-sm text-muted-foreground">{t('runtimeConfigSets.saveBeforeSecrets')}</p>

  return (
    <div className="grid gap-3 rounded-control bg-surface-inset/60 p-4">
      <div className="flex items-start gap-3">
        <KeyRound className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
        <div>
          <p className="text-sm font-medium">{t('runtimeConfigSets.secretVariables')}</p>
          <p className="text-sm text-muted-foreground">{t('runtimeConfigSets.secretVariablesHint')}</p>
        </div>
      </div>
      {keys.length === 0 && <p className="text-sm text-muted-foreground">{t('runtimeConfigSets.noSecrets')}</p>}
      {keys.map(secretKey => (
        <div className="flex items-center justify-between gap-3 rounded-control bg-background p-3" key={secretKey}>
          <div className="min-w-0">
            <p className="truncate text-sm font-medium" title={secretKey}>{secretKey}</p>
            <p className="font-mono text-sm text-muted-foreground">{t('common.secretValueMasked')}</p>
          </div>
          <div className="flex shrink-0 gap-2">
            <Button disabled={update.isPending} size="sm" type="button" variant="outline" onClick={() => update.mutate({ items: [{ generation: { length: 32, encoding: 'base64' }, key: secretKey, operation: 'generate', valueMode: 'secret' }] })}>
              <RefreshCw className="size-4" />
              {t('deploymentsPage.runtimeSecretGenerate')}
            </Button>
            <Button aria-label={t('deploymentsPage.runtimeSecretClear', { key: secretKey })} disabled={update.isPending} size="sm" type="button" variant="ghost" onClick={() => update.mutate({ items: [{ key: secretKey, operation: 'clear', valueMode: 'secret' }] })}>
              <Trash2 className="size-4" />
              {t('deploymentsPage.runtimeSecretClear')}
            </Button>
          </div>
        </div>
      ))}
      <div className="grid gap-3 rounded-control bg-background p-3 md:grid-cols-2">
        <Field label={t('deploymentsPage.runtimeSecretNewKey')}>
          <Input aria-label={t('deploymentsPage.runtimeSecretNewKey')} value={key} onChange={event => setKey(event.target.value)} />
        </Field>
        <Field label={t('deploymentsPage.runtimeSecretNewValue')}>
          <Input aria-label={t('deploymentsPage.runtimeSecretNewValue')} autoComplete="new-password" type="password" value={value} onChange={event => setValue(event.target.value)} />
        </Field>
        <div className="md:col-span-2 flex justify-end">
          <Button
            disabled={update.isPending || !key.trim() || !value}
            type="button"
            onClick={() => update.mutate({ items: [{ key: key.trim(), operation: 'set', value, valueMode: 'secret' }] })}
          >
            {t('deploymentsPage.runtimeSecretAdd')}
          </Button>
        </div>
      </div>
    </div>
  )
}
