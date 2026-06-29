import type { Ref } from 'react'
import type { ProjectMember, ProjectMemberCandidate } from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Trash2, UserPlus, X } from 'lucide-react'
import { useImperativeHandle, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { Link, useParams } from 'react-router-dom'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { DataList } from '@/components/common/data-list'
import { ErrorState } from '@/components/common/error-state'
import { FormField as Field } from '@/components/common/form-field'
import { PageHeader } from '@/components/common/page-header'
import { StatusBadge } from '@/components/common/status-badge'
import { UserAvatar } from '@/components/common/user-avatar'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'

const schema = z.object({
  role: z.enum(['owner', 'admin', 'developer', 'viewer']),
})

type MemberForm = z.infer<typeof schema>

const roleLabels: Record<ProjectMember['role'], string> = {
  owner: 'Owner',
  admin: 'Admin',
  developer: 'Developer',
  viewer: 'Viewer',
}

const PAGE_SIZE_OPTIONS = [10, 20, 50, 100]

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
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [memberSearch, setMemberSearch] = useState('')
  const [selectedUsers, setSelectedUsers] = useState<ProjectMemberCandidate[]>([])
  const members = useQuery({
    queryKey: ['project-members', projectId, page, pageSize],
    queryFn: () => api.listProjectMembersPage(projectId, { page, pageSize, sortBy: 'createdAt', sortOrder: 'asc' }),
    enabled: Boolean(projectId),
  })
  const memberCandidates = useQuery({
    queryKey: ['project-member-candidates', projectId, memberSearch],
    queryFn: () => api.searchProjectMemberCandidates(projectId, { search: memberSearch, limit: 20 }),
    enabled: dialogOpen && Boolean(projectId) && memberSearch.trim().length > 0,
  })
  const form = useForm<MemberForm>({
    resolver: zodResolver(schema),
    mode: 'onChange',
    defaultValues: { role: 'viewer' },
  })
  const candidateItems = (memberCandidates.data ?? []).filter(candidate => !selectedUsers.some(user => user.id === candidate.id))

  const createMember = useMutation({
    mutationFn: (values: MemberForm) =>
      Promise.all(selectedUsers.map(user => api.createProjectMember(projectId, { userId: user.id, role: values.role }))),
    onSuccess: (members) => {
      toast.success(t('projectMembers.addedCount', { count: members.length }))
      form.reset({ role: 'viewer' })
      setMemberSearch('')
      setSelectedUsers([])
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
    form.reset({ role: 'viewer' })
    setMemberSearch('')
    setSelectedUsers([])
    setDialogOpen(true)
  }

  function selectUser(user: ProjectMemberCandidate) {
    setSelectedUsers(current => current.some(item => item.id === user.id) ? current : [...current, user])
    setMemberSearch('')
  }

  function removeSelectedUser(userId: string) {
    setSelectedUsers(current => current.filter(user => user.id !== userId))
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

      {members.isError && <ErrorState title={t('projectMembers.loadFailedTitle')} description={t('projectMembers.loadFailedDescription')} />}
      <DataList
        columns={[
          {
            key: 'member',
            header: t('projectMembers.title'),
            className: 'min-w-64 px-4 py-3 align-middle',
            render: member => (
              <div className="min-w-0">
                <p className="truncate font-medium">{member.name}</p>
                <p className="truncate text-sm text-muted-foreground">{member.email}</p>
              </div>
            ),
          },
          {
            key: 'role',
            header: t('projectMembers.role'),
            className: 'w-[18%] px-4 py-3 align-middle',
            render: member => <StatusBadge>{roleLabels[member.role]}</StatusBadge>,
          },
          {
            key: 'actions',
            header: t('common.actions'),
            className: 'w-[1%] whitespace-nowrap px-4 py-3 align-middle text-right',
            render: member => (
              <div className="flex justify-end gap-2">
                <Select
                  aria-label={t('projectMembers.role')}
                  className="h-9"
                  containerClassName="w-36"
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
            ),
          },
        ]}
        emptyDescription={t('projectMembers.emptyDescription')}
        emptyTitle={t('projectMembers.emptyTitle')}
        items={members.data?.items ?? []}
        pagination={{
          page: members.data?.page ?? page,
          pageSize: members.data?.pageSize ?? pageSize,
          pageSizeOptions: PAGE_SIZE_OPTIONS,
          total: members.data?.total ?? 0,
          totalPages: members.data?.totalPages ?? 0,
          pageInfoLabel: t('pagination.pageInfo', {
            page: members.data?.page ?? page,
            totalPages: members.data?.totalPages ?? 0,
            total: members.data?.total ?? 0,
          }),
          onPageChange: setPage,
          onPageSizeChange: (nextPageSize) => {
            setPageSize(nextPageSize)
            setPage(1)
          },
        }}
        rowKey={member => member.id}
      />

      <Dialog
        open={dialogOpen}
        onOpenChange={(open) => {
          setDialogOpen(open)
          if (!open) {
            form.reset({ role: 'viewer' })
            setMemberSearch('')
            setSelectedUsers([])
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('projectMembers.addTitle')}</DialogTitle>
            <DialogDescription>{t('projectMembers.description')}</DialogDescription>
          </DialogHeader>
          <form className="grid gap-3" onSubmit={form.handleSubmit(values => createMember.mutate(values))}>
            <Field hint={t('projectMembers.userSearchHint')} label={t('projectMembers.userSearch')} required>
              <div className="grid gap-2">
                {selectedUsers.length > 0 && (
                  <div className="flex flex-wrap gap-2 rounded-md border border-border bg-muted/30 p-2">
                    {selectedUsers.map(user => (
                      <span key={user.id} className="inline-flex max-w-full items-center gap-2 rounded-full border border-border bg-background px-2 py-1 text-sm">
                        <UserAvatar className="size-5" user={user} />
                        <span className="min-w-0 truncate">{user.name || user.email}</span>
                        <button
                          aria-label={t('projectMembers.removeSelectedUser', { name: user.name || user.email })}
                          className="text-muted-foreground hover:text-foreground"
                          type="button"
                          onClick={() => removeSelectedUser(user.id)}
                        >
                          <X size={14} />
                        </button>
                      </span>
                    ))}
                  </div>
                )}
                <Input
                  placeholder={t('projectMembers.userSearchPlaceholder')}
                  value={memberSearch}
                  onChange={event => setMemberSearch(event.target.value)}
                />
                {memberSearch.trim().length > 0 && (
                  <div className="grid max-h-56 gap-1 overflow-y-auto rounded-md border border-border bg-background p-2 shadow-sm">
                    {memberCandidates.isFetching && (
                      <p className="px-3 py-2 text-sm text-muted-foreground">{t('projectMembers.searchingUsers')}</p>
                    )}
                    {!memberCandidates.isFetching && candidateItems.map(user => (
                      <button
                        key={user.id}
                        className="flex min-w-0 items-center gap-3 rounded-md px-3 py-2 text-left hover:bg-muted"
                        type="button"
                        onClick={() => selectUser(user)}
                      >
                        <UserAvatar className="size-8" user={user} />
                        <span className="min-w-0">
                          <span className="block truncate text-sm font-medium">{user.name || user.email}</span>
                          <span className="block truncate text-xs text-muted-foreground">{user.email}</span>
                        </span>
                      </button>
                    ))}
                    {!memberCandidates.isFetching && memberCandidates.isSuccess && candidateItems.length === 0 && (
                      <p className="px-3 py-2 text-sm text-muted-foreground">{t('projectMembers.noUserResults')}</p>
                    )}
                  </div>
                )}
                {selectedUsers.length > 0 && (
                  <p className="text-xs text-muted-foreground">{t('projectMembers.selectedUsers', { count: selectedUsers.length })}</p>
                )}
              </div>
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
              <Button disabled={createMember.isPending || !form.formState.isValid || selectedUsers.length === 0} type="submit">
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
