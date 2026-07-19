import type { AccessTokenScopeDefinition } from '@/api'
import { CheckCheck, Search, X } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { accessTokenScopeKey, accessTokenScopeLabel, splitAccessTokenScopes } from '@/components/common/access-token-scope'
import { CheckboxField } from '@/components/common/checkbox-field'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

interface AccessTokenScopeSelectorProps {
  items: AccessTokenScopeDefinition[]
  loading?: boolean
  value: string[]
  onChange: (value: string[]) => void
}

export function AccessTokenScopeSelector({ items, loading = false, value, onChange }: AccessTokenScopeSelectorProps) {
  const { t } = useTranslation()
  const [search, setSearch] = useState('')
  const normalizedSearch = search.trim().toLocaleLowerCase()
  const groups = useMemo(() => {
    const filteredItems = normalizedSearch
      ? items.filter((scope) => {
          const searchableText = [
            scope.value,
            accessTokenScopeLabel(t, scope.value),
            t(`accessTokens.scopeDescriptions.${accessTokenScopeKey(scope.value)}`),
            t(`accessTokens.scopeGroups.${scope.group}`),
          ].join(' ').toLocaleLowerCase()
          return searchableText.includes(normalizedSearch)
        })
      : items

    const groupedItems = new Map<string, AccessTokenScopeDefinition[]>()
    for (const item of filteredItems)
      groupedItems.set(item.group, [...(groupedItems.get(item.group) ?? []), item])

    return Array.from(groupedItems.entries()).map(([group, groupItems]) => ({ group, items: groupItems }))
  }, [items, normalizedSearch, t])
  const recommendedScopes = items
    .filter(scope => scope.recommended && !scope.requiresAdminRole)
    .map(scope => scope.value)

  const toggleScope = (scope: string, checked: boolean) => {
    onChange(checked
      ? Array.from(new Set([...value, scope]))
      : value.filter(item => item !== scope))
  }

  return (
    <div className="overflow-hidden rounded-md border border-border bg-card">
      <div className="grid gap-2 border-b border-border p-3">
        <div className="relative min-w-0">
          <Search aria-hidden="true" className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            aria-label={t('accessTokens.scopeSearchPlaceholder')}
            className="pl-9"
            placeholder={t('accessTokens.scopeSearchPlaceholder')}
            value={search}
            onChange={event => setSearch(event.target.value)}
          />
        </div>
        <div className="flex min-w-0 flex-wrap items-center justify-between gap-2">
          <Badge variant="secondary">{t('common.selectedCount', { count: value.length })}</Badge>
          <div className="flex flex-wrap items-center justify-end gap-2">
            <Button
              disabled={recommendedScopes.length === 0}
              size="sm"
              variant="outline"
              onClick={() => onChange(Array.from(new Set([...value, ...recommendedScopes])))}
            >
              <CheckCheck size={14} />
              {t('accessTokens.addRecommended')}
            </Button>
            <Button disabled={value.length === 0} size="sm" variant="ghost" onClick={() => onChange([])}>
              <X size={14} />
              {t('common.clearSelection')}
            </Button>
          </div>
        </div>
      </div>
      <div className="max-h-80 overflow-y-auto">
        {loading && <p className="px-3 py-3 text-sm text-muted-foreground">{t('common.loading')}</p>}
        {!loading && groups.length === 0 && (
          <p className="px-3 py-3 text-sm text-muted-foreground">
            {normalizedSearch ? t('accessTokens.noMatchingScopes') : t('accessTokens.emptyScopes')}
          </p>
        )}
        {groups.map(group => (
          <section key={group.group} className="border-b border-border last:border-b-0">
            <div className="flex items-center justify-between bg-muted/60 px-3 py-2 text-xs font-semibold text-muted-foreground">
              <span>{t(`accessTokens.scopeGroups.${group.group}`)}</span>
              <span>{group.items.length}</span>
            </div>
            <div className="grid gap-3 p-3 sm:grid-cols-2">
              {group.items.map(scope => (
                <CheckboxField
                  key={scope.value}
                  checked={value.includes(scope.value)}
                  className={cn(scope.requiresAdminRole && 'opacity-60')}
                  description={t(`accessTokens.scopeDescriptions.${accessTokenScopeKey(scope.value)}`)}
                  disabled={scope.requiresAdminRole}
                  onChange={event => toggleScope(scope.value, event.target.checked)}
                >
                  <span className="flex flex-wrap items-center gap-2">
                    {accessTokenScopeLabel(t, scope.value)}
                    {scope.recommended && <Badge variant="secondary">{t('accessTokens.recommended')}</Badge>}
                    {scope.requiresAdminRole && <Badge variant="outline">{t('accessTokens.adminOnly')}</Badge>}
                  </span>
                </CheckboxField>
              ))}
            </div>
          </section>
        ))}
      </div>
    </div>
  )
}

export function AccessTokenScopeBadges({ className, scope }: { className?: string, scope: string | string[] }) {
  const { t } = useTranslation()
  const scopes = Array.isArray(scope) ? scope : splitAccessTokenScopes(scope)

  return (
    <div className={cn('flex max-w-md flex-wrap gap-1', className)}>
      {scopes.map(item => (
        <Badge key={item} title={item} variant="secondary">
          {accessTokenScopeLabel(t, item)}
        </Badge>
      ))}
    </div>
  )
}
