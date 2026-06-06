import type { AuthAdmissionPolicy, AuthProvider } from '../../api/client'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Save, ShieldCheck } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '../../api/client'
import { ErrorState } from '../../components/common/error-state'
import { MotionItem, MotionList } from '../../components/common/motion'
import { PageHeader } from '../../components/common/page-header'
import { StatusBadge } from '../../components/common/status-badge'
import { Button } from '../../components/ui/button'
import { Card } from '../../components/ui/card'
import { Field, Input, Select, Textarea } from '../../components/ui/input'

const providerSchema = z.object({
  name: z.string().min(1),
  enabled: z.boolean(),
  issuerUrl: z.string().url(),
  clientId: z.string().min(1),
  clientSecretRef: z.string(),
  scopes: z.string().min(1),
  groupClaim: z.string().min(1),
  emailClaim: z.string().min(1),
  usernameClaim: z.string().min(1),
  isDefault: z.boolean(),
})

const policySchema = z.object({
  allowLocalLogin: z.boolean(),
  allowOidcLogin: z.boolean(),
  allowedEmailDomains: z.string(),
  allowedOidcGroups: z.string(),
  invitedEmails: z.string(),
  defaultRole: z.enum(['platform_admin', 'user']),
})

type ProviderForm = z.infer<typeof providerSchema>
type PolicyForm = z.infer<typeof policySchema>

const providerDefaults: ProviderForm = {
  name: '',
  enabled: true,
  issuerUrl: '',
  clientId: '',
  clientSecretRef: 'env:OIDC_CLIENT_SECRET',
  scopes: 'openid profile email',
  groupClaim: 'groups',
  emailClaim: 'email',
  usernameClaim: 'preferred_username',
  isDefault: false,
}

