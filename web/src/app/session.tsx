import type { ReactNode } from 'react'
import type { InitializeAdminInput, LoginInput, RecentLoginUser, SessionContextValue } from './session-context'
import type { CurrentUser } from '@/api'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import { api } from '@/api'
import { enableBrowserTelemetry } from '@/lib/telemetry'
import { applyUserBrandColorPreference, clearActiveUserBrandColorPreference } from './brand-theme'
import { applyUserInterfaceStylePreference, clearActiveUserInterfaceStylePreference } from './interface-style'
import { SessionContext } from './session-context'

const currentUserQueryKey = ['current-user'] as const
const recentLoginUsersStorageKey = 'luna-devops.auth.recentUsers'
const maxRecentLoginUsers = 3

export function SessionProvider({ children }: { children: ReactNode }) {
  const { i18n } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [pendingLoginUsername, setPendingLoginUsername] = useState<string>()
  const [recentLoginUsers, setRecentLoginUsers] = useState<RecentLoginUser[]>(() => readRecentLoginUsers())
  const currentUser = useQuery({
    queryKey: currentUserQueryKey,
    queryFn: async () => {
      const user = await api.getCurrentUser()
      applyUserBrandColorPreference(user.id, user.brandColorPreset)
      applyUserInterfaceStylePreference(user.id, user.interfaceStyle)
      setRecentLoginUsers(cacheRecentLoginUser(user))
      return user
    },
    retry: false,
  })
  useEffect(() => {
    if (currentUser.data)
      enableBrowserTelemetry()
  }, [currentUser.data])

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
      clearActiveUserBrandColorPreference()
      clearActiveUserInterfaceStylePreference()
      queryClient.clear()
      navigate('/login')
    },
  })

  const updateLanguageMutation = useMutation({
    mutationFn: api.updateCurrentUser,
    onSuccess: (result) => {
      localStorage.setItem('luna-devops-language', result.language)
      i18n.changeLanguage(result.language)
      setCurrentUser(queryClient, result)
      setRecentLoginUsers(cacheRecentLoginUser(result))
    },
  })

  const updateProfileMutation = useMutation({
    mutationFn: api.updateCurrentUser,
    onSuccess: (result) => {
      localStorage.setItem('luna-devops-language', result.language)
      i18n.changeLanguage(result.language)
      setCurrentUser(queryClient, result)
      setRecentLoginUsers(cacheRecentLoginUser(result))
    },
  })

  const value = useMemo<SessionContextValue>(() => ({
    initialized: currentUser.isFetched,
    isLoading: currentUser.isLoading,
    isLoggingIn: loginMutation.isPending || initializeMutation.isPending || resumeLoginMutation.isPending,
    isLoggingOut: logoutMutation.isPending,
    pendingLoginUsername,
    recentLoginUsers,
    user: currentUser.data,
    async initializeAdmin(input: InitializeAdminInput) {
      const result = await initializeMutation.mutateAsync(input)
      return result.user
    },
    async login(input: LoginInput) {
      setPendingLoginUsername(input.email)
      try {
        const result = await loginMutation.mutateAsync(input)
        return result.user
      }
      finally {
        setPendingLoginUsername(undefined)
      }
    },
    async logout() {
      await logoutMutation.mutateAsync()
    },
    async refreshUser() {
      await queryClient.invalidateQueries({ queryKey: currentUserQueryKey })
    },
    async resumeLogin(userId: string) {
      setPendingLoginUsername(recentLoginUsers.find(user => user.id === userId)?.email)
      try {
        const result = await resumeLoginMutation.mutateAsync({ userId })
        return result.user
      }
      finally {
        setPendingLoginUsername(undefined)
      }
    },
    async updateProfile(input) {
      return updateProfileMutation.mutateAsync(input)
    },
    async updateLanguage(language: CurrentUser['language']) {
      return updateLanguageMutation.mutateAsync({ language })
    },
  }), [currentUser.data, currentUser.isFetched, currentUser.isLoading, initializeMutation, loginMutation, logoutMutation, pendingLoginUsername, queryClient, recentLoginUsers, resumeLoginMutation, updateLanguageMutation, updateProfileMutation])

  return <SessionContext value={value}>{children}</SessionContext>
}

function setCurrentUser(queryClient: ReturnType<typeof useQueryClient>, user: CurrentUser) {
  localStorage.setItem('luna-devops-language', user.language)
  applyUserBrandColorPreference(user.id, user.brandColorPreset)
  applyUserInterfaceStylePreference(user.id, user.interfaceStyle)
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
