import type { AccessToken } from '@/api/client'
import type { DataListColumn } from '@/components/common/data-list'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Copy, Plus, ShieldX } from 'lucide-react'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api/client'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { DataList } from '@/components/common/data-list'
import { FormField as Field } from '@/components/common/form-field'
import { StatusValueBadge } from '@/components/common/status-badge'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'

const schema = z.object({
  name: z.string().min(1),
  scope: z.string().min(1),
  expiresInDays: z.coerce.number().int().refine(value => [0, 7, 15, 30, 90].includes(value)),
})

type TokenFormInput = z.input<typeof schema>
type TokenForm = z.output<typeof schema>

const PAGE_SIZE_OPTIONS = [10, 20, 50, 100]

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
  const form = useForm<TokenFormInput, undefined, TokenForm>({
    resolver: zodResolver(schema),
    mode: 'onChange',
    defaultValues: {
      name: '',
      scope: 'build:trigger',
      expiresInDays: 30,
    },
  })

  const createToken = useMutation({
    mutationFn: api.createAccessToken,
    onSuccess: (result) => {
      setCreatedToken(result.accessToken)
      toast.success(t('accessTokens.created'))
      form.reset()
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
      className: 'w-[18%] px-4 py-3 align-middle',
      render: token => <Badge>{token.scope}</Badge>,
    },
    {
      key: 'createdAt',
      header: t('accessTokens.createdAt'),
      className: 'w-[20%] whitespace-nowrap px-4 py-3 align-middle text-muted-foreground',
      render: token => formatDate(token.createdAt),
    },
    {
      key: 'expiresAt',
      header: t('accessTokens.expiresAt'),
      className: 'w-[20%] whitespace-nowrap px-4 py-3 align-middle text-muted-foreground',
      render: token => token.expiresAt ? formatDate(token.expiresAt) : t('accessTokens.neverExpires'),
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
            form.reset()
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
            <Button variant="secondary" onClick={() => navigator.clipboard.writeText(createdToken)}>
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
            form.reset()
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
            <Field error={form.formState.errors.scope?.message} hint={t('accessTokens.scopeHint')} label={t('accessTokens.scope')} required>
              <Select {...form.register('scope')} aria-invalid={Boolean(form.formState.errors.scope)}>
                <option value="build:trigger">{t('accessTokens.scopeBuildTrigger')}</option>
                <option value="deploy:trigger">{t('accessTokens.scopeDeployTrigger')}</option>
                <option value="project:read">{t('accessTokens.scopeProjectRead')}</option>
              </Select>
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

function formatDate(value: string) {
  return new Date(value).toLocaleString(undefined, {
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    month: '2-digit',
    second: '2-digit',
    year: 'numeric',
  })
}

function tokenStatusValue(token: AccessToken) {
  if (token.revokedAt)
    return 'revoked'
  if (token.expiresAt && new Date(token.expiresAt).getTime() < Date.now())
    return 'expired'
  return 'active'
}