export function AuthProvidersPage() {
  const queryClient = useQueryClient()
  const [editingProvider, setEditingProvider] = useState<AuthProvider | null>(null)
  const providers = useQuery({ queryKey: ['auth-providers', 'admin'], queryFn: () => api.listAuthProviders(true) })
  const policy = useQuery({ queryKey: ['auth-admission-policy'], queryFn: api.getAuthAdmissionPolicy })
  const providerForm = useForm<ProviderForm>({ resolver: zodResolver(providerSchema), mode: 'onChange', defaultValues: providerDefaults })
  const policyForm = useForm<PolicyForm>({
    resolver: zodResolver(policySchema),
    mode: 'onChange',
    defaultValues: {
      allowLocalLogin: true,
      allowOidcLogin: true,
      allowedEmailDomains: '',
      allowedOidcGroups: '',
      invitedEmails: '',
      defaultRole: 'user',
    },
  })

  useEffect(() => {
    if (!editingProvider) {
      providerForm.reset(providerDefaults)
      return
    }
    providerForm.reset({
      name: editingProvider.name,
      enabled: editingProvider.enabled,
      issuerUrl: editingProvider.issuerUrl,
      clientId: editingProvider.clientId,
      clientSecretRef: editingProvider.clientSecretRef,
      scopes: editingProvider.scopes,
      groupClaim: editingProvider.groupClaim,
      emailClaim: editingProvider.emailClaim,
      usernameClaim: editingProvider.usernameClaim,
      isDefault: editingProvider.isDefault,
    })
  }, [editingProvider, providerForm])

  useEffect(() => {
    if (!policy.data)
      return
    policyForm.reset({
      allowLocalLogin: policy.data.allowLocalLogin,
      allowOidcLogin: policy.data.allowOidcLogin,
      allowedEmailDomains: (policy.data.allowedEmailDomains ?? []).join(', '),
      allowedOidcGroups: (policy.data.allowedOidcGroups ?? []).join(', '),
      invitedEmails: (policy.data.invitedEmails ?? []).join(', '),
      defaultRole: policy.data.defaultRole,
    })
  }, [policy.data, policyForm])

  const saveProvider = useMutation({
    mutationFn: (values: ProviderForm) => {
      const payload = { ...values, type: 'oidc' as const }
      if (editingProvider)
        return api.updateAuthProvider(editingProvider.id, payload)
      return api.createAuthProvider(payload)
    },
    onSuccess: () => {
      toast.success(editingProvider ? '身份源已更新' : '身份源已创建')
      setEditingProvider(null)
      queryClient.invalidateQueries({ queryKey: ['auth-providers'] })
    },
    onError: error => toast.error(error.message),
  })

  const savePolicy = useMutation({
    mutationFn: (values: PolicyForm) => api.updateAuthAdmissionPolicy({
      allowLocalLogin: values.allowLocalLogin,
      allowOidcLogin: values.allowOidcLogin,
      allowedEmailDomains: splitText(values.allowedEmailDomains),
      allowedOidcGroups: splitText(values.allowedOidcGroups),
      invitedEmails: splitText(values.invitedEmails),
      defaultRole: values.defaultRole,
    }),
    onSuccess: (result: AuthAdmissionPolicy) => {
      toast.success('准入策略已保存')
      queryClient.setQueryData(['auth-admission-policy'], result)
    },
    onError: error => toast.error(error.message),
  })

  return (
    <div className="grid gap-6">
      <PageHeader
        description="配置多个 OIDC Provider，并用准入策略控制谁能进入平台。"
        title="身份源"
      />

      <div className="grid gap-4 xl:grid-cols-[420px_1fr]">
        <Card>
          <form className="grid gap-3" onSubmit={providerForm.handleSubmit(values => saveProvider.mutate(values))}>
            <h2 className="text-base font-semibold">{editingProvider ? '编辑 OIDC Provider' : '创建 OIDC Provider'}</h2>
            <Field error={providerForm.formState.errors.name?.message} label="名称" required>
              <Input {...providerForm.register('name')} aria-invalid={Boolean(providerForm.formState.errors.name)} placeholder="Casdoor" />
            </Field>
            <Field error={providerForm.formState.errors.issuerUrl?.message} label="Issuer URL" required>
              <Input {...providerForm.register('issuerUrl')} aria-invalid={Boolean(providerForm.formState.errors.issuerUrl)} placeholder="https://sso.example.com" />
            </Field>
            <Field error={providerForm.formState.errors.clientId?.message} label="Client ID" required>
              <Input {...providerForm.register('clientId')} aria-invalid={Boolean(providerForm.formState.errors.clientId)} />
            </Field>
            <Field error={providerForm.formState.errors.clientSecretRef?.message} label="Client Secret 引用">
              <Input {...providerForm.register('clientSecretRef')} aria-invalid={Boolean(providerForm.formState.errors.clientSecretRef)} placeholder="env:OIDC_CLIENT_SECRET" />
            </Field>
            <Field error={providerForm.formState.errors.scopes?.message} label="Scopes" required>
              <Input {...providerForm.register('scopes')} aria-invalid={Boolean(providerForm.formState.errors.scopes)} />
            </Field>
            <div className="grid grid-cols-3 gap-3">
              <Field error={providerForm.formState.errors.groupClaim?.message} label="Group Claim" required>
                <Input {...providerForm.register('groupClaim')} aria-invalid={Boolean(providerForm.formState.errors.groupClaim)} />
              </Field>
              <Field error={providerForm.formState.errors.emailClaim?.message} label="Email Claim" required>
                <Input {...providerForm.register('emailClaim')} aria-invalid={Boolean(providerForm.formState.errors.emailClaim)} />
              </Field>
              <Field error={providerForm.formState.errors.usernameClaim?.message} label="Username Claim" required>
                <Input {...providerForm.register('usernameClaim')} aria-invalid={Boolean(providerForm.formState.errors.usernameClaim)} />
              </Field>
            </div>
            <label className="flex items-center gap-2 text-sm">
              <input type="checkbox" {...providerForm.register('enabled')} />
              启用
            </label>
            <label className="flex items-center gap-2 text-sm">
              <input type="checkbox" {...providerForm.register('isDefault')} />
              默认身份源
            </label>
            <div className="flex gap-2">
              <Button disabled={saveProvider.isPending || !providerForm.formState.isValid} type="submit">
                <Save size={16} />
                {editingProvider ? '保存身份源' : '创建身份源'}
              </Button>
              {editingProvider && (
                <Button type="button" variant="secondary" onClick={() => setEditingProvider(null)}>取消</Button>
              )}
            </div>
          </form>
        </Card>

        <div className="grid gap-4">
          <Card>
            <form className="grid gap-3" onSubmit={policyForm.handleSubmit(values => savePolicy.mutate(values))}>
              <h2 className="text-base font-semibold">准入策略</h2>
              {policy.isError && <ErrorState title="准入策略加载失败" description="请确认当前账号具有平台管理员权限。" />}
              <div className="grid gap-3 md:grid-cols-2">
                <label className="flex items-center gap-2 text-sm">
                  <input type="checkbox" {...policyForm.register('allowLocalLogin')} />
                  允许本地账号登录
                </label>
                <label className="flex items-center gap-2 text-sm">
                  <input type="checkbox" {...policyForm.register('allowOidcLogin')} />
                  允许 OIDC 登录
                </label>
              </div>
              <Field error={policyForm.formState.errors.allowedEmailDomains?.message} label="允许邮箱域">
                <Textarea {...policyForm.register('allowedEmailDomains')} aria-invalid={Boolean(policyForm.formState.errors.allowedEmailDomains)} placeholder="example.com, liteyuki.dev" />
              </Field>
              <Field error={policyForm.formState.errors.allowedOidcGroups?.message} label="允许 OIDC 组">
                <Textarea {...policyForm.register('allowedOidcGroups')} aria-invalid={Boolean(policyForm.formState.errors.allowedOidcGroups)} placeholder="devops-admins, platform-users" />
              </Field>
              <Field error={policyForm.formState.errors.invitedEmails?.message} label="邀请邮箱">
                <Textarea {...policyForm.register('invitedEmails')} aria-invalid={Boolean(policyForm.formState.errors.invitedEmails)} placeholder="user@example.com" />
              </Field>
              <Field error={policyForm.formState.errors.defaultRole?.message} label="默认全局角色" required>
                <Select {...policyForm.register('defaultRole')} aria-invalid={Boolean(policyForm.formState.errors.defaultRole)}>
                  <option value="user">普通用户</option>
                  <option value="platform_admin">平台管理员</option>
                </Select>
              </Field>
              <Button disabled={savePolicy.isPending || !policyForm.formState.isValid} type="submit">
                <ShieldCheck size={16} />
                保存准入策略
              </Button>
            </form>
          </Card>

          <Card>
            {providers.isError && <ErrorState title="身份源加载失败" description="请确认当前账号具有平台管理员权限。" />}
            <MotionList className="grid gap-3">
              {(providers.data ?? []).map(provider => (
                <MotionItem key={provider.id}>
                  <button
                    className="grid w-full gap-2 rounded-md border border-border bg-background p-3 text-left transition duration-150 hover:border-primary hover:shadow-sm"
                    type="button"
                    onClick={() => setEditingProvider(provider)}
                  >
                    <div className="flex items-center justify-between gap-3">
                      <div>
                        <p className="font-medium">{provider.name}</p>
                        <p className="text-sm text-muted-foreground">{provider.issuerUrl}</p>
                      </div>
                      <div className="flex gap-2">
                        {provider.isDefault && <StatusBadge>default</StatusBadge>}
                        <StatusBadge>{provider.enabled ? 'enabled' : 'disabled'}</StatusBadge>
                      </div>
                    </div>
                    <p className="text-xs text-muted-foreground">
                      {provider.groupClaim}
                      {' '}
                      /
                      {' '}
                      {provider.scopes}
                    </p>
                  </button>
                </MotionItem>
              ))}
            </MotionList>
          </Card>
        </div>
      </div>
    </div>
  )
}

function splitText(value: string) {
  return value.split(/[\n,]/).map(item => item.trim()).filter(Boolean)
}
