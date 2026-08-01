import type { CurrentUser } from '@/api'
import { BookOpen, CircleUserRound, GitFork, LogOut } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { UserAvatar } from '@/components/common/user-avatar'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

const GITHUB_REPOSITORY_URL = 'https://github.com/LiteyukiStudio/luna-devops'
const DOCUMENTATION_URL = 'https://luna-devops.liteyuki.org'

/**
 * 全局顶栏的账号入口。
 * 触发器只展示头像，菜单集中承载当前账号信息、账号设置、帮助入口和退出确认。
 */
export function AccountMenu({
  user,
  logoutPending,
  onLogout,
}: {
  user: CurrentUser
  logoutPending?: boolean
  onLogout: () => void | Promise<void>
}) {
  const { t } = useTranslation()
  const [logoutConfirmOpen, setLogoutConfirmOpen] = useState(false)

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            aria-label={t('accountMenu')}
            className="size-9 shrink-0 rounded-full p-0 ring-offset-background transition-shadow hover:ring-2 hover:ring-primary-border focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
            disabled={logoutPending}
            variant="ghost"
          >
            <UserAvatar className="size-8 bg-primary text-primary-foreground" user={user} />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-64 max-w-[calc(100vw-2rem)]" sideOffset={8}>
          <DropdownMenuLabel className="grid gap-0.5 px-2 py-2 font-normal">
            <span className="truncate text-sm font-medium text-foreground">{user.name}</span>
            <span className="truncate text-xs text-muted-foreground">{user.email}</span>
          </DropdownMenuLabel>
          <DropdownMenuSeparator />
          <DropdownMenuItem asChild>
            <Link to="/settings/account">
              <CircleUserRound />
              <span>{t('account')}</span>
            </Link>
          </DropdownMenuItem>
          <DropdownMenuItem asChild>
            <a href={GITHUB_REPOSITORY_URL} rel="noreferrer" target="_blank">
              <GitFork />
              <span>{t('githubRepository')}</span>
            </a>
          </DropdownMenuItem>
          <DropdownMenuItem asChild>
            <a href={DOCUMENTATION_URL} rel="noreferrer" target="_blank">
              <BookOpen />
              <span>{t('documentation')}</span>
            </a>
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem variant="destructive" onSelect={() => setLogoutConfirmOpen(true)}>
            <LogOut />
            <span>{t('logout')}</span>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <ConfirmDialog
        confirmText={t('logout')}
        description={t('logoutConfirmDescription')}
        open={logoutConfirmOpen}
        pending={logoutPending}
        title={t('logoutConfirmTitle')}
        onConfirm={onLogout}
        onOpenChange={setLogoutConfirmOpen}
      />
    </>
  )
}
