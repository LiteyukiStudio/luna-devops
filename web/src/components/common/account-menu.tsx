import type { CurrentUser } from '@/api'
import { CircleUserRound, CreditCard, EllipsisVertical, LogOut } from 'lucide-react'
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

/**
 * 个人账号菜单入口。
 * 触发器展示头像或侧栏底部用户卡片，菜单集中承载当前账号信息、账号设置、账单和退出确认。
 */
export function AccountMenu({
  user,
  logoutPending,
  onLogout,
  variant = 'avatar',
}: {
  user: CurrentUser
  logoutPending?: boolean
  onLogout: () => void | Promise<void>
  /** avatar：只显示头像；sidebar：侧栏底部的用户卡片（头像、名称、邮箱）。 */
  variant?: 'avatar' | 'sidebar'
}) {
  const { t } = useTranslation()
  const [logoutConfirmOpen, setLogoutConfirmOpen] = useState(false)

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          {variant === 'sidebar'
            ? (
                <button
                  aria-label={t('accountMenu')}
                  className="flex w-full min-w-0 items-center gap-2 rounded-control p-2 text-left text-sm outline-none transition-colors hover:bg-sidebar-nav-hover focus-visible:ring-2 focus-visible:ring-ring data-[state=open]:bg-sidebar-nav-active disabled:opacity-50"
                  disabled={logoutPending}
                  type="button"
                >
                  <UserAvatar className="size-8 shrink-0 bg-primary text-primary-foreground" user={user} />
                  <span className="grid min-w-0 flex-1 leading-tight">
                    <span className="truncate font-medium">{user.name}</span>
                    <span className="truncate text-xs text-muted-foreground">{user.email}</span>
                  </span>
                  <EllipsisVertical aria-hidden="true" className="size-4 shrink-0 text-muted-foreground" />
                </button>
              )
            : (
                <Button
                  aria-label={t('accountMenu')}
                  className="size-9 shrink-0 rounded-full p-0 ring-offset-background transition-shadow hover:ring-2 hover:ring-primary-border focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                  disabled={logoutPending}
                  variant="ghost"
                >
                  <UserAvatar className="size-8 bg-primary text-primary-foreground" user={user} />
                </Button>
              )}
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-64 max-w-[calc(100vw-2rem)]" side={variant === 'sidebar' ? 'right' : 'bottom'} sideOffset={8}>
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
            <Link to="/billing">
              <CreditCard />
              <span>{t('billing')}</span>
            </Link>
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
