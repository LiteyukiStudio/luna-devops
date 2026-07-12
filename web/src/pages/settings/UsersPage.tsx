import type { User } from '@/api'
import type { DataListColumn } from '@/components/common/data-list'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Save, ShieldOff, UserPlus } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api'
import { useSession } from '@/app/session-context'
import { CheckboxField } from '@/components/common/checkbox-field'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { DataList } from '@/components/common/data-list'
import { EditActionButton } from '@/components/common/edit-action-button'
import { ErrorState } from '@/components/common/error-state'
import { FormField as Field } from '@/components/common/form-field'
import { PageHeader } from '@/components/common/page-header'
import { StatusBadge, StatusValueBadge } from '@/components/common/status-badge'
import { formatAbsoluteDateTime } from '@/components/common/time-format'
import { UserAvatar } from '@/components/common/user-avatar'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
import i18next from '@/i18n'
import { useBillingAmountDisplay } from '@/lib/billing-display'

const schema = z.object({
  email: z.string().email(i18next.t('common.validEmailRequired')),
  name: z.string().min(1, i18next.t('usersPage.nameRequired')),
  password: z.string(),
  role: z.enum(['platform_admin', 'user']),
  language: z.enum(['zh-CN', 'en-US']),
  disabled: z.boolean(),
})

type UserForm = z.infer<typeof schema>

const defaultValues: UserForm = {
  email: '',
  name: '',
  password: '',
  role: 'user',
  language: 'zh-CN',
  disabled: false,
}

const PAGE_SIZE_OPTIONS = [10, 20, 50, 100]

