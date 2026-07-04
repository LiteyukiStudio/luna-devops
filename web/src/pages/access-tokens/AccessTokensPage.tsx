import type { TFunction } from 'i18next'
import type { AccessToken, AccessTokenScopeDefinition } from '@/api'
import type { DataListColumn } from '@/components/common/data-list'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Copy, Plus, ShieldX } from 'lucide-react'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api'
import { CheckboxField } from '@/components/common/checkbox-field'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { DataList } from '@/components/common/data-list'
import { FormField as Field } from '@/components/common/form-field'
import { StatusValueBadge } from '@/components/common/status-badge'
import { formatAbsoluteDateTime } from '@/components/common/time-format'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'

const schema = z.object({
  name: z.string().min(1),
  scopes: z.array(z.string()).min(1),
  expiresInDays: z.coerce.number().int().refine(value => [0, 7, 15, 30, 90].includes(value)),
})

type TokenFormInput = z.input<typeof schema>
type TokenForm = z.output<typeof schema>

const PAGE_SIZE_OPTIONS = [10, 20, 50, 100]
const DEFAULT_TOKEN_FORM: TokenFormInput = {
  name: '',
  scopes: ['build:trigger'],
  expiresInDays: 30,
}

export function AccessTokensPage() {
  const { t } = useTranslation()

  return (
    <div className="grid gap-6">
      <div>
        <h1 className="text-2xl font-semibold">{t('accessTokens.title')}</h1>
        <p className="mt-1 text-sm text-muted-foreground">{t('accessTokens.description')}</p>
      </div>
      <AccessTokensPanel />
    </div>
  )
}

