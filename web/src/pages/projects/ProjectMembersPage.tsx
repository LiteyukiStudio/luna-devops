import type { Ref } from 'react'
import type { ProjectMember, ProjectMemberCandidate } from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Trash2, UserPlus } from 'lucide-react'
import { useImperativeHandle, useMemo, useState } from 'react'
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
import { SearchMultiSelect } from '@/components/common/search-select'
import { StatusBadge } from '@/components/common/status-badge'
import { UserAvatar } from '@/components/common/user-avatar'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
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
  const candidateUsers = useMemo(() => {
    const candidates = [...selectedUsers, ...(memberCandidates.data ?? [])]
    return [...new Map(candidates.map(candidate => [candidate.id, candidate])).values()]
  }, [memberCandidates.data, selectedUsers])
  const candidateOptions = useMemo(() => candidateUsers.map(candidate => ({
    description: candidate.email,
    icon: <UserAvatar className="size-7" user={candidate} />,
    keywords: candidate.email,
    label: candidate.name || candidate.email,
    value: candidate.id,
  })), [candidateUsers])

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
              <SearchMultiSelect
                className="h-11 rounded-2xl"
                emptyLabel={t('projectMembers.noUserResults')}
                filterLocally={false}
                limited={(memberCandidates.data?.length ?? 0) >= 20}
                loading={memberCandidates.isFetching}
                options={candidateOptions}
                placeholder={t('projectMembers.userSearchPlaceholder')}
                search={memberSearch}
                searchPlaceholder={t('projectMembers.userSearchPlaceholder')}
                selectedLabel={options => options.map(option => option.label).join(', ')}
                value={selectedUsers.map(user => user.id)}
                onSearchChange={setMemberSearch}
                onValueChange={(userIds) => {
                  const usersById = new Map(candidateUsers.map(user => [user.id, user]))
                  setSelectedUsers(userIds.flatMap(userId => usersById.get(userId) ?? []))
                }}
              />
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
