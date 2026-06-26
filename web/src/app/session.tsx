import type { ReactNode } from 'react'
import type { DebugSessionOverride, InitializeAdminInput, LoginInput, RecentLoginUser, SessionContextValue } from './session-context'
import type { CurrentUser } from '@/api'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import { api } from '@/api'
import { SessionContext } from './session-context'

const currentUserQueryKey = ['current-user'] as const
const debugOverrideStorageKey = 'liteyuki-devops.debug.sessionOverride'
const recentLoginUsersStorageKey = 'liteyuki-devops.auth.recentUsers'
const maxRecentLoginUsers = 3
const adminPermissions = [
  'project.create',
  'project.read',
  'project.update',
  'project.delete',
  'application.create',
  'application.read',
  'application.update',
  'application.delete',
  'token.create',
  'token.revoke',
  'user.manage',
]
const userPermissions = ['project.read', 'application.read']

export function SessionProvider({ children }: { children: ReactNode }) {
  const { i18n } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [debugSessionOverride, setDebugSessionOverride] = useState<DebugSessionOverride | undefined>(() => readDebugOverride())
  const [recentLoginUsers, setRecentLoginUsers] = useState<RecentLoginUser[]>(() => readRecentLoginUsers())
  const currentUser = useQuery({
    queryKey: currentUserQueryKey,
    queryFn: async () => {
      const user = await api.getCurrentUser()
      setRecentLoginUsers(cacheRecentLoginUser(user))
      return user
    },
    retry: false,
  })
  const effectiveUser = useMemo(() => applyDebugOverride(currentUser.data, debugSessionOverride), [currentUser.data, debugSessionOverride])

  const loginMutation = useMutation({
    mutationFn: api.login,
    onSuccess: (result) => {
      setCurrentUser(queryClient, result.user)
      setRecentLoginUsers(cacheRecentLoginUser(result.user))
    },
  })

  const resumeLoginMutation = useMutation({
    mutationFn: api.resumeLogin,
    onSuccess: (result) => {
      setCurrentUser(queryClient, result.user)
      setRecentLoginUsers(cacheRecentLoginUser(result.user))
    },
  })

  const initializeMutation = useMutation({
    mutationFn: api.initializeAdmin,
    onSuccess: (result) => {
      setCurrentUser(queryClient, result.user)
      setRecentLoginUsers(cacheRecentLoginUser(result.user))
      queryClient.invalidateQueries({ queryKey: ['bootstrap-status'] })
    },
  })

  const logoutMutation = useMutation({
    mutationFn: api.logout,
    onSuccess: () => {
      queryClient.clear()
      navigate('/login')
    },
  })

  const updateLanguageMutation = useMutation({
    mutationFn: api.updateCurrentUser,
    onSuccess: (result) => {
      localStorage.setItem('liteyuki-language', result.language)
      i18n.changeLanguage(result.language)
      setCurrentUser(queryClient, result)
      setRecentLoginUsers(cacheRecentLoginUser(result))
    },
  })

  const updateProfileMutation = useMutation({
    mutationFn: api.updateCurrentUser,
    onSuccess: (result) => {
      localStorage.setItem('liteyuki-language', result.language)
      i18n.changeLanguage(result.language)
      setCurrentUser(queryClient, result)
      setRecentLoginUsers(cacheRecentLoginUser(result))
    },
  })

  const value = useMemo<SessionContextValue>(() => ({
    actualUser: currentUser.data,
    debugOverride: debugSessionOverride,
    initialized: currentUser.isFetched,
    isLoading: currentUser.isLoading,
    isLoggingIn: loginMutation.isPending || initializeMutation.isPending || resumeLoginMutation.isPending,
    isLoggingOut: logoutMutation.isPending,
    recentLoginUsers,
    user: effectiveUser,
    clearDebugOverride() {
      setDebugSessionOverride(undefined)
      if (import.meta.env.DEV)
        localStorage.removeItem(debugOverrideStorageKey)
    },
    async initializeAdmin(input: InitializeAdminInput) {
      const result = await initializeMutation.mutateAsync(input)
      return result.user
    },
    async login(input: LoginInput) {
      const result = await loginMutation.mutateAsync(input)
      return result.user
    },
    async logout() {
      await logoutMutation.mutateAsync()
    },
    async refreshUser() {
      await queryClient.invalidateQueries({ queryKey: currentUserQueryKey })
    },
    async resumeLogin(userId: string) {
      const result = await resumeLoginMutation.mutateAsync({ userId })
      return result.user
    },
    setDebugOverride(override: DebugSessionOverride) {
      setDebugSessionOverride(override)
      if (import.meta.env.DEV)
        localStorage.setItem(debugOverrideStorageKey, JSON.stringify(override))
    },
    async updateProfile(input) {
      return updateProfileMutation.mutateAsync(input)
    },
    async updateLanguage(language: CurrentUser['language']) {
      return updateLanguageMutation.mutateAsync({ language })
    },
  }), [currentUser.data, currentUser.isFetched, currentUser.isLoading, debugSessionOverride, effectiveUser, initializeMutation, loginMutation, logoutMutation, queryClient, recentLoginUsers, resumeLoginMutation, updateLanguageMutation, updateProfileMutation])

  return <SessionContext value={value}>{children}</SessionContext>
}

