import type { TFunction } from 'i18next'
import type { UseFormReturn } from 'react-hook-form'
import type { AccessTokenScopeDefinition, OAuthApplication, OAuthApplicationInput, OAuthGrant } from '@/api'
import type { DataListColumn } from '@/components/common/data-list'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import i18next from 'i18next'
import { Copy, KeyRound, MoreHorizontal, Pencil, RotateCcwKey, ShieldX, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api'
import { splitAccessTokenScopes } from '@/components/common/access-token-scope'
import { AccessTokenScopeBadges, AccessTokenScopeSelector } from '@/components/common/access-token-scope-selector'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { DataList } from '@/components/common/data-list'
import { ErrorState } from '@/components/common/error-state'
import { FormField as Field } from '@/components/common/form-field'
import { ProgressiveSection } from '@/components/common/progressive-section'
import { formatAbsoluteDateTime } from '@/components/common/time-format'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'
import { OneTimeSecret } from './one-time-secret'

const PAGE_SIZE_OPTIONS = [10, 20, 50]
const oauthApplicationSchema = z.object({
  name: z.string().trim().min(1, { error: () => i18next.t('oauthApps.validation.nameRequired') }),
  description: z.string(),
  homepageUrl: z.string(),
  logoUrl: z.string(),
  redirectUrisText: z.string().trim().min(1, { error: () => i18next.t('oauthApps.validation.redirectRequired') }),
  scopes: z.array(z.string()).min(1, { error: () => i18next.t('oauthApps.validation.scopeRequired') }),
  accessTokenLifetimeDays: z.number().int().refine(value => [0, 1, 7, 30, 90].includes(value)),
})

type OAuthApplicationForm = z.infer<typeof oauthApplicationSchema>

const DEFAULT_APPLICATION_FORM: OAuthApplicationForm = {
  name: '',
  description: '',
  homepageUrl: '',
  logoUrl: '',
  redirectUrisText: '',
  scopes: ['project:read'],
  accessTokenLifetimeDays: 30,
}

export function OAuthApplicationsPanel({ createDialogOpen, onCreateDialogOpenChange }: {
  createDialogOpen: boolean
  onCreateDialogOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [search, setSearch] = useState('')
  const [editingApplication, setEditingApplication] = useState<OAuthApplication>()
  const [createdSecret, setCreatedSecret] = useState('')
  const [rotateTarget, setRotateTarget] = useState<OAuthApplication | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<OAuthApplication | null>(null)
  const applications = useQuery({
    queryKey: ['oauth-applications', page, pageSize, search],
    queryFn: () => api.listOAuthApplications({ page, pageSize, search, sortBy: 'createdAt', sortOrder: 'desc' }),
  })
  const scopes = useQuery({ queryKey: ['access-token-scopes'], queryFn: api.listAccessTokenScopes, staleTime: 5 * 60 * 1000 })
  const form = useForm<OAuthApplicationForm>({
    resolver: zodResolver(oauthApplicationSchema),
    mode: 'onChange',
    defaultValues: DEFAULT_APPLICATION_FORM,
  })

  const save = useMutation({
    mutationFn: (values: OAuthApplicationForm) => {
      const payload = applicationPayload(values)
      return editingApplication
        ? api.updateOAuthApplication(editingApplication.id, payload).then(application => ({ application, clientSecret: '' }))
        : api.createOAuthApplication(payload)
    },
    onSuccess: (result) => {
      if (result.clientSecret)
        setCreatedSecret(result.clientSecret)
      toast.success(t(editingApplication ? 'oauthApps.updated' : 'oauthApps.created'))
      onCreateDialogOpenChange(false)
      setEditingApplication(undefined)
      form.reset(DEFAULT_APPLICATION_FORM)
      queryClient.invalidateQueries({ queryKey: ['oauth-applications'] })
    },
    onError: error => toast.error(error.message),
  })
  const rotateSecret = useMutation({
    mutationFn: api.rotateOAuthApplicationSecret,
    onSuccess: (result) => {
      setRotateTarget(null)
      setCreatedSecret(result.clientSecret)
      toast.success(t('oauthApps.secretRotated'))
    },
    onError: error => toast.error(error.message),
  })
  const remove = useMutation({
    mutationFn: api.deleteOAuthApplication,
    onSuccess: () => {
      setDeleteTarget(null)
      toast.success(t('oauthApps.revoked'))
      queryClient.invalidateQueries({ queryKey: ['oauth-applications'] })
    },
    onError: error => toast.error(error.message),
  })

  const openEdit = (application: OAuthApplication) => {
    onCreateDialogOpenChange(false)
    setEditingApplication(application)
    form.reset({
      name: application.name,
      description: application.description,
      homepageUrl: application.homepageUrl,
      logoUrl: application.logoUrl,
      redirectUrisText: application.redirectUris.join('\n'),
      scopes: splitAccessTokenScopes(application.allowedScopes),
      accessTokenLifetimeDays: application.accessTokenLifetimeDays,
    })
  }
  const closeDialog = () => {
    onCreateDialogOpenChange(false)
    setEditingApplication(undefined)
    form.reset(DEFAULT_APPLICATION_FORM)
  }
  const copyValue = (value: string) => navigator.clipboard.writeText(value)
    .then(() => toast.success(t('common.copied')))
    .catch(error => toast.error(error.message))

  const columns: DataListColumn<OAuthApplication>[] = [
    {
      key: 'name',
      header: t('oauthApps.application'),
      width: 'primary',
      render: application => (
        <div className="min-w-0">
          <p className="font-medium text-foreground">{application.name}</p>
          <p className="truncate text-sm text-muted-foreground">{application.description || t('oauthApps.noDescription')}</p>
        </div>
      ),
    },
    {
      key: 'clientId',
      header: t('oauthApps.clientId'),
      render: application => (
        <div className="flex items-center gap-1">
          <code className="max-w-56 truncate text-xs">{application.clientId}</code>
          <Button aria-label={t('oauthApps.copyClientId')} className="size-7" size="icon" variant="ghost" onClick={() => copyValue(application.clientId)}><Copy className="size-3.5" /></Button>
        </div>
      ),
    },
    {
      key: 'scope',
      header: t('oauthApps.scopes'),
      render: application => <AccessTokenScopeBadges scope={application.allowedScopes} />,
    },
    {
      key: 'lifetime',
      header: t('oauthApps.tokenLifetime'),
      render: application => application.accessTokenLifetimeDays === 0
        ? t('oauthApps.neverExpires')
        : t('oauthApps.days', { count: application.accessTokenLifetimeDays }),
    },
    {
      key: 'actions',
      header: t('common.actions'),
      className: 'text-right whitespace-nowrap',
      sticky: 'right',
      width: 'actions',
      render: application => (
        <div className="flex justify-end">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button aria-label={t('oauthApps.applicationActions', { name: application.name })} size="icon" variant="ghost">
                <MoreHorizontal className="size-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onSelect={() => openEdit(application)}>
                <Pencil className="size-4" />
                {t('common.edit')}
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => setRotateTarget(application)}>
                <RotateCcwKey className="size-4" />
                {t('oauthApps.rotateSecret')}
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem variant="destructive" onSelect={() => setDeleteTarget(application)}>
                <Trash2 className="size-4" />
                {t('oauthApps.revokeApplication')}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      ),
    },
  ]

  return (
    <div className="grid gap-4">
      {createdSecret && <OneTimeSecret label={t('oauthApps.secretOneTime')} value={createdSecret} />}
      {applications.isError
        ? <ErrorState title={t('oauthApps.loadFailed')} description={applications.error.message} />
        : (
            <DataList
              columns={columns}
              emptyTitle={t('oauthApps.emptyApplications')}
              items={applications.data?.items ?? []}
              pagination={paginationProps(applications.data, page, pageSize, setPage, setPageSize, t)}
              rowKey={application => application.id}
              search={{
                value: search,
                placeholder: t('oauthApps.searchApplications'),
                onChange: (value) => {
                  setSearch(value)
                  setPage(1)
                },
              }}
            />
          )}
      <OAuthApplicationDialog
        application={editingApplication}
        form={form}
        open={createDialogOpen || Boolean(editingApplication)}
        pending={save.isPending}
        scopeItems={scopes.data?.items ?? []}
        onClose={closeDialog}
        onSubmit={values => save.mutate(values)}
      />
      <ConfirmDialog
        confirmText={t('oauthApps.rotateSecret')}
        description={t('oauthApps.rotateSecretDescription')}
        open={Boolean(rotateTarget)}
        pending={rotateSecret.isPending}
        title={t('oauthApps.rotateSecretTitle')}
        onConfirm={() => rotateTarget && rotateSecret.mutateAsync(rotateTarget.id).then(() => undefined)}
        onOpenChange={open => !open && setRotateTarget(null)}
      />
      <ConfirmDialog
        confirmText={t('oauthApps.revokeApplication')}
        description={t('oauthApps.revokeApplicationDescription', { name: deleteTarget?.name ?? '' })}
        open={Boolean(deleteTarget)}
        pending={remove.isPending}
        title={t('oauthApps.revokeApplicationTitle')}
        onConfirm={() => deleteTarget && remove.mutateAsync(deleteTarget.id).then(() => undefined)}
        onOpenChange={open => !open && setDeleteTarget(null)}
      />
    </div>
  )
}

export function OAuthGrantsPanel() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const grants = useQuery({
    queryKey: ['oauth-grants', page, pageSize],
    queryFn: () => api.listMyOAuthGrants({ page, pageSize, sortBy: 'updatedAt', sortOrder: 'desc' }),
  })
  const revoke = useMutation({
    mutationFn: api.revokeMyOAuthGrant,
    onSuccess: () => {
      toast.success(t('oauthApps.authorizationRevoked'))
      queryClient.invalidateQueries({ queryKey: ['oauth-grants'] })
    },
    onError: error => toast.error(error.message),
  })
  const columns: DataListColumn<OAuthGrant>[] = [
    {
      key: 'application',
      header: t('oauthApps.application'),
      width: 'primary',
      render: grant => (
        <div className="min-w-0">
          <p className="font-medium text-foreground">{grant.application.name}</p>
          <p className="truncate text-sm text-muted-foreground">{grant.application.homepageUrl || grant.application.description || t('oauthApps.noDescription')}</p>
        </div>
      ),
    },
    {
      key: 'scope',
      header: t('oauthApps.authorizedScopes'),
      render: grant => grant.application.clientId === 'luna-cli'
        ? <span className="text-sm text-muted-foreground">{t('oauthApps.accountPermissions')}</span>
        : <AccessTokenScopeBadges scope={grant.scope} />,
    },
    { key: 'updatedAt', header: t('oauthApps.authorizedAt'), render: grant => formatAbsoluteDateTime(grant.updatedAt) },
    {
      key: 'actions',
      header: t('common.actions'),
      className: 'text-right whitespace-nowrap',
      sticky: 'right',
      width: 'actions',
      render: grant => (
        <ConfirmDialog
          confirmText={t('oauthApps.revokeAuthorization')}
          confirmVariant="destructive"
          description={t('oauthApps.revokeAuthorizationDescription', { name: grant.application.name })}
          pending={revoke.isPending}
          title={t('oauthApps.revokeAuthorizationTitle')}
          onConfirm={() => revoke.mutate(grant.id)}
        >
          <Button size="sm" variant="ghost">
            <ShieldX className="size-4" />
            {t('oauthApps.revokeAuthorization')}
          </Button>
        </ConfirmDialog>
      ),
    },
  ]
  return grants.isError
    ? <ErrorState title={t('oauthApps.loadFailed')} description={grants.error.message} />
    : (
        <DataList
          columns={columns}
          emptyTitle={t('oauthApps.emptyGrants')}
          items={grants.data?.items ?? []}
          pagination={paginationProps(grants.data, page, pageSize, setPage, setPageSize, t)}
          rowKey={grant => grant.id}
        />
      )
}

