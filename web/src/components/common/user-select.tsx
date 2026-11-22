import type { User } from '@/api'
import { Users } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { SearchSelect } from '@/components/common/search-select'
import { UserAvatar } from '@/components/common/user-avatar'

const ALL_USERS_VALUE = '__all_users__'

interface UserSelectProps {
  'aria-describedby'?: string
  'aria-invalid'?: boolean | 'true' | 'false'
  'allLabel'?: string
  'ariaLabel'?: string
  'className'?: string
  'disabled'?: boolean
  'emptyLabel'?: string
  'error'?: boolean
  'id'?: string
  'includeAll'?: boolean
  'loading'?: boolean
  'placeholder': string
  'users': User[]
  'value': string
  'onChange': (value: string) => void
}

export function UserSelect({
  'aria-describedby': ariaDescribedBy,
  'aria-invalid': ariaInvalid,
  allLabel,
  ariaLabel,
  className,
  disabled,
  emptyLabel,
  error,
  id,
  includeAll = false,
  loading,
  placeholder,
  users,
  value,
  onChange,
}: UserSelectProps) {
  const { t } = useTranslation()
  const options = useMemo(() => {
    const userOptions = users.map(user => ({
      description: user.email,
      icon: <UserAvatar className="size-6" user={user} />,
      keywords: user.email,
      label: userDisplayName(user),
      value: user.id,
    }))
    if (!includeAll)
      return userOptions
    return [{
      description: '',
      icon: <Users className="size-6 shrink-0 rounded-full bg-muted p-1 text-muted-foreground" />,
      label: allLabel ?? placeholder,
      value: ALL_USERS_VALUE,
    }, ...userOptions]
  }, [allLabel, includeAll, placeholder, users])

  return (
    <SearchSelect
      aria-describedby={ariaDescribedBy}
      aria-invalid={ariaInvalid}
      ariaLabel={ariaLabel ?? (id ? undefined : placeholder)}
      className={className}
      disabled={disabled}
      emptyLabel={emptyLabel}
      error={error}
      id={id}
      loading={loading}
      options={options}
      placeholder={placeholder}
      searchPlaceholder={t('common.search')}
      value={value || (includeAll ? ALL_USERS_VALUE : '')}
      onValueChange={nextValue => onChange(nextValue === ALL_USERS_VALUE ? '' : nextValue)}
    />
  )
}

function userDisplayName(user: User) {
  return user.name || user.email
}
