import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Box, ShieldPlus } from 'lucide-react'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '../../api/client'
import { usePublicConfig } from '../../app/public-config-context'
import { PageMotion } from '../../components/common/motion'
import { Button } from '../../components/ui/button'
import { Card } from '../../components/ui/card'
import { Field, Input } from '../../components/ui/input'

const schema = z.object({
  email: z.string().email('请输入有效邮箱'),
  name: z.string().min(1, '请输入管理员名称'),
  password: z.string().min(8, '密码至少 8 位'),
})

type BootstrapForm = z.infer<typeof schema>

export function BootstrapPage() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const configs = usePublicConfig()
  const status = useQuery({ queryKey: ['bootstrap-status'], queryFn: api.getBootstrapStatus })
  const form = useForm<BootstrapForm>({
    resolver: zodResolver(schema),
    mode: 'onChange',
    defaultValues: {
      email: '',
      name: 'Platform Admin',
      password: '',
    },
  })

  useEffect(() => {
    if (status.data?.initialized)
      navigate('/login', { replace: true })
  }, [navigate, status.data?.initialized])

  const initialize = useMutation({
    mutationFn: (values: BootstrapForm) => api.initializeAdmin({ ...values, language: 'zh-CN' }),
    onSuccess: () => {
      toast.success('平台管理员已初始化')
      queryClient.invalidateQueries({ queryKey: ['current-user'] })
      queryClient.invalidateQueries({ queryKey: ['bootstrap-status'] })
      navigate('/projects')
    },
    onError: error => toast.error(error.message),
  })

  return (
    <div className="grid min-h-screen place-items-center bg-background px-4 text-foreground">
      <PageMotion className="w-full max-w-md">
        <Card>
          <div className="mb-6 flex items-center gap-3">
            <span className="flex size-10 items-center justify-center rounded-md bg-primary text-primary-foreground">
              {configs['site.logoUrl']
                ? <img alt="" className="size-7 rounded-sm object-contain" src={configs['site.logoUrl']} />
                : <Box size={20} />}
            </span>
            <div>
              <h1 className="text-lg font-semibold">
                初始化
                {configs['site.title'] || 'Liteyuki DevOps'}
              </h1>
              <p className="text-sm text-muted-foreground">创建第一个平台管理员账号。</p>
            </div>
          </div>

          <form className="grid gap-3" onSubmit={form.handleSubmit(values => initialize.mutate(values))}>
            <Field error={form.formState.errors.email?.message} label="管理员邮箱" required>
              <Input {...form.register('email')} aria-invalid={Boolean(form.formState.errors.email)} autoComplete="email" />
            </Field>
            <Field error={form.formState.errors.name?.message} label="管理员名称" required>
              <Input {...form.register('name')} aria-invalid={Boolean(form.formState.errors.name)} autoComplete="name" />
            </Field>
            <Field error={form.formState.errors.password?.message} label="密码" required>
              <Input {...form.register('password')} aria-invalid={Boolean(form.formState.errors.password)} autoComplete="new-password" type="password" />
            </Field>
            <Button disabled={initialize.isPending || status.isLoading || !form.formState.isValid} type="submit">
              <ShieldPlus size={16} />
              创建管理员
            </Button>
            <p className="text-xs text-muted-foreground">
              仅当平台没有任何 PlatformAdmin 时允许初始化。生产环境不会显示开发默认账号。
            </p>
          </form>
        </Card>
      </PageMotion>
    </div>
  )
}
