import type { Project } from '../../api/client'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { FolderKanban, Pencil, Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { Link } from 'react-router-dom'
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
import { Field, Input, Textarea } from '../../components/ui/input'

const schema = z.object({
  name: z.string().min(1, '请输入项目名称'),
  slug: z.string().min(1, '请输入项目标识').regex(/^[a-z0-9-]+$/, '只能包含小写字母、数字和 -'),
  description: z.string().optional(),
})

type ProjectForm = z.infer<typeof schema>

export function ProjectsPage() {
  const queryClient = useQueryClient()
  const [editingProject, setEditingProject] = useState<Project | null>(null)
  const [projectToDelete, setProjectToDelete] = useState<Project | null>(null)
  const projects = useQuery({ queryKey: ['projects'], queryFn: api.listProjects })
  const form = useForm<ProjectForm>({
    resolver: zodResolver(schema),
    mode: 'onChange',
    defaultValues: { name: '', slug: '', description: '' },
  })

  const createProject = useMutation({
    mutationFn: api.createProject,
    onSuccess: () => {
      toast.success('项目已创建')
      form.reset()
      queryClient.invalidateQueries({ queryKey: ['projects'] })
    },
    onError: error => toast.error(error.message),
  })

  const deleteProject = useMutation({
    mutationFn: api.deleteProject,
    onSuccess: () => {
      toast.success('项目已删除')
      setProjectToDelete(null)
      queryClient.invalidateQueries({ queryKey: ['projects'] })
    },
    onError: error => toast.error(error.message),
  })

  const updateProject = useMutation({
    mutationFn: ({ projectId, payload }: { projectId: string, payload: Pick<Project, 'slug' | 'name' | 'description'> }) =>
      api.updateProject(projectId, payload),
    onSuccess: () => {
      toast.success('项目已更新')
      queryClient.invalidateQueries({ queryKey: ['projects'] })
    },
    onError: error => toast.error(error.message),
  })

  return (
    <div className="grid gap-6">
      <PageHeader title="项目" description="每个项目默认映射一个 Kubernetes Namespace，也可以承载多名成员协作。" />
      <div className="grid gap-4 lg:grid-cols-[360px_1fr]">
        <Card>
          <h2 className="mb-4 text-base font-semibold">{editingProject ? '编辑项目' : '创建项目'}</h2>
          <form
            className="grid gap-3"
            onSubmit={form.handleSubmit((values) => {
              const payload = { ...values, description: values.description ?? '' }
              if (editingProject) {
                updateProject.mutate({ projectId: editingProject.id, payload })
                setEditingProject(null)
                form.reset({ name: '', slug: '', description: '' })
                return
              }
              createProject.mutate(payload)
            })}
          >
            <Field error={form.formState.errors.name?.message} label="项目名称" required>
              <Input {...form.register('name')} aria-invalid={Boolean(form.formState.errors.name)} placeholder="轻雪工作台" />
            </Field>
            <Field error={form.formState.errors.slug?.message} label="项目标识" required>
              <Input {...form.register('slug')} aria-invalid={Boolean(form.formState.errors.slug)} placeholder="liteyuki-workbench" />
            </Field>
            <Field error={form.formState.errors.description?.message} label="描述">
              <Textarea {...form.register('description')} placeholder="这个项目负责哪些应用" />
            </Field>
            <Button disabled={createProject.isPending || updateProject.isPending || !form.formState.isValid} type="submit">
              <Plus size={16} />
              {editingProject ? '保存' : '创建'}
            </Button>
            {editingProject && (
              <Button
                variant="ghost"
                onClick={() => {
                  setEditingProject(null)
                  form.reset({ name: '', slug: '', description: '' })
                }}
              >
                取消编辑
              </Button>
            )}
          </form>
        </Card>
        <MotionList className="grid gap-3">
          {projects.isError && <ErrorState title="项目加载失败" description="请确认已经登录，且后端 API 可以访问。" />}
          {(projects.data ?? []).map(project => (
            <MotionItem key={project.id}>
              <ProjectRow
                onDelete={() => setProjectToDelete(project)}
                onEdit={() => {
                  setEditingProject(project)
                  form.reset({
                    name: project.name,
                    slug: project.slug,
                    description: project.description,
                  })
                }}
                project={project}
              />
            </MotionItem>
          ))}
          {projects.data?.length === 0 && <EmptyState title="还没有项目" description="先在左侧创建一个项目。" />}
        </MotionList>
      </div>
      <ConfirmDialog
        confirmText="删除项目"
        description={`项目 ${projectToDelete?.name ?? ''} 下的应用也会失去项目入口，请确认后继续。`}
        open={Boolean(projectToDelete)}
        pending={deleteProject.isPending}
        title="删除项目"
        onConfirm={() => projectToDelete && deleteProject.mutate(projectToDelete.id)}
        onOpenChange={open => !open && setProjectToDelete(null)}
      />
    </div>
  )
}

function ProjectRow({ project, onDelete, onEdit }: { project: Project, onDelete: () => void, onEdit: () => void }) {
  return (
    <Card className="flex items-center justify-between gap-4">
      <div className="flex min-w-0 items-center gap-3">
        <span className="flex size-10 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
          <FolderKanban size={18} />
        </span>
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <h3 className="truncate font-medium">{project.name}</h3>
            <StatusBadge>{project.namespaceStrategy}</StatusBadge>
          </div>
          <p className="truncate text-sm text-muted-foreground">
            {project.slug}
            {' '}
            ·
            {' '}
            {project.description || '暂无描述'}
          </p>
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <Link className="inline-flex h-9 items-center justify-center rounded-md border border-border bg-surface px-3 text-sm font-medium text-foreground transition hover:bg-muted" to={`/projects/${project.id}/apps`}>
          应用
        </Link>
        <Link className="inline-flex h-9 items-center justify-center rounded-md border border-border bg-surface px-3 text-sm font-medium text-foreground transition hover:bg-muted" to={`/projects/${project.id}/members`}>
          成员
        </Link>
        <Button aria-label="编辑项目" variant="ghost" onClick={onEdit}>
          <Pencil size={16} />
        </Button>
        <Button aria-label="删除项目" variant="ghost" onClick={onDelete}>
          <Trash2 size={16} />
        </Button>
      </div>
    </Card>
  )
}
