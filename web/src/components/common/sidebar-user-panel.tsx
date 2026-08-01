import type { CurrentUser } from '@/api'
import { BookOpen, Ellipsis, GitFork, LogOut } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { UserAvatar } from '@/components/common/user-avatar'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

const GITHUB_REPOSITORY_URL = 'https://github.com/LiteyukiStudio/luna-devops'
const DOCUMENTATION_URL = 'https://luna-devops.liteyuki.org'

/**
 * 侧边栏底部的当前用户面板。
 * 用于展示头像、名称、邮箱和账号菜单；内部已处理长邮箱截断，放入固定宽侧边栏时不要再包额外卡片。
 */
export function SidebarUserPanel({
  user,
  logoutPending,
  onLogout,
}: {
  user?: CurrentUser
  logoutPending?: boolean
  onLogout: () => void | Promise<void>
}) {
  const { t } = useTranslation()
  const [logoutConfirmOpen, setLogoutConfirmOpen] = useState(false)

  return (
    <div className="w-full min-w-0 max-w-full px-2 py-1">
      <div className="flex w-full min-w-0 max-w-full items-center gap-3">
        <UserAvatar className="size-9 bg-primary text-primary-foreground" user={user} />
        <div className="min-w-0 flex-1 overflow-hidden">
          <p className="truncate text-sm font-medium">{user?.name ?? 'Demo User'}</p>
          <p className="truncate text-xs text-muted-foreground">{user?.email ?? 'demo@luna.dev'}</p>
        </div>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button aria-label={t('accountMenu')} className="size-8 shrink-0 px-0" disabled={logoutPending} variant="ghost">
              <Ellipsis size={16} />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" side="top">
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
      </div>
      <ConfirmDialog
        confirmText={t('logout')}
        description={t('logoutConfirmDescription')}
        open={logoutConfirmOpen}
        pending={logoutPending}
        title={t('logoutConfirmTitle')}
        onConfirm={onLogout}
        onOpenChange={setLogoutConfirmOpen}
      />
    </div>
  )
}
