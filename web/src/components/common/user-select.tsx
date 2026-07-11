import type { User } from '@/api'
import { Users } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { SearchSelect } from '@/components/common/search-select'
import { UserAvatar } from '@/components/common/user-avatar'
import { cn } from '@/lib/utils'

const ALL_USERS_VALUE = '__all_users__'

interface UserSelectProps {
  allLabel?: string
  ariaLabel?: string
  className?: string
  disabled?: boolean
  emptyLabel?: string
  includeAll?: boolean
  placeholder: string
  users: User[]
  value: string
  onChange: (value: string) => void
}

export function UserSelect({
  allLabel,
  ariaLabel,
  className,
  disabled,
  emptyLabel,
  includeAll = false,
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
      ariaLabel={ariaLabel ?? placeholder}
      className={cn('h-11 rounded-2xl', className)}
      disabled={disabled}
      emptyLabel={emptyLabel}
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
