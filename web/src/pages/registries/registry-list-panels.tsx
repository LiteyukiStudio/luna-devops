import type { ReactNode } from 'react'
import type { CredentialWithRegistry } from './registry-form-model'
import type { ArtifactRegistry, ContainerImage, PaginatedResponse } from '@/api'
import { CheckCircle2, Container, FlaskConical, KeyRound, MoreHorizontal, Package, Pencil, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { CopyableHoverText } from '@/components/common/copyable-hover-text'
import { DataList } from '@/components/common/data-list'
import { ErrorState } from '@/components/common/error-state'
import { StatusBadge, StatusValueBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'

const PAGE_SIZE_OPTIONS = [10, 20, 50, 100]
const IMAGE_PAGE_SIZE_OPTIONS = [10, 20, 50, 100]

interface PageState<T> {
  data?: PaginatedResponse<T>
  page: number
  pageSize: number
  onPageChange: (page: number) => void
  onPageSizeChange: (pageSize: number) => void
}

interface RegistriesPanelProps {
  items: ArtifactRegistry[]
  isError: boolean
  loading: boolean
  toolbar?: ReactNode
  projectMap: Record<string, { name: string }>
  pagination: PageState<ArtifactRegistry>
  testing: boolean
  onSelectCredentials: (registryId: string) => void
  onEdit: (registry: ArtifactRegistry) => void
  onTest: (registryId: string) => void
  onDelete: (registry: ArtifactRegistry) => void
}

export function RegistriesPanel({
  items,
  isError,
  loading,
  projectMap,
  pagination,
  testing,
  toolbar,
  onSelectCredentials,
  onEdit,
  onTest,
  onDelete,
}: RegistriesPanelProps) {
  const { t } = useTranslation()

  if (isError)
    return <ErrorState title={t('registriesPage.loadFailedTitle')} description={t('registriesPage.loadFailedDescription')} />

  return (
    <DataList
      columns={[
        {
          key: 'name',
          header: t('registriesPage.name'),
          width: 'primary',
          render: registry => (
            <div className="grid min-w-0">
              <span className="truncate font-medium">{registry.name}</span>
              <span className="truncate text-xs text-muted-foreground">{registry.endpoint}</span>
            </div>
          ),
        },
        { key: 'provider', header: t('registriesPage.provider'), width: 'secondary', render: registry => <StatusBadge>{registryProviderLabel(registry.provider, t)}</StatusBadge> },
        {
          key: 'scope',
          header: t('common.scope'),
          width: 'normal',
          render: registry => (
            <div className="flex flex-wrap gap-2">
              <StatusBadge>{scopeLabel(registry.scope, t)}</StatusBadge>
              {projectScopeBadges(registry.projectIds, projectMap)}
              {registry.isDefault && <StatusBadge>{t('common.default')}</StatusBadge>}
            </div>
          ),
        },
        { key: 'capabilities', header: t('registriesPage.capabilities'), mobile: 'hidden', width: 'normal', render: registry => <span className="text-sm text-muted-foreground">{registry.capabilities.join(', ') || t('registriesPage.noCapabilities')}</span> },
        {
          key: 'actions',
          header: t('common.actions'),
          sticky: 'right',
          width: 'actions',
          render: registry => (
            <div className="flex items-center justify-end gap-1">
              <Button disabled={testing} size="sm" type="button" variant="ghost" onClick={() => onTest(registry.id)}>
                <FlaskConical />
                {t('registriesPage.test')}
              </Button>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button aria-label={t('common.actions')} className="size-8" size="icon" type="button" variant="ghost">
                    <MoreHorizontal />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem onSelect={() => onSelectCredentials(registry.id)}>
                    <KeyRound />
                    {t('registriesPage.credentialsTab')}
                  </DropdownMenuItem>
                  <DropdownMenuItem onSelect={() => onEdit(registry)}>
                    <Pencil />
                    {t('edit')}
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem variant="destructive" onSelect={() => onDelete(registry)}>
                    <Trash2 />
                    {t('common.delete')}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          ),
        },
      ]}
      emptyTitle={t('registriesPage.emptyTitle')}
      emptyDescription={t('registriesPage.emptyDescription')}
      emptyIcon={<Container className="size-5" />}
      items={items}
      loading={loading}
      pagination={dataListPagination(t, pagination)}
      rowKey={registry => registry.id}
      toolbar={toolbar}
    />
  )
}

function scopeLabel(scope: string, t: ReturnType<typeof useTranslation>['t']) {
  return t(`registriesPage.scope${scope.charAt(0).toUpperCase()}${scope.slice(1)}`, { defaultValue: scope })
}

function registryProviderLabel(provider: ArtifactRegistry['provider'], t: ReturnType<typeof useTranslation>['t']) {
  switch (provider) {
    case 'dockerhub':
      return t('registriesPage.providerDockerHub')
    case 'gitea-registry':
      return t('registriesPage.providerGiteaRegistry')
    case 'generic-oci':
      return t('registriesPage.providerGenericOCI')
    case 'harbor':
    default:
      return t('registriesPage.providerHarbor')
  }
}

interface CredentialsPanelProps {
  items: CredentialWithRegistry[]
  loading: boolean
  toolbar?: ReactNode
  pagination: PageState<CredentialWithRegistry>
  projectMap: Record<string, { name: string }>
  onEdit: (credential: CredentialWithRegistry) => void
  onDelete: (credential: CredentialWithRegistry) => void
}

export function CredentialsPanel({ items, loading, pagination, projectMap, toolbar, onDelete, onEdit }: CredentialsPanelProps) {
  const { t } = useTranslation()

  return (
    <DataList
      columns={[
        {
          key: 'name',
          header: t('registriesPage.name'),
          width: 'primary',
          render: credential => (
            <div className="min-w-0">
              <div className="truncate font-medium">{credential.name}</div>
              <p className="truncate text-xs text-muted-foreground">{credential.username || t('registriesPage.tokenOnly')}</p>
            </div>
          ),
        },
        { key: 'registry', header: t('registries'), width: 'secondary', render: credential => credential.registryName },
        {
          key: 'template',
          header: t('registriesPage.imageTemplate'),
          mobile: 'hidden',
          width: 'normal',
          render: credential => (
            <CopyableHoverText
              className="max-w-72 font-mono text-xs"
              value={`${credential.repositoryTemplate}:${credential.tagTemplate}`}
            />
          ),
        },
        { key: 'usage', header: t('registriesPage.usage'), width: 'status', render: credential => <StatusBadge>{credential.usage}</StatusBadge> },
        {
          key: 'access',
          header: t('registriesPage.credentialAccessScope'),
          width: 'normal',
          render: credential => (
            <div className="flex flex-wrap gap-2">
              <StatusBadge>{scopeLabel(credential.scope, t)}</StatusBadge>
              {projectScopeBadges(credential.projectIds, projectMap)}
            </div>
          ),
        },
        {
          key: 'secret',
          header: t('registriesPage.credential'),
          mobile: 'hidden',
          width: 'secondary',
          render: credential => (
            <div className="flex flex-wrap gap-2">
              {credential.passwordSet && <StatusBadge>{t('registriesPage.passwordSet')}</StatusBadge>}
              {credential.tokenSet && <StatusBadge>{t('registriesPage.tokenSet')}</StatusBadge>}
            </div>
          ),
        },
        {
          key: 'actions',
          header: t('common.actions'),
          sticky: 'right',
          width: 'actions',
          render: credential => (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button aria-label={t('common.actions')} className="size-8" size="icon" type="button" variant="ghost">
                  <MoreHorizontal />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem onSelect={() => onEdit(credential)}>
                  <Pencil />
                  {t('edit')}
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem variant="destructive" onSelect={() => onDelete(credential)}>
                  <Trash2 />
                  {t('common.delete')}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          ),
        },
      ]}
      emptyTitle={t('registriesPage.noCredentialsTitle')}
      emptyDescription={t('registriesPage.noCredentialsDescription')}
      emptyIcon={<KeyRound className="size-5" />}
      items={items}
      loading={loading}
      pagination={dataListPagination(t, pagination)}
      rowKey={credential => credential.id}
      toolbar={toolbar}
    />
  )
}

interface ImagesPanelProps {
  images: ContainerImage[]
  loading: boolean
  toolbar?: ReactNode
  registries: ArtifactRegistry[]
  pagination: PageState<ContainerImage>
}

export function ImagesPanel({ images, loading, registries, pagination, toolbar }: ImagesPanelProps) {
  const { t } = useTranslation()

  return (
    <DataList
      columns={[
        {
          key: 'image',
          header: t('registriesPage.image'),
          width: 'primary',
          render: image => (
            <CopyableHoverText
              className="max-w-xl font-mono text-xs"
              value={image.imageRef}
            />
          ),
        },
        { key: 'registry', header: t('registries'), width: 'secondary', render: image => registries.find(registry => registry.id === image.registryId)?.name ?? image.registryId },
        { key: 'source', header: t('common.type'), mobile: 'hidden', width: 'status', render: image => <StatusBadge>{image.sourceType}</StatusBadge> },
        { key: 'scan', header: t('common.status'), width: 'status', render: image => <StatusValueBadge value={image.scanStatus} /> },
        { key: 'digest', header: t('registriesPage.digest'), mobile: 'hidden', width: 'compact', render: image => image.digest ? <CheckCircle2 aria-label={image.digest} className="size-4 text-success" /> : '-' },
      ]}
      emptyTitle={t('registriesPage.noImagesTitle')}
      emptyDescription={t('registriesPage.noImagesDescription')}
      emptyIcon={<Package className="size-5" />}
      items={images}
      loading={loading}
      constrainedHeight
      pagination={dataListPagination(t, pagination, IMAGE_PAGE_SIZE_OPTIONS)}
      rowKey={image => image.id}
      toolbar={toolbar}
    />
  )
}

function dataListPagination<T>(
  t: (key: string, values?: Record<string, number>) => string,
  pagination: PageState<T>,
  pageSizeOptions = PAGE_SIZE_OPTIONS,
) {
  return {
    page: pagination.data?.page ?? pagination.page,
    pageSize: pagination.data?.pageSize ?? pagination.pageSize,
    pageSizeOptions,
    total: pagination.data?.total ?? 0,
    totalPages: pagination.data?.totalPages ?? 0,
    pageInfoLabel: t('pagination.pageInfo', {
      page: pagination.data?.page ?? pagination.page,
      totalPages: pagination.data?.totalPages ?? 0,
      total: pagination.data?.total ?? 0,
    }),
    onPageChange: pagination.onPageChange,
    onPageSizeChange: pagination.onPageSizeChange,
  }
}

function projectScopeBadges(projectIds: string[] | undefined, projectMap: Record<string, { name: string }>) {
  return (projectIds ?? []).map(projectId => (
    <StatusBadge key={projectId}>{projectMap[projectId]?.name ?? projectId}</StatusBadge>
  ))
}
