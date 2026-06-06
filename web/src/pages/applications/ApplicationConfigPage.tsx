import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Save } from 'lucide-react'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { Link, useParams } from 'react-router-dom'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '../../api/client'
import { ErrorState } from '../../components/common/error-state'
import { MotionItem, MotionList } from '../../components/common/motion'
import { PageHeader } from '../../components/common/page-header'
import { Button } from '../../components/ui/button'
import { Card } from '../../components/ui/card'
import { Field, Input, Select } from '../../components/ui/input'

const schema = z.object({
  name: z.string().min(1, '请输入应用名称'),
  slug: z.string().min(1, '请输入应用标识').regex(/^[a-z0-9-]+$/, '只能包含小写字母、数字和 -'),
  sourceType: z.enum(['repository', 'image']),
  repositoryUrl: z.string().optional(),
  imageReference: z.string().optional(),
  dockerfilePath: z.string().optional(),
  buildContext: z.string().optional(),
  servicePort: z.coerce.number().int('请输入整数端口').positive('端口必须大于 0'),
})

type ApplicationFormInput = z.input<typeof schema>
type ApplicationForm = z.output<typeof schema>

export function ApplicationConfigPage() {
  const { projectId = '', applicationId = '' } = useParams()
  const queryClient = useQueryClient()
  const application = useQuery({
    queryKey: ['application', projectId, applicationId],
    queryFn: () => api.getApplication(projectId, applicationId),
    enabled: Boolean(projectId && applicationId),
  })
  const form = useForm<ApplicationFormInput, undefined, ApplicationForm>({
    resolver: zodResolver(schema),
    mode: 'onChange',
    defaultValues: {
      name: '',
      slug: '',
      sourceType: 'repository',
      repositoryUrl: '',
      imageReference: '',
      dockerfilePath: 'Dockerfile',
      buildContext: '.',
      servicePort: 8080,
    },
  })
  const sourceType = form.watch('sourceType')

  useEffect(() => {
    if (!application.data)
      return

    form.reset({
      name: application.data.name,
      slug: application.data.slug,
      sourceType: application.data.sourceType,
      repositoryUrl: application.data.repositoryUrl,
      imageReference: application.data.imageReference,
      dockerfilePath: application.data.dockerfilePath,
      buildContext: application.data.buildContext,
      servicePort: application.data.servicePort,
    })
  }, [application.data, form])

  const updateApplication = useMutation({
    mutationFn: (payload: ApplicationForm) => api.updateApplication(projectId, applicationId, {
      name: payload.name,
      slug: payload.slug,
      sourceType: payload.sourceType,
      repositoryUrl: payload.repositoryUrl ?? '',
      imageReference: payload.imageReference ?? '',
      dockerfilePath: payload.dockerfilePath ?? 'Dockerfile',
      buildContext: payload.buildContext ?? '.',
      servicePort: payload.servicePort,
    }),
    onSuccess: (result) => {
      toast.success('应用配置已保存')
      queryClient.setQueryData(['application', projectId, applicationId], result)
      queryClient.invalidateQueries({ queryKey: ['applications', projectId] })
    },
    onError: error => toast.error(error.message),
  })

  return (
    <div className="grid gap-6">
      <PageHeader
        actions={<Link className="text-sm text-primary hover:underline" to={`/projects/${projectId}/apps`}>返回应用</Link>}
        description="编辑应用来源、构建入口和服务端口。"
        title="应用配置"
      />

      {application.isError && <ErrorState title="应用加载失败" description="请确认应用存在，并且你有项目访问权限。" />}

      <Card className="max-w-2xl">
        <form onSubmit={form.handleSubmit(values => updateApplication.mutate(values))}>
          <MotionList className="grid gap-4">
            <div className="grid gap-3 md:grid-cols-2">
              <MotionItem><Field error={form.formState.errors.name?.message} label="应用名称" required><Input {...form.register('name')} aria-invalid={Boolean(form.formState.errors.name)} /></Field></MotionItem>
              <MotionItem><Field error={form.formState.errors.slug?.message} label="应用标识" required><Input {...form.register('slug')} aria-invalid={Boolean(form.formState.errors.slug)} /></Field></MotionItem>
            </div>
            <MotionItem>
              <Field error={form.formState.errors.sourceType?.message} label="来源类型" required>
                <Select {...form.register('sourceType')} aria-invalid={Boolean(form.formState.errors.sourceType)}>
                  <option value="repository">代码仓库</option>
                  <option value="image">已有镜像</option>
                </Select>
              </Field>
            </MotionItem>
            {sourceType === 'repository'
              ? (
                  <>
                    <MotionItem>
                      <Field error={form.formState.errors.repositoryUrl?.message} label="仓库地址">
                        <Input {...form.register('repositoryUrl')} aria-invalid={Boolean(form.formState.errors.repositoryUrl)} placeholder="https://github.com/org/repo" />
                      </Field>
                    </MotionItem>
                    <div className="grid gap-3 md:grid-cols-2">
                      <MotionItem><Field error={form.formState.errors.dockerfilePath?.message} label="Dockerfile"><Input {...form.register('dockerfilePath')} aria-invalid={Boolean(form.formState.errors.dockerfilePath)} /></Field></MotionItem>
                      <MotionItem><Field error={form.formState.errors.buildContext?.message} label="构建上下文"><Input {...form.register('buildContext')} aria-invalid={Boolean(form.formState.errors.buildContext)} /></Field></MotionItem>
                    </div>
                  </>
                )
              : (
                  <MotionItem>
                    <Field error={form.formState.errors.imageReference?.message} label="镜像地址">
                      <Input {...form.register('imageReference')} aria-invalid={Boolean(form.formState.errors.imageReference)} placeholder="harbor.local/library/app:latest" />
                    </Field>
                  </MotionItem>
                )}
            <MotionItem>
              <Field error={form.formState.errors.servicePort?.message} label="服务端口" required>
                <Input type="number" {...form.register('servicePort')} aria-invalid={Boolean(form.formState.errors.servicePort)} />
              </Field>
            </MotionItem>
            <MotionItem>
              <Button className="w-fit" disabled={updateApplication.isPending || !form.formState.isValid} type="submit">
                <Save size={16} />
                保存配置
              </Button>
            </MotionItem>
          </MotionList>
        </form>
      </Card>
    </div>
  )
}
