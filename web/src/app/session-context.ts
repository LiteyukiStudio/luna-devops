import type { CurrentUser } from '@/api'
import { createContext, use } from 'react'

export interface LoginInput {
  email: string
  password: string
  rememberMe: boolean
}

export interface RecentLoginUser {
  avatarUrl: string
  email: string
  id: string
  lastLoginAt: string
  name: string
}

export interface SessionContextValue {
  initialized: boolean
  isLoading: boolean
  isLoggingIn: boolean
  isLoggingOut: boolean
  pendingLoginUsername?: string
  recentLoginUsers: RecentLoginUser[]
  user?: CurrentUser
  login: (input: LoginInput) => Promise<CurrentUser>
  logout: () => Promise<void>
  refreshUser: () => Promise<void>
  resumeLogin: (userId: string) => Promise<CurrentUser>
  updateProfile: (input: { name: string, avatarUrl: string, language: CurrentUser['language'], brandColorPreset: CurrentUser['brandColorPreset'], interfaceStyle: CurrentUser['interfaceStyle'] }) => Promise<CurrentUser>
  updateLanguage: (language: CurrentUser['language']) => Promise<CurrentUser>
}

export const SessionContext = createContext<SessionContextValue | null>(null)

export function useSession() {
  const context = use(SessionContext)
  if (!context)
    throw new Error('useSession must be used inside SessionProvider')
  return context
}