export function AccessTokensPanel() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [createdToken, setCreatedToken] = useState('')
  const [dialogOpen, setDialogOpen] = useState(false)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [search, setSearch] = useState('')
  const tokens = useQuery({
    queryKey: ['access-tokens', page, pageSize, search],
    queryFn: () => api.listAccessTokens({ page, pageSize, search, sortBy: 'createdAt', sortOrder: 'desc' }),
  })
  const scopeCatalog = useQuery({
    queryKey: ['access-token-scopes'],
    queryFn: api.listAccessTokenScopes,
    refetchInterval: 10 * 60 * 1000,
    staleTime: 5 * 60 * 1000,
  })
  const form = useForm<TokenFormInput, undefined, TokenForm>({
    resolver: zodResolver(schema),
    mode: 'onChange',
    defaultValues: DEFAULT_TOKEN_FORM,
  })
  const selectedScopes = form.watch('scopes') ?? []
  const scopeGroups = groupAccessTokenScopes(scopeCatalog.data?.items ?? [])

  const createToken = useMutation({
    mutationFn: (values: TokenForm) => api.createAccessToken({
      name: values.name,
      scope: values.scopes.join(','),
      expiresInDays: values.expiresInDays,
    }),
    onSuccess: (result) => {
      setCreatedToken(result.accessToken)
      toast.success(t('accessTokens.created'))
      form.reset(DEFAULT_TOKEN_FORM)
      setDialogOpen(false)
      setPage(1)
      queryClient.invalidateQueries({ queryKey: ['access-tokens'] })
    },
    onError: error => toast.error(error.message),
  })

  const revokeToken = useMutation({
    mutationFn: api.revokeAccessToken,
    onSuccess: () => {
      toast.success(t('accessTokens.revoked'))
      queryClient.invalidateQueries({ queryKey: ['access-tokens'] })
    },
    onError: error => toast.error(error.message),
  })
  const copyCreatedToken = () => {
    navigator.clipboard.writeText(createdToken)
      .then(() => toast.success(t('common.copied')))
      .catch(error => toast.error(error.message))
  }
  const toggleScope = (scope: string, checked: boolean) => {
    const current = form.getValues('scopes') ?? []
    const next = checked
      ? Array.from(new Set([...current, scope]))
      : current.filter(item => item !== scope)
    form.setValue('scopes', next, { shouldDirty: true, shouldValidate: true })
  }

  const tokenItems = tokens.data?.items ?? []
  const columns: DataListColumn<AccessToken>[] = [
    {
      key: 'name',
      header: t('accessTokens.name'),
      className: 'w-[28%] px-4 py-3 align-middle',
      render: token => <span className="font-medium text-foreground">{token.name}</span>,
    },
    {
      key: 'scope',
      header: t('accessTokens.scope'),
      className: 'w-[26%] px-4 py-3 align-middle',
      render: token => (
        <div className="flex max-w-md flex-wrap gap-1">
          {splitAccessTokenScopes(token.scope).map(scope => (
            <Badge key={scope} title={scope} variant="secondary">
              {accessTokenScopeLabel(t, scope)}
            </Badge>
          ))}
        </div>
      ),
    },
    {
      key: 'createdAt',
      header: t('accessTokens.createdAt'),
      className: 'w-[20%] whitespace-nowrap px-4 py-3 align-middle text-muted-foreground',
      render: token => formatAbsoluteDateTime(token.createdAt),
    },
    {
      key: 'expiresAt',
      header: t('accessTokens.expiresAt'),
      className: 'w-[20%] whitespace-nowrap px-4 py-3 align-middle text-muted-foreground',
      render: token => token.expiresAt ? formatAbsoluteDateTime(token.expiresAt) : t('accessTokens.neverExpires'),
    },
    {
      key: 'status',
      header: t('accessTokens.status'),
      className: 'w-[12%] px-4 py-3 align-middle',
      render: token => <StatusValueBadge value={tokenStatusValue(token)} />,
    },
    {
      key: 'actions',
      header: t('accessTokens.actions'),
      className: 'w-[1%] whitespace-nowrap px-4 py-3 align-middle text-right',
      render: token => (
        <ConfirmDialog
          confirmText={t('accessTokens.revoke')}
          description={t('accessTokens.revokeDescription', { name: token.name })}
          pending={revokeToken.isPending}
          title={t('accessTokens.revokeTitle')}
          onConfirm={() => revokeToken.mutate(token.id)}
        >
          <Button disabled={Boolean(token.revokedAt)} variant="ghost">
            <ShieldX size={16} />
            {t('accessTokens.revoke')}
          </Button>
        </ConfirmDialog>
      ),
    },
  ]

  return (
    <div className="grid items-start gap-4">
      <div className="flex justify-end">
        <Button
          onClick={() => {
            form.reset(DEFAULT_TOKEN_FORM)
            setDialogOpen(true)
          }}
        >
          <Plus size={16} />
          {t('accessTokens.createTitle')}
        </Button>
      </div>
      {createdToken && (
        <Card className="rounded-md border border-border bg-muted p-3">
          <p className="mb-2 text-xs font-medium text-muted-foreground">{t('accessTokens.oneTime')}</p>
          <div className="flex items-center gap-2">
            <code className="min-w-0 flex-1 truncate text-xs">{createdToken}</code>
            <Button aria-label={t('common.copy')} variant="secondary" onClick={copyCreatedToken}>
              <Copy size={14} />
            </Button>
          </div>
        </Card>
      )}
      <DataList
        columns={columns}
        emptyDescription={t('accessTokens.emptyDescription')}
        emptyTitle={t('accessTokens.empty')}
        items={tokenItems}
        pagination={{
          page: tokens.data?.page ?? page,
          pageSize: tokens.data?.pageSize ?? pageSize,
          pageSizeOptions: PAGE_SIZE_OPTIONS,
          total: tokens.data?.total ?? 0,
          totalPages: tokens.data?.totalPages ?? 0,
          pageInfoLabel: t('pagination.pageInfo', {
            page: tokens.data?.page ?? page,
            totalPages: tokens.data?.totalPages ?? 0,
            total: tokens.data?.total ?? 0,
          }),
          onPageChange: setPage,
          onPageSizeChange: (nextPageSize) => {
            setPageSize(nextPageSize)
            setPage(1)
          },
        }}
        rowKey={token => token.id}
        search={{
          value: search,
          placeholder: t('accessTokens.searchPlaceholder'),
          onChange: (value) => {
            setSearch(value)
            setPage(1)
          },
        }}
        title={t('accessTokens.listTitle')}
      />
      <Dialog
        open={dialogOpen}
        onOpenChange={(open) => {
          setDialogOpen(open)
          if (!open)
            form.reset(DEFAULT_TOKEN_FORM)
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('accessTokens.createTitle')}</DialogTitle>
            <DialogDescription>{t('accessTokens.description')}</DialogDescription>
          </DialogHeader>
          <form className="grid gap-3" onSubmit={form.handleSubmit(values => createToken.mutate(values))}>
            <Field error={form.formState.errors.name?.message} hint={t('accessTokens.nameHint')} label={t('accessTokens.name')} required>
              <Input {...form.register('name')} aria-invalid={Boolean(form.formState.errors.name)} placeholder={t('accessTokens.namePlaceholder')} />
            </Field>
            <Field error={form.formState.errors.scopes?.message} hint={t('accessTokens.scopeHint')} label={t('accessTokens.scope')} required>
              <div className="max-h-[22rem] overflow-y-auto rounded-md border border-border bg-card">
                {scopeCatalog.isLoading && (
                  <p className="px-3 py-2 text-sm text-muted-foreground">{t('common.loading')}</p>
                )}
                {!scopeCatalog.isLoading && scopeGroups.length === 0 && (
                  <p className="px-3 py-2 text-sm text-muted-foreground">{t('accessTokens.emptyScopes')}</p>
                )}
                {scopeGroups.map(group => (
                  <div key={group.group} className="border-b border-border last:border-b-0">
                    <div className="bg-muted/60 px-3 py-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                      {t(`accessTokens.scopeGroups.${group.group}`)}
                    </div>
                    <div className="grid gap-3 p-3 sm:grid-cols-2">
                      {group.items.map(scope => (
                        <CheckboxField
                          key={scope.value}
                          checked={selectedScopes.includes(scope.value)}
                          description={t(`accessTokens.scopeDescriptions.${scopeKey(scope.value)}`)}
                          disabled={scope.requiresAdminRole}
                          onChange={event => toggleScope(scope.value, event.target.checked)}
                        >
                          <span className="flex items-center gap-2">
                            {accessTokenScopeLabel(t, scope.value)}
                            {scope.recommended && <Badge variant="secondary">{t('accessTokens.recommended')}</Badge>}
                            {scope.requiresAdminRole && <Badge variant="outline">{t('accessTokens.adminOnly')}</Badge>}
                          </span>
                        </CheckboxField>
                      ))}
                    </div>
                  </div>
                ))}
              </div>
            </Field>
            <Field error={form.formState.errors.expiresInDays?.message} hint={t('accessTokens.expiresInDaysHint')} label={t('accessTokens.expiresInDays')} required>
              <Select {...form.register('expiresInDays')} aria-invalid={Boolean(form.formState.errors.expiresInDays)}>
                <option value={7}>{t('accessTokens.expiresIn7Days')}</option>
                <option value={15}>{t('accessTokens.expiresIn15Days')}</option>
                <option value={30}>{t('accessTokens.expiresIn30Days')}</option>
                <option value={90}>{t('accessTokens.expiresIn90Days')}</option>
                <option value={0}>{t('accessTokens.expiresNever')}</option>
              </Select>
            </Field>
            <DialogFooter>
              <Button disabled={createToken.isPending || !form.formState.isValid} type="submit">
                <Plus size={16} />
                {t('accessTokens.create')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function tokenStatusValue(token: AccessToken) {
  if (token.revokedAt)
    return 'revoked'
  if (token.expiresAt && new Date(token.expiresAt).getTime() < Date.now())
    return 'expired'
  return 'active'
}

function splitAccessTokenScopes(scopeText: string) {
  return scopeText.split(',').map(scope => scope.trim()).filter(Boolean)
}

function groupAccessTokenScopes(items: AccessTokenScopeDefinition[]) {
  const groups = new Map<string, AccessTokenScopeDefinition[]>()
  for (const item of items) {
    const current = groups.get(item.group) ?? []
    current.push(item)
    groups.set(item.group, current)
  }
  return Array.from(groups.entries()).map(([group, groupItems]) => ({
    group,
    items: groupItems,
  }))
}

function accessTokenScopeLabel(t: TFunction, scope: string) {
  const key = `accessTokens.scopeLabels.${scopeKey(scope)}`
  const label = t(key)
  return label === key ? scope : label
}

function scopeKey(scope: string) {
  return scope.replaceAll(':', '.').replaceAll('_', '-')
}
