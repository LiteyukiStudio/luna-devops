import type { CurrentUser } from '@/api'
import { LogOut } from 'lucide-react'
import { UserAvatar } from '@/components/common/user-avatar'
import { Button } from '@/components/ui/button'

/**
 * 侧边栏底部的当前用户面板。
 * 用于展示头像、名称、邮箱和登出动作；内部已处理长邮箱截断，放入固定宽侧边栏时不要再包额外卡片。
 */
export function SidebarUserPanel({
  user,
  logoutLabel,
  logoutPending,
  onLogout,
}: {
  user?: CurrentUser
  logoutLabel: string
  logoutPending?: boolean
  onLogout: () => void
}) {
  return (
    <div className="w-full min-w-0 max-w-full overflow-hidden px-2 py-1">
      <div className="flex w-full min-w-0 max-w-full items-center gap-3 overflow-hidden">
        <UserAvatar className="size-9 bg-primary text-primary-foreground" user={user} />
        <div className="min-w-0 flex-1 overflow-hidden">
          <p className="truncate text-sm font-medium">{user?.name ?? 'Demo User'}</p>
          <p className="truncate text-xs text-muted-foreground">{user?.email ?? 'demo@liteyuki.dev'}</p>
        </div>
        <Button aria-label={logoutLabel} className="size-8 shrink-0 px-0" disabled={logoutPending} variant="ghost" onClick={onLogout}>
          <LogOut size={15} />
        </Button>
      </div>
    </div>
  )
}
