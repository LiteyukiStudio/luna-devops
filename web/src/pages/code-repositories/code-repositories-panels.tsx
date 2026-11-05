import type { ReactNode } from 'react'
import type { GitProviderGuide } from './code-repositories-utils'
import type { GitAccount, GitProvider } from '@/api'
import { ChevronDown, Copy, ExternalLink, GitBranch, Info, KeyRound, LinkIcon, MoreHorizontal, Pencil, RefreshCw, Trash2 } from 'lucide-react'
import { Fragment, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { gitOAuthStartUrl } from '@/api'
import { DataList } from '@/components/common/data-list'
import { StatusBadge, StatusValueBadge } from '@/components/common/status-badge'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { normalizeGitBaseUrl } from './code-repositories-utils'

const PAGE_SIZE_OPTIONS = [10, 20, 50, 100]

export function ProvidersPanel({
  canManage,
  loading,
  page,
  pageSize,
  providers,
  projectMap,
  toolbar,
  total,
  totalPages,
  onDelete,
  onEdit,
  onPageChange,
  onPageSizeChange,
}: {
  canManage: boolean
  loading: boolean
  page: number
  pageSize: number
  providers: GitProvider[]
  projectMap: Record<string, string>
  toolbar?: ReactNode
  total: number
  totalPages: number
  onDelete: (provider: GitProvider) => void
  onEdit: (provider: GitProvider) => void
  onPageChange: (page: number) => void
  onPageSizeChange: (pageSize: number) => void
}) {
  const { t } = useTranslation()
  return (
    <DataList
      columns={[
        {
          key: 'name',
          header: t('common.name'),
          width: 'primary',
          render: provider => (
            <div className="flex min-w-0 items-center gap-3">
              <GitProviderIcon baseUrl={provider.baseUrl} type={provider.type} />
              <div className="min-w-0">
                <div className="truncate font-medium">{provider.name}</div>
                <p className="truncate text-xs text-muted-foreground">{provider.baseUrl}</p>
              </div>
            </div>
          ),
        },
        { key: 'type', header: t('common.type'), width: 'status', render: provider => <StatusBadge>{provider.type}</StatusBadge> },
        { key: 'auth', header: t('codeRepositoriesView.authType'), mobile: 'hidden', width: 'status', render: provider => <StatusBadge>{provider.authType}</StatusBadge> },
        {
          key: 'scope',
          header: t('common.scope'),
          width: 'normal',
          render: provider => (
            <div className="flex flex-wrap gap-2">
              <StatusBadge>{scopeLabel(provider.scope, t)}</StatusBadge>
              {projectScopeBadges(provider.projectIds, projectMap)}
            </div>
          ),
        },
        { key: 'secret', header: t('codeRepositoriesView.clientSecret'), mobile: 'hidden', width: 'secondary', render: provider => provider.clientSecretSet ? t('codeRepositoriesView.secretSet') : t('codeRepositoriesView.secretNotSet') },
        { key: 'status', header: t('common.status'), width: 'status', render: provider => <StatusValueBadge value={provider.enabled ? 'enabled' : 'disabled'} /> },
        {
          key: 'actions',
          header: t('common.actions'),
          sticky: 'right',
          width: 'actions',
          render: provider => canManage
            ? (
                <RowActionsMenu
                  items={[
                    { icon: <Pencil />, label: t('edit'), onSelect: () => onEdit(provider) },
                    { destructive: true, icon: <Trash2 />, label: t('common.delete'), onSelect: () => onDelete(provider) },
                  ]}
                />
              )
            : <span className="text-xs text-muted-foreground">{t('common.viewOnly')}</span>,
        },
      ]}
      emptyTitle={t('codeRepositoriesView.noProvidersTitle')}
      emptyDescription={t('codeRepositoriesView.noProvidersDescription')}
      emptyIcon={<GitBranch className="size-5" />}
      items={providers}
      loading={loading}
      pagination={{
        page,
        pageSize,
        pageSizeOptions: PAGE_SIZE_OPTIONS,
        total,
        totalPages,
        pageInfoLabel: t('pagination.pageInfo', { page, total, totalPages }),
        onPageChange,
        onPageSizeChange,
      }}
      rowKey={provider => provider.id}
      toolbar={toolbar}
    />
  )
}

export function CredentialsPanel({
  credentials,
  loading,
  page,
  pageSize,
  providers,
  projectMap,
  refreshPending,
  toolbar,
  total,
  totalPages,
  onDelete,
  onEdit,
  onPageChange,
  onPageSizeChange,
  onRefresh,
}: {
  credentials: GitAccount[]
  loading: boolean
  page: number
  pageSize: number
  providers: GitProvider[]
  projectMap: Record<string, string>
  refreshPending: boolean
  toolbar?: ReactNode
  total: number
  totalPages: number
  onDelete: (credential: GitAccount) => void
  onEdit: (credential: GitAccount) => void
  onPageChange: (page: number) => void
  onPageSizeChange: (pageSize: number) => void
  onRefresh: (credential: GitAccount) => void
}) {
  const { t } = useTranslation()
  const oauthProviders = providers.filter(provider => isGitOAuthReady(provider))
  const oauthBlockedProviders = providers.filter(provider => provider.enabled && provider.authType === 'oauth' && !isGitOAuthReady(provider))
  return (
    <div className="grid gap-4">
      {oauthBlockedProviders.length > 0 && (
        <Alert>
          <Info />
          <AlertTitle>{t('codeRepositoriesView.oauthUnavailableTitle')}</AlertTitle>
          <AlertDescription>
            {t('codeRepositoriesView.oauthUnavailableDescription', { providers: oauthBlockedProviders.map(provider => provider.name).join(', ') })}
          </AlertDescription>
        </Alert>
      )}
      <DataList
        columns={[
          {
            key: 'name',
            header: t('codeRepositoriesView.username'),
            width: 'primary',
            render: credential => (
              <div className="flex min-w-0 items-center gap-3">
                <span className="flex size-8 shrink-0 items-center justify-center rounded-control bg-surface-subtle text-muted-foreground"><KeyRound className="size-4" /></span>
                <div className="min-w-0">
                  <div className="truncate font-medium">{credential.username}</div>
                  <p className="truncate text-xs text-muted-foreground">{credential.scopes || t('codeRepositoriesView.noScopes')}</p>
                </div>
              </div>
            ),
          },
          { key: 'provider', header: t('codeRepositoriesView.provider'), width: 'secondary', render: credential => <ProviderNameBadge provider={providers.find(provider => provider.id === credential.providerId)} providerId={credential.providerId} /> },
          {
            key: 'scope',
            header: t('common.scope'),
            width: 'normal',
            render: credential => (
              <div className="flex flex-wrap gap-2">
                <StatusBadge>{scopeLabel(credential.scope, t)}</StatusBadge>
                {projectScopeBadges(credential.projectIds, projectMap)}
              </div>
            ),
          },
          {
            key: 'tokens',
            header: t('codeRepositoriesView.accessToken'),
            mobile: 'hidden',
            width: 'normal',
            render: credential => (
              <span className="text-sm text-muted-foreground">
                {credential.accessTokenSet ? t('codeRepositoriesView.accessTokenSet') : t('codeRepositoriesView.accessTokenNotSet')}
                {' · '}
                {credential.refreshTokenSet ? t('codeRepositoriesView.refreshTokenSet') : t('codeRepositoriesView.refreshTokenNotSet')}
              </span>
            ),
          },
          { key: 'status', header: t('common.status'), width: 'status', render: credential => <StatusValueBadge value={credential.status} /> },
          {
            key: 'actions',
            header: t('common.actions'),
            sticky: 'right',
            width: 'actions',
            render: credential => (
              <RowActionsMenu
                items={[
                  { icon: <Pencil />, label: t('edit'), onSelect: () => onEdit(credential) },
                  { disabled: refreshPending || !credential.refreshTokenSet, icon: <RefreshCw />, label: t('codeRepositoriesView.refreshCredential'), onSelect: () => onRefresh(credential) },
                  { destructive: true, icon: <Trash2 />, label: t('common.delete'), onSelect: () => onDelete(credential) },
                ]}
              />
            ),
          },
        ]}
        emptyTitle={t('codeRepositoriesView.noCredentialsTitle')}
        emptyDescription={t('codeRepositoriesView.noCredentialsDescription')}
        emptyIcon={<KeyRound className="size-5" />}
        items={credentials}
        loading={loading}
        pagination={{
          page,
          pageSize,
          pageSizeOptions: PAGE_SIZE_OPTIONS,
          total,
          totalPages,
          pageInfoLabel: t('pagination.pageInfo', { page, total, totalPages }),
          onPageChange,
          onPageSizeChange,
        }}
        rowKey={credential => credential.id}
        toolbar={toolbar}
        toolbarActions={oauthProviders.length > 0 && <OAuthConnectMenu providers={oauthProviders} />}
      />
    </div>
  )
}

function OAuthConnectMenu({ providers }: { providers: GitProvider[] }) {
  const { t } = useTranslation()
  const connect = (provider: GitProvider) => {
    window.location.href = gitOAuthStartUrl(provider.id, '/code-repositories', window.location.origin)
  }
  if (providers.length === 1) {
    const provider = providers[0]
    return (
      <Button size="sm" type="button" variant="outline" onClick={() => connect(provider)}>
        <GitProviderIcon baseUrl={provider.baseUrl} className="size-4 shrink-0" type={provider.type} />
        {t('codeRepositoriesView.oauthConnect', { provider: provider.name })}
      </Button>
    )
  }
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button size="sm" type="button" variant="outline">
          <LinkIcon />
          {t('codeRepositoriesView.oauthConnectMenu')}
          <ChevronDown />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {providers.map(provider => (
          <DropdownMenuItem key={provider.id} onSelect={() => connect(provider)}>
            <GitProviderIcon baseUrl={provider.baseUrl} className="size-4 shrink-0" type={provider.type} />
            {provider.name}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

interface RowAction {
  destructive?: boolean
  disabled?: boolean
  icon: ReactNode
  label: string
  onSelect: () => void
}

function RowActionsMenu({ items }: { items: RowAction[] }) {
  const { t } = useTranslation()
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button aria-label={t('common.actions')} className="size-8" size="icon" type="button" variant="ghost">
          <MoreHorizontal />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {items.map(item => (
          <Fragment key={item.label}>
            {item.destructive && <DropdownMenuSeparator />}
            <DropdownMenuItem disabled={item.disabled} variant={item.destructive ? 'destructive' : 'default'} onSelect={item.onSelect}>
              {item.icon}
              {item.label}
            </DropdownMenuItem>
          </Fragment>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function scopeLabel(scope: string, t: ReturnType<typeof useTranslation>['t']) {
  return t(`codeRepositoriesView.scope${scope.charAt(0).toUpperCase()}${scope.slice(1)}`, { defaultValue: scope })
}

function ProviderNameBadge({ provider, providerId }: { provider?: GitProvider, providerId: string }) {
  return (
    <span className="inline-flex min-w-0 items-center gap-1.5 text-sm">
      <GitProviderIcon baseUrl={provider?.baseUrl} className="size-4 shrink-0" type={provider?.type ?? 'github'} />
      <span className="truncate">{provider?.name ?? providerId}</span>
    </span>
  )
}

export function GitProviderIcon({
  baseUrl,
  className,
  type,
}: {
  baseUrl?: string
  className?: string
  type: GitProvider['type']
}) {
  const faviconUrl = gitProviderFaviconUrl(type, baseUrl)

  return (
    <span className={className ?? 'flex size-8 shrink-0 items-center justify-center rounded-control border border-border bg-surface-subtle p-1 text-muted-foreground'}>
      {faviconUrl
        ? <GitProviderFavicon key={faviconUrl} faviconUrl={faviconUrl} type={type} />
        : <GitProviderFallbackIcon type={type} />}
    </span>
  )
}

function GitProviderFavicon({ faviconUrl, type }: { faviconUrl: string, type: GitProvider['type'] }) {
  const [faviconFailed, setFaviconFailed] = useState(false)

  if (faviconFailed)
    return <GitProviderFallbackIcon type={type} />

  return (
    <img
      alt=""
      className="size-full rounded-sm object-contain"
      src={faviconUrl}
      onError={() => setFaviconFailed(true)}
    />
  )
}

function GitProviderFallbackIcon({ type }: { type: GitProvider['type'] }) {
  if (type === 'gitea') {
    return (
      <svg aria-hidden="true" className="size-full" viewBox="0 0 24 24">
        <circle cx="12" cy="12" fill="#609926" r="11" />
        <path d="M7.2 9.1h8.1a3.2 3.2 0 0 1 0 6.4h-.9a4.4 4.4 0 0 1-8.5-1.6V10.4c0-.7.5-1.3 1.3-1.3Z" fill="#fff" />
        <path d="M14.8 11.2h1.1a1.1 1.1 0 1 1 0 2.2h-1.1Z" fill="#609926" />
        <path d="M8.2 11.5h5.4M8.2 13.4h4.2" stroke="#609926" strokeLinecap="round" strokeWidth="1.3" />
      </svg>
    )
  }
  if (type === 'gitlab') {
    return (
      <svg aria-hidden="true" className="size-full" viewBox="0 0 24 24">
        <path d="m12 21 4.2-12.9H7.8Z" fill="#E24329" />
        <path d="m12 21-8.6-6.2 4.2-12.2Z" fill="#FC6D26" />
        <path d="m12 21 8.6-6.2-4.2-12.2Z" fill="#FC6D26" />
        <path d="M3.4 14.8h8.6L7.6 2.6Z" fill="#FCA326" />
        <path d="M20.6 14.8H12l4.4-12.2Z" fill="#FCA326" />
      </svg>
    )
  }
  return (
    <svg aria-hidden="true" className="size-full" viewBox="0 0 24 24">
      <path
        d="M12 2.4a9.6 9.6 0 0 0-3 18.7c.5.1.7-.2.7-.5v-1.7c-2.8.6-3.4-1.2-3.4-1.2-.5-1.2-1.1-1.5-1.1-1.5-.9-.6.1-.6.1-.6 1 .1 1.5 1 1.5 1 .9 1.5 2.3 1.1 2.9.8.1-.7.4-1.1.7-1.4-2.2-.3-4.5-1.1-4.5-4.7 0-1 .4-1.9 1-2.6-.1-.3-.4-1.3.1-2.6 0 0 .8-.3 2.7 1a9.3 9.3 0 0 1 4.9 0c1.9-1.3 2.7-1 2.7-1 .5 1.3.2 2.3.1 2.6.6.7 1 1.6 1 2.6 0 3.7-2.3 4.5-4.5 4.7.4.3.7.9.7 1.8v2.7c0 .3.2.6.7.5A9.6 9.6 0 0 0 12 2.4Z"
        fill="currentColor"
        fillRule="evenodd"
      />
    </svg>
  )
}

export function OAuthAppGuide({ guide }: { guide: GitProviderGuide }) {
  const { t } = useTranslation()
  return (
    <Alert>
      <Info />
      <AlertTitle>{t('codeRepositoriesView.oauthAppGuideTitle')}</AlertTitle>
      <AlertDescription className="gap-3">
        <p>{t(`codeRepositoriesView.${guide.type}OAuthGuide`)}</p>
        <div className="grid w-full gap-2 rounded-control border border-border bg-surface-inset p-3 text-xs text-foreground">
          <GuideValue label={t('codeRepositoriesView.oauthAppName')} value={guide.appName} />
          <GuideValue label={t('codeRepositoriesView.oauthHomepageUrl')} value={guide.homepageUrl} />
          <GuideValue important label={t('codeRepositoriesView.oauthCallbackUrl')} value={guide.callbackUrl} />
          <GuideValue label={t('codeRepositoriesView.oauthScopes')} value={guide.scopes} />
        </div>
        <div className="flex flex-wrap gap-2">
          {guide.createUrl && (
            <Button type="button" variant="secondary" onClick={() => window.open(guide.createUrl, '_blank', 'noopener,noreferrer')}>
              <ExternalLink size={16} />
              {t('codeRepositoriesView.openOAuthAppCreatePage')}
            </Button>
          )}
          <Button type="button" variant="secondary" onClick={() => window.open(guide.docsUrl, '_blank', 'noopener,noreferrer')}>
            <ExternalLink size={16} />
            {t('codeRepositoriesView.openOfficialDocs')}
          </Button>
        </div>
      </AlertDescription>
    </Alert>
  )
}

function GuideValue({ important, label, value }: { important?: boolean, label: string, value: string }) {
  const { t } = useTranslation()
  return (
    <div className="grid gap-1 sm:grid-cols-[9rem_1fr_auto] sm:items-center">
      <span className="text-muted-foreground">{label}</span>
      <code className={important ? 'break-all font-mono text-primary-text' : 'break-all font-mono'}>{value}</code>
      <Button
        className="w-fit"
        type="button"
        variant="ghost"
        onClick={() => {
          navigator.clipboard.writeText(value)
            .then(() => toast.success(t('codeRepositoriesView.copied')))
            .catch(error => toast.error(error.message))
        }}
      >
        <Copy size={14} />
        {t('common.copy')}
      </Button>
    </div>
  )
}

export function CredentialOAuthGuide({ provider }: { provider: GitProvider }) {
  const { t } = useTranslation()
  if (isGitOAuthReady(provider)) {
    return (
      <Alert>
        <Info />
        <AlertTitle>{t('codeRepositoriesView.oauthReadyTitle')}</AlertTitle>
        <AlertDescription>
          <p>{t('codeRepositoriesView.oauthReadyDescription', { provider: provider.name })}</p>
          <Button className="mt-2" type="button" variant="secondary" onClick={() => { window.location.href = gitOAuthStartUrl(provider.id, '/code-repositories', window.location.origin) }}>
            <LinkIcon size={16} />
            {t('codeRepositoriesView.oauthConnect', { provider: provider.name })}
          </Button>
        </AlertDescription>
      </Alert>
    )
  }
  return (
    <Alert>
      <Info />
      <AlertTitle>{t('codeRepositoriesView.oauthManualOnlyTitle')}</AlertTitle>
      <AlertDescription>
        {provider.authType === 'oauth'
          ? t('codeRepositoriesView.oauthManualOnlyDescription', { provider: provider.name })
          : t('codeRepositoriesView.patManualOnlyDescription', { provider: provider.name })}
      </AlertDescription>
    </Alert>
  )
}

function projectScopeBadges(projectIds: string[] | undefined, projectMap: Record<string, string>) {
  return (projectIds ?? []).map(projectId => (
    <StatusBadge key={projectId}>{projectMap[projectId] ?? projectId}</StatusBadge>
  ))
}

function isGitOAuthReady(provider: GitProvider) {
  return provider.enabled && provider.authType === 'oauth' && provider.clientId.trim() !== '' && provider.clientSecretSet
}

function gitProviderFaviconUrl(type: GitProvider['type'], baseUrl?: string) {
  const normalized = normalizeGitBaseUrl(type, baseUrl)
  if (!normalized)
    return ''
  try {
    return `${new URL(normalized).origin}/favicon.ico`
  }
  catch {
    return ''
  }
}
