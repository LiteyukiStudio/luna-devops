import type { Ref } from 'react'
import type { ProjectMember } from '@/api/client'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import i18next from 'i18next'
import { Trash2, UserPlus } from 'lucide-react'
import { useImperativeHandle, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { Link, useParams } from 'react-router-dom'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api/client'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { EmptyState } from '@/components/common/empty-state'
import { ErrorState } from '@/components/common/error-state'
import { FormField as Field } from '@/components/common/form-field'
import { MotionItem, MotionList } from '@/components/common/motion'
import { PageHeader } from '@/components/common/page-header'
import { StatusBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'

const schema = z.object({
  email: z.string().email(i18next.t('common.validEmailRequired')),
  role: z.enum(['owner', 'admin', 'developer', 'viewer']),
})

type MemberForm = z.infer<typeof schema>

const roleLabels: Record<ProjectMember['role'], string> = {
  owner: 'Owner',
  admin: 'Admin',
  developer: 'Developer',
  viewer: 'Viewer',
}

export interface ProjectMembersPageHandle {
  openAddMemberDialog: () => void
}

interface ProjectMembersPageProps {
  embedded?: boolean
  projectId?: string
  ref?: Ref<ProjectMembersPageHandle>
}

export function ProjectMembersPage({ embedded = false, projectId: projectIdProp, ref }: ProjectMembersPageProps = {}) {
  const { t } = useTranslation()
  const { projectId: routeProjectId = '' } = useParams()
  const projectId = projectIdProp ?? routeProjectId
  const queryClient = useQueryClient()
  const [dialogOpen, setDialogOpen] = useState(false)
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
      toast.success(t('projectMembers.added'))
      form.reset({ email: '', role: 'viewer' })
      setDialogOpen(false)
      queryClient.invalidateQueries({ queryKey: ['project-members', projectId] })
    },
    onError: error => toast.error(error.message),
  })

  const updateMember = useMutation({
    mutationFn: ({ memberId, role }: { memberId: string, role: ProjectMember['role'] }) =>
      api.updateProjectMember(projectId, memberId, { role }),
    onSuccess: () => {
      toast.success(t('projectMembers.updated'))
      queryClient.invalidateQueries({ queryKey: ['project-members', projectId] })
    },
    onError: error => toast.error(error.message),
  })

  const deleteMember = useMutation({
    mutationFn: (memberId: string) => api.deleteProjectMember(projectId, memberId),
    onSuccess: () => {
      toast.success(t('projectMembers.removed'))
      queryClient.invalidateQueries({ queryKey: ['project-members', projectId] })
    },
    onError: error => toast.error(error.message),
  })

  const openAddMemberDialog = () => {
    form.reset({ email: '', role: 'viewer' })
    setDialogOpen(true)
  }

  useImperativeHandle(ref, () => ({ openAddMemberDialog }))

  return (
    <div className="grid gap-6">
      {!embedded && (
        <PageHeader
          actions={(
            <div className="flex items-center gap-3">
              <Button onClick={openAddMemberDialog}>
                <UserPlus size={16} />
                {t('projectMembers.addTitle')}
              </Button>
              <Link className="text-sm text-primary hover:underline" to="/projects">{t('backToProjectSpaces')}</Link>
            </div>
          )}
          description={t('projectMembers.description')}
          title={t('projectMembers.title')}
        />
      )}

      <div className="grid gap-4">
        <MotionList className="grid gap-3">
          {members.isError && <ErrorState title={t('projectMembers.loadFailedTitle')} description={t('projectMembers.loadFailedDescription')} />}
          {members.data?.length === 0 && <EmptyState title={t('projectMembers.emptyTitle')} description={t('projectMembers.emptyDescription')} />}
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
                    <option value="viewer">{t('projectMembers.roleViewer')}</option>
                    <option value="developer">{t('projectMembers.roleDeveloper')}</option>
                    <option value="admin">{t('projectMembers.roleAdmin')}</option>
                    <option value="owner">{t('projectMembers.roleOwner')}</option>
                  </Select>
                  <ConfirmDialog
                    confirmText={t('projectMembers.removeConfirm')}
                    description={t('projectMembers.removeDescription', { email: member.email })}
                    pending={deleteMember.isPending}
                    title={t('projectMembers.removeTitle')}
                    onConfirm={() => deleteMember.mutate(member.id)}
                  >
                    <Button aria-label={t('projectMembers.removeAria')} variant="ghost">
                      <Trash2 size={16} />
                    </Button>
                  </ConfirmDialog>
                </div>
              </Card>
            </MotionItem>
          ))}
        </MotionList>
      </div>

      <Dialog
        open={dialogOpen}
        onOpenChange={(open) => {
          setDialogOpen(open)
          if (!open)
            form.reset({ email: '', role: 'viewer' })
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('projectMembers.addTitle')}</DialogTitle>
            <DialogDescription>{t('projectMembers.description')}</DialogDescription>
          </DialogHeader>
          <form className="grid gap-3" onSubmit={form.handleSubmit(values => createMember.mutate(values))}>
            <Field error={form.formState.errors.email?.message} hint={t('projectMembers.emailHint')} label={t('projectMembers.email')} required>
              <Input {...form.register('email')} aria-invalid={Boolean(form.formState.errors.email)} placeholder={t('projectMembers.emailPlaceholder')} />
            </Field>
            <Field error={form.formState.errors.role?.message} hint={t('projectMembers.roleHint')} label={t('projectMembers.role')} required>
              <Select {...form.register('role')} aria-invalid={Boolean(form.formState.errors.role)}>
                <option value="viewer">{t('projectMembers.roleViewer')}</option>
                <option value="developer">{t('projectMembers.roleDeveloper')}</option>
                <option value="admin">{t('projectMembers.roleAdmin')}</option>
                <option value="owner">{t('projectMembers.roleOwner')}</option>
              </Select>
            </Field>
            <DialogFooter>
              <Button disabled={createMember.isPending || !form.formState.isValid} type="submit">
                <UserPlus size={16} />
                {t('projectMembers.add')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}
