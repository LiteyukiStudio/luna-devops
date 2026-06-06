import type { User } from '../../api/client'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Save, UserPlus } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '../../api/client'
import { EmptyState } from '../../components/common/empty-state'
import { ErrorState } from '../../components/common/error-state'
import { MotionItem, MotionList } from '../../components/common/motion'
import { PageHeader } from '../../components/common/page-header'
import { StatusBadge } from '../../components/common/status-badge'
import { Button } from '../../components/ui/button'
import { Card } from '../../components/ui/card'
import { Field, Input, Select } from '../../components/ui/input'

const schema = z.object({
  email: z.string().email('请输入有效邮箱'),
  name: z.string().min(1, '请输入名称'),
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

export function UsersPage() {
  const queryClient = useQueryClient()
  const [editingUser, setEditingUser] = useState<User | null>(null)
  const users = useQuery({ queryKey: ['users'], queryFn: api.listUsers })
  const form = useForm<UserForm>({ resolver: zodResolver(schema), mode: 'onChange', defaultValues })
  const password = form.watch('password')
  const passwordError = !editingUser && password.length < 8 ? '密码至少 8 位' : form.formState.errors.password?.message
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
      toast.success(editingUser ? '用户已更新' : '用户已创建')
      setEditingUser(null)
      queryClient.invalidateQueries({ queryKey: ['users'] })
    },
    onError: error => toast.error(error.message),
  })

  return (
    <div className="grid gap-6">
      <PageHeader
        description="创建和维护本地账号。OIDC 账号绑定会在后续身份源模块中补齐。"
        title="用户管理"
      />

      <div className="grid gap-4 lg:grid-cols-[360px_1fr]">
        <Card>
          <form className="grid gap-3" onSubmit={form.handleSubmit(values => save.mutate(values))}>
            <h2 className="text-base font-semibold">{editingUser ? '编辑用户' : '创建本地用户'}</h2>
            <Field error={form.formState.errors.email?.message} label="邮箱" required>
              <Input {...form.register('email')} aria-invalid={Boolean(form.formState.errors.email)} autoComplete="email" />
            </Field>
            <Field error={form.formState.errors.name?.message} label="名称" required>
              <Input {...form.register('name')} aria-invalid={Boolean(form.formState.errors.name)} autoComplete="name" />
            </Field>
            <Field error={passwordError} label={editingUser ? '重置密码' : '密码'} required={!editingUser}>
              <Input {...form.register('password')} aria-invalid={Boolean(passwordError)} autoComplete="new-password" placeholder={editingUser ? '留空则不修改' : '至少 8 位'} type="password" />
            </Field>
            <Field error={form.formState.errors.role?.message} label="全局角色" required>
              <Select {...form.register('role')} aria-invalid={Boolean(form.formState.errors.role)}>
                <option value="user">普通用户</option>
                <option value="platform_admin">平台管理员</option>
              </Select>
            </Field>
            <Field error={form.formState.errors.language?.message} label="语言" required>
              <Select {...form.register('language')} aria-invalid={Boolean(form.formState.errors.language)}>
                <option value="zh-CN">中文</option>
                <option value="en-US">English</option>
              </Select>
            </Field>
            <label className="flex items-center gap-2 text-sm">
              <input type="checkbox" {...form.register('disabled')} />
              禁用账号
            </label>
            <div className="flex gap-2">
              <Button disabled={save.isPending || !canSubmit} type="submit">
                {editingUser ? <Save size={16} /> : <UserPlus size={16} />}
                {editingUser ? '保存用户' : '创建用户'}
              </Button>
              {editingUser && (
                <Button type="button" variant="secondary" onClick={() => setEditingUser(null)}>
                  取消
                </Button>
              )}
            </div>
          </form>
        </Card>

        <Card>
          {users.isError && <ErrorState title="用户加载失败" description="请确认当前账号具有平台管理员权限。" />}
          {users.data?.length === 0 && <EmptyState title="还没有用户" description="先创建一个本地账号。" />}
          <MotionList className="grid gap-3">
            {(users.data ?? []).map(user => (
              <MotionItem key={user.id}>
                <button
                  className="grid w-full gap-2 rounded-md border border-border bg-background p-3 text-left transition duration-150 hover:border-primary hover:shadow-sm"
                  type="button"
                  onClick={() => setEditingUser(user)}
                >
                  <div className="flex flex-wrap items-center justify-between gap-3">
                    <p className="min-w-0 truncate font-medium">{user.name}</p>
                    <div className="flex flex-wrap items-center justify-end gap-2">
                      <StatusBadge>{user.disabled ? 'disabled' : 'active'}</StatusBadge>
                      <StatusBadge>{user.role === 'platform_admin' ? '平台管理员' : '普通用户'}</StatusBadge>
                      <StatusBadge>{user.authType}</StatusBadge>
                    </div>
                  </div>
                  <p className="truncate text-sm text-muted-foreground">{user.email}</p>
                </button>
              </MotionItem>
            ))}
          </MotionList>
        </Card>
      </div>
    </div>
  )
}
