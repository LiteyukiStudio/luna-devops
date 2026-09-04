export const PlatformRole = {
  Admin: 'platform_admin',
  User: 'user',
} as const

export type PlatformRoleValue = typeof PlatformRole[keyof typeof PlatformRole]

export const PLATFORM_ROLES = [
  PlatformRole.Admin,
  PlatformRole.User,
] as const

export const ProjectRole = {
  Owner: 'owner',
  Admin: 'admin',
  Developer: 'developer',
  Viewer: 'viewer',
} as const

export type ProjectRoleValue = typeof ProjectRole[keyof typeof ProjectRole]

export const PROJECT_ROLES = [
  ProjectRole.Owner,
  ProjectRole.Admin,
  ProjectRole.Developer,
  ProjectRole.Viewer,
] as const

export function isPlatformAdmin(role: string | null | undefined): boolean {
  return role === PlatformRole.Admin
}