function OAuthApplicationDialog({ application, form, open, pending, scopeItems, onClose, onSubmit }: {
  application?: OAuthApplication
  form: UseFormReturn<OAuthApplicationForm>
  open: boolean
  pending: boolean
  scopeItems: AccessTokenScopeDefinition[]
  onClose: () => void
  onSubmit: (values: OAuthApplicationForm) => void
}) {
  const { t } = useTranslation()
  const selectedScopes = form.watch('scopes') ?? []
  const presentationSummary = [form.watch('homepageUrl'), form.watch('description')].map(value => value?.trim()).filter(Boolean).join(' · ')
  return (
    <Dialog open={open} onOpenChange={nextOpen => !nextOpen && onClose()}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{t(application ? 'oauthApps.editApplication' : 'oauthApps.createApplication')}</DialogTitle>
          <DialogDescription>{t('oauthApps.applicationDialogDescription')}</DialogDescription>
        </DialogHeader>
        <form className="grid gap-3" onSubmit={form.handleSubmit(onSubmit)}>
          <Field error={form.formState.errors.name?.message} label={t('oauthApps.name')} required><Input {...form.register('name')} /></Field>
          <Field error={form.formState.errors.redirectUrisText?.message} hint={t('oauthApps.redirectUrisHint')} label={t('oauthApps.redirectUris')} required>
            <Textarea {...form.register('redirectUrisText')} className="font-mono" rows={3} placeholder="https://example.com/oauth/callback" />
          </Field>
          <Field error={form.formState.errors.scopes?.message} hint={t('oauthApps.scopesHint')} label={t('oauthApps.scopes')} required>
            <AccessTokenScopeSelector
              items={scopeItems}
              value={selectedScopes}
              onChange={value => form.setValue('scopes', value, { shouldDirty: true, shouldValidate: true })}
            />
          </Field>
          <Field label={t('oauthApps.tokenLifetime')} required>
            <Select {...form.register('accessTokenLifetimeDays', { valueAsNumber: true })}>
              <option value={1}>{t('oauthApps.days', { count: 1 })}</option>
              <option value={7}>{t('oauthApps.days', { count: 7 })}</option>
              <option value={30}>{t('oauthApps.days', { count: 30 })}</option>
              <option value={90}>{t('oauthApps.days', { count: 90 })}</option>
              <option value={0}>{t('oauthApps.neverExpires')}</option>
            </Select>
          </Field>
          <ProgressiveSection
            summary={presentationSummary || t('oauthApps.presentationEmpty')}
            title={t('oauthApps.presentationSection')}
          >
            <Field label={t('oauthApps.description')}><Textarea {...form.register('description')} rows={2} /></Field>
            <div className="grid gap-3 sm:grid-cols-2">
              <Field label={t('oauthApps.homepageUrl')}><Input {...form.register('homepageUrl')} placeholder="https://example.com" /></Field>
              <Field label={t('oauthApps.logoUrl')}><Input {...form.register('logoUrl')} placeholder="https://example.com/logo.png" /></Field>
            </div>
          </ProgressiveSection>
          <DialogFooter>
            <Button disabled={pending} type="button" variant="outline" onClick={onClose}>{t('common.cancel')}</Button>
            <Button disabled={pending || !form.formState.isValid} type="submit">
              <KeyRound size={16} />
              {t(application ? 'common.save' : 'oauthApps.createApplication')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function applicationPayload(values: OAuthApplicationForm): OAuthApplicationInput {
  return {
    name: values.name,
    description: values.description,
    homepageUrl: values.homepageUrl,
    logoUrl: values.logoUrl,
    redirectUris: values.redirectUrisText.split('\n').map(item => item.trim()).filter(Boolean),
    allowedScopes: values.scopes.join(','),
    accessTokenLifetimeDays: values.accessTokenLifetimeDays,
  }
}

function paginationProps(data: { page: number, pageSize: number, total: number, totalPages: number } | undefined, page: number, pageSize: number, setPage: (value: number) => void, setPageSize: (value: number) => void, t: TFunction) {
  return {
    page: data?.page ?? page,
    pageSize: data?.pageSize ?? pageSize,
    pageSizeOptions: PAGE_SIZE_OPTIONS,
    total: data?.total ?? 0,
    totalPages: data?.totalPages ?? 0,
    pageInfoLabel: t('pagination.pageInfo', { page: data?.page ?? page, totalPages: data?.totalPages ?? 0, total: data?.total ?? 0 }),
    onPageChange: setPage,
    onPageSizeChange: (value: number) => {
      setPageSize(value)
      setPage(1)
    },
  }
}
