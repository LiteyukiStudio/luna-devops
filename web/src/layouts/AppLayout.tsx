import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Box, LayoutDashboard, Link2, Settings, ShieldCheck, Users } from 'lucide-react'
import { AnimatePresence } from 'motion/react'
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { api } from '../api/client'
import { usePublicConfig } from '../app/public-config-context'
import { useTheme } from '../app/theme-context'
import { AuthErrorPage } from '../components/common/auth-error-page'
import { PageMotion } from '../components/common/motion'
import { SidebarUserPanel } from '../components/common/sidebar-user-panel'
import { ThemeModeSegmented } from '../components/common/theme-mode-segmented'
import { Button } from '../components/ui/button'
import { cn } from '../lib/utils'

const navSections = [
  {
    title: 'DevOps',
    items: [
      { to: '/projects', labelKey: 'projects', icon: LayoutDashboard },
    ],
  },
  {
    title: '个人工作区',
    items: [
      { to: '/settings/security', labelKey: 'security', icon: Link2 },
    ],
  },
  {
    title: '系统管理',
    items: [
      { to: '/settings/auth-providers', labelKey: 'authProviders', icon: ShieldCheck, permission: 'user.manage' },
      { to: '/settings/users', labelKey: 'users', icon: Users, permission: 'user.manage' },
      { to: '/settings/site', labelKey: 'siteSettings', icon: Settings },
    ],
  },
]

export function AppLayout() {
  const { i18n, t } = useTranslation()
  const { mode, setMode } = useTheme()
  const configs = usePublicConfig()
  const navigate = useNavigate()
  const location = useLocation()
  const queryClient = useQueryClient()
  const user = useQuery({ queryKey: ['current-user'], queryFn: api.getCurrentUser })
  const logout = useMutation({
    mutationFn: api.logout,
    onSuccess: () => {
      queryClient.clear()
      navigate('/login')
    },
    onError: error => toast.error(error.message),
  })
  const updateLanguage = useMutation({
    mutationFn: api.updateCurrentUser,
    onSuccess: (result) => {
      i18n.changeLanguage(result.language)
      queryClient.setQueryData(['current-user'], result)
    },
    onError: error => toast.error(error.message),
  })

  const handleLogout = () => {
    logout.mutate()
  }

  useEffect(() => {
    if (user.data?.language && i18n.language !== user.data.language)
      i18n.changeLanguage(user.data.language)
  }, [i18n, user.data?.language])

  if (user.isError) {
    return (
      <AuthErrorPage
        title="需要登录"
        description="请先使用本地账号或已授权的 OIDC 账号登录平台。"
      />
    )
  }

  return (
    <div className="min-h-screen bg-background text-foreground">
      <aside className="fixed inset-y-0 left-0 hidden w-64 grid-rows-[auto_1fr_auto] overflow-hidden border-r border-border bg-surface lg:grid">
        <Link
          aria-label={configs['site.title'] || t('appName')}
          className="flex h-16 min-w-0 items-center gap-3 border-b border-border px-5"
          to="/projects"
        >
          <span className="flex size-9 shrink-0 items-center justify-center rounded-md bg-primary text-primary-foreground">
            {configs['site.logoUrl']
              ? <img alt="" className="size-6 rounded-sm object-contain" src={configs['site.logoUrl']} />
              : <Box size={18} />}
          </span>
          <span className="truncate font-semibold">{configs['site.title'] || t('appName')}</span>
        </Link>
        <nav className="overflow-y-auto px-3 py-4">
          {navSections.map((section, index) => {
            const items = section.items.filter(item => !item.permission || user.data?.permissions.includes(item.permission))
            if (items.length === 0)
              return null

            return (
              <section key={section.title} className={cn(index > 0 && 'mt-4 border-t border-border pt-4')}>
                <p className="mb-2 px-3 text-xs font-medium uppercase tracking-normal text-muted-foreground">{section.title}</p>
                <div className="space-y-1">
                  {items.map(item => (
                    <NavLink
                      key={item.to}
                      className={({ isActive }) => cn(
                        'flex h-10 items-center gap-3 rounded-md px-3 text-sm text-muted-foreground transition-all duration-150 hover:bg-muted hover:text-foreground',
                        isActive && 'bg-muted text-foreground',
                      )}
                      title={t(item.labelKey)}
                      to={item.to}
                    >
                      <item.icon size={17} />
                      <span className="truncate">{t(item.labelKey)}</span>
                    </NavLink>
                  ))}
                </div>
              </section>
            )
          })}
        </nav>
        <div className="grid gap-3 border-t border-border p-3">
          <ThemeModeSegmented mode={mode} setMode={setMode} />
          <div className="flex items-center gap-2">
            <select
              aria-label="语言"
              className="h-9 flex-1 rounded-md border border-border bg-background px-2 text-sm transition duration-150 focus:border-primary"
              value={user.data?.language || 'zh-CN'}
              onChange={event => updateLanguage.mutate({ language: event.target.value as 'zh-CN' | 'en-US' })}
            >
              <option value="zh-CN">中文</option>
              <option value="en-US">English</option>
            </select>
          </div>
          <SidebarUserPanel
            logoutLabel={t('logout')}
            logoutPending={logout.isPending}
            user={user.data}
            onLogout={handleLogout}
          />
        </div>
      </aside>

      <div className="lg:min-h-screen lg:pl-64">
        <header className="sticky top-0 z-10 flex h-16 items-center justify-between border-b border-border bg-background/90 px-4 backdrop-blur lg:px-6">
          <div className="min-w-0">
            <p className="truncate text-sm font-medium">{configs['site.title'] || t('appName')}</p>
            <p className="text-xs text-muted-foreground lg:hidden">{user.data?.email ?? 'demo@liteyuki.dev'}</p>
          </div>
          <div className="flex items-center gap-2 lg:hidden">
            <select
              aria-label="语言"
              className="h-9 rounded-md border border-border bg-surface px-2 text-sm"
              value={user.data?.language || 'zh-CN'}
              onChange={event => updateLanguage.mutate({ language: event.target.value as 'zh-CN' | 'en-US' })}
            >
              <option value="zh-CN">中文</option>
              <option value="en-US">English</option>
            </select>
            <Button aria-label={t('logout')} disabled={logout.isPending} variant="ghost" onClick={handleLogout}>
              {t('logout')}
            </Button>
          </div>
        </header>
        <main className="mx-auto max-w-7xl px-4 py-6 lg:px-6">
          <AnimatePresence mode="wait">
            <PageMotion key={location.pathname}>
              <Outlet />
            </PageMotion>
          </AnimatePresence>
        </main>
      </div>
    </div>
  )
}
