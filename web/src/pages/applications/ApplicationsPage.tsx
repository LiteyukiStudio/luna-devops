import type { Application } from '../../api/client'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Box, ExternalLink, Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { Link, useParams } from 'react-router-dom'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '../../api/client'
import { ConfirmDialog } from '../../components/common/confirm-dialog'
import { EmptyState } from '../../components/common/empty-state'
import { ErrorState } from '../../components/common/error-state'
import { MotionItem, MotionList } from '../../components/common/motion'
import { PageHeader } from '../../components/common/page-header'
import { StatusBadge } from '../../components/common/status-badge'
import { Button } from '../../components/ui/button'
import { Card } from '../../components/ui/card'
import { Field, Input, Select, Textarea } from '../../components/ui/input'

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

export function ApplicationsPage() {
  const { projectId = '' } = useParams()
  const queryClient = useQueryClient()
  const [applicationToDelete, setApplicationToDelete] = useState<Application | null>(null)
  const [appYaml, setAppYaml] = useState('')
  const applications = useQuery({
    queryKey: ['applications', projectId],
    queryFn: () => api.listApplications(projectId),
    enabled: Boolean(projectId),
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

  const createApplication = useMutation({
    mutationFn: (payload: ApplicationForm) => api.createApplication(projectId, {
      name: payload.name,
      slug: payload.slug,
      sourceType: payload.sourceType,
      repositoryUrl: payload.repositoryUrl ?? '',
      imageReference: payload.imageReference ?? '',
      dockerfilePath: payload.dockerfilePath ?? 'Dockerfile',
      buildContext: payload.buildContext ?? '.',
      servicePort: payload.servicePort,
    }),
    onSuccess: () => {
      toast.success('应用已创建')
      form.reset()
      queryClient.invalidateQueries({ queryKey: ['applications', projectId] })
    },
    onError: error => toast.error(error.message),
  })

  const parseConfig = useMutation({
    mutationFn: () => api.parseApplicationConfig(projectId, appYaml),
    onSuccess: (result) => {
      form.reset({
        name: result.name,
        slug: result.slug,
        sourceType: result.sourceType,
        repositoryUrl: result.repositoryUrl,
        imageReference: result.imageReference,
        dockerfilePath: result.dockerfilePath,
        buildContext: result.buildContext,
        servicePort: result.servicePort,
      })
      toast.success('.devops/app.yaml 已解析')
    },
    onError: error => toast.error(error.message),
  })

  const deleteApplication = useMutation({
    mutationFn: (applicationId: string) => api.deleteApplication(projectId, applicationId),
    onSuccess: () => {
      toast.success('应用已删除')
      setApplicationToDelete(null)
      queryClient.invalidateQueries({ queryKey: ['applications', projectId] })
    },
    onError: error => toast.error(error.message),
  })

  return (
    <div className="grid gap-6">
      <PageHeader
        actions={<Link className="text-sm text-primary hover:underline" to="/projects">返回项目</Link>}
        description="先支持 repository 和 image 两类来源，部署和构建后续联调。"
        title="应用"
      />

      <div className="grid gap-4 lg:grid-cols-[420px_1fr]">
        <Card>
          <h2 className="mb-4 text-base font-semibold">创建应用</h2>
          <form className="grid gap-3" onSubmit={form.handleSubmit(values => createApplication.mutate(values))}>
            <Field label=".devops/app.yaml">
              <Textarea
                placeholder="粘贴 .devops/app.yaml 内容后点击解析"
                value={appYaml}
                onChange={event => setAppYaml(event.target.value)}
              />
            </Field>
            <Button disabled={parseConfig.isPending || appYaml.trim() === ''} type="button" variant="secondary" onClick={() => parseConfig.mutate()}>
              解析配置
            </Button>
            <div className="grid grid-cols-2 gap-3">
              <Field error={form.formState.errors.name?.message} label="应用名称" required>
                <Input {...form.register('name')} aria-invalid={Boolean(form.formState.errors.name)} placeholder="控制台" />
              </Field>
              <Field error={form.formState.errors.slug?.message} label="应用标识" required>
                <Input {...form.register('slug')} aria-invalid={Boolean(form.formState.errors.slug)} placeholder="console" />
              </Field>
            </div>
            <Field error={form.formState.errors.sourceType?.message} label="来源类型" required>
              <Select {...form.register('sourceType')} aria-invalid={Boolean(form.formState.errors.sourceType)}>
                <option value="repository">代码仓库</option>
                <option value="image">已有镜像</option>
              </Select>
            </Field>
            {sourceType === 'repository'
              ? (
                  <>
                    <Field error={form.formState.errors.repositoryUrl?.message} label="仓库地址">
                      <Input {...form.register('repositoryUrl')} aria-invalid={Boolean(form.formState.errors.repositoryUrl)} placeholder="https://github.com/org/repo" />
                    </Field>
                    <div className="grid grid-cols-2 gap-3">
                      <Field error={form.formState.errors.dockerfilePath?.message} label="Dockerfile">
                        <Input {...form.register('dockerfilePath')} aria-invalid={Boolean(form.formState.errors.dockerfilePath)} />
                      </Field>
                      <Field error={form.formState.errors.buildContext?.message} label="构建上下文">
                        <Input {...form.register('buildContext')} aria-invalid={Boolean(form.formState.errors.buildContext)} />
                      </Field>
                    </div>
                  </>
                )
              : (
                  <Field error={form.formState.errors.imageReference?.message} label="镜像地址">
                    <Input {...form.register('imageReference')} aria-invalid={Boolean(form.formState.errors.imageReference)} placeholder="harbor.local/library/app:latest" />
                  </Field>
                )}
            <Field error={form.formState.errors.servicePort?.message} label="服务端口" required>
              <Input type="number" {...form.register('servicePort')} aria-invalid={Boolean(form.formState.errors.servicePort)} />
            </Field>
            <Button disabled={createApplication.isPending || !form.formState.isValid} type="submit">
              <Plus size={16} />
              创建应用
            </Button>
          </form>
        </Card>

        <MotionList className="grid gap-3">
          {applications.isError && <ErrorState title="应用加载失败" description="请确认项目存在，并且你有项目访问权限。" />}
          {(applications.data ?? []).map(application => (
            <MotionItem key={application.id}>
              <ApplicationRow
                application={application}
                onDelete={() => setApplicationToDelete(application)}
              />
            </MotionItem>
          ))}
          {applications.data?.length === 0 && <EmptyState title="还没有应用" description="先创建一个 repository 或 image 来源的应用。" />}
        </MotionList>
      </div>
      <ConfirmDialog
        confirmText="删除应用"
        description={`应用 ${applicationToDelete?.name ?? ''} 会被删除，请确认后继续。`}
        open={Boolean(applicationToDelete)}
        pending={deleteApplication.isPending}
        title="删除应用"
        onConfirm={() => applicationToDelete && deleteApplication.mutate(applicationToDelete.id)}
        onOpenChange={open => !open && setApplicationToDelete(null)}
      />
    </div>
  )
}

function ApplicationRow({ application, onDelete }: { application: Application, onDelete: () => void }) {
  return (
    <Card className="flex items-center justify-between gap-4">
      <div className="flex min-w-0 items-center gap-3">
        <span className="flex size-10 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
          <Box size={18} />
        </span>
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <h3 className="truncate font-medium">{application.name}</h3>
            <StatusBadge>{application.sourceType}</StatusBadge>
          </div>
          <p className="truncate text-sm text-muted-foreground">
            {application.sourceType === 'repository' ? application.repositoryUrl : application.imageReference}
          </p>
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <StatusBadge>
          {application.servicePort}
          /tcp
        </StatusBadge>
        <Link
          aria-label="打开应用配置"
          className="inline-flex h-9 items-center justify-center rounded-md px-3 text-sm font-medium text-muted-foreground transition hover:bg-muted hover:text-foreground"
          to={`/projects/${application.projectId}/apps/${application.id}`}
        >
          <ExternalLink size={16} />
        </Link>
        <Button aria-label="删除应用" variant="ghost" onClick={onDelete}>
          <Trash2 size={16} />
        </Button>
      </div>
    </Card>
  )
}