export function UsersPage() {
  const { i18n, t } = useTranslation()
  const queryClient = useQueryClient()
  const { user: currentUser } = useSession()
  const billingDisplay = useBillingAmountDisplay(i18n.language)
  const [editingUser, setEditingUser] = useState<User | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [search, setSearch] = useState('')
  const users = useQuery({
    queryKey: ['users', page, pageSize, search],
    queryFn: () => api.listUsers({ page, pageSize, search, sortBy: 'createdAt', sortOrder: 'desc' }),
  })
  const form = useForm<UserForm>({ resolver: zodResolver(schema), mode: 'onChange', defaultValues })
  const password = form.watch('password')
  const passwordError = !editingUser && password.length < 8 ? t('usersPage.passwordMin') : form.formState.errors.password?.message
  const canSubmit = form.formState.isValid && (editingUser || password.length >= 8)

  useEffect(() => {
    if (!editingUser) {
      form.reset(defaultValues)
      return
    }
    form.reset({
      email: editingUser.email,
      name: editingUser.name,
      password: '',
      role: editingUser.role,
      language: editingUser.language,
      disabled: editingUser.disabled,
    })
  }, [editingUser, form])

  const save = useMutation({
    mutationFn: (values: UserForm) => {
      if (editingUser)
        return api.updateUser(editingUser.id, values)
      return api.createUser(values)
    },
    onSuccess: () => {
      toast.success(editingUser ? t('usersPage.updated') : t('usersPage.created'))
      if (!editingUser)
        setPage(1)
      setEditingUser(null)
      setDialogOpen(false)
      form.reset(defaultValues)
      queryClient.invalidateQueries({ queryKey: ['users'] })
    },
    onError: error => toast.error(error.message),
  })

  const resetMFA = useMutation({
    mutationFn: api.resetUserMFA,
    onSuccess: () => {
      toast.success(t('usersPage.mfaResetSuccess'))
      queryClient.invalidateQueries({ queryKey: ['users'] })
    },
    onError: error => toast.error(error.message),
  })

  const userItems = users.data?.items ?? []
  const columns: DataListColumn<User>[] = [
    {
      key: 'name',
      header: t('usersPage.name'),
      className: 'w-[20%] px-4 py-3 align-middle',
      render: user => (
        <div className="flex min-w-0 items-center gap-3">
          <UserAvatar className="size-9" user={user} />
          <div className="min-w-0">
            <p className="truncate font-medium text-foreground">{user.name}</p>
            <p className="truncate text-xs text-muted-foreground">{user.email}</p>
          </div>
        </div>
      ),
    },
    {
      key: 'role',
      header: t('usersPage.globalRole'),
      className: 'w-[13%] px-4 py-3 align-middle',
      render: user => <StatusBadge>{user.role === 'platform_admin' ? t('usersPage.platformAdmin') : t('usersPage.normalUser')}</StatusBadge>,
    },
    {
      key: 'authType',
      header: t('usersPage.authType'),
      className: 'w-[10%] px-4 py-3 align-middle',
      render: user => <StatusBadge>{user.authType}</StatusBadge>,
    },
    {
      key: 'mfa',
      header: t('usersPage.mfa'),
      className: 'w-[10%] px-4 py-3 align-middle',
      render: user => (
        <StatusBadge tone={user.mfaEnabled ? 'success' : 'neutral'}>
          {user.mfaEnabled ? t('common.enabled') : t('common.disabled')}
        </StatusBadge>
      ),
    },
    {
      key: 'language',
      header: t('language'),
      className: 'w-[10%] px-4 py-3 align-middle text-muted-foreground',
      render: user => user.language === 'en-US' ? t('languages.enUS') : t('languages.zhCN'),
    },
    {
      key: 'status',
      header: t('usersPage.status'),
      className: 'w-[10%] px-4 py-3 align-middle',
      render: user => <StatusValueBadge value={user.disabled ? 'disabled' : 'active'} />,
    },
    {
      key: 'balance',
      header: t('usersPage.balance'),
      className: 'w-[12%] px-4 py-3 align-middle font-medium tabular-nums',
      render: user => billingDisplay.formatAmountWithUnit(user.balanceCredits),
    },
    {
      key: 'createdAt',
      header: t('usersPage.createdAt'),
      className: 'w-[13%] px-4 py-3 align-middle text-muted-foreground',
      render: user => formatAbsoluteDateTime(user.createdAt),
    },
    {
      key: 'actions',
      header: t('usersPage.actions'),
      className: 'w-[1%] whitespace-nowrap px-4 py-3 align-middle text-right',
      render: user => (
        <div className="flex justify-end gap-1">
          <EditActionButton
            type="button"
            label={t('edit')}
            onClick={() => {
              setEditingUser(user)
              setDialogOpen(true)
            }}
          />
          {user.mfaEnabled && user.id !== currentUser?.id && (
            <ConfirmDialog
              confirmText={t('usersPage.resetMFA')}
              description={t('usersPage.resetMFADescription', { name: user.name })}
              pending={resetMFA.isPending && resetMFA.variables === user.id}
              title={t('usersPage.resetMFATitle')}
              onConfirm={() => resetMFA.mutateAsync(user.id)}
            >
              <Button className="text-danger hover:text-danger" variant="ghost">
                <ShieldOff size={16} />
                {t('usersPage.resetMFA')}
              </Button>
            </ConfirmDialog>
          )}
        </div>
      ),
    },
  ]

  return (
    <div className="grid gap-6">
      <PageHeader
        actions={(
          <Button
            onClick={() => {
              setEditingUser(null)
              form.reset(defaultValues)
              setDialogOpen(true)
            }}
          >
            <UserPlus size={16} />
            {t('usersPage.createTitle')}
          </Button>
        )}
        description={t('usersPage.description')}
        title={t('usersPage.title')}
      />

      <div className="grid min-w-0 self-start">
        {users.isError && <ErrorState title={t('usersPage.loadFailedTitle')} description={t('common.platformAdminPermissionRequired')} />}
        <DataList
          columns={columns}
          emptyDescription={t('usersPage.emptyDescription')}
          emptyTitle={t('usersPage.emptyTitle')}
          items={userItems}
          pagination={{
            page: users.data?.page ?? page,
            pageSize: users.data?.pageSize ?? pageSize,
            pageSizeOptions: PAGE_SIZE_OPTIONS,
            total: users.data?.total ?? 0,
            totalPages: users.data?.totalPages ?? 0,
            pageInfoLabel: t('pagination.pageInfo', {
              page: users.data?.page ?? page,
              totalPages: users.data?.totalPages ?? 0,
              total: users.data?.total ?? 0,
            }),
            onPageChange: setPage,
            onPageSizeChange: (nextPageSize) => {
              setPageSize(nextPageSize)
              setPage(1)
            },
          }}
          rowKey={user => user.id}
          search={{
            value: search,
            placeholder: t('usersPage.searchPlaceholder'),
            onChange: (value) => {
              setSearch(value)
              setPage(1)
            },
          }}
          title={t('usersPage.listTitle')}
        />
      </div>

      <Dialog
        open={dialogOpen}
        onOpenChange={(open) => {
          setDialogOpen(open)
          if (!open) {
            setEditingUser(null)
            form.reset(defaultValues)
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editingUser ? t('usersPage.editTitle') : t('usersPage.createTitle')}</DialogTitle>
            <DialogDescription>{t('usersPage.description')}</DialogDescription>
          </DialogHeader>
          <form className="grid gap-3" onSubmit={form.handleSubmit(values => save.mutate(values))}>
            <Field error={form.formState.errors.email?.message} hint={t('usersPage.emailHint')} label={t('usersPage.email')} required>
              <Input {...form.register('email')} aria-invalid={Boolean(form.formState.errors.email)} autoComplete="email" />
            </Field>
            <Field error={form.formState.errors.name?.message} hint={t('usersPage.nameHint')} label={t('usersPage.name')} required>
              <Input {...form.register('name')} aria-invalid={Boolean(form.formState.errors.name)} autoComplete="name" />
            </Field>
            <Field error={passwordError} hint={editingUser ? t('usersPage.resetPasswordHint') : t('usersPage.passwordHint')} label={editingUser ? t('usersPage.resetPassword') : t('usersPage.password')} required={!editingUser}>
              <Input {...form.register('password')} aria-invalid={Boolean(passwordError)} autoComplete="new-password" placeholder={editingUser ? t('usersPage.resetPasswordPlaceholder') : t('usersPage.passwordPlaceholder')} type="password" />
            </Field>
            <Field error={form.formState.errors.role?.message} hint={t('usersPage.globalRoleHint')} label={t('usersPage.globalRole')} required>
              <Select {...form.register('role')} aria-invalid={Boolean(form.formState.errors.role)}>
                <option value="user">{t('usersPage.normalUser')}</option>
                <option value="platform_admin">{t('usersPage.platformAdmin')}</option>
              </Select>
            </Field>
            <Field error={form.formState.errors.language?.message} hint={t('usersPage.languageHint')} label={t('language')} required>
              <Select {...form.register('language')} aria-invalid={Boolean(form.formState.errors.language)}>
                <option value="zh-CN">{t('languages.zhCN')}</option>
                <option value="en-US">{t('languages.enUS')}</option>
              </Select>
            </Field>
            <CheckboxField {...form.register('disabled')}>
              {t('usersPage.disabled')}
            </CheckboxField>
            <DialogFooter>
              <Button disabled={save.isPending || !canSubmit} type="submit">
                {editingUser ? <Save size={16} /> : <UserPlus size={16} />}
                {editingUser ? t('usersPage.save') : t('usersPage.create')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}
