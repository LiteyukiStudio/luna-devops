import type { CredentialWithRegistry } from './registry-form-model'
import type { ArtifactRegistry, ContainerImage, PaginatedResponse } from '@/api'
import { CheckCircle2, RefreshCw, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { CopyableHoverText } from '@/components/common/copyable-hover-text'
import { DataList } from '@/components/common/data-list'
import { EditActionButton } from '@/components/common/edit-action-button'
import { ErrorState } from '@/components/common/error-state'
import { StatusBadge, StatusValueBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'

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
  projectMap,
  pagination,
  testing,
  onSelectCredentials,
  onEdit,
  onTest,
  onDelete,
}: RegistriesPanelProps) {
  const { t } = useTranslation()

  return (
    <>
      {isError && <ErrorState title={t('registriesPage.loadFailedTitle')} description={t('registriesPage.loadFailedDescription')} />}
      <DataList
        columns={[
          {
            key: 'name',
            header: t('registriesPage.name'),
            render: registry => (
              <button
                className="grid min-w-0 text-left"
                type="button"
                onClick={() => onSelectCredentials(registry.id)}
              >
                <span className="truncate font-medium">{registry.name}</span>
                <span className="truncate text-sm text-muted-foreground">{registry.endpoint}</span>
              </button>
            ),
          },
          { key: 'provider', header: t('registriesPage.provider'), render: registry => <StatusBadge>{registryProviderLabel(registry.provider, t)}</StatusBadge> },
          {
            key: 'scope',
            header: t('common.scope'),
            render: registry => (
              <div className="flex flex-wrap gap-2">
                <StatusBadge>{registry.scope}</StatusBadge>
                {projectScopeBadges(registry.projectIds, projectMap)}
                {registry.isDefault && <StatusBadge>{t('common.default')}</StatusBadge>}
              </div>
            ),
          },
          { key: 'capabilities', header: t('registriesPage.capabilities'), render: registry => <span className="text-sm text-muted-foreground">{registry.capabilities.join(', ') || t('registriesPage.noCapabilities')}</span> },
          {
            key: 'actions',
            header: t('common.actions'),
            className: 'text-right whitespace-nowrap',
            render: registry => (
              <div className="flex justify-end gap-2">
                <EditActionButton type="button" label={t('edit')} onClick={() => onEdit(registry)} />
                <Button disabled={testing} type="button" variant="ghost" onClick={() => onTest(registry.id)}>
                  <RefreshCw size={16} />
                  {t('registriesPage.test')}
                </Button>
                <Button aria-label={t('registriesPage.deleteRegistryAria')} type="button" variant="ghost" onClick={() => onDelete(registry)}>
                  <Trash2 size={16} />
                </Button>
              </div>
            ),
          },
        ]}
        emptyTitle={t('registriesPage.emptyTitle')}
        emptyDescription={t('registriesPage.emptyDescription')}
        items={items}
        pagination={dataListPagination(t, pagination)}
        rowKey={registry => registry.id}
      />
    </>
  )
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
  registryFilterId: string
  pagination: PageState<CredentialWithRegistry>
  projectMap: Record<string, { name: string }>
  onEdit: (credential: CredentialWithRegistry) => void
  onDelete: (credential: CredentialWithRegistry) => void
}

export function CredentialsPanel({ items, registryFilterId, pagination, projectMap, onDelete, onEdit }: CredentialsPanelProps) {
  const { t } = useTranslation()

  return (
    <DataList
      columns={[
        {
          key: 'name',
          header: t('registriesPage.name'),
          render: credential => (
            <div className="min-w-0">
              <div className="truncate font-medium">{credential.name}</div>
              <p className="truncate text-sm text-muted-foreground">{credential.username || t('registriesPage.tokenOnly')}</p>
            </div>
          ),
        },
        { key: 'registry', header: t('registries'), render: credential => credential.registryName },
        {
          key: 'template',
          header: t('registriesPage.imageTemplate'),
          className: 'min-w-56',
          render: credential => (
            <CopyableHoverText
              className="max-w-72 rounded bg-background px-2 py-1 font-mono text-xs"
              value={`${credential.repositoryTemplate}:${credential.tagTemplate}`}
            />
          ),
        },
        { key: 'usage', header: t('registriesPage.usage'), render: credential => <StatusBadge>{credential.usage}</StatusBadge> },
        {
          key: 'access',
          header: t('registriesPage.credentialAccessScope'),
          render: credential => (
            <div className="flex flex-wrap gap-2">
              <StatusBadge>{t(`registriesPage.scope${credential.scope.charAt(0).toUpperCase()}${credential.scope.slice(1)}`)}</StatusBadge>
              {projectScopeBadges(credential.projectIds, projectMap)}
            </div>
          ),
        },
        {
          key: 'secret',
          header: t('registriesPage.credential'),
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
          className: 'text-right whitespace-nowrap',
          render: credential => (
            <div className="flex justify-end gap-2">
              <EditActionButton type="button" label={t('edit')} onClick={() => onEdit(credential)} />
              <Button aria-label={t('registriesPage.deleteCredentialAria')} variant="ghost" onClick={() => onDelete(credential)}>
                <Trash2 size={16} />
              </Button>
            </div>
          ),
        },
      ]}
      emptyTitle={t('registriesPage.noCredentialsTitle')}
      emptyDescription={t('registriesPage.noCredentialsDescription')}
      items={items}
      pagination={registryFilterId ? dataListPagination(t, pagination) : undefined}
      rowKey={credential => credential.id}
    />
  )
}

interface ImagesPanelProps {
  images: ContainerImage[]
  registries: ArtifactRegistry[]
  pagination: PageState<ContainerImage>
}

export function ImagesPanel({ images, registries, pagination }: ImagesPanelProps) {
  const { t } = useTranslation()

  return (
    <DataList
      columns={[
        {
          key: 'image',
          header: t('registriesPage.image'),
          render: image => (
            <CopyableHoverText
              className="max-w-xl rounded bg-background px-2 py-1 font-mono text-xs"
              value={image.imageRef}
            />
          ),
        },
        { key: 'registry', header: t('registries'), render: image => registries.find(registry => registry.id === image.registryId)?.name ?? image.registryId },
        { key: 'source', header: t('common.type'), render: image => <StatusBadge>{image.sourceType}</StatusBadge> },
        { key: 'scan', header: t('common.status'), render: image => <StatusValueBadge value={image.scanStatus} /> },
        { key: 'digest', header: t('registriesPage.digest'), render: image => image.digest ? <CheckCircle2 className="text-primary" size={16} /> : '-' },
      ]}
      emptyTitle={t('registriesPage.noImagesTitle')}
      emptyDescription={t('registriesPage.noImagesDescription')}
      items={images}
      pagination={dataListPagination(t, pagination, IMAGE_PAGE_SIZE_OPTIONS)}
      rowKey={image => image.id}
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
