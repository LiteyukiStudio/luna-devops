import type { GitAccount, GitProvider, GitRepository } from '@/api'
import { useQuery } from '@tanstack/react-query'
import { Globe, Search } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '@/api'
import { EmptyState } from '@/components/common/empty-state'
import { FormField as Field } from '@/components/common/form-field'
import { StatusBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'

// anonymousGitModeAccountId 是公开仓库匿名访问占位符，与后端 anonymousGitAccountID 对应。
const anonymousGitModeAccountId = 'anonymous'

export interface GitRepositoryPickerValue {
  gitAccountId: string
  owner: string
  repo: string
  cloneUrl: string
  defaultBranch: string
  /** 匿名模式使用的 providerId */
  providerId?: string
}

export function GitRepositoryPicker({
  accounts,
  disabled,
  providers,
  value,
  onChange,
}: {
  accounts: GitAccount[]
  disabled?: boolean
  providers: GitProvider[]
  value: GitRepositoryPickerValue
  onChange: (value: GitRepositoryPickerValue) => void
}) {
  const { t } = useTranslation()
  const isAnonymous = value.gitAccountId === anonymousGitModeAccountId
  const [search, setSearch] = useState(value.owner && value.repo ? `${value.owner}/${value.repo}` : '')
  const repositories = useQuery({
    queryKey: ['git-repositories', value.gitAccountId, value.providerId, search],
    queryFn: () => api.listGitRepositories(value.gitAccountId, {
      page: 1,
      pageSize: 50,
      search,
      includePublic: !isAnonymous,
      providerId: isAnonymous ? value.providerId : undefined,
    }),
    enabled: Boolean(value.gitAccountId) && (!isAnonymous || Boolean(value.providerId)),
  })

  function selectRepository(repository: GitRepository) {
    onChange({
      gitAccountId: value.gitAccountId,
      owner: repository.owner,
      repo: repository.name,
      cloneUrl: repository.cloneUrl,
      defaultBranch: repository.defaultBranch || 'main',
      providerId: value.providerId,
    })
    setSearch(repository.fullName)
  }

  function gitAccountLabel(account: GitAccount) {
    const provider = providers.find(item => item.id === account.providerId)
    const scope = t(`codeRepositoriesView.scope${account.scope.charAt(0).toUpperCase()}${account.scope.slice(1)}`)
    return `${provider?.name ?? account.providerId} / ${account.username} (${scope})`
  }

  const enabledProviders = providers.filter(p => p.enabled !== false)

  return (
    <div className="grid gap-3">
      <Field hint={t('repositories.accessModeHint')} label={t('repositories.accessMode')} required>
        <RadioGroup
          disabled={disabled}
          value={isAnonymous ? 'public' : 'account'}
          onValueChange={(mode) => {
            if (mode === 'public') {
              onChange({ gitAccountId: anonymousGitModeAccountId, owner: '', repo: '', cloneUrl: '', defaultBranch: 'main', providerId: enabledProviders[0]?.id ?? '' })
            }
            else {
              onChange({ gitAccountId: '', owner: '', repo: '', cloneUrl: '', defaultBranch: 'main' })
            }
            setSearch('')
          }}
        >
          <div className="flex items-center space-x-2">
            <RadioGroupItem id="access-mode-account" value="account" />
            <label htmlFor="access-mode-account" className="text-sm text-foreground">
              {t('repositories.accessModeAccount')}
            </label>
          </div>
          <div className="flex items-center space-x-2">
            <RadioGroupItem id="access-mode-public" value="public" />
            <label htmlFor="access-mode-public" className="flex items-center gap-1 text-sm text-foreground">
              <Globe size={14} className="text-muted-foreground" />
              {t('repositories.accessModePublic')}
            </label>
          </div>
        </RadioGroup>
      </Field>
      {isAnonymous
        ? (
            <Field hint={t('repositories.gitPlatformHint')} label={t('repositories.gitPlatform')} required>
              <Select
                disabled={disabled}
                value={value.providerId ?? ''}
                onChange={(event) => {
                  onChange({ ...value, providerId: event.target.value, owner: '', repo: '', cloneUrl: '', defaultBranch: 'main' })
                  setSearch('')
                }}
              >
                <option value="">{t('repositories.selectGitPlatform')}</option>
                {enabledProviders.map(provider => (
                  <option key={provider.id} value={provider.id}>
                    {provider.name}
                    {' '}
                    (
                    {provider.type}
                    )
                  </option>
                ))}
              </Select>
            </Field>
          )
        : (
            <Field hint={t('repositories.gitAccountHint')} label={t('repositories.gitAccount')} required>
              <Select
                disabled={disabled}
                value={value.gitAccountId}
                onChange={(event) => {
                  onChange({ gitAccountId: event.target.value, owner: '', repo: '', cloneUrl: '', defaultBranch: 'main' })
                  setSearch('')
                }}
              >
                <option value="">{t('repositories.selectAccount')}</option>
                {accounts.map(account => (
                  <option key={account.id} value={account.id}>
                    {gitAccountLabel(account)}
                  </option>
                ))}
              </Select>
            </Field>
          )}
      <Field hint={t('repositories.repositorySearchHint')} label={t('repositories.repositorySearch')} required>
        <div className="flex gap-2">
          <Input
            disabled={disabled || !value.gitAccountId || (isAnonymous && !value.providerId)}
            placeholder={t('repositories.repositorySearchPlaceholder')}
            value={search}
            onChange={event => setSearch(event.target.value)}
            onFocus={() => {
              if (value.owner && value.repo && !search)
                setSearch(`${value.owner}/${value.repo}`)
            }}
          />
          <Button disabled={disabled || !value.gitAccountId || (isAnonymous && !value.providerId) || repositories.isFetching} type="button" variant="secondary" onClick={() => repositories.refetch()}>
            <Search size={16} />
            {t('repositories.search')}
          </Button>
        </div>
      </Field>
      {value.gitAccountId && (isAnonymous ? Boolean(value.providerId) : true) && (
        <div className="grid max-h-56 gap-2 overflow-y-auto rounded-md border border-border p-2">
          {(repositories.data?.items ?? []).map(repository => (
            <button
              key={repository.fullName}
              className="rounded-md px-3 py-2 text-left hover:bg-muted"
              disabled={disabled}
              type="button"
              onClick={() => selectRepository(repository)}
            >
              <span className="flex items-center gap-2 text-sm font-medium">
                <span className="min-w-0 flex-1 truncate">{repository.fullName}</span>
                <StatusBadge tone={repository.source === 'public' ? 'info' : 'neutral'}>
                  {t(`repositories.repositorySources.${repository.source || 'accessible'}`)}
                </StatusBadge>
              </span>
              <span className="block truncate text-xs text-muted-foreground">{repository.cloneUrl}</span>
            </button>
          ))}
          {repositories.isSuccess && repositories.data.items.length === 0 && (
            <EmptyState description={t('repositories.noRepositoriesDescription')} title={t('repositories.noRepositoriesTitle')} variant="plain" />
          )}
        </div>
      )}
      {value.owner && value.repo && (
        <div className="grid gap-1 rounded-md border border-border bg-muted/30 px-3 py-2 text-xs text-muted-foreground md:grid-cols-3">
          <span className="truncate">
            {t('repositories.owner')}
            :
            {' '}
            <strong className="text-foreground">{value.owner}</strong>
          </span>
          <span className="truncate">
            {t('repositories.repo')}
            :
            {' '}
            <strong className="text-foreground">{value.repo}</strong>
          </span>
          <span className="truncate">
            {t('repositories.defaultBranch')}
            :
            {' '}
            <strong className="text-foreground">{value.defaultBranch || 'main'}</strong>
          </span>
        </div>
      )}
    </div>
  )
}
