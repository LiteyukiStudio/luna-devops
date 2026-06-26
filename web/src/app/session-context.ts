import type { CurrentUser } from '@/api'
import { createContext, use } from 'react'

export interface LoginInput {
  email: string
  password: string
}

export interface RecentLoginUser {
  avatarUrl: string
  email: string
  id: string
  lastLoginAt: string
  name: string
}

export interface InitializeAdminInput {
  email: string
  name: string
  password: string
  language: CurrentUser['language']
}

interface DebugRoleOverride {
  role: 'platform_admin' | 'user'
  type: 'role'
}

export type DebugSessionOverride = DebugRoleOverride

export interface SessionContextValue {
  actualUser?: CurrentUser
  debugOverride?: DebugSessionOverride
  initialized: boolean
  isLoading: boolean
  isLoggingIn: boolean
  isLoggingOut: boolean
  recentLoginUsers: RecentLoginUser[]
  user?: CurrentUser
  clearDebugOverride: () => void
  initializeAdmin: (input: InitializeAdminInput) => Promise<CurrentUser>
  login: (input: LoginInput) => Promise<CurrentUser>
  logout: () => Promise<void>
  refreshUser: () => Promise<void>
  resumeLogin: (userId: string) => Promise<CurrentUser>
  setDebugOverride: (override: DebugSessionOverride) => void
  updateProfile: (input: { name: string, avatarUrl: string, language: CurrentUser['language'] }) => Promise<CurrentUser>
  updateLanguage: (language: CurrentUser['language']) => Promise<CurrentUser>
}

export const SessionContext = createContext<SessionContextValue | null>(null)

export function useSession() {
  const context = use(SessionContext)
  if (!context)
    throw new Error('useSession must be used inside SessionProvider')
  return context
}