function setCurrentUser(queryClient: ReturnType<typeof useQueryClient>, user: CurrentUser) {
  localStorage.setItem('liteyuki-language', user.language)
  queryClient.setQueryData(currentUserQueryKey, user)
}

function cacheRecentLoginUser(user: CurrentUser) {
  return (currentUsers: RecentLoginUser[]) => {
    const nextUser: RecentLoginUser = {
      avatarUrl: user.avatarUrl || '',
      email: user.email,
      id: user.id,
      lastLoginAt: new Date().toISOString(),
      name: user.name,
    }
    const nextUsers = [
      nextUser,
      ...currentUsers.filter(item => item.id !== user.id && item.email !== user.email),
    ].slice(0, maxRecentLoginUsers)

    writeRecentLoginUsers(nextUsers)
    return nextUsers
  }
}

function readRecentLoginUsers(): RecentLoginUser[] {
  try {
    const raw = localStorage.getItem(recentLoginUsersStorageKey)
    if (!raw)
      return []

    const parsed = JSON.parse(raw) as RecentLoginUser[]
    if (!Array.isArray(parsed))
      return []

    return parsed
      .filter(isRecentLoginUser)
      .slice(0, maxRecentLoginUsers)
  }
  catch {
    localStorage.removeItem(recentLoginUsersStorageKey)
    return []
  }
}

function writeRecentLoginUsers(users: RecentLoginUser[]) {
  localStorage.setItem(recentLoginUsersStorageKey, JSON.stringify(users))
}

function isRecentLoginUser(value: unknown): value is RecentLoginUser {
  if (!value || typeof value !== 'object')
    return false

  const user = value as Partial<RecentLoginUser>
  return Boolean(user.id && user.email && user.lastLoginAt)
}

function readDebugOverride(): DebugSessionOverride | undefined {
  if (!import.meta.env.DEV)
    return undefined

  try {
    const raw = localStorage.getItem(debugOverrideStorageKey)
    if (!raw)
      return undefined

    const parsed = JSON.parse(raw) as DebugSessionOverride
    if (parsed.type === 'role' && (parsed.role === 'platform_admin' || parsed.role === 'user'))
      return parsed
  }
  catch {
    localStorage.removeItem(debugOverrideStorageKey)
  }
  return undefined
}

function applyDebugOverride(user: CurrentUser | undefined, override: DebugSessionOverride | undefined): CurrentUser | undefined {
  if (!import.meta.env.DEV || !user || !override)
    return user

  if (override.type === 'role') {
    return {
      ...user,
      role: override.role,
      permissions: permissionsForRole(override.role),
    }
  }

  return user
}

function permissionsForRole(role: string) {
  return role === 'platform_admin' ? adminPermissions : userPermissions
}
