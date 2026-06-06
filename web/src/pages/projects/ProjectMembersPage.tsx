import type { ProjectMember } from '../../api/client'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Trash2, UserPlus } from 'lucide-react'
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
import { Field, Input, Select } from '../../components/ui/input'

const schema = z.object({
  email: z.string().email('请输入有效邮箱'),
  role: z.enum(['owner', 'admin', 'developer', 'viewer']),
})

type MemberForm = z.infer<typeof schema>

const roleLabels: Record<ProjectMember['role'], string> = {
  owner: 'Owner',
  admin: 'Admin',
  developer: 'Developer',
  viewer: 'Viewer',
}

export function ProjectMembersPage() {
  const { projectId = '' } = useParams()
  const queryClient = useQueryClient()
  const [memberToDelete, setMemberToDelete] = useState<ProjectMember | null>(null)
  const members = useQuery({
    queryKey: ['project-members', projectId],
    queryFn: () => api.listProjectMembers(projectId),
    enabled: Boolean(projectId),
  })
  const form = useForm<MemberForm>({
    resolver: zodResolver(schema),
    mode: 'onChange',
    defaultValues: { email: '', role: 'viewer' },
  })

  const createMember = useMutation({
    mutationFn: (values: MemberForm) => api.createProjectMember(projectId, values),
    onSuccess: () => {
      toast.success('成员已添加')
      form.reset({ email: '', role: 'viewer' })
      queryClient.invalidateQueries({ queryKey: ['project-members', projectId] })
    },
    onError: error => toast.error(error.message),
  })

  const updateMember = useMutation({
    mutationFn: ({ memberId, role }: { memberId: string, role: ProjectMember['role'] }) =>
      api.updateProjectMember(projectId, memberId, { role }),
    onSuccess: () => {
      toast.success('成员角色已更新')
      queryClient.invalidateQueries({ queryKey: ['project-members', projectId] })
    },
    onError: error => toast.error(error.message),
  })

  const deleteMember = useMutation({
    mutationFn: (memberId: string) => api.deleteProjectMember(projectId, memberId),
    onSuccess: () => {
      toast.success('成员已移除')
      setMemberToDelete(null)
      queryClient.invalidateQueries({ queryKey: ['project-members', projectId] })
    },
    onError: error => toast.error(error.message),
  })

  return (
    <div className="grid gap-6">
      <PageHeader
        actions={<Link className="text-sm text-primary hover:underline" to="/projects">返回项目</Link>}
        description="Owner/Admin 可以维护成员，Developer 可管理应用，Viewer 只读。"
        title="项目成员"
      />

      <div className="grid gap-4 lg:grid-cols-[360px_1fr]">
        <Card>
          <form className="grid gap-3" onSubmit={form.handleSubmit(values => createMember.mutate(values))}>
            <h2 className="text-base font-semibold">添加成员</h2>
            <Field error={form.formState.errors.email?.message} label="用户邮箱" required>
              <Input {...form.register('email')} aria-invalid={Boolean(form.formState.errors.email)} placeholder="user@example.com" />
            </Field>
            <Field error={form.formState.errors.role?.message} label="项目角色" required>
              <Select {...form.register('role')} aria-invalid={Boolean(form.formState.errors.role)}>
                <option value="viewer">Viewer</option>
                <option value="developer">Developer</option>
                <option value="admin">Admin</option>
                <option value="owner">Owner</option>
              </Select>
            </Field>
            <Button disabled={createMember.isPending || !form.formState.isValid} type="submit">
              <UserPlus size={16} />
              添加成员
            </Button>
          </form>
        </Card>

        <MotionList className="grid gap-3">
          {members.isError && <ErrorState title="成员加载失败" description="请确认项目存在，并且你有项目访问权限。" />}
          {members.data?.length === 0 && <EmptyState title="还没有成员" description="至少需要保留一个 Owner。" />}
          {(members.data ?? []).map(member => (
            <MotionItem key={member.id}>
              <Card className="flex items-center justify-between gap-4">
                <div className="min-w-0">
                  <div className="flex items-center gap-2">
                    <p className="truncate font-medium">{member.name}</p>
                    <StatusBadge>{roleLabels[member.role]}</StatusBadge>
                  </div>
                  <p className="truncate text-sm text-muted-foreground">{member.email}</p>
                </div>
                <div className="flex shrink-0 items-center gap-2">
                  <Select
                    className="w-32"
                    value={member.role}
                    onChange={event => updateMember.mutate({ memberId: member.id, role: event.target.value as ProjectMember['role'] })}
                  >
                    <option value="viewer">Viewer</option>
                    <option value="developer">Developer</option>
                    <option value="admin">Admin</option>
                    <option value="owner">Owner</option>
                  </Select>
                  <Button aria-label="移除成员" variant="ghost" onClick={() => setMemberToDelete(member)}>
                    <Trash2 size={16} />
                  </Button>
                </div>
              </Card>
            </MotionItem>
          ))}
        </MotionList>
      </div>

      <ConfirmDialog
        confirmText="移除成员"
        description={`成员 ${memberToDelete?.email ?? ''} 将失去该项目访问权限。`}
        open={Boolean(memberToDelete)}
        pending={deleteMember.isPending}
        title="移除项目成员"
        onConfirm={() => memberToDelete && deleteMember.mutate(memberToDelete.id)}
        onOpenChange={open => !open && setMemberToDelete(null)}
      />
    </div>
  )
}
