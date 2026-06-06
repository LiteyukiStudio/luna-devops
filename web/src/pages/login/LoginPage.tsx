import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Box, LogIn } from 'lucide-react'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { toast } from 'sonner'
import { z } from 'zod'
import { api, oidcStartUrl } from '../../api/client'
import { usePublicConfig } from '../../app/public-config-context'
import { PageMotion } from '../../components/common/motion'
import { Button } from '../../components/ui/button'
import { Card } from '../../components/ui/card'
import { Field, Input } from '../../components/ui/input'

const schema = z.object({
  email: z.string().email('请输入有效邮箱'),
  password: z.string().min(1, '请输入密码'),
})

type LoginForm = z.infer<typeof schema>

export function LoginPage() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const queryClient = useQueryClient()
  const configs = usePublicConfig()
  const status = useQuery({ queryKey: ['bootstrap-status'], queryFn: api.getBootstrapStatus })
  const providers = useQuery({ queryKey: ['auth-providers'], queryFn: () => api.listAuthProviders(false) })
  const form = useForm<LoginForm>({
    resolver: zodResolver(schema),
    mode: 'onChange',
    defaultValues: {
      email: '',
      password: '',
    },
  })

  useEffect(() => {
    if (status.data && !status.data.initialized)
      navigate('/bootstrap', { replace: true })
    const devLoginHint = status.data?.mode === 'development' && status.data.devLoginEnabled
      ? status.data.devLoginHint
      : undefined
    if (devLoginHint) {
      form.reset({
        email: devLoginHint.email,
        password: devLoginHint.password,
      })
    }
  }, [form, navigate, status.data])

  useEffect(() => {
    const errorCode = searchParams.get('auth_error')
    if (errorCode)
      toast.error(authErrorMessage(errorCode))
  }, [searchParams])

  const login = useMutation({
    mutationFn: api.login,
    onSuccess: () => {
      toast.success('登录成功')
      queryClient.invalidateQueries({ queryKey: ['current-user'] })
      navigate('/projects')
    },
    onError: error => toast.error(error.message),
  })

  return (
    <div className="grid min-h-screen place-items-center bg-background px-4 text-foreground">
      <PageMotion className="w-full max-w-sm">
        <Card>
          <div className="mb-6 flex items-center gap-3">
            <span className="flex size-10 items-center justify-center rounded-md bg-primary text-primary-foreground">
              {configs['site.logoUrl']
                ? <img alt="" className="size-7 rounded-sm object-contain" src={configs['site.logoUrl']} />
                : <Box size={20} />}
            </span>
            <div>
              <h1 className="text-lg font-semibold">{configs['site.title'] || 'Liteyuki DevOps'}</h1>
              <p className="text-sm text-muted-foreground">{configs['site.loginSubtitle'] || '使用本地账号登录控制台'}</p>
            </div>
          </div>
          <form className="grid gap-3" onSubmit={form.handleSubmit(values => login.mutate(values))}>
            <Field error={form.formState.errors.email?.message} label="邮箱" required>
              <Input {...form.register('email')} aria-invalid={Boolean(form.formState.errors.email)} autoComplete="email" />
            </Field>
            <Field error={form.formState.errors.password?.message} label="密码" required>
              <Input {...form.register('password')} aria-invalid={Boolean(form.formState.errors.password)} autoComplete="current-password" type="password" />
            </Field>
            <Button disabled={login.isPending || !form.formState.isValid} type="submit">
              <LogIn size={16} />
              登录
            </Button>
            {status.data?.mode === 'development' && status.data.devLoginEnabled && status.data.devLoginHint && (
              <p className="text-xs text-muted-foreground">
                开发默认账号：
                {status.data.devLoginHint.email}
                {' '}
                /
                {' '}
                {status.data.devLoginHint.password}
              </p>
            )}
          </form>
          {(providers.data ?? []).length > 0 && (
            <div className="mt-5 grid gap-2 border-t border-border pt-5">
              {(providers.data ?? []).map(provider => (
                <Button
                  key={provider.id}
                  type="button"
                  variant="secondary"
                  onClick={() => {
                    window.location.href = oidcStartUrl(provider.id, 'login')
                  }}
                >
                  <LogIn size={16} />
                  使用
                  {' '}
                  {provider.name}
                  {' '}
                  登录
                </Button>
              ))}
            </div>
          )}
        </Card>
      </PageMotion>
    </div>
  )
}

function authErrorMessage(code: string) {
  const messages: Record<string, string> = {
    oidc_state_invalid: '登录状态已失效，请重新发起 OIDC 登录。',
    oidc_group_denied: '当前账号未命中允许的 OIDC 组，请联系平台管理员。',
    oidc_email_required: 'OIDC 账号需要提供已验证邮箱。',
    oidc_admission_denied: '当前账号未被邀请，也不在允许的邮箱域或组中。',
    oidc_provider_disabled: '该 OIDC 身份源已被禁用。',
    oidc_bind_failed: '第三方登录绑定失败，请确认该身份未绑定其他账号。',
    auth_forbidden: '当前账号没有执行此操作的权限。',
  }
  return messages[code] ?? 'OIDC 登录失败，请稍后重试或联系平台管理员。'
}
